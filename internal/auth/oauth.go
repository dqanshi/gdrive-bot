// Package auth implements the Google OAuth2 "loopback" login flow used by
// rclone-style Drive bots:
//
//  1. Bot sends an authorization URL. The user opens it, grants access,
//     and is redirected to http://127.0.0.1:1/?code=... (which fails to
//     load locally — that's expected, the user just needs the code).
//  2. The user pastes the *entire* redirect URL back to the bot. We parse
//     out `code` ourselves rather than asking the user to extract it.
//  3. We exchange that code for an access + refresh token, store both,
//     and create a matching rclone remote so uploads can use rclone's
//     transfer engine while we use the Drive API directly for everything
//     interactive (listing, rename, delete, share).
//
// This intentionally avoids string sessions and service accounts per the
// spec — only OAuth user credentials are supported.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"

	"gdrive-bot/internal/models"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// DriveScopes is intentionally the full drive scope (not drive.file) so
// the explorer can browse the user's entire My Drive, matching "Real
// folder navigation" from root.
var DriveScopes = []string{
	"https://www.googleapis.com/auth/drive",
}

// Manager builds OAuth configs and performs the code <-> token exchange.
type Manager struct {
	oauthCfg *oauth2.Config
}

func NewManager(clientID, clientSecret, redirectURI string) *Manager {
	return &Manager{
		oauthCfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURI,
			Scopes:       DriveScopes,
			Endpoint:     google.Endpoint,
		},
	}
}

// RandomState generates an unguessable OAuth `state` parameter to guard
// against CSRF on the redirect.
func RandomState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// AuthURL returns the URL to send the user for "Step 1/2". offline access
// + force consent guarantees we get a refresh_token even on repeat logins.
func (m *Manager) AuthURL(state string) string {
	return m.oauthCfg.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
	)
}

// ExtractCode pulls the `code` query parameter out of the full redirect
// URL the user pastes back (http://127.0.0.1:1/?code=...&scope=...).
// Returns an error if the text isn't a parseable URL with a code param,
// so the handler can ask the user to resend it.
func ExtractCode(pastedURL string) (string, error) {
	u, err := url.Parse(pastedURL)
	if err != nil {
		return "", fmt.Errorf("auth: not a valid URL: %w", err)
	}
	code := u.Query().Get("code")
	if code == "" {
		return "", fmt.Errorf("auth: no 'code' parameter found in URL")
	}
	return code, nil
}

// Exchange trades an authorization code for tokens ("Step 2/2": the user
// sends the 4/0Afr.... code after authorizing rclone-side access).
func (m *Manager) Exchange(ctx context.Context, code string) (*models.GoogleToken, error) {
	tok, err := m.oauthCfg.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	return tokenToModel(tok), nil
}

func tokenToModel(tok *oauth2.Token) *models.GoogleToken {
	return &models.GoogleToken{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		Expiry:       tok.Expiry,
	}
}

// TokenSource returns an oauth2.TokenSource that auto-refreshes using the
// stored refresh token, calling onRefresh whenever a new access token is
// minted so the caller can persist it back to MongoDB. This is what gives
// us "login only once" — refreshing never requires user interaction.
func (m *Manager) TokenSource(ctx context.Context, stored *models.GoogleToken, onRefresh func(access string, expiry time.Time)) oauth2.TokenSource {
	base := &oauth2.Token{
		AccessToken:  stored.AccessToken,
		RefreshToken: stored.RefreshToken,
		TokenType:    stored.TokenType,
		Expiry:       stored.Expiry,
	}
	reuse := oauth2.ReuseTokenSource(base, m.oauthCfg.TokenSource(ctx, base))
	return &persistingTokenSource{inner: reuse, onRefresh: onRefresh}
}

type persistingTokenSource struct {
	inner     oauth2.TokenSource
	onRefresh func(access string, expiry time.Time)
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.inner.Token()
	if err != nil {
		return nil, err
	}
	if p.onRefresh != nil {
		p.onRefresh(tok.AccessToken, tok.Expiry)
	}
	return tok, nil
}

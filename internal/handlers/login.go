package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gdrive-bot/internal/auth"
	"gdrive-bot/internal/models"
	"gdrive-bot/internal/rclone"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

const pendingLoginTTL = 15 * time.Minute

// Login handles /login — Step 1/2 of the OAuth flow.
// Sends the Google consent URL and records a PendingLogin so the next
// message from this user is routed to HandleLoginMessage.
func (d *Deps) Login(b *gotgbot.Bot, ctx *ext.Context) error {
	userID := ctx.EffectiveUser.Id
	chatID := ctx.EffectiveChat.Id

	stateToken := auth.RandomState()
	authURL := d.Auth.AuthURL(stateToken)

	text := "🔑 <b>Authorization Step 1/2</b>\n\n" +
		"1. Tap <b>Authorize Google</b> below and sign in.\n" +
		"2. Grant Drive access when prompted.\n" +
		"3. You'll land on a page that can't load — that's expected.\n" +
		"4. Copy the <b>full address-bar URL</b> (starts with <code>http://127.0.0.1:1/?code=</code>) and send it here."
	kb := gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
		{{Text: "🔑 Authorize Google", Url: authURL}},
	}}
	msg, err := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{
		ParseMode: "HTML", ReplyMarkup: kb,
	})
	if err != nil {
		return err
	}

	pending := &models.PendingLogin{
		TelegramID: userID,
		Step:       1,
		ChatID:     chatID,
		MessageID:  msg.MessageId,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(pendingLoginTTL),
	}
	return d.DB.PendingLogins.Upsert(context.Background(), pending)
}

// HandleLoginMessage is called by IncomingText for every text message from
// a user with a PendingLogin. Returns (true, err) if it consumed the
// message, (false, nil) if there was no pending login.
func (d *Deps) HandleLoginMessage(b *gotgbot.Bot, ctx *ext.Context) (bool, error) {
	userID := ctx.EffectiveUser.Id
	text := strings.TrimSpace(ctx.EffectiveMessage.Text)

	pending, err := d.DB.PendingLogins.Get(context.Background(), userID)
	if err != nil {
		return false, nil // no pending login
	}
	if time.Now().After(pending.ExpiresAt) {
		_ = d.DB.PendingLogins.Delete(context.Background(), userID)
		_, _ = ctx.EffectiveMessage.Reply(b, "⌛ Login session expired. Send /login to start again.", nil)
		return true, nil
	}

	// Accept either the full redirect URL or a bare code (starts with "4/").
	code, codeErr := auth.ExtractCode(text)
	if codeErr != nil {
		if strings.HasPrefix(text, "4/") {
			code = text
		} else {
			_, _ = ctx.EffectiveMessage.Reply(b,
				"That doesn't look right. Please paste the <b>full URL</b> from your browser's address bar "+
					"(e.g. <code>http://127.0.0.1:1/?code=4/0A…</code>).",
				&gotgbot.SendMessageOpts{ParseMode: "HTML"})
			return true, nil
		}
	}

	pending.AuthCode = code
	pending.Step = 2
	if err := d.DB.PendingLogins.Upsert(context.Background(), pending); err != nil {
		return true, err
	}

	text2 := "🔐 <b>Authorization Step 2/2</b>\n\nGot your code. Tap below to finish connecting your Google Drive."
	kb := gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
		{{Text: "🔐 Continue Login", CallbackData: "login:continue"}},
	}}
	_, err = ctx.EffectiveMessage.Reply(b, text2, &gotgbot.SendMessageOpts{
		ParseMode: "HTML", ReplyMarkup: kb,
	})
	return true, err
}

// CompleteLogin exchanges the stored auth code for tokens and creates the
// rclone remote. Called from the "🔐 Continue Login" callback button.
func (d *Deps) CompleteLogin(ctx context.Context, userID int64) (string, error) {
	pending, err := d.DB.PendingLogins.Get(ctx, userID)
	if err != nil || pending.AuthCode == "" {
		return "", fmt.Errorf("no pending login found — send /login to start again")
	}

	token, err := d.Auth.Exchange(ctx, pending.AuthCode)
	if err != nil {
		return "", fmt.Errorf("exchanging code with Google: %w", err)
	}

	remote := rclone.RemoteName(userID)
	if err := d.Rclone.CreateOrUpdateRemote(ctx, remote,
		d.Config.GoogleClientID, d.Config.GoogleClientSecret, token, ""); err != nil {
		return "", fmt.Errorf("setting up rclone remote: %w", err)
	}

	if err := d.DB.Users.SaveGoogleToken(ctx, userID, token, remote); err != nil {
		return "", fmt.Errorf("saving credentials: %w", err)
	}

	_ = d.DB.PendingLogins.Delete(ctx, userID)
	return remote, nil
}

// Logout removes the user's Drive credentials from the bot (DB + rclone).
func (d *Deps) Logout(b *gotgbot.Bot, ctx *ext.Context) error {
	userID := ctx.EffectiveUser.Id
	u, err := d.DB.Users.Get(context.Background(), userID)
	if err != nil || u.GoogleToken == nil {
		_, err := ctx.EffectiveMessage.Reply(b, "You're not currently logged in.", nil)
		return err
	}
	if u.RcloneRemote != "" {
		_ = d.Rclone.RemoveRemote(context.Background(), u.RcloneRemote)
	}
	if err := d.DB.Users.ClearGoogleToken(context.Background(), userID); err != nil {
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b, "✅ Logged out. Your Drive credentials have been removed.", nil)
	return err
}

// Status shows login state to the user.
func (d *Deps) Status(b *gotgbot.Bot, ctx *ext.Context) error {
	userID := ctx.EffectiveUser.Id
	u, err := d.DB.Users.Get(context.Background(), userID)
	loggedIn := err == nil && u != nil && u.GoogleToken != nil

	var text string
	if loggedIn {
		text = fmt.Sprintf("✅ <b>Logged in</b>\nRclone remote: <code>%s</code>", u.RcloneRemote)
	} else {
		text = "❌ <b>Not logged in.</b> Send /login to connect your Google Drive."
	}
	_, err = ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return err
}

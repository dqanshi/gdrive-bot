// Package middleware provides the authorization gate every command and
// callback handler runs through: unknown users are denied outright,
// auth'd users get normal access, and the owner always has unlimited
// access regardless of the auth list.
package middleware

import (
	"context"
	"strconv"
	"time"

	"gdrive-bot/internal/cache"
	"gdrive-bot/internal/database"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

const authCacheTTL = 5 * time.Minute

// Guard wraps authorization checks behind a short-lived cache so every
// single incoming update (including high-frequency callback queries
// during explorer navigation) doesn't hit MongoDB.
type Guard struct {
	db      *database.DB
	cache   cache.Cache
	ownerID int64
}

func NewGuard(db *database.DB, c cache.Cache, ownerID int64) *Guard {
	return &Guard{db: db, cache: c, ownerID: ownerID}
}

func (g *Guard) cacheKey(userID int64) string {
	return "authz:" + strconv.FormatInt(userID, 10)
}

// IsAuthorized reports whether userID may use the bot, checking the cache
// first and falling back to MongoDB (which also lazily creates the user
// document so /users and /stats see them).
func (g *Guard) IsAuthorized(ctx context.Context, userID int64, username string) (bool, error) {
	if userID == g.ownerID {
		return true, nil
	}

	var authed bool
	if hit, _ := g.cache.Get(ctx, g.cacheKey(userID), &authed); hit {
		return authed, nil
	}

	u, err := g.db.Users.EnsureExists(ctx, userID, username, userID == g.ownerID)
	if err != nil {
		return false, err
	}
	authed = u.IsOwner || u.IsAuthed
	_ = g.cache.Set(ctx, g.cacheKey(userID), authed, authCacheTTL)
	return authed, nil
}

// Invalidate clears the cached authorization state for a user, called
// immediately after /auth or /unauth so the change takes effect on the
// user's very next message instead of waiting out the TTL.
func (g *Guard) Invalidate(ctx context.Context, userID int64) {
	_ = g.cache.Delete(ctx, g.cacheKey(userID))
}

// RequireAuth is a gotgbot dispatcher-style guard: call at the top of
// every handler, return immediately if it returns false (it already sent
// the denial message).
func (g *Guard) RequireAuth(b *gotgbot.Bot, ctx *ext.Context) (bool, error) {
	user := ctx.EffectiveUser
	if user == nil {
		return false, nil
	}
	ok, err := g.IsAuthorized(context.Background(), user.Id, user.Username)
	if err != nil {
		return false, err
	}
	if !ok {
		_, _ = ctx.EffectiveMessage.Reply(b, "🚫 You are not authorized to use this bot. Ask the owner to run /auth on your Telegram ID: "+strconv.FormatInt(user.Id, 10), nil)
		return false, nil
	}
	return true, nil
}

// IsOwner reports whether userID is the configured bot owner.
func (g *Guard) IsOwner(userID int64) bool {
	return userID == g.ownerID
}

package database

import (
	"context"
	"errors"
	"time"

	"gdrive-bot/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrNotFound is returned by repository Get* methods when no document
// matches. Callers compare with errors.Is.
var ErrNotFound = errors.New("database: not found")

// UserRepo persists per-Telegram-user state: auth status and Google tokens.
type UserRepo struct {
	col *mongo.Collection
}

// Get fetches a user by Telegram ID, or ErrNotFound if they've never
// interacted with the bot.
func (r *UserRepo) Get(ctx context.Context, telegramID int64) (*models.User, error) {
	var u models.User
	err := r.col.FindOne(ctx, bson.M{"telegram_id": telegramID}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// EnsureExists creates a bare user document (unauthorized, no tokens) the
// first time we see a Telegram ID, so /stats and /users have a complete
// picture of everyone who has ever messaged the bot. isOwner is set based
// on config.OwnerID at call time.
func (r *UserRepo) EnsureExists(ctx context.Context, telegramID int64, username string, isOwner bool) (*models.User, error) {
	now := time.Now()
	update := bson.M{
		"$setOnInsert": bson.M{
			"telegram_id": telegramID,
			"is_owner":    isOwner,
			"is_authed":   isOwner, // owner is always authed
			"created_at":  now,
		},
		"$set": bson.M{
			"username":   username,
			"updated_at": now,
		},
	}
	opts := options.Update().SetUpsert(true)
	if _, err := r.col.UpdateOne(ctx, bson.M{"telegram_id": telegramID}, update, opts); err != nil {
		return nil, err
	}
	return r.Get(ctx, telegramID)
}

// SetAuthed grants or revokes bot access for a user (owner commands
// /auth and /unauth).
func (r *UserRepo) SetAuthed(ctx context.Context, telegramID int64, authed bool, by int64) error {
	set := bson.M{
		"is_authed":  authed,
		"updated_at": time.Now(),
	}
	if authed {
		set["authed_by"] = by
		set["authed_at"] = time.Now()
	}
	_, err := r.col.UpdateOne(ctx, bson.M{"telegram_id": telegramID}, bson.M{"$set": set})
	return err
}

// SaveGoogleToken persists the OAuth token + rclone remote name after a
// successful /login.
func (r *UserRepo) SaveGoogleToken(ctx context.Context, telegramID int64, token *models.GoogleToken, rcloneRemote string) error {
	_, err := r.col.UpdateOne(ctx, bson.M{"telegram_id": telegramID}, bson.M{
		"$set": bson.M{
			"google_token":  token,
			"rclone_remote": rcloneRemote,
			"updated_at":    time.Now(),
		},
	})
	return err
}

// UpdateAccessToken is called by the token refresher whenever a stored
// access token is refreshed, so the next request reuses it instead of
// hitting Google's token endpoint again.
func (r *UserRepo) UpdateAccessToken(ctx context.Context, telegramID int64, accessToken string, expiry time.Time) error {
	_, err := r.col.UpdateOne(ctx, bson.M{"telegram_id": telegramID}, bson.M{
		"$set": bson.M{
			"google_token.access_token": accessToken,
			"google_token.expiry":       expiry,
			"updated_at":                time.Now(),
		},
	})
	return err
}

// ClearGoogleToken removes Google credentials and the rclone remote
// reference on /logout. The actual rclone remote config entry is removed
// separately via internal/rclone.
func (r *UserRepo) ClearGoogleToken(ctx context.Context, telegramID int64) error {
	_, err := r.col.UpdateOne(ctx, bson.M{"telegram_id": telegramID}, bson.M{
		"$unset": bson.M{"google_token": "", "rclone_remote": ""},
		"$set":   bson.M{"updated_at": time.Now()},
	})
	return err
}

// IsAuthorized reports whether a user may use the bot at all (owner or
// explicitly /auth'd). Used by middleware; prefer the cached wrapper in
// internal/cache for the hot path.
func (r *UserRepo) IsAuthorized(ctx context.Context, telegramID int64) (bool, error) {
	u, err := r.Get(ctx, telegramID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return u.IsOwner || u.IsAuthed, nil
}

// ListAuthorized returns every user with bot access, for /users and
// /broadcast.
func (r *UserRepo) ListAuthorized(ctx context.Context) ([]models.User, error) {
	cur, err := r.col.Find(ctx, bson.M{"$or": []bson.M{{"is_authed": true}, {"is_owner": true}}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var users []models.User
	if err := cur.All(ctx, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// CountAll returns total users ever seen and total currently authorized,
// for /stats.
func (r *UserRepo) CountAll(ctx context.Context) (total int64, authorized int64, err error) {
	total, err = r.col.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, 0, err
	}
	authorized, err = r.col.CountDocuments(ctx, bson.M{"$or": []bson.M{{"is_authed": true}, {"is_owner": true}}})
	if err != nil {
		return 0, 0, err
	}
	return total, authorized, nil
}

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

// ExplorerStateRepo persists per-message file explorer state (breadcrumb,
// pagination, search, pending rename) so /myfiles can always edit the
// existing message instead of sending new ones.
type ExplorerStateRepo struct {
	col *mongo.Collection
}

func (r *ExplorerStateRepo) key(userID, chatID, messageID int64) bson.M {
	return bson.M{"user_id": userID, "chat_id": chatID, "message_id": messageID}
}

func (r *ExplorerStateRepo) Get(ctx context.Context, userID, chatID, messageID int64) (*models.ExplorerState, error) {
	var s models.ExplorerState
	err := r.col.FindOne(ctx, r.key(userID, chatID, messageID)).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *ExplorerStateRepo) Upsert(ctx context.Context, s *models.ExplorerState) error {
	s.UpdatedAt = time.Now()
	opts := options.Replace().SetUpsert(true)
	_, err := r.col.ReplaceOne(ctx, r.key(s.UserID, s.ChatID, s.MessageID), s, opts)
	return err
}

func (r *ExplorerStateRepo) Delete(ctx context.Context, userID, chatID, messageID int64) error {
	_, err := r.col.DeleteOne(ctx, r.key(userID, chatID, messageID))
	return err
}

// FindAwaiting locates the (at most one, in practice) explorer message in
// this chat currently waiting on a rename or search reply from the user,
// so a plain text reply works whether or not the user actually used
// Telegram's native "reply to message" feature.
func (r *ExplorerStateRepo) FindAwaiting(ctx context.Context, userID, chatID int64) (*models.ExplorerState, error) {
	var s models.ExplorerState
	filter := bson.M{
		"user_id": userID,
		"chat_id": chatID,
		"$or": []bson.M{
			{"awaiting_rename_file_id": bson.M{"$exists": true, "$ne": ""}},
			{"awaiting_search": true},
		},
	}
	err := r.col.FindOne(ctx, filter).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// PendingLoginRepo persists in-progress /login conversations.
type PendingLoginRepo struct {
	col *mongo.Collection
}

func (r *PendingLoginRepo) Get(ctx context.Context, telegramID int64) (*models.PendingLogin, error) {
	var p models.PendingLogin
	err := r.col.FindOne(ctx, bson.M{"telegram_id": telegramID}).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PendingLoginRepo) Upsert(ctx context.Context, p *models.PendingLogin) error {
	opts := options.Replace().SetUpsert(true)
	_, err := r.col.ReplaceOne(ctx, bson.M{"telegram_id": p.TelegramID}, p, opts)
	return err
}

func (r *PendingLoginRepo) Delete(ctx context.Context, telegramID int64) error {
	_, err := r.col.DeleteOne(ctx, bson.M{"telegram_id": telegramID})
	return err
}

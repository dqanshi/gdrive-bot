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

const settingsDocID = "global"

// SettingsRepo persists the single global Settings document.
type SettingsRepo struct {
	col *mongo.Collection
}

// GetOrCreate fetches the global settings, seeding it with defaults on
// first run.
func (r *SettingsRepo) GetOrCreate(ctx context.Context, defaults models.Settings) (*models.Settings, error) {
	var s models.Settings
	err := r.col.FindOne(ctx, bson.M{"_id": settingsDocID}).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		defaults.ID = settingsDocID
		if _, insErr := r.col.InsertOne(ctx, defaults); insErr != nil {
			return nil, insErr
		}
		return &defaults, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Update applies a partial update (only non-nil fields) and returns the
// resulting document. Used by /setworkers, /setqueue, /setdownloadlimit,
// /setuploadlimit, and the /settings menu.
func (r *SettingsRepo) Update(ctx context.Context, by int64, mutate func(s *models.Settings)) (*models.Settings, error) {
	current, err := r.GetOrCreate(ctx, models.DefaultSettings(4, 50, 0, 0))
	if err != nil {
		return nil, err
	}
	mutate(current)
	current.UpdatedAt = time.Now()
	current.UpdatedBy = by

	opts := options.Replace().SetUpsert(true)
	if _, err := r.col.ReplaceOne(ctx, bson.M{"_id": settingsDocID}, current, opts); err != nil {
		return nil, err
	}
	return current, nil
}

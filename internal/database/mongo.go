// Package database wraps MongoDB access behind small repository types
// (Users, Settings, Tasks, ExplorerStates, PendingLogins) so handlers never
// touch *mongo.Collection directly. Every read in a hot path (auth checks,
// settings) is expected to go through internal/cache first; these
// repositories are the source of truth underneath that cache.
package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DB bundles the Mongo client/database handle plus every repository.
type DB struct {
	Client *mongo.Client
	Name   string

	Users          *UserRepo
	Settings       *SettingsRepo
	Tasks          *TaskRepo
	ExplorerStates *ExplorerStateRepo
	PendingLogins  *PendingLoginRepo
}

// Connect dials MongoDB, verifies connectivity, builds indexes, and wires
// up every repository.
func Connect(ctx context.Context, uri, dbName string) (*DB, error) {
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(connectCtx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("database: connect: %w", err)
	}
	if err := client.Ping(connectCtx, nil); err != nil {
		return nil, fmt.Errorf("database: ping: %w", err)
	}

	d := client.Database(dbName)
	db := &DB{
		Client:         client,
		Name:           dbName,
		Users:          &UserRepo{col: d.Collection("users")},
		Settings:       &SettingsRepo{col: d.Collection("settings")},
		Tasks:          &TaskRepo{col: d.Collection("tasks")},
		ExplorerStates: &ExplorerStateRepo{col: d.Collection("explorer_states")},
		PendingLogins:  &PendingLoginRepo{col: d.Collection("pending_logins")},
	}

	if err := db.ensureIndexes(ctx); err != nil {
		return nil, fmt.Errorf("database: ensure indexes: %w", err)
	}
	return db, nil
}

func (db *DB) ensureIndexes(ctx context.Context) error {
	if _, err := db.Users.col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "telegram_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return err
	}
	if _, err := db.Tasks.col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "user_id", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}}},
		{Keys: bson.D{{Key: "created_at", Value: 1}}},
	}); err != nil {
		return err
	}
	if _, err := db.ExplorerStates.col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "chat_id", Value: 1}, {Key: "message_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return err
	}
	if _, err := db.PendingLogins.col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "telegram_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return err
	}
	return nil
}

// Disconnect closes the underlying Mongo client. Call on shutdown.
func (db *DB) Disconnect(ctx context.Context) error {
	return db.Client.Disconnect(ctx)
}

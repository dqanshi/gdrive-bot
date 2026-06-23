// Package handlers implements every Telegram command and the plain-text
// message handler. Callback-query handling lives in internal/callbacks,
// sharing this same Deps bundle.
package handlers

import (
	"context"
	"fmt"
	"time"

	"gdrive-bot/internal/auth"
	"gdrive-bot/internal/cache"
	"gdrive-bot/internal/config"
	"gdrive-bot/internal/database"
	"gdrive-bot/internal/drive"
	"gdrive-bot/internal/explorer"
	"gdrive-bot/internal/middleware"
	"gdrive-bot/internal/models"
	"gdrive-bot/internal/queue"
	"gdrive-bot/internal/rclone"
)

// Deps bundles every dependency a handler needs. One instance is built at
// startup and shared across all handlers — nothing here is request-scoped.
type Deps struct {
	Config   *config.Config
	DB       *database.DB
	Cache    cache.Cache
	Guard    *middleware.Guard
	Auth     *auth.Manager
	Rclone   *rclone.Manager
	Queue    *queue.Manager
	Explorer *explorer.Service

	StartedAt time.Time
}

// DriveClientFor builds an authenticated Drive API client for a logged-in
// user, auto-refreshing and persisting access tokens via MongoDB.
func (d *Deps) DriveClientFor(ctx context.Context, userID int64) (*drive.Client, error) {
	u, err := d.DB.Users.Get(ctx, userID)
	if err != nil || u.GoogleToken == nil {
		return nil, fmt.Errorf("you haven't logged in yet — send /login first")
	}
	ts := d.Auth.TokenSource(ctx, u.GoogleToken, func(access string, expiry time.Time) {
		_ = d.DB.Users.UpdateAccessToken(context.Background(), userID, access, expiry)
	})
	return drive.NewClient(ctx, ts)
}

// SettingsPublic fetches live owner-editable settings, returning all
// fields needed by the processor and queue. Exported so the bot package
// (processor.go) can call it without an import cycle.
func (d *Deps) SettingsPublic(ctx context.Context) (workers, queueSize int, downloadLimitMB, uploadLimitMB int64, pageSize, maxRetries int) {
	return d.settings(ctx)
}

// settings is the internal version used by handlers in this package.
func (d *Deps) settings(ctx context.Context) (workers, queueSize int, downloadLimitMB, uploadLimitMB int64, pageSize, maxRetries int) {
	def := defaultSettings(d.Config)
	s, err := d.DB.Settings.GetOrCreate(ctx, def)
	if err != nil {
		return d.Config.DefaultWorkers, d.Config.DefaultQueueSize,
			d.Config.DefaultDownloadLimitMB, d.Config.DefaultUploadLimitMB, 10, 10
	}
	return s.Workers, s.QueueSize, s.DownloadLimitMB, s.UploadLimitMB, s.PageSize, s.MaxRetries
}

func defaultSettings(cfg *config.Config) models.Settings {
	return models.DefaultSettings(cfg.DefaultWorkers, cfg.DefaultQueueSize,
		cfg.DefaultDownloadLimitMB, cfg.DefaultUploadLimitMB)
}

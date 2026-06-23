package models

import "time"

// Settings is a singleton document (_id: "global") holding every
// owner-editable runtime setting. Nothing here requires a VPS edit or
// restart — handlers read this fresh from the cache/DB on each use.
type Settings struct {
	ID string `bson:"_id"` // always "global"

	Workers         int   `bson:"workers"`
	QueueSize       int   `bson:"queue_size"`
	DownloadLimitMB int64 `bson:"download_limit_mb"` // 0 = unlimited
	UploadLimitMB   int64 `bson:"upload_limit_mb"`   // 0 = unlimited

	// PageSize controls how many entries the file explorer shows per page.
	PageSize int `bson:"page_size"`

	// AutoCleanIntervalMin: minutes between automatic downloads/ cleanups.
	// 0 disables automatic cleanup (manual /clean only).
	AutoCleanIntervalMin int `bson:"auto_clean_interval_min"`

	// MaxRetries applies to network/API/rclone operations.
	MaxRetries int `bson:"max_retries"`

	UpdatedAt time.Time `bson:"updated_at"`
	UpdatedBy int64     `bson:"updated_by"`
}

// DefaultSettings returns the baseline settings document seeded on first
// run, using the static defaults from config as a starting point.
func DefaultSettings(workers, queueSize int, downloadLimitMB, uploadLimitMB int64) Settings {
	return Settings{
		ID:                   "global",
		Workers:              workers,
		QueueSize:            queueSize,
		DownloadLimitMB:      downloadLimitMB,
		UploadLimitMB:        uploadLimitMB,
		PageSize:             10,
		AutoCleanIntervalMin: 60,
		MaxRetries:           10,
		UpdatedAt:            time.Now(),
	}
}

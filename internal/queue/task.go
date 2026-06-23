// Package queue implements a worker-pool based task queue: handlers
// enqueue download->upload jobs (from Telegram files or direct links),
// a configurable number of workers pull from the queue concurrently, and
// every operation is wrapped in the shared retry helper. Tasks persist in
// MongoDB so a restart resumes in-flight work instead of losing it.
package queue

import (
	"context"

	"gdrive-bot/internal/models"
)

// Job is the in-memory unit pushed through the Go channel queue. It wraps
// the persisted Task plus a context so a /restart or per-task cancel can
// stop in-flight work cleanly.
type Job struct {
	Task   *models.Task
	Ctx    context.Context
	Cancel context.CancelFunc
}

// Processor performs the actual download+upload for a job. Implemented by
// internal/bot (which has access to the Telegram bot, drive clients, and
// rclone manager) and injected into the Manager so this package stays
// free of those dependencies.
type Processor interface {
	Process(job *Job) error
}

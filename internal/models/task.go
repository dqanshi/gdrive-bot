package models

import "time"

// TaskType distinguishes how a task's source file should be obtained.
type TaskType string

const (
	TaskTypeTelegramFile TaskType = "telegram_file"
	TaskTypeDirectLink   TaskType = "direct_link"
)

// TaskStatus tracks a task through its lifecycle.
type TaskStatus string

const (
	StatusQueued      TaskStatus = "queued"
	StatusDownloading TaskStatus = "downloading"
	StatusUploading   TaskStatus = "uploading"
	StatusCompleted   TaskStatus = "completed"
	StatusFailed      TaskStatus = "failed"
	StatusCanceled    TaskStatus = "canceled"
)

// Task is one unit of work in the queue: fetch a file (from Telegram or a
// direct HTTP(S) link) and upload it to the user's Google Drive via rclone.
type Task struct {
	ID     string     `bson:"_id"`
	UserID int64      `bson:"user_id"`
	ChatID int64      `bson:"chat_id"`
	Type   TaskType   `bson:"type"`
	Status TaskStatus `bson:"status"`

	// Source: Telegram file_id for TaskTypeTelegramFile, or the URL for
	// TaskTypeDirectLink.
	Source   string `bson:"source"`
	FileName string `bson:"file_name"`
	FileSize int64  `bson:"file_size_bytes"`

	// DestFolderID is the Drive folder ID the file is uploaded into
	// (defaults to the user's current explorer location).
	DestFolderID string `bson:"dest_folder_id"`

	LocalPath string `bson:"local_path,omitempty"`

	// StatusMessageID/ChatID identify the Telegram message edited in place
	// to show download/upload progress (never spammed with new messages).
	StatusMessageID int64 `bson:"status_message_id"`

	RetryCount int    `bson:"retry_count"`
	LastError  string `bson:"last_error,omitempty"`

	CreatedAt   time.Time `bson:"created_at"`
	StartedAt   time.Time `bson:"started_at,omitempty"`
	CompletedAt time.Time `bson:"completed_at,omitempty"`

	// ResultFileID is the Drive file ID once upload succeeds.
	ResultFileID string `bson:"result_file_id,omitempty"`
}

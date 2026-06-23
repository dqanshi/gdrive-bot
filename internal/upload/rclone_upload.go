// Package upload orchestrates moving a downloaded local file into the
// user's Google Drive via internal/rclone, translating rclone's progress
// output into a simple channel-based API the queue worker can consume
// alongside download progress.
package upload

import (
	"context"
	"fmt"

	"gdrive-bot/internal/rclone"
)

// Progress mirrors download.Progress so handlers can render both stages
// with the same formatting helpers (internal/utils.ProgressBar etc.)
// without an import cycle between download and upload.
type Progress struct {
	BytesDone  int64
	BytesTotal int64
	SpeedBps   float64
	ETASeconds int64
	Done       bool
	Err        error
}

// ToDrive uploads localPath into the given user's rclone remote /
// destination folder, streaming Progress on progressCh until done.
func ToDrive(ctx context.Context, rc *rclone.Manager, remote, destFolderID, localPath string, maxRetries int, progressCh chan<- Progress) error {
	defer close(progressCh)

	rcloneCh := make(chan rclone.UploadProgress)
	errCh := make(chan error, 1)

	go func() {
		errCh <- rc.Upload(ctx, localPath, remote, destFolderID, maxRetries, rcloneCh)
	}()

	for p := range rcloneCh {
		progressCh <- Progress{
			BytesDone:  p.BytesDone,
			BytesTotal: p.BytesTotal,
			SpeedBps:   p.SpeedBps,
			ETASeconds: p.ETASeconds,
			Done:       p.Done,
			Err:        p.Err,
		}
	}

	if err := <-errCh; err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	return nil
}

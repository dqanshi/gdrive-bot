package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// telegramSpeedWindow is a tiny ring buffer used to compute instantaneous
// speed (bytes since last tick / time since last tick) rather than a
// lifetime average, which is far more useful for an ETA.
type speedTracker struct {
	lastBytes int64
	lastTime  time.Time
}

func newSpeedTracker() *speedTracker {
	return &speedTracker{lastTime: time.Now()}
}

func (s *speedTracker) update(totalDone int64) float64 {
	now := time.Now()
	elapsed := now.Sub(s.lastTime).Seconds()
	if elapsed <= 0 {
		return 0
	}
	speed := float64(totalDone-s.lastBytes) / elapsed
	s.lastBytes = totalDone
	s.lastTime = now
	if speed < 0 {
		speed = 0
	}
	return speed
}

// TelegramFile downloads a file by file_id from the Bot API to localPath,
// streaming Progress updates. gotgbot's bot.GetFile resolves the file_id
// to a download path, then we stream the bytes ourselves (rather than
// using the SDK's buffered helper) so we can report progress.
func TelegramFile(ctx context.Context, bot *gotgbot.Bot, fileID, localPath string, progressCh chan<- Progress) error {
	defer close(progressCh)

	tgFile, err := bot.GetFile(fileID, nil)
	if err != nil {
		return fmt.Errorf("download: get file: %w", err)
	}
	url := tgFile.URL(bot, nil)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("download: build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: telegram returned status %d", resp.StatusCode)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("download: create local file: %w", err)
	}
	defer out.Close()

	total := int64(tgFile.FileSize)
	return streamWithProgress(ctx, resp.Body, out, total, progressCh)
}

// streamWithProgress copies src->dst in chunks, periodically emitting
// Progress on progressCh. Shared by the Telegram and direct-link
// downloaders below.
func streamWithProgress(ctx context.Context, src io.Reader, dst io.Writer, total int64, progressCh chan<- Progress) error {
	buf := make([]byte, 256*1024)
	tracker := newSpeedTracker()
	ticker := newProgressTicker()
	var done int64

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("download: write: %w", writeErr)
			}
			done += int64(n)
			if ticker.ready() {
				progressCh <- Progress{
					BytesDone:  done,
					BytesTotal: total,
					SpeedBps:   tracker.update(done),
				}
			}
		}
		if readErr == io.EOF {
			progressCh <- Progress{BytesDone: done, BytesTotal: total, Done: true}
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("download: read: %w", readErr)
		}
	}
}

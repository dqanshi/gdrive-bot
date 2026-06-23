// Package bot is the composition root — it wires all subsystems and
// implements queue.Processor (the actual download → upload pipeline).
package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gdrive-bot/internal/download"
	"gdrive-bot/internal/drive"
	"gdrive-bot/internal/explorer"
	"gdrive-bot/internal/handlers"
	"gdrive-bot/internal/models"
	"gdrive-bot/internal/queue"
	"gdrive-bot/internal/upload"
	"gdrive-bot/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// Processor implements queue.Processor using the Telegram bot client and
// the shared handler dependencies.
type Processor struct {
	bot  *gotgbot.Bot
	deps *handlers.Deps
}

// NewProcessor wires a Processor. Called after deps.Queue is set so
// there's no circular dependency at startup.
func NewProcessor(b *gotgbot.Bot, deps *handlers.Deps) *Processor {
	return &Processor{bot: b, deps: deps}
}

// Process performs one download→upload cycle end-to-end.
// A non-nil return retries via the queue's retry wrapper.
func (p *Processor) Process(job *queue.Job) error {
	t := job.Task
	ctx := job.Ctx

	p.editStatus(t, fmt.Sprintf("📥 Starting: %s", t.FileName))
	_ = p.deps.DB.Tasks.UpdateStatus(context.Background(), t.ID, models.StatusDownloading, "")

	if err := utils.EnsureDir(p.deps.Config.DownloadDir); err != nil {
		return fmt.Errorf("ensure download dir: %w", err)
	}
	localPath := filepath.Join(p.deps.Config.DownloadDir,
		t.ID+"_"+utils.SafeFilename(t.FileName))
	defer os.Remove(localPath)

	if err := p.runDownload(ctx, t, localPath); err != nil {
		_ = p.deps.DB.Tasks.UpdateStatus(context.Background(), t.ID, models.StatusFailed, err.Error())
		p.editStatus(t, fmt.Sprintf("❌ Download failed: %s", err.Error()))
		return err
	}

	if fi, err := os.Stat(localPath); err == nil {
		t.FileSize = fi.Size()
	}

	// Enforce per-user upload size limit (0 = unlimited).
	_, _, _, uploadLimitMB, _, _ := p.deps.SettingsPublic(context.Background())
	if uploadLimitMB > 0 && t.FileSize > uploadLimitMB*1024*1024 {
		msg := fmt.Sprintf("❌ File too large (%s). Upload limit is %s.",
			utils.HumanBytes(t.FileSize), utils.HumanBytes(uploadLimitMB*1024*1024))
		p.editStatus(t, msg)
		_ = p.deps.DB.Tasks.UpdateStatus(context.Background(), t.ID, models.StatusFailed, msg)
		return utils.NonRetryable(fmt.Errorf("file exceeds upload limit"))
	}

	u, err := p.deps.DB.Users.Get(context.Background(), t.UserID)
	if err != nil || u.GoogleToken == nil || u.RcloneRemote == "" {
		return utils.NonRetryable(fmt.Errorf("user not logged in"))
	}

	_ = p.deps.DB.Tasks.UpdateStatus(context.Background(), t.ID, models.StatusUploading, "")
	_, _, _, _, _, maxRetries := p.deps.SettingsPublic(context.Background())

	if err := p.runUpload(ctx, t, u.RcloneRemote, localPath, maxRetries); err != nil {
		_ = p.deps.DB.Tasks.UpdateStatus(context.Background(), t.ID, models.StatusFailed, err.Error())
		p.editStatus(t, fmt.Sprintf("❌ Upload failed: %s", err.Error()))
		return err
	}

	return p.finish(ctx, t)
}

func (p *Processor) runDownload(ctx context.Context, t *models.Task, localPath string) error {
	progressCh := make(chan download.Progress, 4)
	errCh := make(chan error, 1)

	go func() {
		switch t.Type {
		case models.TaskTypeTelegramFile:
			errCh <- download.TelegramFile(ctx, p.bot, t.Source, localPath, progressCh)
		case models.TaskTypeDirectLink:
			errCh <- download.DirectLink(ctx, t.Source, localPath, progressCh)
		default:
			close(progressCh)
			errCh <- fmt.Errorf("unknown task type %q", t.Type)
		}
	}()

	for prog := range progressCh {
		if prog.Err != nil {
			continue
		}
		bar := utils.ProgressBar(prog.BytesDone, prog.BytesTotal, 16)
		text := fmt.Sprintf("⬇ Downloading: <b>%s</b>\n%s\n%s",
			t.FileName, bar, utils.HumanSpeed(prog.SpeedBps))
		p.editStatus(t, text)
	}
	return <-errCh
}

func (p *Processor) runUpload(ctx context.Context, t *models.Task, remote, localPath string, maxRetries int) error {
	progressCh := make(chan upload.Progress, 4)
	errCh := make(chan error, 1)

	go func() {
		errCh <- upload.ToDrive(ctx, p.deps.Rclone, remote, t.DestFolderID, localPath, maxRetries, progressCh)
	}()

	for prog := range progressCh {
		if prog.Err != nil {
			continue
		}
		bar := utils.ProgressBar(prog.BytesDone, prog.BytesTotal, 16)
		eta := ""
		if prog.ETASeconds > 0 {
			eta = " • ETA " + utils.HumanDuration(time.Duration(prog.ETASeconds)*time.Second)
		}
		text := fmt.Sprintf("⬆ Uploading: <b>%s</b>\n%s\n%s%s",
			t.FileName, bar, utils.HumanSpeed(prog.SpeedBps), eta)
		p.editStatus(t, text)
	}
	return <-errCh
}

// finish locates the newly-uploaded Drive file by name and replaces the
// status message with the standard file menu ("After upload: Return file
// menu." from the spec).
func (p *Processor) finish(ctx context.Context, t *models.Task) error {
	dc, err := p.deps.DriveClientFor(ctx, t.UserID)
	if err != nil {
		p.editStatus(t, fmt.Sprintf("✅ Uploaded: %s", t.FileName))
		_ = p.deps.DB.Tasks.UpdateStatus(context.Background(), t.ID, models.StatusCompleted, "")
		return nil
	}

	entry, err := findUploadedFile(ctx, dc, t.DestFolderID, t.FileName)
	if err != nil {
		p.editStatus(t, fmt.Sprintf("✅ Uploaded: %s", t.FileName))
		_ = p.deps.DB.Tasks.UpdateStatus(context.Background(), t.ID, models.StatusCompleted, "")
		return nil
	}

	_ = p.deps.DB.Tasks.SetResult(context.Background(), t.ID, entry.ID)
	view := explorer.FileMenu(entry)
	kb := gotgbot.InlineKeyboardMarkup{InlineKeyboard: view.Keyboard}
	_, _, _ = p.bot.EditMessageText(
		"✅ <b>Upload complete!</b>\n\n"+view.Text,
		&gotgbot.EditMessageTextOpts{
			ChatId:      t.ChatID,
			MessageId:   t.StatusMessageID,
			ParseMode:   "HTML",
			ReplyMarkup: kb,
		},
	)
	return nil
}

// findUploadedFile searches the destination folder for an exact name
// match, retrying up to 5 times (Drive indexing can lag rclone by 1-2 s).
func findUploadedFile(ctx context.Context, dc *drive.Client, destFolderID, fileName string) (drive.Entry, error) {
	folder := destFolderID
	if folder == "" {
		folder = "root"
	}
	var lastErr error
	for i := 0; i < 5; i++ {
		entries, _, err := dc.ListPage(ctx, folder, fileName, 5, "")
		if err == nil {
			for _, e := range entries {
				if e.Name == fileName {
					return e, nil
				}
			}
		} else {
			lastErr = err
		}
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		return drive.Entry{}, lastErr
	}
	return drive.Entry{}, fmt.Errorf("file %q not found in Drive listing after upload", fileName)
}

// editStatus edits the task's progress message in place, rate-limited to
// one edit per ~1.2 s by Telegram's flood control. We ignore errors here
// because a failed progress edit is cosmetic — the actual task continues.
func (p *Processor) editStatus(t *models.Task, text string) {
	_, _, _ = p.bot.EditMessageText(text, &gotgbot.EditMessageTextOpts{
		ChatId:    t.ChatID,
		MessageId: t.StatusMessageID,
		ParseMode: "HTML",
	})
}

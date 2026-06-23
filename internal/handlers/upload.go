package handlers

import (
	"context"
	"fmt"
	"strings"

	"gdrive-bot/internal/download"
	"gdrive-bot/internal/models"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// maxConcurrentPerUser limits how many tasks a non-owner can queue simultaneously.
const maxConcurrentPerUser = 25

// IncomingFile handles messages containing a document, video, audio, or
// photo attachment — extracting the file_id and queuing an upload task.
func (d *Deps) IncomingFile(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	userID := ctx.EffectiveUser.Id
	chatID := ctx.EffectiveChat.Id

	fileID, fileName, fileSize := extractFile(msg)
	if fileID == "" {
		return nil
	}

	c := context.Background()
	if err := d.requireLoggedIn(b, ctx, userID); err != nil {
		return err
	}
	if err := d.checkUserQueueLimit(c, userID); err != nil {
		_, _ = msg.Reply(b, "⚠️ "+err.Error(), nil)
		return nil
	}

	statusMsg, err := msg.Reply(b, "📥 Queued: "+fileName, nil)
	if err != nil {
		return err
	}

	task := &models.Task{
		UserID:          userID,
		ChatID:          chatID,
		Type:            models.TaskTypeTelegramFile,
		Source:          fileID,
		FileName:        fileName,
		FileSize:        fileSize,
		StatusMessageID: statusMsg.MessageId,
	}
	if _, err := d.Queue.Enqueue(c, task); err != nil {
		_, _, _ = statusMsg.EditText(b, "❌ Couldn't queue upload: "+err.Error(), nil)
	}
	return nil
}

// IncomingText routes plain text messages through three gates in order:
//  1. Active /login flow (URL or bare code paste)
//  2. Pending rename/search reply to an explorer prompt
//  3. Direct HTTP(S) link upload
func (d *Deps) IncomingText(b *gotgbot.Bot, ctx *ext.Context) error {
	c := context.Background()
	msg := ctx.EffectiveMessage
	userID := ctx.EffectiveUser.Id
	chatID := ctx.EffectiveChat.Id
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return nil
	}

	// Gate 1 — /login flow
	if consumed, err := d.HandleLoginMessage(b, ctx); consumed {
		return err
	}

	// Gate 2 — pending rename or search prompt
	if pending, err := d.Explorer.FindPending(c, userID, chatID); err == nil && pending != nil {
		if pending.RenameFileID != "" {
			return d.applyRename(b, ctx, pending.RenameFileID, text)
		}
		if pending.IsSearch {
			return d.applySearch(b, ctx, pending.MessageID, text)
		}
	}

	// Gate 3 — direct link upload
	if !download.IsDirectLink(text) {
		return nil
	}
	if err := d.requireLoggedIn(b, ctx, userID); err != nil {
		return err
	}
	if err := d.checkUserQueueLimit(c, userID); err != nil {
		_, _ = msg.Reply(b, "⚠️ "+err.Error(), nil)
		return nil
	}

	fileName := download.SuggestFilename(text)
	statusMsg, err := msg.Reply(b, "📥 Queued: "+fileName, nil)
	if err != nil {
		return err
	}

	task := &models.Task{
		UserID:          userID,
		ChatID:          chatID,
		Type:            models.TaskTypeDirectLink,
		Source:          text,
		FileName:        fileName,
		StatusMessageID: statusMsg.MessageId,
	}
	if _, err := d.Queue.Enqueue(c, task); err != nil {
		_, _, _ = statusMsg.EditText(b, "❌ Couldn't queue upload: "+err.Error(), nil)
	}
	return nil
}

// requireLoggedIn replies and returns an error if the user hasn't /login'd.
func (d *Deps) requireLoggedIn(b *gotgbot.Bot, ctx *ext.Context, userID int64) error {
	u, err := d.DB.Users.Get(context.Background(), userID)
	if err != nil || u.GoogleToken == nil {
		_, _ = ctx.EffectiveMessage.Reply(b, "❌ Please /login before uploading.", nil)
		return fmt.Errorf("not logged in")
	}
	return nil
}

// checkUserQueueLimit prevents a single non-owner from flooding the queue.
func (d *Deps) checkUserQueueLimit(ctx context.Context, userID int64) error {
	if d.Guard.IsOwner(userID) {
		return nil
	}
	n, err := d.DB.Tasks.CountForUser(ctx, userID, []models.TaskStatus{
		models.StatusQueued, models.StatusDownloading, models.StatusUploading,
	})
	if err != nil {
		return nil
	}
	if n >= maxConcurrentPerUser {
		return fmt.Errorf("you have %d uploads in progress — wait for one to finish", n)
	}
	return nil
}

// extractFile returns (fileID, fileName, fileSize) from any file-carrying
// message type the bot accepts.
func extractFile(msg *gotgbot.Message) (fileID, fileName string, fileSize int64) {
	switch {
	case msg.Document != nil:
		return msg.Document.FileId, coalesce(msg.Document.FileName, "document"), msg.Document.FileSize
	case msg.Video != nil:
		return msg.Video.FileId, coalesce(msg.Video.FileName, "video.mp4"), msg.Video.FileSize
	case msg.Audio != nil:
		return msg.Audio.FileId, coalesce(msg.Audio.FileName, "audio.mp3"), msg.Audio.FileSize
	case len(msg.Photo) > 0:
		p := msg.Photo[len(msg.Photo)-1]
		return p.FileId, "photo.jpg", p.FileSize
	default:
		return "", "", 0
	}
}

func coalesce(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

package callbacks

import (
	"context"
	"strings"

	"gdrive-bot/internal/drive"
	"gdrive-bot/internal/explorer"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

func (r *Router) openFileMenu(b *gotgbot.Bot, ctx *ext.Context, dc *drive.Client, fileID string) error {
	cq := ctx.Update.CallbackQuery
	entry, err := dc.FileMeta(context.Background(), fileID)
	if err != nil {
		return err
	}
	v := explorer.FileMenu(entry)
	return r.editView(b, cq, &v)
}

func (r *Router) handleFile(b *gotgbot.Bot, ctx *ext.Context, action string) error {
	c := context.Background()
	cq := ctx.Update.CallbackQuery
	userID := cq.From.Id
	chatID := cq.Message.GetChat().Id
	msgID := cq.Message.GetMessageId()

	dc, err := r.d.DriveClientFor(c, userID)
	if err != nil {
		return err
	}

	switch {
	case strings.HasPrefix(action, "rename:"):
		fileID := strings.TrimPrefix(action, "rename:")
		entry, err := dc.FileMeta(c, fileID)
		if err != nil {
			return err
		}
		if err := r.d.Explorer.AwaitRename(c, userID, chatID, msgID, fileID); err != nil {
			return err
		}
		v := explorer.RenamePrompt(entry)
		return r.editView(b, cq, &v)

	case strings.HasPrefix(action, "delete:"):
		fileID := strings.TrimPrefix(action, "delete:")
		entry, err := dc.FileMeta(c, fileID)
		if err != nil {
			return err
		}
		v := explorer.DeleteConfirm(entry)
		return r.editView(b, cq, &v)

	case strings.HasPrefix(action, "delconfirm:"):
		fileID := strings.TrimPrefix(action, "delconfirm:")
		if err := dc.Delete(c, fileID); err != nil {
			return err
		}
		r.d.Cache.InvalidatePrefix(c, "explist:")
		view, err := r.d.Explorer.RenderCurrent(c, dc, userID, chatID, msgID)
		if err != nil {
			return err
		}
		return r.editView(b, cq, view)

	case strings.HasPrefix(action, "delcancel:"):
		fileID := strings.TrimPrefix(action, "delcancel:")
		entry, err := dc.FileMeta(c, fileID)
		if err != nil {
			return err
		}
		v := explorer.FileMenu(entry)
		return r.editView(b, cq, &v)

	case strings.HasPrefix(action, "download:"):
		fileID := strings.TrimPrefix(action, "download:")
		entry, err := dc.FileMeta(c, fileID)
		if err != nil {
			return err
		}
		view, err := explorer.DownloadMenu(c, dc, entry)
		if err != nil {
			return err
		}
		return r.editView(b, cq, &view)

	case strings.HasPrefix(action, "back:"):
		view, err := r.d.Explorer.RenderCurrent(c, dc, userID, chatID, msgID)
		if err != nil {
			return err
		}
		return r.editView(b, cq, view)
	}

	return nil
}

func (r *Router) handleDownloadMenu(b *gotgbot.Bot, ctx *ext.Context, action string) error {
	c := context.Background()
	cq := ctx.Update.CallbackQuery
	userID := cq.From.Id

	dc, err := r.d.DriveClientFor(c, userID)
	if err != nil {
		return err
	}

	switch {
	case strings.HasPrefix(action, "share:"):
		fileID := strings.TrimPrefix(action, "share:")
		public, err := dc.IsPublic(c, fileID)
		if err != nil {
			return err
		}
		if err := dc.SetPublic(c, fileID, !public); err != nil {
			return err
		}
		entry, err := dc.FileMeta(c, fileID)
		if err != nil {
			return err
		}
		view, err := explorer.DownloadMenu(c, dc, entry)
		if err != nil {
			return err
		}
		return r.editView(b, cq, &view)

	case strings.HasPrefix(action, "viewlink:"):
		fileID := strings.TrimPrefix(action, "viewlink:")
		_, _ = cq.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text: drive.ViewLink(fileID), ShowAlert: true,
		})
		return nil

	case strings.HasPrefix(action, "directlink:"):
		fileID := strings.TrimPrefix(action, "directlink:")
		_, _ = cq.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text: drive.DirectLink(fileID), ShowAlert: true,
		})
		return nil

	case strings.HasPrefix(action, "back:"):
		fileID := strings.TrimPrefix(action, "back:")
		entry, err := dc.FileMeta(c, fileID)
		if err != nil {
			return err
		}
		v := explorer.FileMenu(entry)
		return r.editView(b, cq, &v)
	}
	return nil
}

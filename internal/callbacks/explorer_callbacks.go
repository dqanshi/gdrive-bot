package callbacks

import (
	"context"
	"fmt"
	"strings"

	"gdrive-bot/internal/explorer"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

func (r *Router) handleExplorer(b *gotgbot.Bot, ctx *ext.Context, action string) error {
	c := context.Background()
	cq := ctx.Update.CallbackQuery
	userID := cq.From.Id
	chatID := cq.Message.GetChat().Id
	msgID := cq.Message.GetMessageId()

	dc, err := r.d.DriveClientFor(c, userID)
	if err != nil {
		return err
	}

	var view *explorer.View
	switch {
	case strings.HasPrefix(action, "open:"):
		view, err = r.d.Explorer.OpenFolder(c, dc, userID, chatID, msgID, strings.TrimPrefix(action, "open:"))
	case strings.HasPrefix(action, "file:"):
		return r.openFileMenu(b, ctx, dc, strings.TrimPrefix(action, "file:"))
	case action == "back":
		view, err = r.d.Explorer.Back(c, dc, userID, chatID, msgID)
	case action == "home":
		view, err = r.d.Explorer.Home(c, dc, userID, chatID, msgID)
	case action == "refresh":
		view, err = r.d.Explorer.Refresh(c, dc, userID, chatID, msgID)
	case action == "prevpage":
		view, err = r.d.Explorer.Page(c, dc, userID, chatID, msgID, -1)
	case action == "nextpage":
		view, err = r.d.Explorer.Page(c, dc, userID, chatID, msgID, 1)
	case action == "search":
		if err := r.d.Explorer.AwaitSearch(c, userID, chatID, msgID); err != nil {
			return err
		}
		return r.editPlain(b, cq, "🔍 Send the filename (or partial name) you want to search for.\n\nReply to this message or just type and send.")
	case action == "clearsearch":
		view, err = r.d.Explorer.ClearSearch(c, dc, userID, chatID, msgID)
	default:
		return fmt.Errorf("unknown explorer action: %q", action)
	}

	if err != nil {
		return err
	}
	return r.editView(b, cq, view)
}

// editView edits the callback's message with a rendered explorer View.
func (r *Router) editView(b *gotgbot.Bot, cq *gotgbot.CallbackQuery, view *explorer.View) error {
	kb := gotgbot.InlineKeyboardMarkup{InlineKeyboard: view.Keyboard}
	return r.editPlainKb(b, cq, view.Text, &kb)
}

// editPlain edits the message to plain text with no keyboard.
func (r *Router) editPlain(b *gotgbot.Bot, cq *gotgbot.CallbackQuery, text string) error {
	return r.editMessage(b, cq, text, nil)
}

func (r *Router) editPlainKb(b *gotgbot.Bot, cq *gotgbot.CallbackQuery, text string, kb *gotgbot.InlineKeyboardMarkup) error {
	return r.editMessage(b, cq, text, kb)
}

func (r *Router) editMessage(b *gotgbot.Bot, cq *gotgbot.CallbackQuery, text string, kb *gotgbot.InlineKeyboardMarkup) error {
	opts := &gotgbot.EditMessageTextOpts{
		ChatId:    cq.Message.GetChat().Id,
		MessageId: cq.Message.GetMessageId(),
	}
	if kb != nil {
		opts.ReplyMarkup = *kb
	}
	_, _, err := b.EditMessageText(text, opts)
	return err
}

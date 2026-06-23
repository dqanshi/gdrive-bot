package handlers

import (
	"context"
	"fmt"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// MyFiles handles /myfiles: sends the explorer message at root and stores
// its state for in-place editing during navigation.
func (d *Deps) MyFiles(b *gotgbot.Bot, ctx *ext.Context) error {
	c := context.Background()
	userID := ctx.EffectiveUser.Id
	chatID := ctx.EffectiveChat.Id

	dc, err := d.DriveClientFor(c, userID)
	if err != nil {
		_, replyErr := ctx.EffectiveMessage.Reply(b, "❌ "+err.Error(), nil)
		return replyErr
	}

	placeholder, err := ctx.EffectiveMessage.Reply(b, "📂 Loading your Drive…", nil)
	if err != nil {
		return err
	}

	_, _, _, _, pageSize, _ := d.settings(c)
	view, err := d.Explorer.Open(c, dc, userID, chatID, placeholder.MessageId, pageSize)
	if err != nil {
		_, _, editErr := placeholder.EditText(b, "❌ Couldn't load Drive: "+err.Error(), nil)
		return editErr
	}

	kb := gotgbot.InlineKeyboardMarkup{InlineKeyboard: view.Keyboard}
	_, _, err = placeholder.EditText(b, view.Text, &gotgbot.EditMessageTextOpts{ReplyMarkup: kb})
	return err
}

// applyRename performs the Drive rename after the user replies with a new
// filename, then confirms inline.
func (d *Deps) applyRename(b *gotgbot.Bot, ctx *ext.Context, fileID, newName string) error {
	c := context.Background()
	dc, err := d.DriveClientFor(c, ctx.EffectiveUser.Id)
	if err != nil {
		_, replyErr := ctx.EffectiveMessage.Reply(b, "❌ "+err.Error(), nil)
		return replyErr
	}
	if err := dc.Rename(c, fileID, newName); err != nil {
		_, replyErr := ctx.EffectiveMessage.Reply(b, "❌ Rename failed: "+err.Error(), nil)
		return replyErr
	}
	d.Cache.InvalidatePrefix(c, "explist:")
	_, err = ctx.EffectiveMessage.Reply(b, fmt.Sprintf(`✅ Renamed to "%s".`, newName), nil)
	return err
}

// applySearch re-renders the explorer with a search filter, editing the
// original explorer message in place.
func (d *Deps) applySearch(b *gotgbot.Bot, ctx *ext.Context, explorerMsgID int64, query string) error {
	c := context.Background()
	userID := ctx.EffectiveUser.Id
	chatID := ctx.EffectiveChat.Id

	dc, err := d.DriveClientFor(c, userID)
	if err != nil {
		_, replyErr := ctx.EffectiveMessage.Reply(b, "❌ "+err.Error(), nil)
		return replyErr
	}
	view, err := d.Explorer.Search(c, dc, userID, chatID, explorerMsgID, query)
	if err != nil {
		_, replyErr := ctx.EffectiveMessage.Reply(b, "❌ Search failed: "+err.Error(), nil)
		return replyErr
	}

	kb := gotgbot.InlineKeyboardMarkup{InlineKeyboard: view.Keyboard}
	_, _, editErr := b.EditMessageText(view.Text, &gotgbot.EditMessageTextOpts{
		ChatId:      chatID,
		MessageId:   explorerMsgID,
		ReplyMarkup: kb,
	})
	if editErr != nil {
		// Original message no longer editable; send a fresh one.
		_, err = ctx.EffectiveMessage.Reply(b, view.Text, &gotgbot.SendMessageOpts{ReplyMarkup: kb})
		return err
	}
	return nil
}

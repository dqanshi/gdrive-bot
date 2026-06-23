package explorer

import (
	"context"
	"fmt"

	"gdrive-bot/internal/drive"
	"gdrive-bot/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// FileMenu renders the ✏ Rename / ⬇ Download / 🗑 Delete / 🔙 Back menu
// shown after tapping a file in the explorer, or right after an upload
// completes (per the spec's "After upload: Return file menu.").
func FileMenu(e drive.Entry) View {
	text := fmt.Sprintf("📄 %s\nSize: %s", e.Name, utils.HumanBytes(e.Size))
	kb := [][]gotgbot.InlineKeyboardButton{
		{
			{Text: "✏ Rename", CallbackData: "file:rename:" + e.ID},
			{Text: "⬇ Download", CallbackData: "file:download:" + e.ID},
		},
		{
			{Text: "🗑 Delete", CallbackData: "file:delete:" + e.ID},
		},
		{
			{Text: "🔙 Back", CallbackData: "file:back:" + e.ID},
		},
	}
	return View{Text: text, Keyboard: kb}
}

// DownloadMenu renders the 🔗 Share ON/OFF / 👁 View link / ⬇ Direct link /
// 🔙 Back menu, reflecting current share state with a live Drive API call.
func DownloadMenu(ctx context.Context, dc *drive.Client, e drive.Entry) (View, error) {
	public, err := dc.IsPublic(ctx, e.ID)
	if err != nil {
		return View{}, err
	}

	shareLabel := "🔗 Share: OFF (tap to enable)"
	if public {
		shareLabel = "🔗 Share: ON (tap to disable)"
	}

	text := fmt.Sprintf("📄 %s\nSize: %s\n\nSharing: %s", e.Name, utils.HumanBytes(e.Size), shareStatusWord(public))
	kb := [][]gotgbot.InlineKeyboardButton{
		{{Text: shareLabel, CallbackData: "dl:share:" + e.ID}},
		{
			{Text: "👁 View link", CallbackData: "dl:viewlink:" + e.ID},
			{Text: "⬇ Direct link", CallbackData: "dl:directlink:" + e.ID},
		},
		{{Text: "🔙 Back", CallbackData: "dl:back:" + e.ID}},
	}
	return View{Text: text, Keyboard: kb}, nil
}

func shareStatusWord(public bool) string {
	if public {
		return "Public (anyone with the link)"
	}
	return "Private"
}

// DeleteConfirm renders the confirmation step required before a Drive
// file is permanently deleted.
func DeleteConfirm(e drive.Entry) View {
	text := fmt.Sprintf("🗑 Delete \"%s\"?\n\nThis cannot be undone.", e.Name)
	kb := [][]gotgbot.InlineKeyboardButton{
		{
			{Text: "✅ Confirm delete", CallbackData: "file:delconfirm:" + e.ID},
			{Text: "❌ Cancel", CallbackData: "file:delcancel:" + e.ID},
		},
	}
	return View{Text: text, Keyboard: kb}
}

// RenamePrompt renders the "Send new filename." interactive prompt.
func RenamePrompt(e drive.Entry) View {
	text := fmt.Sprintf("✏ Renaming \"%s\"\n\nSend new filename.", e.Name)
	kb := [][]gotgbot.InlineKeyboardButton{
		{{Text: "❌ Cancel", CallbackData: "file:back:" + e.ID}},
	}
	return View{Text: text, Keyboard: kb}
}

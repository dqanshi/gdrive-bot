package handlers

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// Start handles /start.
func (d *Deps) Start(b *gotgbot.Bot, ctx *ext.Context) error {
	text := "👋 <b>Welcome to your private Google Drive manager.</b>\n\n" +
		"<b>Get started:</b>\n" +
		"/login — connect your Google Drive\n" +
		"/myfiles — browse, rename, delete, share\n" +
		"/status — check login status\n\n" +
		"Send any file or a direct https:// link and it will be uploaded to your Drive."
	_, err := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{
		ParseMode: "HTML",
	})
	return err
}

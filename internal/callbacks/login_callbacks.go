package callbacks

import (
	"context"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

func (r *Router) handleLoginContinue(b *gotgbot.Bot, ctx *ext.Context) error {
	cq := ctx.Update.CallbackQuery
	userID := cq.From.Id

	remote, err := r.d.CompleteLogin(context.Background(), userID)
	if err != nil {
		return err
	}

	text := "✅ <b>Login complete!</b> Your Google Drive is now connected.\n\n" +
		"Rclone remote: <code>" + remote + "</code>\n\n" +
		"Send /myfiles to start browsing, or just send any file or link to upload."
	_, _, _ = b.EditMessageText(text, &gotgbot.EditMessageTextOpts{
		ChatId:    cq.Message.GetChat().Id,
		MessageId: cq.Message.GetMessageId(),
		ParseMode: "HTML",
	})
	return nil
}

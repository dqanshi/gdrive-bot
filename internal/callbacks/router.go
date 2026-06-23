// Package callbacks handles every inline keyboard button press
// (CallbackQuery updates): explorer navigation, the file menu, the
// download/share menu, delete confirmation, and the login flow's
// "Continue Login" button. Every handler edits the existing message in
// place — the bot never sends a new message in response to a button tap.
package callbacks

import (
	"context"
	"strings"

	"gdrive-bot/internal/handlers"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// Router dispatches callback queries using the same Deps bundle as
// command handlers.
type Router struct {
	d *handlers.Deps
}

func NewRouter(d *handlers.Deps) *Router {
	return &Router{d: d}
}

// Dispatch is registered against gotgbot's CallbackQuery update type.
func (r *Router) Dispatch(b *gotgbot.Bot, ctx *ext.Context) error {
	cq := ctx.Update.CallbackQuery
	if cq == nil {
		return nil
	}
	userID := cq.From.Id

	authorized, err := r.d.Guard.IsAuthorized(context.Background(), userID, cq.From.Username)
	if err != nil {
		return err
	}
	if !authorized {
		_, _ = cq.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "🚫 Not authorized.", ShowAlert: true})
		return nil
	}

	data := cq.Data
	var handleErr error
	switch {
	case strings.HasPrefix(data, "expl:"):
		handleErr = r.handleExplorer(b, ctx, strings.TrimPrefix(data, "expl:"))
	case strings.HasPrefix(data, "file:"):
		handleErr = r.handleFile(b, ctx, strings.TrimPrefix(data, "file:"))
	case strings.HasPrefix(data, "dl:"):
		handleErr = r.handleDownloadMenu(b, ctx, strings.TrimPrefix(data, "dl:"))
	case data == "login:continue":
		handleErr = r.handleLoginContinue(b, ctx)
	}

	// Always acknowledge the tap so Telegram stops showing the loading
	// spinner on the button, even if the underlying action failed (the
	// failure is shown via the edited message text instead).
	ackText := ""
	if handleErr != nil {
		ackText = "❌ " + handleErr.Error()
	}
	_, _ = cq.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: ackText})
	return handleErr
}

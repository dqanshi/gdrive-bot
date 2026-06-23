package bot

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	gotghandlers "github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
)

// newCommandHandler returns a handler that matches /<cmd> (with or without
// the optional @BotUsername suffix Telegram appends in groups).
func newCommandHandler(cmd string, fn func(*gotgbot.Bot, *ext.Context) error) ext.Handler {
	return gotghandlers.NewCommand(cmd, fn)
}

// newMessageHandler returns a handler that fires on messages passing
// the predicate filter.
func newMessageHandler(filter func(*gotgbot.Message) bool, fn func(*gotgbot.Bot, *ext.Context) error) ext.Handler {
	return gotghandlers.NewMessage(filter, fn)
}

// newCallbackHandler fires on every CallbackQuery (all inline buttons).
func newCallbackHandler(fn func(*gotgbot.Bot, *ext.Context) error) ext.Handler {
	return gotghandlers.NewCallback(callbackquery.All, fn)
}

// Package progress provides a rate-limited Telegram message editor so
// progress bars don't hit the "429 Too Many Requests" flood-wait limit.
// Telegram allows roughly one edit/second per message; this wrapper
// coalesces rapid updates and sends only the most recent.
package progress

import (
	"context"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

const editInterval = 1200 * time.Millisecond

// Editor rate-limits EditMessageText calls for one progress message.
type Editor struct {
	bot       *gotgbot.Bot
	chatID    int64
	messageID int64
	parseMode string

	mu      sync.Mutex
	pending *string
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// New starts an Editor for the given message. Call Close() when the
// underlying operation finishes so the last progress state is flushed.
func New(b *gotgbot.Bot, chatID, messageID int64, parseMode string) *Editor {
	ctx, cancel := context.WithCancel(context.Background())
	e := &Editor{
		bot:       b,
		chatID:    chatID,
		messageID: messageID,
		parseMode: parseMode,
		cancel:    cancel,
	}
	e.wg.Add(1)
	go e.loop(ctx)
	return e
}

// Update queues a new text. Overwrites any unsent previous update —
// only the latest state matters for a progress bar.
func (e *Editor) Update(text string) {
	e.mu.Lock()
	e.pending = &text
	e.mu.Unlock()
}

// Close flushes any pending update and stops the background goroutine.
// Always call this (typically via defer) when the operation finishes.
func (e *Editor) Close() {
	e.cancel()
	e.wg.Wait()
	// Final flush — show the terminal state immediately.
	e.mu.Lock()
	p := e.pending
	e.pending = nil
	e.mu.Unlock()
	if p != nil {
		e.send(*p)
	}
}

func (e *Editor) loop(ctx context.Context) {
	defer e.wg.Done()
	ticker := time.NewTicker(editInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.mu.Lock()
			p := e.pending
			e.pending = nil
			e.mu.Unlock()
			if p != nil {
				e.send(*p)
			}
		}
	}
}

func (e *Editor) send(text string) {
	_, _, _ = e.bot.EditMessageText(text, &gotgbot.EditMessageTextOpts{
		ChatId:    e.chatID,
		MessageId: e.messageID,
		ParseMode: e.parseMode,
	})
}

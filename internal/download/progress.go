// Package download fetches source files onto local disk before they're
// handed to internal/upload — either by streaming a Telegram file_id
// through the Bot API, or by streaming a direct HTTP(S) URL. Both report
// progress (bytes done/total, speed, ETA) through the same channel type
// so the queue worker can render one consistent status message.
package download

import "time"

// Progress is emitted periodically while a download runs.
type Progress struct {
	BytesDone  int64
	BytesTotal int64 // 0 if unknown (e.g. server didn't send Content-Length)
	SpeedBps   float64
	Done       bool
	Err        error
}

// progressTicker throttles how often we push Progress updates so editing
// the Telegram status message doesn't hit rate limits (Telegram allows
// roughly one edit/sec per message).
type progressTicker struct {
	last     time.Time
	minDelta time.Duration
}

func newProgressTicker() *progressTicker {
	return &progressTicker{minDelta: 1200 * time.Millisecond}
}

func (t *progressTicker) ready() bool {
	if time.Since(t.last) < t.minDelta {
		return false
	}
	t.last = time.Now()
	return true
}

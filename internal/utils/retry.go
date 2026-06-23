package utils

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// RetryConfig controls the backoff/retry helper below.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// DefaultRetryConfig matches the "10x retry" requirement: ten attempts,
// exponential backoff with jitter, capped at 30s between attempts.
func DefaultRetryConfig(maxAttempts int) RetryConfig {
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	return RetryConfig{
		MaxAttempts: maxAttempts,
		BaseDelay:   500 * time.Millisecond,
		MaxDelay:    30 * time.Second,
	}
}

// Retry runs fn until it succeeds, the context is canceled, or
// MaxAttempts is exhausted. fn should return a wrapped error each
// attempt; the last error is returned if every attempt fails. Non-retryable
// errors can be signaled by wrapping with utils.NonRetryable.
func Retry(ctx context.Context, cfg RetryConfig, fn func(attempt int) error) error {
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn(attempt)
		if err == nil {
			return nil
		}
		lastErr = err
		if isNonRetryable(err) {
			return err
		}
		if attempt == cfg.MaxAttempts {
			break
		}
		delay := backoffDelay(cfg, attempt)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("retry: exhausted %d attempts: %w", cfg.MaxAttempts, lastErr)
}

func backoffDelay(cfg RetryConfig, attempt int) time.Duration {
	exp := float64(cfg.BaseDelay) * math.Pow(2, float64(attempt-1))
	jitter := 1 + (rand.Float64()*0.4 - 0.2) // +/-20%
	d := time.Duration(exp * jitter)
	if d > cfg.MaxDelay {
		d = cfg.MaxDelay
	}
	return d
}

// nonRetryableError marks an error as permanent (e.g. 401/403/404 from an
// API, or invalid input) so Retry stops immediately instead of burning
// through every attempt.
type nonRetryableError struct{ err error }

func (e *nonRetryableError) Error() string { return e.err.Error() }
func (e *nonRetryableError) Unwrap() error { return e.err }

// NonRetryable wraps err so Retry treats it as terminal.
func NonRetryable(err error) error {
	if err == nil {
		return nil
	}
	return &nonRetryableError{err: err}
}

func isNonRetryable(err error) bool {
	_, ok := err.(*nonRetryableError)
	return ok
}

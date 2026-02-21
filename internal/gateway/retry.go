package gateway

import (
	"math"
	"strings"
	"time"
)

// RetryPolicy controls how failed runs are retried with exponential backoff.
type RetryPolicy struct {
	MaxAttempts  int
	InitialDelay time.Duration
	Multiplier   float64
	MaxDelay     time.Duration
}

// DefaultRetryPolicy returns a RetryPolicy with sensible defaults:
// 3 attempts, 1s initial delay, 2x multiplier, 30s max delay.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		Multiplier:   2.0,
		MaxDelay:     30 * time.Second,
	}
}

// ShouldRetry returns true if the error is retryable and the attempt count
// has not exceeded MaxAttempts.
func (p *RetryPolicy) ShouldRetry(err error, attempt int) bool {
	if attempt > p.MaxAttempts {
		return false
	}
	return p.isRetryable(err)
}

// isRetryable classifies errors as retryable or permanent based on their message.
// Transient errors (connection, timeout) are retryable; auth/validation errors are not.
// Unknown errors default to retryable.
func (p *RetryPolicy) isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())

	// Transient / retryable errors
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporary failure") {
		return true
	}

	// Permanent / non-retryable errors
	if strings.Contains(msg, "invalid") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "forbidden") {
		return false
	}

	// Default: retryable
	return true
}

// NextDelay returns the backoff delay for the given attempt number (1-indexed).
// The delay is InitialDelay * Multiplier^(attempt-1), capped at MaxDelay.
func (p *RetryPolicy) NextDelay(attempt int) time.Duration {
	delay := float64(p.InitialDelay) * math.Pow(p.Multiplier, float64(attempt-1))
	if delay > float64(p.MaxDelay) {
		return p.MaxDelay
	}
	return time.Duration(delay)
}

// Execute runs fn up to MaxAttempts times, sleeping between retries with
// exponential backoff. Returns nil on success or the last error if all
// attempts fail or the error is non-retryable.
func (p *RetryPolicy) Execute(fn func() error) error {
	var lastErr error
	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if !p.ShouldRetry(err, attempt) {
			return err
		}
		if attempt < p.MaxAttempts {
			time.Sleep(p.NextDelay(attempt))
		}
	}
	return lastErr
}

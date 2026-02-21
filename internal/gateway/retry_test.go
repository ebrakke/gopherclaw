package gateway

import (
	"errors"
	"testing"
	"time"
)

func TestRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	if !policy.ShouldRetry(errors.New("connection refused"), 1) {
		t.Error("expected connection error to be retryable")
	}

	if policy.ShouldRetry(errors.New("error"), 4) {
		t.Error("should not retry after max attempts")
	}

	delay := policy.NextDelay(1)
	if delay != 1*time.Second {
		t.Errorf("expected 1s delay, got %v", delay)
	}

	delay = policy.NextDelay(2)
	if delay != 2*time.Second {
		t.Errorf("expected 2s delay, got %v", delay)
	}

	delay = policy.NextDelay(3)
	if delay != 4*time.Second {
		t.Errorf("expected 4s delay, got %v", delay)
	}
}

func TestRetryPolicyNonRetryable(t *testing.T) {
	policy := DefaultRetryPolicy()

	if policy.ShouldRetry(errors.New("invalid request"), 1) {
		t.Error("expected 'invalid' error to be non-retryable")
	}
	if policy.ShouldRetry(errors.New("unauthorized"), 1) {
		t.Error("expected 'unauthorized' error to be non-retryable")
	}
	if policy.ShouldRetry(errors.New("forbidden"), 1) {
		t.Error("expected 'forbidden' error to be non-retryable")
	}
}

func TestRetryPolicyNilError(t *testing.T) {
	policy := DefaultRetryPolicy()
	if policy.ShouldRetry(nil, 1) {
		t.Error("nil error should not be retryable")
	}
}

func TestRetryPolicyMaxDelayCap(t *testing.T) {
	policy := &RetryPolicy{
		MaxAttempts:  10,
		InitialDelay: 1 * time.Second,
		Multiplier:   10.0,
		MaxDelay:     30 * time.Second,
	}

	delay := policy.NextDelay(5)
	if delay > policy.MaxDelay {
		t.Errorf("delay %v exceeds max delay %v", delay, policy.MaxDelay)
	}
}

func TestRetryPolicyExecuteSuccess(t *testing.T) {
	policy := DefaultRetryPolicy()
	calls := 0

	err := policy.Execute(func() error {
		calls++
		if calls < 3 {
			return errors.New("temporary failure")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestRetryPolicyExecuteNonRetryable(t *testing.T) {
	policy := DefaultRetryPolicy()
	calls := 0

	err := policy.Execute(func() error {
		calls++
		return errors.New("invalid request")
	})

	if err == nil {
		t.Error("expected error for non-retryable failure")
	}
	if calls != 1 {
		t.Errorf("expected 1 call for non-retryable error, got %d", calls)
	}
}

func TestRetryPolicyExecuteAllFail(t *testing.T) {
	policy := &RetryPolicy{
		MaxAttempts:  2,
		InitialDelay: 1 * time.Millisecond,
		Multiplier:   1.0,
		MaxDelay:     10 * time.Millisecond,
	}
	calls := 0

	err := policy.Execute(func() error {
		calls++
		return errors.New("timeout")
	})

	if err == nil {
		t.Error("expected error after all attempts exhausted")
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

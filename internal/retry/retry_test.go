package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRetrySucceedsFirstAttempt verifies that when a function succeeds on the first try,
// no retries are attempted and no error is returned
func TestRetrySucceedsFirstAttempt(t *testing.T) {
	attempts := 0
	err := Retry(context.Background(), func(ctx context.Context) error {
		attempts++
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

// TestRetrySucceedsAfterSeveralAttempts verifies that the retry mechanism correctly
// retries a failing function until it succeeds
func TestRetrySucceedsAfterSeveralAttempts(t *testing.T) {
	attempts := 0
	targetAttempts := 3

	err := Retry(context.Background(), func(ctx context.Context) error {
		attempts++
		if attempts < targetAttempts {
			return errors.New("temporary error")
		}
		return nil
	}, WithConstantDelay(1*time.Millisecond), WithMaxAttempts(5))

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if attempts != targetAttempts {
		t.Errorf("expected %d attempts, got %d", targetAttempts, attempts)
	}
}

// TestRetryFailsAfterMaxAttempts verifies that the retry mechanism correctly gives up
// after the maximum number of attempts and returns the appropriate error
func TestRetryFailsAfterMaxAttempts(t *testing.T) {
	attempts := 0
	maxAttempts := 3

	err := Retry(context.Background(), func(ctx context.Context) error {
		attempts++
		return errors.New("persistent error")
	}, WithConstantDelay(1*time.Millisecond), WithMaxAttempts(maxAttempts))

	if err == nil {
		t.Error("expected error, got nil")
	}

	if !errors.Is(err, ErrMaxAttemptsReached) {
		t.Errorf("expected ErrMaxAttemptsReached, got %v", err)
	}

	if attempts != maxAttempts {
		t.Errorf("expected %d attempts, got %d", maxAttempts, attempts)
	}
}

// TestRetryRespectsContextCancellation verifies that the retry mechanism respects
// context cancellation and stops retrying when the context is canceled
func TestRetryRespectsContextCancellation(t *testing.T) {
	attempts := 0
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		// Cancel after a short delay
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	err := Retry(ctx, func(ctx context.Context) error {
		attempts++
		time.Sleep(1 * time.Millisecond)
		return errors.New("error that would normally cause retry")
	}, WithConstantDelay(1*time.Millisecond))

	if err == nil {
		t.Error("expected error due to cancellation, got nil")
	}

	if ctx.Err() == nil {
		t.Error("expected context to be canceled")
	}
}

// TestRetryRespectsTimeout verifies that the timeout option works correctly,
// stopping the retry process after the specified duration
func TestRetryRespectsTimeout(t *testing.T) {
	attempts := 0
	startTime := time.Now()
	timeout := 50 * time.Millisecond

	err := Retry(context.Background(), func(ctx context.Context) error {
		attempts++
		return errors.New("persistent error")
	}, WithConstantDelay(10*time.Millisecond), WithTimeout(timeout))

	duration := time.Since(startTime)

	if err == nil {
		t.Error("expected error due to timeout, got nil")
	}

	if !errors.Is(err, ErrTimeoutReached) {
		t.Errorf("expected ErrTimeoutReached, got %v", err)
	}

	// Verify that the timeout worked approximately correctly
	// (giving some margin for execution overhead)
	if duration < timeout || duration > timeout*2 {
		t.Errorf("expected duration around %v, got %v", timeout, duration)
	}
}

// TestRetryUsesExponentialBackoff verifies that the exponential backoff mechanism
// increases the delay between retry attempts
func TestRetryUsesExponentialBackoff(t *testing.T) {
	attempts := 0
	maxAttempts := 4
	baseDelay := 5 * time.Millisecond

	// Track timing of attempts
	var timestamps []time.Time

	err := Retry(context.Background(), func(ctx context.Context) error {
		timestamps = append(timestamps, time.Now())
		attempts++
		return errors.New("persistent error")
	}, WithExponentialBackoff(baseDelay, 2.0, false, 0), WithMaxAttempts(maxAttempts))

	if err == nil {
		t.Error("expected error, got nil")
	}

	// We should have maxAttempts timestamps
	if len(timestamps) != maxAttempts {
		t.Fatalf("expected %d timestamps, got %d", maxAttempts, len(timestamps))
	}

	// Verify that delays are increasing (with some tolerance for execution overhead)
	for i := 2; i < len(timestamps); i++ {
		prevInterval := timestamps[i-1].Sub(timestamps[i-2])
		currentInterval := timestamps[i].Sub(timestamps[i-1])

		if currentInterval < prevInterval {
			t.Errorf("expected increasing delays, but interval %d (%v) is smaller than interval %d (%v)",
				i, currentInterval, i-1, prevInterval)
		}
	}
}

// TestMustRetrySucceeds verifies that MustRetry does not panic when the operation succeeds
func TestMustRetrySucceeds(t *testing.T) {
	// Shouldn't panic
	MustRetry(context.Background(), func(ctx context.Context) error {
		return nil
	})
}

// TestMustRetryPanicsOnFailure verifies that MustRetry panics when the operation fails
func TestMustRetryPanicsOnFailure(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic but none occurred")
		}
	}()

	MustRetry(context.Background(), func(ctx context.Context) error {
		return errors.New("persistent error")
	}, WithConstantDelay(1*time.Millisecond), WithMaxAttempts(2))
}

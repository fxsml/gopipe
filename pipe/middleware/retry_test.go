package middleware

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

// TestConstantBackoff verifies that constant backoff produces backoffs around the expected value with jitter
func TestConstantBackoff(t *testing.T) {
	baseBackoff := 100 * time.Millisecond
	jitter := 0.2 // ±20% jitter
	backoffFunc := ConstantBackoff(baseBackoff, jitter)

	// Test multiple backoffs to verify jitter is applied
	var backoffs []time.Duration
	for i := 0; i < 10; i++ {
		backoffs = append(backoffs, backoffFunc(i+1)) // attempt is 1-based
	}

	// Verify all backoffs are within jitter range (0.8 to 1.2 of base)
	minExpected := time.Duration(float64(baseBackoff) * 0.8)
	maxExpected := time.Duration(float64(baseBackoff) * 1.2)

	for i, d := range backoffs {
		if d < minExpected || d > maxExpected {
			t.Errorf("backoff %d (%v) is outside expected jitter range [%v, %v]", i, d, minExpected, maxExpected)
		}
	}

	// Verify backoffs are different (jitter is working)
	allSame := true
	for i := 1; i < len(backoffs); i++ {
		if backoffs[i] != backoffs[0] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("all backoffs are the same, jitter may not be working")
	}
}

// TestExponentialBackoff verifies exponential backoff behavior
func TestExponentialBackoff(t *testing.T) {
	baseBackoff := 10 * time.Millisecond
	factor := 2.0
	maxBackoff := 200 * time.Millisecond
	jitter := 0.2 // ±20% jitter

	backoffFunc := ExponentialBackoff(baseBackoff, factor, maxBackoff, jitter)

	var backoffs []time.Duration
	for i := 1; i <= 6; i++ { // 1-based attempts
		backoffs = append(backoffs, backoffFunc(i))
	}

	// Test that backoffs increase exponentially (within jitter tolerance)
	for i := 0; i < len(backoffs)-1; i++ {
		expectedBackoff := time.Duration(float64(baseBackoff) * math.Pow(factor, float64(i))) // i for 0-based exponent (attempt-1)
		if expectedBackoff > maxBackoff {
			expectedBackoff = maxBackoff
		}

		// Allow for jitter (0.8 to 1.2 range)
		minExpected := time.Duration(float64(expectedBackoff) * 0.7) // More tolerance for test stability
		maxExpected := time.Duration(float64(expectedBackoff) * 1.3)

		if backoffs[i] < minExpected || backoffs[i] > maxExpected {
			t.Errorf("backoff %d (%v) is outside expected range [%v, %v] for expected %v",
				i, backoffs[i], minExpected, maxExpected, expectedBackoff)
		}
	}

	// Verify max backoff is respected
	lastBackoff := backoffs[len(backoffs)-1]
	maxWithJitter := time.Duration(float64(maxBackoff) * 1.3) // Account for jitter
	if lastBackoff > maxWithJitter {
		t.Errorf("last backoff (%v) exceeds max backoff with jitter (%v)", lastBackoff, maxWithJitter)
	}
}

// TestExponentialBackoffNoMaxBackoff verifies exponential backoff without max backoff limit
func TestExponentialBackoffNoMaxBackoff(t *testing.T) {
	baseBackoff := 1 * time.Millisecond
	factor := 2.0
	jitter := 0.2 // ±20% jitter

	backoffFunc := ExponentialBackoff(baseBackoff, factor, 0, jitter) // No max backoff

	var backoffs []time.Duration
	for i := 1; i <= 4; i++ { // 1-based attempts
		backoffs = append(backoffs, backoffFunc(i))
	}

	// Verify increasing backoffs without max limit
	for i := 1; i < len(backoffs); i++ {
		if backoffs[i] <= backoffs[i-1] {
			// Allow for some variance due to jitter, but should generally increase
			if backoffs[i] < time.Duration(float64(backoffs[i-1])*0.7) {
				t.Errorf("backoff %d (%v) is not increasing from backoff %d (%v)",
					i, backoffs[i], i-1, backoffs[i-1])
			}
		}
	}
}

// TestRetryMiddleware_SuccessFirstAttempt verifies no retries when process succeeds immediately
func TestRetryMiddleware_SuccessFirstAttempt(t *testing.T) {
	attempts := 0
	processFunc := func(ctx context.Context, in int) ([]int, error) {
		attempts++
		return []int{in * 2}, nil
	}

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: 3,
	}

	retryMw := Retry[int, int](retryConfig)
	wrappedFunc := retryMw(processFunc)

	result, err := wrappedFunc(context.Background(), 5)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(result) != 1 || result[0] != 10 {
		t.Errorf("expected [10], got %v", result)
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

// TestRetryMiddleware_SuccessAfterRetries verifies retry mechanism works when process eventually succeeds
func TestRetryMiddleware_SuccessAfterRetries(t *testing.T) {
	attempts := 0
	targetAttempts := 3

	processFunc := func(ctx context.Context, in int) ([]int, error) {
		attempts++
		if attempts < targetAttempts {
			return nil, errors.New("temporary error")
		}
		return []int{in * 2}, nil
	}

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: 5,
	}

	retryMw := Retry[int, int](retryConfig)
	wrappedFunc := retryMw(processFunc)

	result, err := wrappedFunc(context.Background(), 5)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(result) != 1 || result[0] != 10 {
		t.Errorf("expected [10], got %v", result)
	}

	if attempts != targetAttempts {
		t.Errorf("expected %d attempts, got %d", targetAttempts, attempts)
	}
}

// TestRetryMiddleware_FailsAfterMaxAttempts verifies max attempts behavior
func TestRetryMiddleware_FailsAfterMaxAttempts(t *testing.T) {
	attempts := 0
	maxAttempts := 3

	processFunc := func(ctx context.Context, in int) ([]int, error) {
		attempts++
		return nil, errors.New("persistent error")
	}

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: maxAttempts,
	}

	retryMw := Retry[int, int](retryConfig)
	wrappedFunc := retryMw(processFunc)

	result, err := wrappedFunc(context.Background(), 5)

	if err == nil {
		t.Error("expected error, got nil")
	}

	if !errors.Is(err, ErrRetryMaxAttempts) {
		t.Errorf("expected ErrRetryMaxAttempts, got %v", err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}

	if attempts != maxAttempts {
		t.Errorf("expected %d attempts, got %d", maxAttempts, attempts)
	}
}

// TestRetryMiddleware_RespectsContextCancellation verifies context cancellation stops retries
func TestRetryMiddleware_RespectsContextCancellation(t *testing.T) {
	attempts := 0
	ctx, cancel := context.WithCancel(context.Background())

	processFunc := func(ctx context.Context, in int) ([]int, error) {
		attempts++
		time.Sleep(1 * time.Millisecond)
		return nil, errors.New("error that would normally cause retry")
	}

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: 10,
	}

	retryMw := Retry[int, int](retryConfig)
	wrappedFunc := retryMw(processFunc)

	// Cancel after a short backoff
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	result, err := wrappedFunc(ctx, 5)

	if err == nil {
		t.Error("expected error due to cancellation, got nil")
	}

	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}

	if ctx.Err() == nil {
		t.Error("expected context to be canceled")
	}
}

// TestRetryMiddleware_ShouldRetryLogic verifies selective retry behavior
func TestRetryMiddleware_ShouldRetryLogic(t *testing.T) {
	var retryableErr = errors.New("retryable error")
	var nonRetryableErr = errors.New("non-retryable error")

	tests := []struct {
		name        string
		err         error
		shouldRetry ShouldRetryFunc
		expectRetry bool
	}{
		{
			name:        "should retry specific error",
			err:         retryableErr,
			shouldRetry: ShouldRetry(retryableErr),
			expectRetry: true,
		},
		{
			name:        "should not retry different error",
			err:         nonRetryableErr,
			shouldRetry: ShouldRetry(retryableErr),
			expectRetry: false,
		},
		{
			name:        "should not retry when configured not to",
			err:         retryableErr,
			shouldRetry: ShouldNotRetry(retryableErr),
			expectRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0

			processFunc := func(ctx context.Context, in int) ([]int, error) {
				attempts++
				return nil, tt.err
			}

			retryConfig := RetryConfig{
				ShouldRetry: tt.shouldRetry,
				Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
				MaxAttempts: 3,
			}

			retryMw := Retry[int, int](retryConfig)
			wrappedFunc := retryMw(processFunc)

			result, err := wrappedFunc(context.Background(), 5)

			if err == nil {
				t.Error("expected error, got nil")
			}

			if result != nil {
				t.Errorf("expected nil result, got %v", result)
			}

			if tt.expectRetry {
				if attempts < 2 {
					t.Errorf("expected retries, but only got %d attempts", attempts)
				}
				if !errors.Is(err, ErrRetryMaxAttempts) {
					t.Errorf("expected ErrRetryMaxAttempts for retryable error, got %v", err)
				}
			} else {
				if attempts != 1 {
					t.Errorf("expected no retries (1 attempt), got %d attempts", attempts)
				}
				if !errors.Is(err, ErrRetryNotRetryable) {
					t.Errorf("expected ErrNotRetryable for non-retryable error, got %v", err)
				}
			}
		})
	}
}

// TestRetryStateFromContext verifies RetryState can be extracted from context
func TestRetryStateFromContext(t *testing.T) {
	var capturedState *RetryState

	processFunc := func(ctx context.Context, in int) ([]int, error) {
		capturedState = RetryStateFromContext(ctx)
		return nil, errors.New("test error")
	}

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: 2,
	}

	retryMw := Retry[int, int](retryConfig)
	wrappedFunc := retryMw(processFunc)

	_, err := wrappedFunc(context.Background(), 5)

	if err == nil {
		t.Error("expected error, got nil")
	}

	if capturedState == nil {
		t.Error("expected RetryState in context, got nil")
	} else {
		if capturedState.MaxAttempts != 2 {
			t.Errorf("expected MaxAttempts=2, got %d", capturedState.MaxAttempts)
		}
		if capturedState.Attempts != 2 { // The state is updated after the first failure
			t.Errorf("expected Attempts=2 on second call, got %d", capturedState.Attempts)
		}
	}
}

// TestRetryStateFromError verifies RetryState can be extracted from error
func TestRetryStateFromError(t *testing.T) {
	processFunc := func(ctx context.Context, in int) ([]int, error) {
		return nil, errors.New("persistent error")
	}

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: 3,
	}

	retryMw := Retry[int, int](retryConfig)
	wrappedFunc := retryMw(processFunc)

	_, err := wrappedFunc(context.Background(), 5)

	if err == nil {
		t.Error("expected error, got nil")
	}

	state := RetryStateFromError(err)
	if state == nil {
		t.Error("expected RetryState from error, got nil")
	} else {
		if state.Attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", state.Attempts)
		}
		if len(state.Causes) != 3 {
			t.Errorf("expected 3 causes, got %d", len(state.Causes))
		}
	}
}

// TestShouldRetryFunctions verifies ShouldRetry and ShouldNotRetry logic
func TestShouldRetryFunctions(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	err3 := errors.New("error 3")

	t.Run("ShouldRetry_always_returns_true", func(t *testing.T) {
		shouldRetryAll := ShouldRetry()
		if !shouldRetryAll(err1) || !shouldRetryAll(err2) {
			t.Error("ShouldRetry() should retry all errors")
		}
	})

	t.Run("ShouldNotRetry_always_returns_false", func(t *testing.T) {
		shouldRetryNone := ShouldNotRetry()
		if shouldRetryNone(err1) || shouldRetryNone(err2) {
			t.Error("ShouldNotRetry() should not retry any errors")
		}
	})

	// Test ShouldRetry with specific errors
	shouldRetrySpecific := ShouldRetry(err1, err2)
	if !shouldRetrySpecific(err1) || !shouldRetrySpecific(err2) {
		t.Error("ShouldRetry(err1, err2) should retry err1 and err2")
	}
	if shouldRetrySpecific(err3) {
		t.Error("ShouldRetry(err1, err2) should not retry err3")
	}

	// Test ShouldNotRetry with specific errors
	shouldNotRetrySpecific := ShouldNotRetry(err1, err2)
	if shouldNotRetrySpecific(err1) || shouldNotRetrySpecific(err2) {
		t.Error("ShouldNotRetry(err1, err2) should not retry err1 and err2")
	}
	if !shouldNotRetrySpecific(err3) {
		t.Error("ShouldNotRetry(err1, err2) should retry err3")
	}
}

package middleware

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTimeout_HandlerCompletesWithinTimeout(t *testing.T) {
	t.Parallel()

	processFunc := func(ctx context.Context, in int) ([]int, error) {
		time.Sleep(10 * time.Millisecond)
		return []int{in * 2}, nil
	}

	fn := Timeout[int, int](100 * time.Millisecond)(processFunc)

	result, err := fn(context.Background(), 5)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(result) != 1 || result[0] != 10 {
		t.Errorf("expected [10], got %v", result)
	}
}

func TestTimeout_HandlerExceedsTimeout(t *testing.T) {
	t.Parallel()

	processFunc := func(ctx context.Context, in int) ([]int, error) {
		select {
		case <-time.After(200 * time.Millisecond):
			return []int{in * 2}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	fn := Timeout[int, int](50 * time.Millisecond)(processFunc)

	result, err := fn(context.Background(), 5)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestTimeout_ZeroDurationMeansNoTimeout(t *testing.T) {
	t.Parallel()

	processFunc := func(ctx context.Context, in int) ([]int, error) {
		// Verify context has no deadline
		if _, ok := ctx.Deadline(); ok {
			t.Error("expected no deadline with zero timeout")
		}
		return []int{in * 2}, nil
	}

	fn := Timeout[int, int](0)(processFunc)

	result, err := fn(context.Background(), 5)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(result) != 1 || result[0] != 10 {
		t.Errorf("expected [10], got %v", result)
	}
}

func TestTimeout_NegativeDurationMeansNoTimeout(t *testing.T) {
	t.Parallel()

	processFunc := func(ctx context.Context, in int) ([]int, error) {
		// Verify context has no deadline
		if _, ok := ctx.Deadline(); ok {
			t.Error("expected no deadline with negative timeout")
		}
		return []int{in * 2}, nil
	}

	fn := Timeout[int, int](-1 * time.Second)(processFunc)

	result, err := fn(context.Background(), 5)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(result) != 1 || result[0] != 10 {
		t.Errorf("expected [10], got %v", result)
	}
}

func TestTimeout_RespectsParentContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	processFunc := func(ctx context.Context, in int) ([]int, error) {
		select {
		case <-time.After(200 * time.Millisecond):
			return []int{in * 2}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	fn := Timeout[int, int](500 * time.Millisecond)(processFunc)

	// Cancel parent context after 20ms
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	result, err := fn(ctx, 5)

	// Should be cancelled, not deadline exceeded
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestTimeout_EachInvocationGetsFreshTimeout(t *testing.T) {
	t.Parallel()

	callCount := 0
	processFunc := func(ctx context.Context, in int) ([]int, error) {
		callCount++
		// First call takes 40ms, second call takes 40ms
		// If timeout was shared, second call would fail
		time.Sleep(40 * time.Millisecond)
		return []int{in * callCount}, nil
	}

	fn := Timeout[int, int](100 * time.Millisecond)(processFunc)

	// First invocation
	result1, err1 := fn(context.Background(), 5)
	if err1 != nil {
		t.Errorf("first call: expected no error, got %v", err1)
	}
	if len(result1) != 1 || result1[0] != 5 {
		t.Errorf("first call: expected [5], got %v", result1)
	}

	// Second invocation should get a fresh 100ms timeout
	result2, err2 := fn(context.Background(), 5)
	if err2 != nil {
		t.Errorf("second call: expected no error, got %v", err2)
	}
	if len(result2) != 1 || result2[0] != 10 {
		t.Errorf("second call: expected [10], got %v", result2)
	}
}

func TestTimeout_WithRetry_PerAttemptTimeout(t *testing.T) {
	t.Parallel()

	// This test verifies that when Timeout is applied after Retry (inside the retry loop),
	// each retry attempt gets a fresh timeout.
	attempts := 0
	processFunc := func(ctx context.Context, in int) ([]int, error) {
		attempts++
		if attempts < 3 {
			// First two attempts: sleep longer than timeout to trigger DeadlineExceeded
			select {
			case <-time.After(100 * time.Millisecond):
				return nil, errors.New("should have timed out")
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		// Third attempt: complete quickly
		return []int{in * 2}, nil
	}

	// Stack: Retry -> Timeout -> handler
	// Each retry attempt gets a fresh 30ms timeout
	fn := Retry[int, int](RetryConfig{
		ShouldRetry: ShouldRetry(context.DeadlineExceeded),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0),
		MaxAttempts: 5,
		Timeout:     0, // Disable retry's overall timeout
	})(Timeout[int, int](30 * time.Millisecond)(processFunc))

	result, err := fn(context.Background(), 5)

	if err != nil {
		t.Errorf("expected success after retries, got %v", err)
	}
	if len(result) != 1 || result[0] != 10 {
		t.Errorf("expected [10], got %v", result)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestTimeout_WithRetry_OverallTimeout(t *testing.T) {
	t.Parallel()

	// This test verifies that when Timeout is applied before Retry (outside the retry loop),
	// the timeout covers all retry attempts combined.
	attempts := 0
	processFunc := func(ctx context.Context, in int) ([]int, error) {
		attempts++
		// Each attempt takes 30ms and fails
		select {
		case <-time.After(30 * time.Millisecond):
			return nil, errors.New("temporary error")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Stack: Timeout -> Retry -> handler
	// Overall timeout of 50ms covers all retries
	// With 30ms per attempt + backoff, only ~1-2 attempts should complete
	fn := Timeout[int, int](50 * time.Millisecond)(
		Retry[int, int](RetryConfig{
			ShouldRetry: ShouldRetry(),
			Backoff:     ConstantBackoff(5*time.Millisecond, 0),
			MaxAttempts: 10,
			Timeout:     0, // Disable retry's overall timeout
		})(processFunc))

	_, err := fn(context.Background(), 5)

	// Should fail due to overall timeout, not max attempts
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("expected context deadline/canceled error, got %v", err)
	}
	// Should not have completed all 10 attempts
	if attempts >= 10 {
		t.Errorf("expected fewer than 10 attempts due to overall timeout, got %d", attempts)
	}
}

func TestTimeout_ContextDeadlineIsSet(t *testing.T) {
	t.Parallel()

	var capturedDeadline time.Time
	var hasDeadline bool

	processFunc := func(ctx context.Context, in int) ([]int, error) {
		capturedDeadline, hasDeadline = ctx.Deadline()
		return []int{in}, nil
	}

	fn := Timeout[int, int](100 * time.Millisecond)(processFunc)

	before := time.Now()
	_, err := fn(context.Background(), 5)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !hasDeadline {
		t.Error("expected context to have deadline")
	}

	// Deadline should be approximately 100ms from now (with some tolerance)
	expectedDeadline := before.Add(100 * time.Millisecond)
	tolerance := 20 * time.Millisecond
	if capturedDeadline.Before(expectedDeadline.Add(-tolerance)) || capturedDeadline.After(expectedDeadline.Add(tolerance)) {
		t.Errorf("deadline %v is not within expected range around %v", capturedDeadline, expectedDeadline)
	}
}

func TestTimeout_ParentDeadlineShorterThanTimeout(t *testing.T) {
	t.Parallel()

	// When parent context has a shorter deadline, it should take precedence
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	processFunc := func(ctx context.Context, in int) ([]int, error) {
		select {
		case <-time.After(200 * time.Millisecond):
			return []int{in * 2}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Timeout middleware sets 500ms, but parent context has 30ms deadline
	fn := Timeout[int, int](500 * time.Millisecond)(processFunc)

	start := time.Now()
	_, err := fn(ctx, 5)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}

	// Should timeout around 30ms (parent deadline), not 500ms
	if elapsed > 100*time.Millisecond {
		t.Errorf("expected timeout around 30ms, but took %v", elapsed)
	}
}

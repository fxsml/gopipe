package gopipe

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"testing"
	"time"
)

// Test helper to create a simple processor for testing
func createTestProcessor[In, Out any](processFunc func(context.Context, In) ([]Out, error)) Processor[In, Out] {
	return NewProcessor(
		processFunc,
		func(in In, err error) {
			// Simple cancel function for testing
		},
	)
}

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

// TestUseRetry_SuccessFirstAttempt verifies no retries when processor succeeds immediately
func TestUseRetry_SuccessFirstAttempt(t *testing.T) {
	attempts := 0
	processor := createTestProcessor(func(ctx context.Context, in int) ([]int, error) {
		attempts++
		return []int{in * 2}, nil
	})

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: 3,
	}

	retryMiddleware := useRetry[int, int](retryConfig)
	wrappedProcessor := retryMiddleware(processor)

	result, err := wrappedProcessor.Process(context.Background(), 5)

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

// TestUseRetry_SuccessAfterRetries verifies retry mechanism works when processor eventually succeeds
func TestUseRetry_SuccessAfterRetries(t *testing.T) {
	attempts := 0
	targetAttempts := 3

	processor := createTestProcessor(func(ctx context.Context, in int) ([]int, error) {
		attempts++
		if attempts < targetAttempts {
			return nil, errors.New("temporary error")
		}
		return []int{in * 2}, nil
	})

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: 5,
	}

	retryMiddleware := useRetry[int, int](retryConfig)
	wrappedProcessor := retryMiddleware(processor)

	result, err := wrappedProcessor.Process(context.Background(), 5)

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

// TestUseRetry_FailsAfterMaxAttempts verifies max attempts behavior
func TestUseRetry_FailsAfterMaxAttempts(t *testing.T) {
	attempts := 0
	maxAttempts := 3

	processor := createTestProcessor(func(ctx context.Context, in int) ([]int, error) {
		attempts++
		return nil, errors.New("persistent error")
	})

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: maxAttempts,
	}

	retryMiddleware := useRetry[int, int](retryConfig)
	wrappedProcessor := retryMiddleware(processor)

	result, err := wrappedProcessor.Process(context.Background(), 5)

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

// TestUseRetry_RespectsContextCancellation verifies context cancellation stops retries
func TestUseRetry_RespectsContextCancellation(t *testing.T) {
	attempts := 0
	ctx, cancel := context.WithCancel(context.Background())

	processor := createTestProcessor(func(ctx context.Context, in int) ([]int, error) {
		attempts++
		time.Sleep(1 * time.Millisecond)
		return nil, errors.New("error that would normally cause retry")
	})

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: 10,
	}

	retryMiddleware := useRetry[int, int](retryConfig)
	wrappedProcessor := retryMiddleware(processor)

	// Cancel after a short backoff
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	result, err := wrappedProcessor.Process(ctx, 5)

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

// TestUseRetry_RespectsTimeout verifies timeout functionality
func TestUseRetry_RespectsTimeout(t *testing.T) {
	attempts := 0
	timeout := 50 * time.Millisecond
	startTime := time.Now()

	processor := createTestProcessor(func(ctx context.Context, in int) ([]int, error) {
		attempts++
		return nil, errors.New("persistent error")
	})

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(10*time.Millisecond, 0.0),
		MaxAttempts: 10,
		Timeout:     timeout,
	}

	retryMiddleware := useRetry[int, int](retryConfig)
	wrappedProcessor := retryMiddleware(processor)

	result, err := wrappedProcessor.Process(context.Background(), 5)

	duration := time.Since(startTime)

	if err == nil {
		t.Error("expected error due to timeout, got nil")
	}

	if !errors.Is(err, ErrRetryTimeout) {
		t.Errorf("expected ErrRetryTimeout, got %v", err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}

	// Verify timeout worked approximately correctly
	if duration < timeout || duration > timeout*2 {
		t.Errorf("expected duration around %v, got %v", timeout, duration)
	}
}

// TestUseRetry_ShouldRetryLogic verifies selective retry behavior
func TestUseRetry_ShouldRetryLogic(t *testing.T) {
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
		{
			name:        "should retry different error when not in blacklist",
			err:         nonRetryableErr,
			shouldRetry: ShouldNotRetry(retryableErr),
			expectRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0

			processor := createTestProcessor(func(ctx context.Context, in int) ([]int, error) {
				attempts++
				return nil, tt.err
			})

			retryConfig := RetryConfig{
				ShouldRetry: tt.shouldRetry,
				Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
				MaxAttempts: 3,
			}

			retryMiddleware := useRetry[int, int](retryConfig)
			wrappedProcessor := retryMiddleware(processor)

			result, err := wrappedProcessor.Process(context.Background(), 5)

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

// TestUseRetry_UsesExponentialBackoff verifies exponential backoff timing
func TestUseRetry_UsesExponentialBackoff(t *testing.T) {
	attempts := 0
	maxAttempts := 4
	baseBackoff := 5 * time.Millisecond

	var timestamps []time.Time

	processor := createTestProcessor(func(ctx context.Context, in int) ([]int, error) {
		timestamps = append(timestamps, time.Now())
		attempts++
		return nil, errors.New("persistent error")
	})

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ExponentialBackoff(baseBackoff, 2.0, 0, 0.0),
		MaxAttempts: maxAttempts,
	}

	retryMiddleware := useRetry[int, int](retryConfig)
	wrappedProcessor := retryMiddleware(processor)

	_, err := wrappedProcessor.Process(context.Background(), 5)

	if err == nil {
		t.Error("expected error, got nil")
	}

	if len(timestamps) != maxAttempts {
		t.Fatalf("expected %d timestamps, got %d", maxAttempts, len(timestamps))
	}

	// Verify backoffs are increasing (with tolerance for execution overhead and jitter)
	for i := 2; i < len(timestamps); i++ {
		prevInterval := timestamps[i-1].Sub(timestamps[i-2])
		currentInterval := timestamps[i].Sub(timestamps[i-1])

		// Allow for some variance due to jitter and execution time
		if currentInterval < time.Duration(float64(prevInterval)*0.7) {
			t.Errorf("expected increasing backoffs, but interval %d (%v) is much smaller than interval %d (%v)",
				i, currentInterval, i-1, prevInterval)
		}
	}
}

// TestRetryStateFromContext verifies RetryState can be extracted from context
func TestRetryStateFromContext(t *testing.T) {
	var capturedState *RetryState

	processor := createTestProcessor(func(ctx context.Context, in int) ([]int, error) {
		capturedState = RetryStateFromContext(ctx)
		return nil, errors.New("test error")
	})

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: 2,
	}

	retryMiddleware := useRetry[int, int](retryConfig)
	wrappedProcessor := retryMiddleware(processor)

	_, err := wrappedProcessor.Process(context.Background(), 5)

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
	processor := createTestProcessor(func(ctx context.Context, in int) ([]int, error) {
		return nil, errors.New("persistent error")
	})

	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: 3,
	}

	retryMiddleware := useRetry[int, int](retryConfig)
	wrappedProcessor := retryMiddleware(processor)

	_, err := wrappedProcessor.Process(context.Background(), 5)

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

// TestRetryConfig_Parse verifies config parsing with defaults
func TestRetryConfig_Parse(t *testing.T) {
	// Test empty config gets defaults
	emptyConfig := RetryConfig{}
	parsed := emptyConfig.parse()

	if parsed.ShouldRetry == nil {
		t.Error("expected default ShouldRetry")
	}
	if parsed.Backoff == nil {
		t.Error("expected default Backoff")
	}
	if parsed.MaxAttempts != defaultRetryConfig.MaxAttempts {
		t.Errorf("expected default MaxAttempts %d, got %d", defaultRetryConfig.MaxAttempts, parsed.MaxAttempts)
	}

	// Test partial config keeps existing values
	partialConfig := RetryConfig{
		MaxAttempts: 5,
	}
	parsed = partialConfig.parse()

	if parsed.MaxAttempts != 5 {
		t.Errorf("expected MaxAttempts=5, got %d", parsed.MaxAttempts)
	}
	if parsed.ShouldRetry == nil {
		t.Error("expected default ShouldRetry for partial config")
	}
}

// TestUseRetry_NoTimeoutConfigured verifies that retry works without timeout configuration
func TestUseRetry_NoTimeoutConfigured(t *testing.T) {
	attempts := 0
	targetAttempts := 3

	processor := createTestProcessor(func(ctx context.Context, in int) ([]int, error) {
		attempts++
		if attempts < targetAttempts {
			return nil, errors.New("temporary error")
		}
		return []int{in * 2}, nil
	})

	// Test with no timeout configured (should use default of 0)
	retryConfig := RetryConfig{
		ShouldRetry: ShouldRetry(),
		Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
		MaxAttempts: 5,
		// Timeout is not set, should default to 0 (no timeout)
	}

	retryMiddleware := useRetry[int, int](retryConfig)
	wrappedProcessor := retryMiddleware(processor)

	result, err := wrappedProcessor.Process(context.Background(), 5)

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

// TestShouldRetryFunctions verifies ShouldRetry and ShouldNotRetry logic
func TestShouldRetryFunctions(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	err3 := errors.New("error 3")

	// Test ShouldRetry with no errors (retry all)
	shouldRetryAll := ShouldRetry()
	if !shouldRetryAll(err1) || !shouldRetryAll(err2) {
		t.Error("ShouldRetry() should retry all errors")
	}

	// Test ShouldRetry with specific errors
	shouldRetrySpecific := ShouldRetry(err1, err2)
	if !shouldRetrySpecific(err1) || !shouldRetrySpecific(err2) {
		t.Error("ShouldRetry(err1, err2) should retry err1 and err2")
	}
	if shouldRetrySpecific(err3) {
		t.Error("ShouldRetry(err1, err2) should not retry err3")
	}

	// Test ShouldNotRetry with no errors (retry none)
	shouldRetryNone := ShouldNotRetry()
	if shouldRetryNone(err1) || shouldRetryNone(err2) {
		t.Error("ShouldNotRetry() should not retry any errors")
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

// Integration tests for retry with pipes

// TestWithRetryConfig_ProcessPipe provides comprehensive testing of retry with ProcessPipe
func TestWithRetryConfig_ProcessPipe(t *testing.T) {
	t.Run("SuccessAfterRetries", func(t *testing.T) {
		attempts := 0
		processFunc := func(ctx context.Context, val int) ([]int, error) {
			attempts++
			if attempts < 3 {
				return nil, errors.New("temporary process error")
			}
			// Process: multiply by 2 and also add 10
			return []int{val * 2, val + 10}, nil
		}

		retryConfig := RetryConfig{
			ShouldRetry: ShouldRetry(),
			Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
			MaxAttempts: 5,
		}

		pipe := NewProcessPipe(
			processFunc,
			WithRetryConfig[int, int](retryConfig),
		)

		// Create input channel
		in := make(chan int, 1)
		in <- 5
		close(in)

		// Start the pipe
		out := pipe.Start(context.Background(), in)

		// Collect results
		var results []int
		for val := range out {
			results = append(results, val)
		}

		// Verify results
		expectedResults := []int{10, 15} // 5*2=10, 5+10=15
		if len(results) != len(expectedResults) {
			t.Errorf("expected %d results, got %d: %v", len(expectedResults), len(results), results)
		}

		for i, result := range results {
			if result != expectedResults[i] {
				t.Errorf("expected result %d to be %d, got %d", i, expectedResults[i], result)
			}
		}

		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("MaxAttemptsReached", func(t *testing.T) {
		attempts := 0
		processFunc := func(ctx context.Context, val int) ([]int, error) {
			attempts++
			return nil, errors.New("persistent process error")
		}

		retryConfig := RetryConfig{
			ShouldRetry: ShouldRetry(),
			Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
			MaxAttempts: 3,
		}

		pipe := NewProcessPipe(
			processFunc,
			WithRetryConfig[int, int](retryConfig),
		)

		// Create input channel
		in := make(chan int, 1)
		in <- 5
		close(in)

		// Start the pipe
		out := pipe.Start(context.Background(), in)

		// Collect results (should be empty due to failure)
		var results []int
		for val := range out {
			results = append(results, val)
		}

		// Should have no results due to retry failure
		if len(results) != 0 {
			t.Errorf("expected 0 results due to retry failure, got %d: %v", len(results), results)
		}

		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("NonRetryableError", func(t *testing.T) {
		attempts := 0
		retryableErr := errors.New("retryable error")
		nonRetryableErr := errors.New("non-retryable error")

		processFunc := func(ctx context.Context, val int) ([]int, error) {
			attempts++
			if val == 1 {
				return nil, retryableErr // This should be retried
			}
			return nil, nonRetryableErr // This should not be retried
		}

		retryConfig := RetryConfig{
			ShouldRetry: ShouldRetry(retryableErr), // Only retry specific error
			Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
			MaxAttempts: 3,
		}

		pipe := NewProcessPipe(
			processFunc,
			WithRetryConfig[int, int](retryConfig),
		)

		// Test retryable error
		t.Run("RetryableError", func(t *testing.T) {
			attempts = 0
			in := make(chan int, 1)
			in <- 1 // Will cause retryable error
			close(in)

			out := pipe.Start(context.Background(), in)
			var results []int
			for val := range out {
				results = append(results, val)
			}

			if len(results) != 0 {
				t.Errorf("expected 0 results, got %d: %v", len(results), results)
			}
			if attempts != 3 { // Should retry to max attempts
				t.Errorf("expected 3 attempts for retryable error, got %d", attempts)
			}
		})

		// Test non-retryable error
		t.Run("NonRetryableError", func(t *testing.T) {
			attempts = 0
			in := make(chan int, 1)
			in <- 2 // Will cause non-retryable error
			close(in)

			out := pipe.Start(context.Background(), in)
			var results []int
			for val := range out {
				results = append(results, val)
			}

			if len(results) != 0 {
				t.Errorf("expected 0 results, got %d: %v", len(results), results)
			}
			if attempts != 1 { // Should not retry
				t.Errorf("expected 1 attempt for non-retryable error, got %d", attempts)
			}
		})
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		attempts := 0
		processFunc := func(ctx context.Context, val int) ([]int, error) {
			attempts++
			// Add some backoff to allow cancellation
			time.Sleep(5 * time.Millisecond)
			return nil, errors.New("always fails")
		}

		retryConfig := RetryConfig{
			ShouldRetry: ShouldRetry(),
			Backoff:     ConstantBackoff(5*time.Millisecond, 0.0),
			MaxAttempts: 10,
		}

		pipe := NewProcessPipe(
			processFunc,
			WithRetryConfig[int, int](retryConfig),
		)

		// Create cancellable context
		ctx, cancel := context.WithCancel(context.Background())

		// Cancel after a short backoff
		go func() {
			time.Sleep(15 * time.Millisecond)
			cancel()
		}()

		// Create input channel
		in := make(chan int, 1)
		in <- 5
		close(in)

		// Start the pipe
		out := pipe.Start(ctx, in)

		// Collect results
		var results []int
		for val := range out {
			results = append(results, val)
		}

		// Should have no results due to cancellation
		if len(results) != 0 {
			t.Errorf("expected 0 results due to cancellation, got %d: %v", len(results), results)
		}

		// Should have attempted at least once but not reached max attempts
		if attempts == 0 {
			t.Error("expected at least 1 attempt")
		}
		if attempts >= 10 {
			t.Errorf("expected fewer than 10 attempts due to cancellation, got %d", attempts)
		}
	})

	t.Run("ExponentialBackoff", func(t *testing.T) {
		attempts := 0
		var timestamps []time.Time

		processFunc := func(ctx context.Context, val int) ([]int, error) {
			timestamps = append(timestamps, time.Now())
			attempts++
			return nil, errors.New("always fails")
		}

		retryConfig := RetryConfig{
			ShouldRetry: ShouldRetry(),
			Backoff:     ExponentialBackoff(5*time.Millisecond, 2.0, 50*time.Millisecond, 0.0),
			MaxAttempts: 4,
		}

		pipe := NewProcessPipe(
			processFunc,
			WithRetryConfig[int, int](retryConfig),
		)

		// Create input channel
		in := make(chan int, 1)
		in <- 5
		close(in)

		// Start the pipe
		out := pipe.Start(context.Background(), in)

		// Collect results
		var results []int
		for val := range out {
			results = append(results, val)
		}

		// Verify exponential backoff timing
		if len(timestamps) != 4 {
			t.Fatalf("expected 4 timestamps, got %d", len(timestamps))
		}

		// Check that backoffs are increasing (with tolerance for jitter and execution time)
		for i := 2; i < len(timestamps); i++ {
			prevInterval := timestamps[i-1].Sub(timestamps[i-2])
			currentInterval := timestamps[i].Sub(timestamps[i-1])

			// Allow for variance due to jitter and execution time, but should generally increase
			if currentInterval < time.Duration(float64(prevInterval)*0.7) {
				t.Errorf("expected increasing backoffs with exponential backoff, but interval %d (%v) is much smaller than interval %d (%v)",
					i, currentInterval, i-1, prevInterval)
			}
		}
	})

	t.Run("MultipleInputsConcurrent", func(t *testing.T) {
		attemptsByValue := make(map[int]int)
		var mu sync.Mutex

		processFunc := func(ctx context.Context, val int) ([]int, error) {
			mu.Lock()
			attemptsByValue[val]++
			attempts := attemptsByValue[val]
			mu.Unlock()

			if attempts < 2 {
				return nil, fmt.Errorf("temporary error for value %d", val)
			}
			return []int{val * 2}, nil
		}

		retryConfig := RetryConfig{
			ShouldRetry: ShouldRetry(),
			Backoff:     ConstantBackoff(1*time.Millisecond, 0.0),
			MaxAttempts: 3,
		}

		pipe := NewProcessPipe(
			processFunc,
			WithRetryConfig[int, int](retryConfig),
			WithConcurrency[int, int](3), // Enable concurrent processing
		)

		// Create input channel with multiple values
		in := make(chan int, 5)
		expectedInputs := []int{1, 2, 3, 4, 5}
		for _, val := range expectedInputs {
			in <- val
		}
		close(in)

		// Start the pipe
		out := pipe.Start(context.Background(), in)

		// Collect results
		var results []int
		for val := range out {
			results = append(results, val)
		}

		// Verify results (should be input values * 2)
		expectedResults := []int{2, 4, 6, 8, 10}
		if len(results) != len(expectedResults) {
			t.Errorf("expected %d results, got %d: %v", len(expectedResults), len(results), results)
		}

		// Sort both slices to compare (concurrent processing may change order)
		sort.Ints(results)
		sort.Ints(expectedResults)

		for i, result := range results {
			if result != expectedResults[i] {
				t.Errorf("expected result %d to be %d, got %d", i, expectedResults[i], result)
			}
		}

		// Each input should have been attempted twice
		mu.Lock()
		for _, val := range expectedInputs {
			if attemptsByValue[val] != 2 {
				t.Errorf("expected 2 attempts for value %d, got %d", val, attemptsByValue[val])
			}
		}
		mu.Unlock()
	})
}

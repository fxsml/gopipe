package pipe

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestProcessing_Basic(t *testing.T) {
	in := make(chan int)

	process := func(ctx context.Context, v int) ([]int, error) {
		return []int{v * 2}, nil
	}

	out := startProcessing(context.Background(), in, process, Config{Concurrency: 2})

	go func() {
		in <- 1
		in <- 2
		close(in)
	}()

	var got []int
	for v := range out {
		got = append(got, v)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %v", got)
	}
}

func TestProcessing_CleanupBeforeChannelClose(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	in := make(chan int, 1)
	in <- 3
	close(in)
	cleanupStarted := make(chan struct{})
	cleanupRelease := make(chan struct{})
	cleanupDone := make(chan struct{})
	cleanup := func(ctx context.Context) {
		close(cleanupStarted)
		<-cleanupRelease
		close(cleanupDone)
	}
	process := func(ctx context.Context, val int) ([]int, error) {
		return []int{val * 4}, nil
	}

	out := startProcessing(ctx, in, process, Config{
		CleanupHandler: cleanup,
		CleanupTimeout: 0,
	})
	outputClosed := make(chan struct{})
	go func() {
		for range out {
		}
		close(outputClosed)
	}()
	<-cleanupStarted
	select {
	case <-outputClosed:
		t.Error("Output channel closed before cleanup finished")
	default:
	}
	close(cleanupRelease)
	<-cleanupDone
	select {
	case <-outputClosed:
	case <-time.After(100 * time.Millisecond):
		t.Error("Output channel not closed after cleanup finished")
	}
}

func TestProcessing_ErrorCallsErrorHandler(t *testing.T) {
	in := make(chan int)

	process := func(ctx context.Context, v int) ([]int, error) {
		return nil, errors.New("fail")
	}

	var mu sync.Mutex
	var errorCalls []int

	out := startProcessing(context.Background(), in, process, Config{
		Concurrency: 1,
		ErrorHandler: func(in any, err error) {
			mu.Lock()
			errorCalls = append(errorCalls, in.(int))
			mu.Unlock()
		},
	})

	go func() {
		in <- 7
		close(in)
	}()

	// drain out and wait a bit for error handler callback
	go func() {
		for range out {
		}
	}()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(errorCalls) != 1 || errorCalls[0] != 7 {
		t.Fatalf("expected error handler called with 7, got %v", errorCalls)
	}
	mu.Unlock()
}

func TestProcessing_WithBufferAndConcurrency(t *testing.T) {
	in := make(chan int)

	process := func(ctx context.Context, v int) ([]int, error) {
		return []int{v + 1}, nil
	}

	out := startProcessing(context.Background(), in, process, Config{
		Concurrency: 3,
		BufferSize:  2,
	})

	go func() {
		in <- 1
		in <- 2
		in <- 3
		close(in)
	}()

	var got []int
	for v := range out {
		got = append(got, v)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %v", got)
	}
}

func TestProcessing_NoGoroutineLeakOnChannelClose(t *testing.T) {
	// Count goroutines before starting
	initialGoroutines := runtime.NumGoroutine()

	in := make(chan int)

	process := func(ctx context.Context, v int) ([]int, error) {
		return []int{v * 2}, nil
	}

	ctx := context.Background()
	out := startProcessing(ctx, in, process, Config{})

	// Add one item and then close the input channel without cancelling the context
	go func() {
		in <- 1
		close(in)
	}()

	// Drain the output channel - this will complete
	for range out {
	}

	// Check if we have a goroutine leak
	finalGoroutines := runtime.NumGoroutine()
	leakedGoroutines := finalGoroutines - initialGoroutines

	if leakedGoroutines > 0 {
		t.Errorf("Unexpected goroutine leak: %d goroutine(s) leaked even though the channel was closed", leakedGoroutines)
	}
}

func TestProcessing_WithErrorHandler(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Track error handler calls
	var (
		errorHandlerCalled bool
		errorVal           any
		errorErr           error
		mu                 sync.Mutex
	)

	process := func(ctx context.Context, val int) ([]int, error) {
		if val < 0 {
			return nil, errors.New("negative value")
		}
		return []int{val * 2}, nil
	}

	in := make(chan int, 2)
	in <- 5
	in <- -1 // This will fail
	close(in)

	out := startProcessing(ctx, in, process, Config{
		ErrorHandler: func(val any, err error) {
			mu.Lock()
			defer mu.Unlock()
			errorHandlerCalled = true
			errorVal = val
			errorErr = err
		},
	})

	// Collect results
	var results []int
	for result := range out {
		results = append(results, result)
	}

	// Give time for error handler to execute
	time.Sleep(50 * time.Millisecond)

	// Verify error handler was called for failed value
	mu.Lock()
	defer mu.Unlock()

	if !errorHandlerCalled {
		t.Error("Expected error handler function to be called")
	}

	if errorVal != -1 {
		t.Errorf("Expected error value -1, got %v", errorVal)
	}

	if errorErr == nil {
		t.Error("Expected error handler to receive an error")
	}

	// Verify successful result
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	if len(results) > 0 && results[0] != 10 {
		t.Errorf("Expected result 10, got %d", results[0])
	}
}

func TestProcessing_MultipleErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Track error handler calls in order
	var (
		errorCalls []any
		mu         sync.Mutex
	)

	process := func(ctx context.Context, val int) ([]int, error) {
		return nil, errors.New("fail")
	}

	in := make(chan int, 3)
	in <- 1
	in <- 2
	in <- 3
	close(in)

	out := startProcessing(ctx, in, process, Config{
		Concurrency: 1,
		ErrorHandler: func(val any, err error) {
			mu.Lock()
			errorCalls = append(errorCalls, val)
			mu.Unlock()
		},
	})

	// Drain output
	for range out {
	}

	// Give time for error handlers to execute
	time.Sleep(50 * time.Millisecond)

	// Verify all errors were handled
	mu.Lock()
	defer mu.Unlock()

	if len(errorCalls) != 3 {
		t.Errorf("Expected 3 error handler calls, got %d: %v", len(errorCalls), errorCalls)
	}
}

func TestProcessing_ShutdownTimeout_ForcedExit(t *testing.T) {
	// Test that workers stop forwarding after ShutdownTimeout, drain input,
	// and exit when input closes.

	ctx, cancel := context.WithCancel(context.Background())

	var dropped int
	var mu sync.Mutex
	in := make(chan int, 10)
	for i := 0; i < 10; i++ {
		in <- i
	}

	process := func(ctx context.Context, v int) ([]int, error) {
		return []int{v}, nil
	}

	out := startProcessing(ctx, in, process, Config{
		ShutdownTimeout: 50 * time.Millisecond,
		BufferSize:      1, // Small buffer to cause blocking
		ErrorHandler: func(in any, err error) {
			if err == ErrShutdownDropped {
				mu.Lock()
				dropped++
				mu.Unlock()
			}
		},
	})

	// Read one to start processing, then let output fill
	<-out
	time.Sleep(10 * time.Millisecond)

	// Cancel context - should trigger forced shutdown after 50ms
	cancel()

	// Wait for timeout to fire before closing input
	time.Sleep(60 * time.Millisecond)
	close(in)

	// Drain remaining output
	for range out {
	}

	// Verify some messages were reported as dropped during drain
	mu.Lock()
	d := dropped
	mu.Unlock()
	if d == 0 {
		t.Error("expected some messages to be reported as dropped")
	}
}

func TestProcessing_ShutdownTimeout_NaturalCompletion(t *testing.T) {
	// Test that workers exit naturally when input closes, even with ShutdownTimeout set.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in := make(chan int, 2)
	in <- 1
	in <- 2
	close(in)

	var results []int
	var mu sync.Mutex

	process := func(ctx context.Context, v int) ([]int, error) {
		return []int{v * 2}, nil
	}

	out := startProcessing(ctx, in, process, Config{
		ShutdownTimeout: 50 * time.Millisecond,
	})

	for v := range out {
		mu.Lock()
		results = append(results, v)
		mu.Unlock()
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestProcessing_ShutdownTimeout_BlockedOnOutput(t *testing.T) {
	// Test that workers blocked on output write escape after ShutdownTimeout,
	// then drain remaining input.

	ctx, cancel := context.WithCancel(context.Background())

	var dropped int
	var mu sync.Mutex
	in := make(chan int, 5)
	for i := 0; i < 5; i++ {
		in <- i
	}

	aboutToWrite := make(chan struct{}, 1)
	process := func(ctx context.Context, v int) ([]int, error) {
		select {
		case aboutToWrite <- struct{}{}:
		default:
		}
		return []int{v}, nil
	}

	// Unbuffered output - worker will block on write if nobody reads
	out := startProcessing(ctx, in, process, Config{
		BufferSize:      0,
		ShutdownTimeout: 50 * time.Millisecond,
		ErrorHandler: func(in any, err error) {
			if err == ErrShutdownDropped {
				mu.Lock()
				dropped++
				mu.Unlock()
			}
		},
	})

	// Wait for worker to be about to write to output
	<-aboutToWrite

	// Give worker time to actually enter the blocked write
	time.Sleep(20 * time.Millisecond)

	// Cancel context - should trigger forced shutdown
	start := time.Now()
	cancel()

	// Wait for timeout to fire before closing input
	time.Sleep(60 * time.Millisecond)
	close(in)

	// Drain output and detect close
	closed := make(chan struct{})
	go func() {
		for range out {
		}
		close(closed)
	}()

	select {
	case <-closed:
		elapsed := time.Since(start)
		if elapsed < 40*time.Millisecond {
			t.Errorf("output closed too quickly (%v), expected ~50ms timeout", elapsed)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("output channel not closed after shutdown timeout (worker blocked on output)")
	}

	// Verify remaining messages were reported as dropped
	mu.Lock()
	d := dropped
	mu.Unlock()
	if d == 0 {
		t.Error("expected some messages to be reported as dropped")
	}
}

func TestProcessing_ZeroShutdownTimeout_ImmediateShutdown(t *testing.T) {
	// Test that with ShutdownTimeout <= 0, shutdown is immediate (no grace period).
	// Shutdown should complete quickly without waiting for slow processing.

	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan int, 10)
	for i := 0; i < 10; i++ {
		in <- i
	}
	close(in)

	started := make(chan struct{})
	blockProcessing := make(chan struct{})
	var startedOnce sync.Once

	// Block processing until we signal - simulates slow work
	process := func(ctx context.Context, v int) ([]int, error) {
		startedOnce.Do(func() { close(started) })
		<-blockProcessing
		return []int{v}, nil
	}

	out := startProcessing(ctx, in, process, Config{
		ShutdownTimeout: 0, // Immediate shutdown, no grace period
		BufferSize:      10,
	})

	// Wait for processing to start
	<-started

	// Cancel while worker is blocked
	cancel()

	// Unblock processing
	close(blockProcessing)

	// Output channel should close quickly (not wait for all items)
	done := make(chan struct{})
	go func() {
		for range out {
		}
		close(done)
	}()

	select {
	case <-done:
		// Success - shutdown completed quickly
	case <-time.After(100 * time.Millisecond):
		t.Error("shutdown took too long with ShutdownTimeout=0")
	}
}

func TestProcessing_ShutdownDrain_ComprehensiveBehavior(t *testing.T) {
	// This test verifies the complete shutdown/drain behavior:
	// - ShutdownTimeout > 0: grace period, then forced shutdown with drain
	// - ShutdownTimeout <= 0: immediate forced shutdown with drain (no grace)
	// - All drained messages are reported via ErrorHandler with ErrShutdownDropped
	// - Input must be closed by user for drain to complete

	t.Run("grace period then drain", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		var dropped []int
		var mu sync.Mutex

		in := make(chan int, 20)
		for i := 0; i < 20; i++ {
			in <- i
		}

		processed := make(chan int, 5)
		process := func(ctx context.Context, v int) ([]int, error) {
			processed <- v
			return []int{v}, nil
		}

		out := startProcessing(ctx, in, process, Config{
			ShutdownTimeout: 50 * time.Millisecond, // 50ms grace period
			BufferSize:      1,                     // Small buffer to cause blocking
			ErrorHandler: func(val any, err error) {
				if err == ErrShutdownDropped {
					mu.Lock()
					dropped = append(dropped, val.(int))
					mu.Unlock()
				}
			},
		})

		// Read one output to start processing
		<-out

		// Wait for a few to be processed
		time.Sleep(10 * time.Millisecond)

		// Cancel - grace period starts, timeout will fire after 50ms
		cancel()

		// Wait for timeout to fire
		time.Sleep(60 * time.Millisecond)

		// Close input to allow drain to complete
		close(in)

		// Drain output
		for range out {
		}

		// Verify drops were reported
		mu.Lock()
		droppedCount := len(dropped)
		mu.Unlock()

		if droppedCount == 0 {
			t.Error("expected some messages to be reported as dropped during drain")
		}

		close(processed)
	})

	t.Run("zero timeout means immediate shutdown", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		// Buffered channel with items ready to be processed
		in := make(chan int, 10)
		for i := 0; i < 10; i++ {
			in <- i
		}
		close(in)

		started := make(chan struct{})
		blockProcessing := make(chan struct{})
		var startedOnce sync.Once

		// Block processing until we signal - simulates slow work
		process := func(ctx context.Context, v int) ([]int, error) {
			startedOnce.Do(func() { close(started) })
			<-blockProcessing
			return []int{v}, nil
		}

		out := startProcessing(ctx, in, process, Config{
			ShutdownTimeout: 0, // No grace period - immediate shutdown
			BufferSize:      10,
		})

		// Wait for processing to start (first item is being processed)
		<-started

		// Cancel while worker is blocked on processing
		cancel()

		// Unblock processing
		close(blockProcessing)

		// Output channel should close quickly (not wait for all items)
		// With zero timeout, shutdown is immediate once we unblock
		done := make(chan struct{})
		go func() {
			for range out {
			}
			close(done)
		}()

		select {
		case <-done:
			// Success - shutdown completed
		case <-time.After(100 * time.Millisecond):
			t.Error("shutdown took too long with ShutdownTimeout=0")
		}
	})
}

// TestProcessTimeout_NormalOperation tests that ProcessTimeout is enforced during normal operation
func TestProcessTimeout_NormalOperation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	in := make(chan int, 1)
	in <- 1
	close(in)

	var handlerErr error
	var mu sync.Mutex

	// Handler that takes longer than ProcessTimeout
	process := func(ctx context.Context, v int) ([]int, error) {
		select {
		case <-time.After(200 * time.Millisecond):
			return []int{v}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	out := startProcessing(ctx, in, process, Config{
		ProcessTimeout: 50 * time.Millisecond, // Should timeout after 50ms
		ErrorHandler: func(val any, err error) {
			mu.Lock()
			handlerErr = err
			mu.Unlock()
		},
	})

	// Drain output
	for range out {
	}

	// Give time for error handler
	time.Sleep(10 * time.Millisecond)

	// Verify handler was cancelled due to timeout
	mu.Lock()
	defer mu.Unlock()
	if !errors.Is(handlerErr, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", handlerErr)
	}
}

// TestProcessTimeout_HandlerCompletesWithinTimeout tests that handlers complete normally when under timeout
func TestProcessTimeout_HandlerCompletesWithinTimeout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	in := make(chan int, 1)
	in <- 42
	close(in)

	// Handler completes quickly
	process := func(ctx context.Context, v int) ([]int, error) {
		time.Sleep(10 * time.Millisecond)
		return []int{v * 2}, nil
	}

	out := startProcessing(ctx, in, process, Config{
		ProcessTimeout: 100 * time.Millisecond, // Plenty of time
	})

	// Collect results
	var results []int
	for v := range out {
		results = append(results, v)
	}

	if len(results) != 1 || results[0] != 84 {
		t.Errorf("expected [84], got %v", results)
	}
}

// TestProcessTimeout_GracefulShutdown tests that ProcessTimeout is still enforced during graceful shutdown
func TestProcessTimeout_GracefulShutdown(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	in := make(chan int, 2)
	in <- 1
	in <- 2
	close(in) // Graceful shutdown - input closed

	var timeoutCount int
	var mu sync.Mutex

	// Handler that takes longer than ProcessTimeout
	process := func(ctx context.Context, v int) ([]int, error) {
		select {
		case <-time.After(200 * time.Millisecond):
			return []int{v}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	out := startProcessing(ctx, in, process, Config{
		ProcessTimeout: 50 * time.Millisecond,
		ErrorHandler: func(val any, err error) {
			if errors.Is(err, context.DeadlineExceeded) {
				mu.Lock()
				timeoutCount++
				mu.Unlock()
			}
		},
	})

	// Drain output
	for range out {
	}

	// Give time for error handlers
	time.Sleep(10 * time.Millisecond)

	// Both handlers should timeout
	mu.Lock()
	defer mu.Unlock()
	if timeoutCount != 2 {
		t.Errorf("expected 2 timeouts, got %d", timeoutCount)
	}
}

// TestProcessTimeout_DuringGracePeriod tests that handlers continue with ProcessTimeout during grace period
func TestProcessTimeout_DuringGracePeriod(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan int, 1)
	in <- 1

	handlerStarted := make(chan struct{})
	var handlerCompleted bool
	var handlerErr error
	var mu sync.Mutex

	// Handler that takes 100ms (longer than grace period, but within ProcessTimeout)
	process := func(ctx context.Context, v int) ([]int, error) {
		close(handlerStarted)
		select {
		case <-time.After(100 * time.Millisecond):
			mu.Lock()
			handlerCompleted = true
			mu.Unlock()
			return []int{v}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	out := startProcessing(ctx, in, process, Config{
		ProcessTimeout:  200 * time.Millisecond, // ProcessTimeout is 200ms
		ShutdownTimeout: 50 * time.Millisecond,  // Grace period is 50ms
		ErrorHandler: func(val any, err error) {
			mu.Lock()
			handlerErr = err
			mu.Unlock()
		},
	})

	// Wait for handler to start
	<-handlerStarted

	// Cancel context - grace period starts
	cancel()

	// Grace period (50ms) should expire while handler is still running (needs 100ms total)
	// Handler should be cancelled when grace period expires
	time.Sleep(60 * time.Millisecond)
	close(in) // Close input to allow drain

	// Drain output
	for range out {
	}

	// Give time for error handler
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Handler should NOT have completed naturally (was cancelled during grace period expiry)
	if handlerCompleted {
		t.Error("handler should have been cancelled when grace period expired, but it completed")
	}

	// Handler should be cancelled (not deadline exceeded, because it was cancelled by shutdown)
	if handlerErr == nil {
		t.Error("expected handler to be cancelled")
	}
	if errors.Is(handlerErr, context.DeadlineExceeded) {
		t.Error("expected cancellation, got deadline exceeded")
	}
}

// TestProcessTimeout_HandlerCompletesWithinGracePeriod tests that handlers can complete during grace period
func TestProcessTimeout_HandlerCompletesWithinGracePeriod(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan int, 1)
	in <- 1

	handlerStarted := make(chan struct{})
	var handlerCompleted bool
	var mu sync.Mutex

	// Handler that completes quickly (30ms)
	process := func(ctx context.Context, v int) ([]int, error) {
		close(handlerStarted)
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		handlerCompleted = true
		mu.Unlock()
		return []int{v * 2}, nil
	}

	out := startProcessing(ctx, in, process, Config{
		ProcessTimeout:  200 * time.Millisecond, // Plenty of time
		ShutdownTimeout: 100 * time.Millisecond, // Grace period is 100ms
	})

	// Wait for handler to start
	<-handlerStarted

	// Cancel context - grace period starts
	cancel()

	// Handler should complete within grace period (30ms < 100ms)
	time.Sleep(50 * time.Millisecond)
	close(in)

	// Collect results
	var results []int
	for v := range out {
		results = append(results, v)
	}

	mu.Lock()
	defer mu.Unlock()

	// Handler should have completed naturally
	if !handlerCompleted {
		t.Error("handler should have completed within grace period")
	}

	if len(results) != 1 || results[0] != 2 {
		t.Errorf("expected [2], got %v", results)
	}
}

// TestProcessTimeout_ImmediateForcedShutdown tests that ProcessTimeout is ignored during immediate forced shutdown
func TestProcessTimeout_ImmediateForcedShutdown(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan int, 1)
	in <- 1

	handlerStarted := make(chan struct{})
	var handlerCompleted bool
	var handlerCancelled bool
	var mu sync.Mutex

	// Handler that would take 200ms (within ProcessTimeout)
	process := func(ctx context.Context, v int) ([]int, error) {
		close(handlerStarted)
		select {
		case <-time.After(200 * time.Millisecond):
			mu.Lock()
			handlerCompleted = true
			mu.Unlock()
			return []int{v}, nil
		case <-ctx.Done():
			mu.Lock()
			handlerCancelled = true
			mu.Unlock()
			return nil, ctx.Err()
		}
	}

	out := startProcessing(ctx, in, process, Config{
		ProcessTimeout:  500 * time.Millisecond, // Handler should have plenty of time
		ShutdownTimeout: 0,                      // No grace period - immediate shutdown
		ErrorHandler:    func(val any, err error) {},
	})

	// Wait for handler to start
	<-handlerStarted

	// Cancel context - immediate forced shutdown (no grace period)
	cancel()

	time.Sleep(20 * time.Millisecond)
	close(in)

	// Drain output
	for range out {
	}

	// Give time for handler to react
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Handler should be cancelled immediately, not complete
	if handlerCompleted {
		t.Error("handler should have been cancelled immediately, but it completed")
	}

	if !handlerCancelled {
		t.Error("handler should have been cancelled during immediate forced shutdown")
	}
}

// TestProcessTimeout_ZeroMeansNoTimeout tests that ProcessTimeout=0 means no timeout
func TestProcessTimeout_ZeroMeansNoTimeout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	in := make(chan int, 1)
	in <- 1
	close(in)

	var handlerCompleted bool
	var mu sync.Mutex

	// Handler that takes 100ms
	process := func(ctx context.Context, v int) ([]int, error) {
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		handlerCompleted = true
		mu.Unlock()
		return []int{v * 2}, nil
	}

	out := startProcessing(ctx, in, process, Config{
		ProcessTimeout: 0, // No timeout
	})

	// Collect results
	var results []int
	for v := range out {
		results = append(results, v)
	}

	mu.Lock()
	defer mu.Unlock()

	// Handler should complete naturally
	if !handlerCompleted {
		t.Error("handler should have completed without timeout")
	}

	if len(results) != 1 || results[0] != 2 {
		t.Errorf("expected [2], got %v", results)
	}
}

// TestProcessTimeout_IndependentFromShutdownTimeout verifies that ProcessTimeout and ShutdownTimeout are independent
func TestProcessTimeout_IndependentFromShutdownTimeout(t *testing.T) {
	t.Parallel()

	t.Run("ProcessTimeout expires before shutdown", func(t *testing.T) {
		ctx := context.Background()
		in := make(chan int, 1)
		in <- 1
		close(in)

		var handlerErr error
		var mu sync.Mutex

		// Handler that takes 200ms
		process := func(ctx context.Context, v int) ([]int, error) {
			select {
			case <-time.After(200 * time.Millisecond):
				return []int{v}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		out := startProcessing(ctx, in, process, Config{
			ProcessTimeout:  50 * time.Millisecond,  // ProcessTimeout is shorter
			ShutdownTimeout: 500 * time.Millisecond, // ShutdownTimeout is longer
			ErrorHandler: func(val any, err error) {
				mu.Lock()
				handlerErr = err
				mu.Unlock()
			},
		})

		// Drain output
		for range out {
		}

		time.Sleep(10 * time.Millisecond)

		// Handler should timeout due to ProcessTimeout, not ShutdownTimeout
		mu.Lock()
		defer mu.Unlock()
		if !errors.Is(handlerErr, context.DeadlineExceeded) {
			t.Errorf("expected ProcessTimeout to cause deadline exceeded, got %v", handlerErr)
		}
	})

	t.Run("ShutdownTimeout forces shutdown before ProcessTimeout", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		in := make(chan int, 1)
		in <- 1

		handlerStarted := make(chan struct{})
		var handlerErr error
		var mu sync.Mutex

		// Handler that takes 200ms
		process := func(ctx context.Context, v int) ([]int, error) {
			close(handlerStarted)
			select {
			case <-time.After(200 * time.Millisecond):
				return []int{v}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		out := startProcessing(ctx, in, process, Config{
			ProcessTimeout:  500 * time.Millisecond, // ProcessTimeout is longer
			ShutdownTimeout: 50 * time.Millisecond,  // ShutdownTimeout is shorter
			ErrorHandler: func(val any, err error) {
				mu.Lock()
				handlerErr = err
				mu.Unlock()
			},
		})

		// Wait for handler to start
		<-handlerStarted

		// Cancel context - grace period starts
		cancel()

		// Grace period should expire before ProcessTimeout
		time.Sleep(60 * time.Millisecond)
		close(in)

		// Drain output
		for range out {
		}

		time.Sleep(10 * time.Millisecond)

		// Handler should be cancelled due to ShutdownTimeout, not ProcessTimeout
		mu.Lock()
		defer mu.Unlock()
		if handlerErr == nil {
			t.Error("expected handler to be cancelled")
		}
		// Should be Cancelled, not DeadlineExceeded (cancelled by shutdown, not by ProcessTimeout)
		if errors.Is(handlerErr, context.DeadlineExceeded) {
			t.Error("expected cancellation due to shutdown, got deadline exceeded from ProcessTimeout")
		}
	})
}

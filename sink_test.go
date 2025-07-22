package gopipe_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/fxsml/gopipe"
)

func TestSink_Basic(t *testing.T) {
	ctx := t.Context()

	count := 10
	in := make(chan int, count)
	for i := range count {
		in <- i
	}
	close(in)

	var processedCount atomic.Int32
	var sum atomic.Int32

	handler := func(ctx context.Context, val int) error {
		processedCount.Add(1)
		sum.Add(int32(val))
		return nil
	}

	Sink(ctx, in, handler)

	// Allow some time for all values to be processed
	time.Sleep(50 * time.Millisecond)

	if processedCount.Load() != int32(count) {
		t.Errorf("Expected %d items processed, got %d", count, processedCount.Load())
	}

	// Sum of 0 to 9 = 45
	expectedSum := int32(45)
	if sum.Load() != expectedSum {
		t.Errorf("Expected sum to be %d, got %d", expectedSum, sum.Load())
	}
}

func TestSink_ErrorHandler(t *testing.T) {
	ctx := t.Context()

	in := make(chan int, 3)
	in <- 1
	in <- -1 // Will cause an error
	in <- 2
	close(in)

	var processedCount atomic.Int32
	var errorCount atomic.Int32

	handler := func(ctx context.Context, val int) error {
		if val < 0 {
			return errors.New("negative value")
		}
		processedCount.Add(1)
		return nil
	}

	errHandler := func(input any, err error) {
		errorCount.Add(1)
		// Verify error wrapping
		if !errors.Is(err, ErrSink) {
			t.Errorf("Expected error to wrap ErrSink, but it didn't")
		}
	}

	Sink(ctx, in, handler, WithErrorHandler(errHandler))

	// Allow some time for all values to be processed
	time.Sleep(50 * time.Millisecond)

	if processedCount.Load() != 2 {
		t.Errorf("Expected 2 items successfully processed, got %d", processedCount.Load())
	}

	if errorCount.Load() != 1 {
		t.Errorf("Expected 1 error to be caught, got %d", errorCount.Load())
	}
}

func TestSink_Concurrency(t *testing.T) {
	ctx := t.Context()

	concurrency := 5
	inputCount := 20

	in := make(chan int, inputCount)
	for i := 0; i < inputCount; i++ {
		in <- i
	}
	close(in)

	var processedCount atomic.Int32
	var mu sync.Mutex
	activeWorkers := make(map[int]bool)
	maxConcurrent := 0

	handler := func(ctx context.Context, val int) error {
		// Track concurrent executions
		workerID := int(processedCount.Add(1))

		mu.Lock()
		activeWorkers[workerID] = true
		if len(activeWorkers) > maxConcurrent {
			maxConcurrent = len(activeWorkers)
		}
		mu.Unlock()

		// Simulate work
		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		delete(activeWorkers, workerID)
		mu.Unlock()

		return nil
	}

	startTime := time.Now()
	Sink(ctx, in, handler, WithConcurrency(concurrency))

	// Wait for all processing to complete
	// With concurrency 5, 20 items taking 10ms each should take ~40ms
	time.Sleep(100 * time.Millisecond)
	duration := time.Since(startTime)

	if processedCount.Load() != int32(inputCount) {
		t.Errorf("Expected %d items processed, got %d", inputCount, processedCount.Load())
	}

	// Verify concurrency was actually used
	if maxConcurrent < concurrency {
		t.Errorf("Expected to use %d concurrent workers, but only detected %d", concurrency, maxConcurrent)
	}

	// With concurrency=5, processing 20 items that take 10ms each should take
	// approximately 40ms (20/5 * 10ms), not 200ms
	maxExpectedDuration := 200 * time.Millisecond // Add substantial buffer for test reliability
	if duration > maxExpectedDuration {
		t.Errorf("Expected concurrent processing to complete in about 40ms, took %v", duration)
	}
}

func TestSink_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create a channel that won't be closed to simulate continuous stream
	in := make(chan int, 100)
	for i := 0; i < 50; i++ {
		in <- i
	}

	var processedCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(1)

	handler := func(ctx context.Context, val int) error {
		// First few items process quickly
		if val < 3 {
			processedCount.Add(1)
			return nil
		}

		// Later items take longer
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
			processedCount.Add(1)
			return nil
		}
	}

	// Start processing in a goroutine
	go func() {
		defer wg.Done()
		Sink(ctx, in, handler)
	}()

	// Let a few items process
	time.Sleep(20 * time.Millisecond)

	// Cancel the context
	cancel()

	// Wait to ensure handler has responded to cancellation
	time.Sleep(50 * time.Millisecond)

	// Verify processing stopped after cancellation
	processedBefore := processedCount.Load()
	time.Sleep(100 * time.Millisecond)
	processedAfter := processedCount.Load()

	if processedAfter > processedBefore {
		t.Errorf("Processing continued after context cancellation: %d before, %d after",
			processedBefore, processedAfter)
	}
}

func TestSink_Timeout(t *testing.T) {
	// Create a parent context
	parentCtx := t.Context()

	// Create a channel
	in := make(chan int, 10)
	in <- 1
	in <- 2
	in <- 3
	close(in)

	var processedCount atomic.Int32
	var contextErrors atomic.Int32

	// Handler with artificial delay that exceeds the timeout
	handler := func(ctx context.Context, val int) error {
		// Slow operation
		select {
		case <-ctx.Done():
			contextErrors.Add(1)
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			processedCount.Add(1)
			return nil
		}
	}

	// Use a timeout for each process operation
	Sink(parentCtx, in, handler, WithProcessTimeout(20*time.Millisecond))

	// Wait for all processing to complete or timeout
	time.Sleep(200 * time.Millisecond)

	// Verify that items timed out
	if contextErrors.Load() == 0 {
		t.Error("Expected context timeouts to occur, but none detected")
	}

	// Some may complete, some may time out
	totalHandled := processedCount.Load() + contextErrors.Load()
	if totalHandled != 3 {
		t.Errorf("Expected 3 items to be handled (either processed or timed out), got %d", totalHandled)
	}
}

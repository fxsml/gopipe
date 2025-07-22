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

func TestRoute_Basic(t *testing.T) {
	ctx := t.Context()

	// Create input channel
	in := make(chan int, 10)
	for i := 0; i < 10; i++ {
		in <- i
	}
	close(in)

	// Simple route function that routes based on even/odd
	routeFunc := func(item int) int {
		return item % 2 // Route 0 for even numbers, 1 for odd
	}

	// Create routes (2 outputs)
	outputs := Route(ctx, in, routeFunc, 2)

	// Should have exactly 2 output channels
	if len(outputs) != 2 {
		t.Fatalf("Expected 2 output channels, got %d", len(outputs))
	}

	// Count items in each channel
	evenCount := 0
	oddCount := 0

	// Use WaitGroup to wait for both collection goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	// Collect from even channel
	go func() {
		defer wg.Done()
		for range outputs[0] {
			evenCount++
		}
	}()

	// Collect from odd channel
	go func() {
		defer wg.Done()
		for range outputs[1] {
			oddCount++
		}
	}()

	// Wait for collection to complete
	wg.Wait()

	// Verify counts
	expectedEvenCount := 5 // 0, 2, 4, 6, 8
	expectedOddCount := 5  // 1, 3, 5, 7, 9
	if evenCount != expectedEvenCount {
		t.Errorf("Expected %d even items, got %d", expectedEvenCount, evenCount)
	}
	if oddCount != expectedOddCount {
		t.Errorf("Expected %d odd items, got %d", expectedOddCount, oddCount)
	}
}

func TestRoute_OutOfRange(t *testing.T) {
	ctx := t.Context()

	// Create input channel
	in := make(chan int, 5)
	in <- 0  // Route to index 0
	in <- 1  // Route to index 1
	in <- -1 // Out of range (negative)
	in <- 5  // Out of range (too high)
	in <- 2  // Route to index 2
	close(in)

	// Route function that might return out-of-range indices
	routeFunc := func(item int) int {
		return item // Route to index equal to the value
	}

	var errorCount atomic.Int32

	// Create error handler
	errHandler := func(input any, err error) {
		errorCount.Add(1)
		// Verify it's the correct error type
		if !errors.Is(err, ErrRouteIndexOutOfRange) {
			t.Errorf("Expected ErrRouteIndexOutOfRange, got: %v", err)
		}
	}

	// Create 3 output channels (valid indices: 0, 1, 2)
	outputs := Route(ctx, in, routeFunc, 3, WithErrorHandler(errHandler))

	// Count items in each channel
	counts := make([]int, len(outputs))
	var wg sync.WaitGroup
	wg.Add(len(outputs))

	for i := range outputs {
		go func(idx int) {
			defer wg.Done()
			for range outputs[idx] {
				counts[idx]++
			}
		}(i)
	}

	wg.Wait()

	// Verify counts
	expectedCounts := []int{1, 1, 1} // One item each for indices 0, 1, and 2
	for i, count := range counts {
		if count != expectedCounts[i] {
			t.Errorf("Output %d: expected %d items, got %d", i, expectedCounts[i], count)
		}
	}

	// Verify error count (should be 2: one for -1 and one for 5)
	if errorCount.Load() != 2 {
		t.Errorf("Expected 2 routing errors, got %d", errorCount.Load())
	}
}

func TestRoute_Concurrency(t *testing.T) {
	ctx := t.Context()

	numWorkers := 5
	numOutputs := 3
	numItems := 100

	// Create input channel with many items
	in := make(chan int, numItems)
	for i := 0; i < numItems; i++ {
		in <- i
	}
	close(in)

	// Track concurrent executions
	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32
	var mu sync.Mutex
	processed := make(map[int]bool)

	// Simple route function that adds a delay to test concurrency
	routeFunc := func(item int) int {
		// Track concurrency
		current := currentConcurrent.Add(1)
		defer currentConcurrent.Add(-1)

		// Update max concurrency seen
		if current > maxConcurrent.Load() {
			maxConcurrent.Store(current)
		}

		// Track processed items
		mu.Lock()
		processed[item] = true
		mu.Unlock()

		// Small delay to ensure overlap in concurrent processing
		time.Sleep(1 * time.Millisecond)

		return item % numOutputs
	}

	startTime := time.Now()

	// Create routes with concurrency option
	outputs := Route(ctx, in, routeFunc, numOutputs, WithConcurrency(numWorkers))

	// Drain all outputs
	var wg sync.WaitGroup
	wg.Add(numOutputs)

	for i := range outputs {
		go func(idx int) {
			defer wg.Done()
			for range outputs[idx] {
				// Just drain the channel
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Verify concurrency was used
	if maxConcurrent.Load() < 2 {
		t.Errorf("Expected multiple concurrent executions, but max concurrency was %d", maxConcurrent.Load())
	}

	// Verify all items were processed
	mu.Lock()
	processedCount := len(processed)
	mu.Unlock()

	if processedCount != numItems {
		t.Errorf("Expected %d items to be processed, got %d", numItems, processedCount)
	}

	// With 5 workers processing 100 items at 1ms each, it should take ~20ms, not 100ms
	// Add buffer for test reliability
	maxExpectedDuration := 50 * time.Millisecond
	if duration > maxExpectedDuration {
		t.Errorf("Expected concurrent processing to complete in about 20ms, took %v", duration)
	}
}

func TestRoute_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create an unbuffered channel to better control the test
	in := make(chan int)

	// Route function that does nothing special
	routeFunc := func(item int) int {
		return item % 2
	}

	// Create routes
	outputs := Route(ctx, in, routeFunc, 2)

	// Start a goroutine that will eventually cancel the context
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Start a producer that will try to send after cancellation
	go func() {
		// Send a few items
		for i := 0; i < 5; i++ {
			in <- i
		}

		// Wait past the cancellation point
		time.Sleep(100 * time.Millisecond)

		// Try to send more items after cancellation
		// These should be dropped due to context cancellation
		for i := 5; i < 10; i++ {
			select {
			case in <- i:
				// Item sent
			case <-time.After(10 * time.Millisecond):
				// Channel might be blocked/closed after cancellation
				return
			}
		}
	}()

	// Collect from outputs until they close
	var wg sync.WaitGroup
	var totalReceived atomic.Int32

	wg.Add(len(outputs))
	for i := range outputs {
		go func(ch <-chan int) {
			defer wg.Done()
			for range ch {
				totalReceived.Add(1)
			}
		}(outputs[i])
	}

	// Wait for all output channels to close
	wg.Wait()

	// Verify that not all items were processed due to cancellation
	if totalReceived.Load() > 8 { // We expect fewer than all 10 items
		t.Errorf("Expected fewer than 10 items to be processed after cancellation, got %d", totalReceived.Load())
	}
}

func TestRoute_EmptyOutput(t *testing.T) {
	ctx := t.Context()

	// Create input channel
	in := make(chan int, 5)
	in <- 0
	in <- 1
	in <- 2
	in <- 3
	in <- 4
	close(in)

	// Route function that sends everything to channel 0
	routeFunc := func(item int) int {
		return 0
	}

	// Create 3 output channels
	outputs := Route(ctx, in, routeFunc, 3)

	// Count items in each channel
	counts := make([]int, len(outputs))
	var wg sync.WaitGroup
	wg.Add(len(outputs))

	for i := range outputs {
		go func(idx int) {
			defer wg.Done()
			for range outputs[idx] {
				counts[idx]++
			}
		}(i)
	}

	wg.Wait()

	// Verify counts - all items should be in first channel
	expectedCounts := []int{5, 0, 0}
	for i, count := range counts {
		if count != expectedCounts[i] {
			t.Errorf("Output %d: expected %d items, got %d", i, expectedCounts[i], count)
		}
	}
}

func TestRoute_AllChannelsClose(t *testing.T) {
	ctx := t.Context()

	// Create input channel
	in := make(chan int, 5)
	in <- 0
	in <- 1
	in <- 2
	in <- 3
	in <- 4
	close(in)

	// Route function
	routeFunc := func(item int) int {
		return item % 3
	}

	// Create output channels
	outputs := Route(ctx, in, routeFunc, 3)

	// Verify all channels close
	channelsClosed := make([]bool, len(outputs))
	var wg sync.WaitGroup
	wg.Add(len(outputs))

	for i := range outputs {
		go func(idx int) {
			defer wg.Done()
			// Drain the channel
			for range outputs[idx] {
			}
			// Channel closed
			channelsClosed[idx] = true
		}(i)
	}

	// Use a timeout to avoid deadlock in case of failure
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Continue
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timed out waiting for output channels to close")
	}

	// Verify all channels closed
	for i, closed := range channelsClosed {
		if !closed {
			t.Errorf("Output channel %d did not close", i)
		}
	}
}

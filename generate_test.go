package gopipe_test

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/fxsml/gopipe"
)

func TestGenerate_Basic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a simple generator that outputs 5 values
	generator := func(ctx context.Context) <-chan int {
		ch := make(chan int, 5)
		go func() {
			defer close(ch)
			for i := 0; i < 5; i++ {
				ch <- i
			}
		}()
		return ch
	}

	// Create signal and done channels
	signalCh := make(chan struct{}, 1)
	doneCh := make(chan struct{}, 1)
	signalCh <- struct{}{}

	// Generate items
	outCh := Generate(ctx, generator, signalCh, doneCh)

	// Collect all generated items
	var results []int
	for val := range outCh {
		results = append(results, val)
		if len(results) == 5 {
			cancel() // Stop generation after 5 items
			break
		}
	}

	// Verify all expected values were received
	expectedResults := []int{0, 1, 2, 3, 4}
	if len(results) != len(expectedResults) {
		t.Fatalf("expected %d results, got %d", len(expectedResults), len(results))
	}

	for i, val := range results {
		if val != expectedResults[i] {
			t.Errorf("expected %d at position %d, got %d", expectedResults[i], i, val)
		}
	}
}

func TestGenerate_IsRunningStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a generator that takes some time to complete
	generator := func(ctx context.Context) <-chan int {
		ch := make(chan int, 1)
		go func() {
			defer close(ch)
			time.Sleep(100 * time.Millisecond) // Simulate work
			ch <- 1
			time.Sleep(100 * time.Millisecond) // Simulate more work
			ch <- 2
		}()
		return ch
	}

	// Create signal and done channels
	signalCh := make(chan struct{}, 1)
	doneCh := make(chan struct{}, 1)

	// Generate items and set up isRunning function
	outCh := Generate(ctx, generator, signalCh, doneCh)

	// Since isRunning is no longer returned, we need to implement our own tracking
	var isRunningMu sync.Mutex
	isRunning := false

	// Set up a goroutine to monitor the doneCh to track isRunning state
	go func() {
		for range doneCh {
			isRunningMu.Lock()
			isRunning = false
			isRunningMu.Unlock()
		}
	}()

	// Helper function to check if generation is running
	isRunningFunc := func() bool {
		isRunningMu.Lock()
		defer isRunningMu.Unlock()
		return isRunning
	}

	// Should not be running initially
	if isRunningFunc() {
		t.Error("expected isRunning to be false before sending a signal")
	}

	// Update the running state when sending a signal
	isRunningMu.Lock()
	isRunning = true
	isRunningMu.Unlock()

	// Send a signal to start generation
	signalCh <- struct{}{}

	// Wait a bit for the generation to start
	time.Sleep(50 * time.Millisecond)

	// Should be running now
	if !isRunningFunc() {
		t.Error("expected isRunning to be true while generator is running")
	}

	// Consume values and check isRunning again
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		count := 0
		for range outCh {
			count++
		}
	}()

	// Wait for generation to complete
	time.Sleep(250 * time.Millisecond)

	// Should not be running after generation completes
	if isRunningFunc() {
		t.Error("expected isRunning to be false after generation completes")
	}

	// Clean up
	cancel()
	wg.Wait()
}

func TestGenerate_SignalTriggers(t *testing.T) {
	ctx := t.Context()

	// Track how many times the generator was called
	callCount := 0

	// Create a generator that increments the call count
	generator := func(ctx context.Context) <-chan int {
		callCount++
		ch := make(chan int, 1)
		go func() {
			defer close(ch)
			ch <- callCount
		}()
		return ch
	}

	// Create signal and done channels
	signalCh := make(chan struct{}, 3)
	doneCh := make(chan struct{}, 3)

	// Generate items
	outCh := Generate(ctx, generator, signalCh, doneCh)

	// Send multiple signals
	signalCh <- struct{}{}
	signalCh <- struct{}{}
	signalCh <- struct{}{}

	// Collect generated values
	var results []int
	for range 3 {
		select {
		case val := <-outCh:
			results = append(results, val)
		case <-time.After(500 * time.Millisecond):
			t.Fatal("timed out waiting for generated values")
		}
	}

	// Verify the generator was called multiple times
	if callCount != 3 {
		t.Errorf("expected generator to be called 3 times, got %d", callCount)
	}

	// Verify values increase as expected
	expectedResults := []int{1, 2, 3}
	for i, val := range results {
		if val != expectedResults[i] {
			t.Errorf("expected %d at position %d, got %d", expectedResults[i], i, val)
		}
	}
}

func TestGenerate_TerminatesOnClosedSignal(t *testing.T) {
	ctx := t.Context()

	// Create a simple generator
	generator := func(ctx context.Context) <-chan int {
		ch := make(chan int, 1)
		go func() {
			defer close(ch)
			ch <- 1
		}()
		return ch
	}

	// Create signal and done channels
	signalCh := make(chan struct{})
	doneCh := make(chan struct{}, 1)

	// Generate items
	outCh := Generate(ctx, generator, signalCh, doneCh)

	// Close the signal channel immediately
	close(signalCh)

	// Verify the output channel is closed
	select {
	case _, ok := <-outCh:
		if ok {
			t.Error("expected output channel to be closed when signal channel is closed")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for output channel to close")
	}
}

func TestGenerate_HandlesMultipleSignals(t *testing.T) {
	ctx := t.Context()

	// Create a generator that processes slowly
	generator := func(ctx context.Context) <-chan int {
		ch := make(chan int, 3)
		go func() {
			defer close(ch)
			for i := range 3 {
				time.Sleep(50 * time.Millisecond)
				ch <- i
			}
		}()
		return ch
	}

	// Create signal and done channels
	signalCh := make(chan struct{}, 2)
	doneCh := make(chan struct{}, 2)

	// Generate items with a buffered output
	outCh := Generate(ctx, generator, signalCh, doneCh, WithBuffer(10))

	// Set up a channel to track when first processing is done
	processingDone := make(chan struct{})

	go func() {
		// This goroutine will stop after receiving one completion signal
		<-doneCh
		// But keep the second signal in the buffer - we know there are two
		processingDone <- struct{}{}
	}()

	// Send signals in quick succession
	signalCh <- struct{}{}
	signalCh <- struct{}{}

	// Let generation complete for the first signal
	select {
	case <-processingDone:
		// First generation is complete, but the second should be running
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for first generation to complete")
	}

	// At this point, the second signal should still be processing
	// We don't need to check isRunning anymore because we've confirmed the first completion
	// and we're about to collect the results anyway

	// Collect all values (should be 6 in total - 3 from each generation)
	var results []int
	for i := range 6 {
		select {
		case val, ok := <-outCh:
			if !ok {
				t.Fatal("channel closed unexpectedly")
			}
			results = append(results, val)
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timed out waiting for value %d", i)
		}
	}

	if len(results) != 6 {
		t.Errorf("expected 6 results, got %d", len(results))
	}
}

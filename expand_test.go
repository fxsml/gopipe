package gopipe_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/fxsml/gopipe"
)

func TestExpand_Basic(t *testing.T) {
	ctx := t.Context()

	count := 20
	total := 0
	in := make(chan int, count)
	for i := range count {
		total += i
		in <- i
	}
	close(in)

	handler := func(ctx context.Context, val int) ([]string, error) {
		result := make([]string, val)
		for i := range val {
			result[i] = "item"
		}
		return result, nil
	}

	out := Expand(ctx, in, handler)

	countOut := 0
	for range out {
		countOut++
	}

	if countOut != total {
		t.Errorf("Expected %d items, got %d", total, countOut)
	}
}

func TestExpand_ErrorHandler(t *testing.T) {
	ctx := t.Context()

	in := make(chan int, 2)
	in <- 1
	in <- -1 // Will cause an error
	close(in)

	var errorsCaught atomic.Int32

	handler := func(ctx context.Context, val int) ([]string, error) {
		if val < 0 {
			return nil, errors.New("negative value")
		}
		return []string{"value"}, nil
	}

	errHandler := func(input any, err error) {
		errorsCaught.Add(1)
		// Verify error wrapping
		if !errors.Is(err, ErrExpand) {
			t.Errorf("Expected error to wrap ErrExpand, but it didn't")
		}
	}

	out := Expand(ctx, in, handler, WithErrorHandler(errHandler))

	count := 0
	for range out {
		count++
	}

	if count != 1 {
		t.Errorf("Expected 1 item, got %d", count)
	}

	if errorsCaught.Load() != 1 {
		t.Errorf("Expected 1 error to be caught, got %d", errorsCaught.Load())
	}
}

func TestExpand_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	in := make(chan int, 10)
	for i := range 10 {
		in <- i
	}
	// Don't close the channel to simulate an ongoing stream

	handler := func(ctx context.Context, val int) ([]string, error) {
		// Simulate some work
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
			return []string{"processed"}, nil
		}
	}

	out := Expand(ctx, in, handler)

	// Read a few items then cancel
	for range 3 {
		<-out
	}

	cancel()

	// Should not block forever and eventually stop
	timeout := time.After(100 * time.Millisecond)
	done := make(chan struct{})

	go func() {
		for range out {
			// Drain remaining items
		}
		close(done)
	}()

	select {
	case <-done:
		// Test passed
	case <-timeout:
		t.Error("Timed out waiting for channel to close after context cancellation")
	}
}

func TestExpand_Concurrency(t *testing.T) {
	ctx := t.Context()

	concurrency := 3
	inputs := 10

	in := make(chan int, inputs)
	for range inputs {
		in <- 1
	}
	close(in)

	var processedCount atomic.Int32

	handler := func(ctx context.Context, val int) ([]string, error) {
		processedCount.Add(1)
		// Simulate work with a sleep
		time.Sleep(10 * time.Millisecond)
		return []string{"processed"}, nil
	}

	startTime := time.Now()
	out := Expand(ctx, in, handler, WithConcurrency(concurrency))

	// Drain output
	outputCount := 0
	for range out {
		outputCount++
	}
	duration := time.Since(startTime)

	// With concurrency=3, processing 10 items that take 10ms each should take
	// approximately 40ms (10/3 rounded up * 10ms), not 100ms
	// Add some buffer for test reliability
	if duration > 60*time.Millisecond {
		t.Errorf("Expected concurrent processing in around 40ms, took %v", duration)
	}

	if int(processedCount.Load()) != inputs {
		t.Errorf("Expected %d items processed, got %d", inputs, processedCount.Load())
	}

	if outputCount != inputs {
		t.Errorf("Expected %d output items, got %d", inputs, outputCount)
	}
}

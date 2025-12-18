package pipe

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestStartProcessor_Basic(t *testing.T) {
	in := make(chan int)

	process := func(ctx context.Context, v int) ([]int, error) {
		return []int{v * 2}, nil
	}
	var mu sync.Mutex
	var cancelled []int
	cancel := func(v int, err error) {
		mu.Lock()
		cancelled = append(cancelled, v)
		mu.Unlock()
	}

	proc := NewProcessor(process, cancel)

	out := StartProcessor(context.Background(), in, proc, WithConcurrency[int, int](2))

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

func TestStartProcessor_CleanupBeforeChannelClose(t *testing.T) {
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
	proc := NewProcessor(
		func(ctx context.Context, val int) ([]int, error) {
			return []int{val * 4}, nil
		},
		func(val int, err error) {},
	)
	out := StartProcessor(ctx, in, proc, WithCleanup[int, int](cleanup, 0))
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

func TestStartProcessor_ErrorCallsCancel(t *testing.T) {
	in := make(chan int)

	process := func(ctx context.Context, v int) ([]int, error) {
		return nil, errors.New("fail")
	}

	var mu sync.Mutex
	var cancelled []int
	cancel := func(v int, err error) {
		mu.Lock()
		cancelled = append(cancelled, v)
		mu.Unlock()
	}

	proc := NewProcessor(process, cancel)

	out := StartProcessor(context.Background(), in, proc, WithConcurrency[int, int](1))

	go func() {
		in <- 7
		close(in)
	}()

	// drain out and wait a bit for cancel callback
	go func() {
		for range out {
		}
	}()
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(cancelled) != 1 || cancelled[0] != 7 {
		t.Fatalf("expected cancel called with 7, got %v", cancelled)
	}
	mu.Unlock()
}

func TestStartProcessor_WithBufferAndConcurrency(t *testing.T) {
	in := make(chan int)

	process := func(ctx context.Context, v int) ([]int, error) {
		return []int{v + 1}, nil
	}
	cancel := func(v int, err error) {}

	proc := NewProcessor(process, cancel)

	out := StartProcessor(context.Background(), in, proc, WithConcurrency[int, int](3), WithBuffer[int, int](2))

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

func TestStartProcessor_WithTimeoutCancelsProcess(t *testing.T) {
	in := make(chan int)
	process := func(ctx context.Context, v int) ([]int, error) {
		select {
		case <-time.After(200 * time.Millisecond):
			return []int{v}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	var cancelled []int
	cancel := func(v int, err error) {
		cancelled = append(cancelled, v)
	}

	proc := NewProcessor(process, cancel)

	out := StartProcessor(context.Background(), in, proc, WithTimeout[int, int](50*time.Millisecond), WithConcurrency[int, int](1))

	go func() {
		in <- 10
		close(in)
	}()

	// drain out
	for range out {
	}

	if len(cancelled) != 1 {
		t.Fatalf("expected process to be cancelled, got %v", cancelled)
	}
}

func TestStartProcessor_WithoutContextPropagation(t *testing.T) {
	in := make(chan int)
	process := func(ctx context.Context, v int) ([]int, error) {
		// ctx should not be the parent context - this is expected behavior
		// with WithoutContextPropagation option
		_ = ctx // Prevent staticcheck empty branch warning
		return []int{v}, nil
	}
	cancel := func(v int, err error) {}

	proc := NewProcessor(process, cancel)

	out := StartProcessor(context.Background(), in, proc, WithoutContextPropagation[int, int](), WithConcurrency[int, int](1))

	go func() {
		in <- 1
		close(in)
	}()

	for range out {
	}
}

func TestStartProcessor_NoGoroutineLeakOnChannelClose(t *testing.T) {
	// Count goroutines before starting
	initialGoroutines := runtime.NumGoroutine()

	in := make(chan int)

	process := func(ctx context.Context, v int) ([]int, error) {
		return []int{v * 2}, nil
	}
	cancel := func(v int, err error) {}

	proc := NewProcessor(process, cancel)

	ctx := context.Background()
	out := StartProcessor(ctx, in, proc, WithLogConfig[int, int](LogConfig{
		Disabled: false,
	}))

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

func TestStartProcessor_WithCancel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Track cancel calls
	var (
		cancelCalled bool
		canceledVal  int
		cancelErr    error
		mu           sync.Mutex
	)

	proc := NewProcessor(
		func(ctx context.Context, val int) ([]int, error) {
			if val < 0 {
				return nil, errors.New("negative value")
			}
			return []int{val * 2}, nil
		},
		func(val int, err error) {
			// Default cancel
		},
	)

	in := make(chan int, 2)
	in <- 5
	in <- -1 // This will fail
	close(in)

	out := StartProcessor(ctx, in, proc, WithCancel[int, int](func(val int, err error) {
		mu.Lock()
		defer mu.Unlock()
		cancelCalled = true
		canceledVal = val
		cancelErr = err
	}))

	// Collect results
	var results []int
	for result := range out {
		results = append(results, result)
	}

	// Give time for cancel to execute
	time.Sleep(50 * time.Millisecond)

	// Verify cancel was called for failed value
	mu.Lock()
	defer mu.Unlock()

	if !cancelCalled {
		t.Error("Expected cancel function to be called")
	}

	if canceledVal != -1 {
		t.Errorf("Expected canceled value -1, got %d", canceledVal)
	}

	if cancelErr == nil {
		t.Error("Expected cancel to receive an error")
	}

	// Verify successful result
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	if len(results) > 0 && results[0] != 10 {
		t.Errorf("Expected result 10, got %d", results[0])
	}
}

func TestStartProcessor_WithMultipleCancels(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Track cancel calls in order
	var (
		callOrder []string
		mu        sync.Mutex
	)

	proc := NewProcessor(
		func(ctx context.Context, val int) ([]int, error) {
			return nil, errors.New("fail")
		},
		func(val int, err error) {
			mu.Lock()
			callOrder = append(callOrder, "processor")
			mu.Unlock()
		},
	)

	in := make(chan int, 1)
	in <- 42
	close(in)

	// Add multiple cancel functions
	out := StartProcessor(ctx, in, proc,
		WithCancel[int, int](func(val int, err error) {
			mu.Lock()
			callOrder = append(callOrder, "cancel1")
			mu.Unlock()
		}),
		WithCancel[int, int](func(val int, err error) {
			mu.Lock()
			callOrder = append(callOrder, "cancel2")
			mu.Unlock()
		}),
		WithCancel[int, int](func(val int, err error) {
			mu.Lock()
			callOrder = append(callOrder, "cancel3")
			mu.Unlock()
		}),
	)

	// Drain output
	for range out {
	}

	// Give time for cancels to execute
	time.Sleep(50 * time.Millisecond)

	// Verify call order (LIFO for WithCancel, then processor's Cancel)
	mu.Lock()
	defer mu.Unlock()

	expectedOrder := []string{"cancel3", "cancel2", "cancel1", "processor"}
	if len(callOrder) != len(expectedOrder) {
		t.Errorf("Expected %d cancel calls, got %d: %v", len(expectedOrder), len(callOrder), callOrder)
	}

	for i, expected := range expectedOrder {
		if i >= len(callOrder) {
			t.Errorf("Missing call at position %d, expected %s", i, expected)
			continue
		}
		if callOrder[i] != expected {
			t.Errorf("Call order at position %d: expected %s, got %s", i, expected, callOrder[i])
		}
	}
}

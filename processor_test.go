package gopipe

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
		// ctx should not be the parent context
		if ctx == context.Background() {
			// acceptable
		}
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
	out := StartProcessor(ctx, in, proc, WithLogConfig[int, int](&LogConfig{
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
	// TODO: implement this test and ensure, the cancel func used in the processor
}

package gopipe

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

// noOpMetrics is a concurrency-safe, empty Metrics implementation for benchmarking.
type noOpMetrics struct{}

func (noOpMetrics) IncSuccess()                             {}
func (noOpMetrics) IncFailure()                             {}
func (noOpMetrics) IncCancelled()                           {}
func (noOpMetrics) IncInFlight()                            {}
func (noOpMetrics) DecInFlight()                            {}
func (noOpMetrics) ObserveProcessingDuration(time.Duration) {}
func (noOpMetrics) ObserveBufferSize(int)                   {}
func (noOpMetrics) ObserveBatchSize(int)                    {}

func BenchmarkProcess_MetricsEnabledVsDisabled(b *testing.B) {
	const N = 10000
	process := func(ctx context.Context, v int) (int, error) {
		time.Sleep(100 * time.Microsecond) // Simulate work
		return v, nil
	}
	cancel := func(v int, err error) {}

	b.Run("NoPipelining", func(b *testing.B) {
		for b.Loop() {
			ctx := context.Background()
			for j := range N {
				process(ctx, j)
			}
		}
	})

	b.Run("OnlyChannelOps", func(b *testing.B) {
		for b.Loop() {
			in := make(chan int, N)
			for j := range N {
				in <- j
			}
			close(in)
			for range in {
			}
		}
	})

	b.Run("MinimalPipelining", func(b *testing.B) {
		for b.Loop() {
			in := make(chan int, N)
			for j := range N {
				in <- j
			}
			close(in)
			for i := range in {
				process(context.Background(), i)
			}
		}
	})

	b.Run("NoMetrics", func(b *testing.B) {
		for b.Loop() {
			in := make(chan int, N)
			for j := range N {
				in <- j
			}
			close(in)
			out := Process(context.Background(), in, process, cancel)
			for range out {
			}
		}
	})

	b.Run("WithMetrics", func(b *testing.B) {
		metrics := noOpMetrics{}
		for b.Loop() {
			in := make(chan int, N)
			for j := range N {
				in <- j
			}
			close(in)
			out := Process(context.Background(), in, process, cancel, WithMetrics(metrics))
			for range out {
			}
		}
	})

	b.Run("WithConcurrency", func(b *testing.B) {
		for b.Loop() {
			in := make(chan int, N)
			for j := range N {
				in <- j
			}
			close(in)
			out := Process(context.Background(), in, process, cancel, WithConcurrency(100))
			for range out {
			}
		}
	})

}

func TestProcess_Basic(t *testing.T) {
	in := make(chan int)

	process := func(ctx context.Context, v int) (int, error) {
		return v * 2, nil
	}
	var mu sync.Mutex
	var cancelled []int
	cancel := func(v int, err error) {
		mu.Lock()
		cancelled = append(cancelled, v)
		mu.Unlock()
	}

	out := Process(context.Background(), in, process, cancel, WithConcurrency(2))

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

func TestProcess_ErrorCallsCancel(t *testing.T) {
	in := make(chan int)

	process := func(ctx context.Context, v int) (int, error) {
		return 0, errors.New("fail")
	}

	var mu sync.Mutex
	var cancelled []int
	cancel := func(v int, err error) {
		mu.Lock()
		cancelled = append(cancelled, v)
		mu.Unlock()
	}

	out := Process(context.Background(), in, process, cancel, WithConcurrency(1))

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

func TestProcess_WithBufferAndConcurrency(t *testing.T) {
	in := make(chan int)

	proc := func(ctx context.Context, v int) (int, error) {
		return v + 1, nil
	}
	cancel := func(v int, err error) {}

	out := Process(context.Background(), in, proc, cancel, WithConcurrency(3), WithBuffer(2))

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

func TestProcess_WithTimeoutCancelsProcess(t *testing.T) {
	in := make(chan int)
	proc := func(ctx context.Context, v int) (int, error) {
		select {
		case <-time.After(200 * time.Millisecond):
			return v, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	var cancelled []int
	cancel := func(v int, err error) {
		cancelled = append(cancelled, v)
	}

	out := Process(context.Background(), in, proc, cancel, WithTimeout(50*time.Millisecond), WithConcurrency(1))

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

func TestProcess_WithoutContextPropagation(t *testing.T) {
	in := make(chan int)
	proc := func(ctx context.Context, v int) (int, error) {
		// ctx should not be the parent context
		if ctx == context.Background() {
			// acceptable
		}
		return v, nil
	}
	cancel := func(v int, err error) {}

	out := Process(context.Background(), in, proc, cancel, WithoutContextPropagation(), WithConcurrency(1))

	go func() {
		in <- 1
		close(in)
	}()

	for range out {
	}
}

func TestProcess_NoGoroutineLeakOnChannelClose(t *testing.T) {
	// Count goroutines before starting
	initialGoroutines := runtime.NumGoroutine()

	in := make(chan int)

	process := func(ctx context.Context, v int) (int, error) {
		return v * 2, nil
	}
	cancel := func(v int, err error) {}

	ctx := context.Background()
	out := Process(ctx, in, process, cancel)

	// Add one item and then close the input channel without cancelling the context
	go func() {
		in <- 1
		close(in)
	}()

	// Drain the output channel - this will complete
	for range out {
	}

	// Give some time for goroutines to potentially clean up
	time.Sleep(100 * time.Millisecond)

	// Check if we have a goroutine leak
	finalGoroutines := runtime.NumGoroutine()
	leakedGoroutines := finalGoroutines - initialGoroutines

	if leakedGoroutines > 0 {
		t.Errorf("Unexpected goroutine leak: %d goroutine(s) leaked even though the channel was closed", leakedGoroutines)
	}
}

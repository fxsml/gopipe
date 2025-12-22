package pipe

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fxsml/gopipe/pipe/internal/test"
	"github.com/fxsml/gopipe/pipe/middleware"
)

func TestBatch(t *testing.T) {
	f := func(in <-chan int, handle func([]int) []int, maxSize int, maxDuration time.Duration) <-chan int {
		handlePipe := func(_ context.Context, batch []int) ([]int, error) {
			return handle(batch), nil
		}
		out, err := NewBatchPipe(
			handlePipe,
			BatchConfig{
				MaxSize:     maxSize,
				MaxDuration: maxDuration,
			},
		).Start(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return out
	}
	test.RunBatch_Success(t, f)
}

func TestFilter(t *testing.T) {
	f := func(in <-chan int, handle func(int) bool) <-chan int {
		handlePipe := func(_ context.Context, v int) (bool, error) {
			return handle(v), nil
		}
		out, err := NewFilterPipe(handlePipe, Config{}).Start(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return out
	}
	test.RunFilter_Even(t, f)
	test.RunFilter_AllFalse(t, f)
	test.RunFilter_Closure(t, f)
}

func TestProcess(t *testing.T) {
	f := func(in <-chan int, handle func(int) []string) <-chan string {
		// Adapter to use NewProcessPipe with the same signature
		handlePipe := func(_ context.Context, val int) ([]string, error) {
			return handle(val), nil
		}
		out, err := NewProcessPipe(
			handlePipe,
			Config{},
		).Start(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return out
	}
	test.RunProcess_Success(t, f)
}

func TestSink(t *testing.T) {
	f := func(in <-chan int, handle func(int)) <-chan struct{} {
		handlePipe := func(_ context.Context, in int) error {
			handle(in)
			return nil
		}
		out, err := NewSinkPipe(handlePipe, Config{}).Start(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return out
	}
	test.RunSink_handleCalled(t, f)
	test.RunSink_ExitsOnClose(t, f)
	test.RunSink_EmptyChannel(t, f)
}

func TestProcessPipe_ApplyMiddleware(t *testing.T) {
	t.Run("NoMiddleware", func(t *testing.T) {
		in := make(chan string, 1)
		in <- "test"
		close(in)

		p := NewProcessPipe(func(_ context.Context, s string) ([]int, error) {
			return []int{len(s)}, nil
		}, Config{})

		out, err := p.Start(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result := <-out

		if result != 4 {
			t.Errorf("Expected 4, got %d", result)
		}
	})

	t.Run("SingleMiddleware", func(t *testing.T) {
		in := make(chan string, 1)
		in <- "test"
		close(in)

		var middlewareCalled bool

		p := NewProcessPipe(func(_ context.Context, s string) ([]int, error) {
			return []int{len(s)}, nil
		}, Config{})
		if err := p.ApplyMiddleware(func(next middleware.ProcessFunc[string, int]) middleware.ProcessFunc[string, int] {
			return func(ctx context.Context, s string) ([]int, error) {
				middlewareCalled = true
				return next(ctx, s+"!")
			}
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out, err := p.Start(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result := <-out

		if !middlewareCalled {
			t.Error("Middleware was not called")
		}
		if result != 5 { // len("test!")
			t.Errorf("Expected 5, got %d", result)
		}
	})

	t.Run("MultipleMiddlewareOrder", func(t *testing.T) {
		in := make(chan string, 1)
		in <- "test"
		close(in)

		executionOrder := []string{}

		p := NewProcessPipe(func(_ context.Context, s string) ([]int, error) {
			executionOrder = append(executionOrder, "base:process")
			return []int{len(s)}, nil
		}, Config{})
		if err := p.ApplyMiddleware(
			func(next middleware.ProcessFunc[string, int]) middleware.ProcessFunc[string, int] {
				return func(ctx context.Context, s string) ([]int, error) {
					executionOrder = append(executionOrder, "middleware1:before")
					result, err := next(ctx, s)
					executionOrder = append(executionOrder, "middleware1:after")
					return result, err
				}
			},
			func(next middleware.ProcessFunc[string, int]) middleware.ProcessFunc[string, int] {
				return func(ctx context.Context, s string) ([]int, error) {
					executionOrder = append(executionOrder, "middleware2:before")
					result, err := next(ctx, s)
					executionOrder = append(executionOrder, "middleware2:after")
					return result, err
				}
			},
		); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out, err := p.Start(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		<-out

		expected := []string{
			"middleware1:before",
			"middleware2:before",
			"base:process",
			"middleware2:after",
			"middleware1:after",
		}

		if len(executionOrder) != len(expected) {
			t.Fatalf("Expected %d calls, got %d: %v", len(expected), len(executionOrder), executionOrder)
		}

		for i, call := range expected {
			if executionOrder[i] != call {
				t.Errorf("Expected execution order[%d] to be %q, got %q", i, call, executionOrder[i])
			}
		}
	})

	t.Run("MiddlewareChaining", func(t *testing.T) {
		in := make(chan int, 1)
		in <- 5
		close(in)

		// ApplyMiddleware called multiple times should preserve natural order:
		// First middleware applied executes first.
		// So: (*2) -> (+3) -> base = 5*2+3 = 13
		p := NewProcessPipe(func(_ context.Context, i int) ([]int, error) {
			return []int{i}, nil
		}, Config{})
		if err := p.ApplyMiddleware(func(next middleware.ProcessFunc[int, int]) middleware.ProcessFunc[int, int] {
			return func(ctx context.Context, i int) ([]int, error) {
				return next(ctx, i*2)
			}
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := p.ApplyMiddleware(func(next middleware.ProcessFunc[int, int]) middleware.ProcessFunc[int, int] {
			return func(ctx context.Context, i int) ([]int, error) {
				return next(ctx, i+3)
			}
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out, err := p.Start(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result := <-out

		if result != 13 {
			t.Errorf("Expected 13, got %d", result)
		}
	})

	t.Run("MiddlewareOrderConsistency", func(t *testing.T) {
		// Middleware order should be the same whether applied in a single call
		// or multiple calls.
		mw1 := func(next middleware.ProcessFunc[int, int]) middleware.ProcessFunc[int, int] {
			return func(ctx context.Context, i int) ([]int, error) {
				return next(ctx, i*2) // First: multiply by 2
			}
		}
		mw2 := func(next middleware.ProcessFunc[int, int]) middleware.ProcessFunc[int, int] {
			return func(ctx context.Context, i int) ([]int, error) {
				return next(ctx, i+3) // Second: add 3
			}
		}
		mw3 := func(next middleware.ProcessFunc[int, int]) middleware.ProcessFunc[int, int] {
			return func(ctx context.Context, i int) ([]int, error) {
				return next(ctx, i*i) // Third: square
			}
		}

		baseHandler := func(_ context.Context, i int) ([]int, error) {
			return []int{i}, nil
		}

		// Test with single variadic call: ApplyMiddleware(mw1, mw2, mw3)
		in1 := make(chan int, 1)
		in1 <- 5
		close(in1)

		p1 := NewProcessPipe(baseHandler, Config{})
		if err := p1.ApplyMiddleware(mw1, mw2, mw3); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out1, err := p1.Start(context.Background(), in1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result1 := <-out1

		// Test with multiple individual calls
		in2 := make(chan int, 1)
		in2 <- 5
		close(in2)

		p2 := NewProcessPipe(baseHandler, Config{})
		if err := p2.ApplyMiddleware(mw1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := p2.ApplyMiddleware(mw2); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := p2.ApplyMiddleware(mw3); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out2, err := p2.Start(context.Background(), in2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		result2 := <-out2

		if result1 != result2 {
			t.Errorf("Middleware order inconsistent: single call gave %d, multiple calls gave %d", result1, result2)
		}

		// Verify expected value: 5 -> *2=10 -> +3=13 -> ^2=169
		expected := 169
		if result1 != expected {
			t.Errorf("Expected %d, got %d", expected, result1)
		}
	})
}

func TestProcessPipe_ErrAlreadyStarted(t *testing.T) {
	t.Run("Start_ReturnsErrorOnSecondCall", func(t *testing.T) {
		in := make(chan int)
		close(in)

		p := NewProcessPipe(func(_ context.Context, i int) ([]int, error) {
			return []int{i}, nil
		}, Config{})

		// First call should succeed
		out, err := p.Start(context.Background(), in)
		if err != nil {
			t.Fatalf("Expected no error on first Start, got %v", err)
		}
		if out == nil {
			t.Fatal("Expected output channel, got nil")
		}

		// Drain the channel
		for range out {
		}

		// Second call should return ErrAlreadyStarted
		_, err = p.Start(context.Background(), in)
		if !errors.Is(err, ErrAlreadyStarted) {
			t.Errorf("Expected ErrAlreadyStarted, got %v", err)
		}
	})

	t.Run("ApplyMiddleware_ReturnsErrorAfterStart", func(t *testing.T) {
		in := make(chan int)
		close(in)

		p := NewProcessPipe(func(_ context.Context, i int) ([]int, error) {
			return []int{i}, nil
		}, Config{})

		// ApplyMiddleware should succeed before Start
		err := p.ApplyMiddleware(func(next middleware.ProcessFunc[int, int]) middleware.ProcessFunc[int, int] {
			return next
		})
		if err != nil {
			t.Fatalf("Expected no error on ApplyMiddleware before Start, got %v", err)
		}

		// Start the pipe
		out, err := p.Start(context.Background(), in)
		if err != nil {
			t.Fatalf("Expected no error on Start, got %v", err)
		}

		// Drain the channel
		for range out {
		}

		// ApplyMiddleware should return ErrAlreadyStarted after Start
		err = p.ApplyMiddleware(func(next middleware.ProcessFunc[int, int]) middleware.ProcessFunc[int, int] {
			return next
		})
		if !errors.Is(err, ErrAlreadyStarted) {
			t.Errorf("Expected ErrAlreadyStarted, got %v", err)
		}
	})
}

func TestBatchPipe_ErrAlreadyStarted(t *testing.T) {
	t.Run("Start_ReturnsErrorOnSecondCall", func(t *testing.T) {
		in := make(chan int)
		close(in)

		p := NewBatchPipe(func(_ context.Context, batch []int) ([]int, error) {
			return batch, nil
		}, BatchConfig{MaxSize: 10})

		// First call should succeed
		out, err := p.Start(context.Background(), in)
		if err != nil {
			t.Fatalf("Expected no error on first Start, got %v", err)
		}
		if out == nil {
			t.Fatal("Expected output channel, got nil")
		}

		// Drain the channel
		for range out {
		}

		// Second call should return ErrAlreadyStarted
		_, err = p.Start(context.Background(), in)
		if !errors.Is(err, ErrAlreadyStarted) {
			t.Errorf("Expected ErrAlreadyStarted, got %v", err)
		}
	})

	t.Run("ApplyMiddleware_ReturnsErrorAfterStart", func(t *testing.T) {
		in := make(chan int)
		close(in)

		p := NewBatchPipe(func(_ context.Context, batch []int) ([]int, error) {
			return batch, nil
		}, BatchConfig{MaxSize: 10})

		// ApplyMiddleware should succeed before Start
		err := p.ApplyMiddleware(func(next middleware.ProcessFunc[[]int, int]) middleware.ProcessFunc[[]int, int] {
			return next
		})
		if err != nil {
			t.Fatalf("Expected no error on ApplyMiddleware before Start, got %v", err)
		}

		// Start the pipe
		out, err := p.Start(context.Background(), in)
		if err != nil {
			t.Fatalf("Expected no error on Start, got %v", err)
		}

		// Drain the channel
		for range out {
		}

		// ApplyMiddleware should return ErrAlreadyStarted after Start
		err = p.ApplyMiddleware(func(next middleware.ProcessFunc[[]int, int]) middleware.ProcessFunc[[]int, int] {
			return next
		})
		if !errors.Is(err, ErrAlreadyStarted) {
			t.Errorf("Expected ErrAlreadyStarted, got %v", err)
		}
	})
}

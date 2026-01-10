package pipe

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fxsml/gopipe/pipe/middleware"
)

func TestNewGenerator_Basic(t *testing.T) {
	var counter int64
	gen := NewGenerator(func(ctx context.Context) ([]int, error) {
		val := atomic.AddInt64(&counter, 1)
		return []int{int(val)}, nil
	}, Config{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := gen.Generate(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read a few values
	for i := 1; i <= 3; i++ {
		select {
		case val := <-ch:
			if val != i {
				t.Errorf("expected %d, got %d", i, val)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timeout waiting for value")
		}
	}
}

func TestNewGenerator_ContextCancellation(t *testing.T) {
	gen := NewGenerator(func(ctx context.Context) ([]int, error) {
		return []int{1}, nil
	}, Config{})

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := gen.Generate(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read one value to ensure generator is running
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for first value")
	}

	// Cancel context
	cancel()

	// Channel should close eventually (drain any buffered values first)
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				// Channel closed - success
				return
			}
			// Drain buffered values
		case <-timeout:
			t.Fatal("timeout waiting for channel to close")
		}
	}
}

func TestGeneratePipe_ErrAlreadyStarted(t *testing.T) {
	t.Run("Generate_ReturnsErrorOnSecondCall", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		p := NewGenerator(func(_ context.Context) ([]int, error) {
			return []int{1}, nil
		}, Config{})

		// First call should succeed
		out, err := p.Generate(ctx)
		if err != nil {
			t.Fatalf("Expected no error on first Generate, got %v", err)
		}
		if out == nil {
			t.Fatal("Expected output channel, got nil")
		}

		// Cancel to stop the generator
		cancel()

		// Drain the channel
		for range out {
		}

		// Second call should return ErrAlreadyStarted
		_, err = p.Generate(context.Background())
		if !errors.Is(err, ErrAlreadyStarted) {
			t.Errorf("Expected ErrAlreadyStarted, got %v", err)
		}
	})

	t.Run("Use_ReturnsErrorAfterGenerate", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		p := NewGenerator(func(_ context.Context) ([]int, error) {
			return []int{1}, nil
		}, Config{})

		// Use should succeed before Generate
		err := p.Use(func(next middleware.ProcessFunc[struct{}, int]) middleware.ProcessFunc[struct{}, int] {
			return next
		})
		if err != nil {
			t.Fatalf("Expected no error on Use before Generate, got %v", err)
		}

		// Start the generator
		out, err := p.Generate(ctx)
		if err != nil {
			t.Fatalf("Expected no error on Generate, got %v", err)
		}

		// Cancel to stop the generator
		cancel()

		// Drain the channel
		for range out {
		}

		// Use should return ErrAlreadyStarted after Generate
		err = p.Use(func(next middleware.ProcessFunc[struct{}, int]) middleware.ProcessFunc[struct{}, int] {
			return next
		})
		if !errors.Is(err, ErrAlreadyStarted) {
			t.Errorf("Expected ErrAlreadyStarted, got %v", err)
		}
	})
}

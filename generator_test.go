package gopipe

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewGenerator_Basic(t *testing.T) {
	var counter int64
	gen := NewGenerator(func(ctx context.Context) ([]int, error) {
		val := atomic.AddInt64(&counter, 1)
		return []int{int(val)}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := gen.Generate(ctx)

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
	})

	ctx, cancel := context.WithCancel(context.Background())
	ch := gen.Generate(ctx)

	// Read one value to ensure generator is running
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for first value")
	}

	// Cancel context
	cancel()

	// Channel should close
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed after context cancellation")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for channel to close")
	}
}

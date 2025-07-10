package gopipeline_test

import (
	"context"
	"testing"
	"time"

	. "github.com/fxsml/gopipeline"
)

func TestDrain_Basic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := make(chan int, 2)
	in <- 1
	in <- 2
	close(in)

	Drain(ctx, in)
	// Should not panic or deadlock, and should return quickly
	time.Sleep(10 * time.Millisecond)
}

func TestDrain_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan int)
	Drain(ctx, in)
	cancel()
	time.Sleep(10 * time.Millisecond)
	// Should not panic or deadlock
}

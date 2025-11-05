package gopipe

import (
	"context"
	"sync"
	"time"
)

// FanIn merges multiple input channels into a single output channel.
// It safely handles concurrent Add() calls and provides graceful shutdown.
type FanIn[T any] struct {
	out     chan T
	mu      sync.Mutex
	wg      sync.WaitGroup
	done    chan struct{}
	config  FanInConfig
	closed  bool
	started bool
}

// FanInConfig configures FanIn behavior.
type FanInConfig struct {
	// Buffer size for the output channel
	Buffer int
	// ShutdownDuration is the max time to wait for input channels to drain.
	// If 0, waits indefinitely for clean shutdown.
	ShutdownDuration time.Duration
}

// NewFanIn creates a new FanIn instance.
// Add input channels with Add(), then call Start() exactly once.
func NewFanIn[T any](config FanInConfig) *FanIn[T] {
	return &FanIn[T]{
		out:    make(chan T, config.Buffer),
		done:   make(chan struct{}),
		config: config,
	}
}

// Add registers an input channel to be merged into the output.
// Safe to call concurrently. Ignored if called after shutdown begins.
func (f *FanIn[T]) Add(ch <-chan T) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return
	}

	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		for {
			select {
			case <-f.done:
				return
			case v, ok := <-ch:
				if !ok {
					return
				}
				select {
				case f.out <- v:
				case <-f.done:
					return
				}
			}
		}
	}()
}

// Start begins merging input channels and returns the output channel.
// The output channel closes when the context is cancelled and all input
// channels have been drained (up to ShutdownDuration).
// Must be called exactly once. Panics if called multiple times.
func (f *FanIn[T]) Start(ctx context.Context) <-chan T {
	f.mu.Lock()
	if f.started {
		f.mu.Unlock()
		panic("FanIn.Start() called multiple times")
	}
	f.started = true
	f.mu.Unlock()

	go func() {
		<-ctx.Done()

		f.mu.Lock()
		f.closed = true
		f.mu.Unlock()

		wgDone := make(chan struct{})
		go func() {
			f.wg.Wait()
			close(wgDone)
		}()

		if f.config.ShutdownDuration > 0 {
			select {
			case <-wgDone:
			case <-time.After(f.config.ShutdownDuration):
				close(f.done)
			}
		}
		<-wgDone
		close(f.out)
	}()

	return f.out
}

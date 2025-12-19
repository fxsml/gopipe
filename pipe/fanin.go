package pipe

import (
	"context"
	"errors"
	"sync"
	"time"
)

// FanIn merges multiple input channels into a single output channel.
// It safely handles concurrent Add() calls and provides graceful shutdown.
type FanIn[T any] struct {
	out       chan T
	mu        sync.Mutex
	wg        sync.WaitGroup
	done      chan struct{}
	config    FanInConfig
	closed    bool
	inputs    []<-chan T
	inputDone map[<-chan T]chan struct{}
}

// FanInConfig configures FanIn behavior.
type FanInConfig struct {
	// Buffer size for the output channel
	Buffer int
	// ShutdownTimeout is the max time to wait for input channels to drain.
	// If 0, waits indefinitely for clean shutdown.
	ShutdownTimeout time.Duration
}

// NewFanIn creates a new FanIn instance.
// Add input channels with Add(), then call Start() exactly once.
func NewFanIn[T any](config FanInConfig) *FanIn[T] {
	return &FanIn[T]{
		out:    make(chan T, config.Buffer),
		done:   make(chan struct{}),
		config: config,
		inputs: make([]<-chan T, 0),
	}
}

// Add registers an input channel to be merged into the output.
// Safe to call concurrently. Returns a done channel that closes when all messages
// from the input channel have been processed, and an error if FanIn is already closed.
func (f *FanIn[T]) Add(ch <-chan T) (<-chan struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		done := make(chan struct{})
		close(done)
		return done, errors.New("gopipe fanin: closed")
	}

	done := make(chan struct{})

	if f.isStarted() {
		f.startInput(ch, done)
	} else {
		f.inputs = append(f.inputs, ch)
		// For pre-Start added channels, we'll create the done channel in Start
		// Store the done channel for later use
		if f.inputDone == nil {
			f.inputDone = make(map[<-chan T]chan struct{})
		}
		f.inputDone[ch] = done
	}

	return done, nil
}

// Start begins merging input channels and returns the output channel.
// The output channel closes when the context is cancelled and all input
// channels have been drained (up to ShutdownDuration).
// Must be called exactly once. Panics if called multiple times.
func (f *FanIn[T]) Start(ctx context.Context) <-chan T {
	f.mu.Lock()
	if f.isStarted() {
		f.mu.Unlock()
		panic("gopipe fanin: start called multiple times")
	}
	for _, ch := range f.inputs {
		done := f.inputDone[ch]
		f.startInput(ch, done)
	}
	f.setStarted()
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

		if f.config.ShutdownTimeout > 0 {
			select {
			case <-wgDone:
			case <-time.After(f.config.ShutdownTimeout):
				close(f.done)
			}
		}
		<-wgDone
		close(f.out)
	}()

	return f.out
}

func (f *FanIn[T]) startInput(ch <-chan T, done chan struct{}) {
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		if done != nil {
			defer close(done)
		}
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

func (f *FanIn[T]) isStarted() bool {
	return f.inputs == nil
}

func (f *FanIn[T]) setStarted() {
	f.inputs = nil
}

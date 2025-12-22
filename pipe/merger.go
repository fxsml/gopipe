package pipe

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Merger merges multiple input channels into a single output channel.
// It safely handles concurrent Add() calls and provides graceful shutdown.
type Merger[T any] struct {
	out       chan T
	mu        sync.Mutex
	wg        sync.WaitGroup
	done      chan struct{}
	config    MergerConfig
	closed    bool
	inputs    []<-chan T
	inputDone map[<-chan T]chan struct{}
}

// MergerConfig configures Merger behavior.
type MergerConfig struct {
	// Buffer size for the output channel
	Buffer int
	// ShutdownTimeout is the max time to wait for input channels to drain.
	// If 0, waits indefinitely for clean shutdown.
	ShutdownTimeout time.Duration
}

// NewMerger creates a new Merger instance.
// Add input channels with Add(), then call Merge() exactly once.
func NewMerger[T any](config MergerConfig) *Merger[T] {
	return &Merger[T]{
		out:    make(chan T, config.Buffer),
		done:   make(chan struct{}),
		config: config,
		inputs: make([]<-chan T, 0),
	}
}

// Add registers an input channel to be merged into the output.
// Safe to call concurrently. Returns a done channel that closes when all messages
// from the input channel have been processed, and an error if Merger is already closed.
func (m *Merger[T]) Add(ch <-chan T) (<-chan struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		done := make(chan struct{})
		close(done)
		return done, errors.New("gopipe merger: closed")
	}

	done := make(chan struct{})

	if m.isStarted() {
		m.startInput(ch, done)
	} else {
		m.inputs = append(m.inputs, ch)
		// For pre-Merge added channels, we'll create the done channel in Merge
		// Store the done channel for later use
		if m.inputDone == nil {
			m.inputDone = make(map[<-chan T]chan struct{})
		}
		m.inputDone[ch] = done
	}

	return done, nil
}

// Merge begins merging input channels and returns the output channel.
// The output channel closes when the context is cancelled and all input
// channels have been drained (up to ShutdownDuration).
// Returns ErrAlreadyStarted if called multiple times.
func (m *Merger[T]) Merge(ctx context.Context) (<-chan T, error) {
	m.mu.Lock()
	if m.isStarted() {
		m.mu.Unlock()
		return nil, ErrAlreadyStarted
	}
	for _, ch := range m.inputs {
		done := m.inputDone[ch]
		m.startInput(ch, done)
	}
	m.setStarted()
	m.mu.Unlock()

	go func() {
		<-ctx.Done()

		m.mu.Lock()
		m.closed = true
		m.mu.Unlock()

		wgDone := make(chan struct{})
		go func() {
			m.wg.Wait()
			close(wgDone)
		}()

		if m.config.ShutdownTimeout > 0 {
			select {
			case <-wgDone:
			case <-time.After(m.config.ShutdownTimeout):
				close(m.done)
			}
		}
		<-wgDone
		close(m.out)
	}()

	return m.out, nil
}

func (m *Merger[T]) startInput(ch <-chan T, done chan struct{}) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		if done != nil {
			defer close(done)
		}
		for {
			select {
			case <-m.done:
				return
			case v, ok := <-ch:
				if !ok {
					return
				}
				select {
				case m.out <- v:
				case <-m.done:
					return
				}
			}
		}
	}()
}

func (m *Merger[T]) isStarted() bool {
	return m.inputs == nil
}

func (m *Merger[T]) setStarted() {
	m.inputs = nil
}

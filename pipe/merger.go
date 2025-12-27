package pipe

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Merger merges multiple input channels into a single output channel.
// It is the inverse of Distributor: many inputs → one output.
//
// Key features:
//   - Dynamic AddInput() during runtime (concurrent-safe)
//   - Per-input done channels for tracking completion
//   - Graceful shutdown with configurable timeout
//   - Thread-safe for concurrent AddInput() calls
//
// Example:
//
//	merger := pipe.NewMerger[int](pipe.MergerConfig{Buffer: 10})
//	merger.AddInput(ch1)
//	merger.AddInput(ch2)
//	out, _ := merger.Merge(ctx)
//	for v := range out { ... }
type Merger[T any] struct {
	mu        sync.Mutex
	wg        sync.WaitGroup
	out       chan T
	done      chan struct{}
	config    MergerConfig
	closed    bool
	inputs    []<-chan T
	inputDone map[<-chan T]chan struct{}
}

// MergerConfig configures Merger behavior.
type MergerConfig struct {
	// Buffer size for the output channel.
	Buffer int
	// ShutdownTimeout is the max time to wait for input channels to drain.
	// If 0, waits indefinitely for clean shutdown.
	ShutdownTimeout time.Duration
}

// NewMerger creates a new Merger instance.
// Add input channels with AddInput(), then call Merge() exactly once.
func NewMerger[T any](config MergerConfig) *Merger[T] {
	return &Merger[T]{
		out:    make(chan T, config.Buffer),
		done:   make(chan struct{}),
		config: config,
		inputs: make([]<-chan T, 0),
	}
}

// AddInput registers an input channel to be merged into the output.
// Returns a done channel that closes when all messages from this input
// have been forwarded to the output channel.
// Safe to call concurrently and after Merge() has been called.
// Returns an error if the Merger is already closed.
func (m *Merger[T]) AddInput(ch <-chan T) (<-chan struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		done := make(chan struct{})
		close(done)
		return done, errors.New("merger: closed")
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

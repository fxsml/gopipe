package pipe

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Merger merges multiple input channels into a single output channel.
// Inverse of Distributor. Supports dynamic AddInput() after Merge().
type Merger[T any] struct {
	mu        sync.Mutex
	wg        sync.WaitGroup
	out       chan T
	done      chan struct{}
	cfg       MergerConfig
	closed    bool
	inputs    []<-chan T
	inputDone map[<-chan T]chan struct{}
}

// MergerConfig configures Merger behavior.
type MergerConfig struct {
	// Buffer is the output channel buffer size.
	Buffer int
	// ShutdownTimeout controls shutdown behavior on context cancellation.
	// If <= 0, waits indefinitely for inputs to close naturally.
	// If > 0, waits up to this duration then forces shutdown.
	ShutdownTimeout time.Duration
	// ErrorHandler is called when a message cannot be forwarded.
	// Called with ErrShutdownDropped when a message is dropped due to shutdown.
	// Default logs via slog.Error.
	ErrorHandler func(in any, err error)
}

func (c MergerConfig) parse() MergerConfig {
	if c.ErrorHandler == nil {
		c.ErrorHandler = func(in any, err error) {
			slog.Error("[GOPIPE] Merger error", slog.Any("input", in), slog.Any("error", err))
		}
	}
	return c
}

// NewMerger creates a new Merger.
func NewMerger[T any](cfg MergerConfig) *Merger[T] {
	cfg = cfg.parse()
	return &Merger[T]{
		out:    make(chan T, cfg.Buffer),
		done:   make(chan struct{}),
		cfg:    cfg,
		inputs: make([]<-chan T, 0),
	}
}

// AddInput registers an input channel. Safe to call after Merge().
func (m *Merger[T]) AddInput(ch <-chan T) (<-chan struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, errors.New("merger: closed")
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

// Merge starts merging and returns the output channel.
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

		if m.cfg.ShutdownTimeout > 0 {
			// Wait for natural completion or timeout
			select {
			case <-wgDone:
				// Inputs finished naturally
			case <-time.After(m.cfg.ShutdownTimeout):
				// Force shutdown after timeout
				close(m.done)
			}
		}
		// If timeout <= 0, wait indefinitely for inputs to close naturally
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
					m.cfg.ErrorHandler(v, ErrShutdownDropped)
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

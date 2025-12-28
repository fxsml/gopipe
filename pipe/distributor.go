package pipe

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Distributor routes messages from a single input to multiple output channels.
// Inverse of Merger. First-match-wins routing. Supports dynamic AddOutput() after Distribute().
type Distributor[T any] struct {
	mu        sync.RWMutex
	wg        sync.WaitGroup
	done      chan struct{}
	config    DistributorConfig[T]
	closed    bool
	started   bool
	outputs   []outputEntry[T]
	inputDone chan struct{}
}

type outputEntry[T any] struct {
	ch      chan T
	matcher func(T) bool
}

// DistributorConfig configures Distributor behavior.
type DistributorConfig[T any] struct {
	Buffer          int
	ShutdownTimeout time.Duration
	NoMatchHandler  func(T)
}

// NewDistributor creates a new Distributor.
func NewDistributor[T any](config DistributorConfig[T]) *Distributor[T] {
	return &Distributor[T]{
		done:      make(chan struct{}),
		inputDone: make(chan struct{}),
		config:    config,
		outputs:   make([]outputEntry[T], 0),
	}
}

// AddOutput registers an output with a matcher. Safe to call after Distribute().
func (d *Distributor[T]) AddOutput(matcher func(T) bool) (<-chan T, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		ch := make(chan T)
		close(ch)
		return ch, errors.New("distributor: closed")
	}

	ch := make(chan T, d.config.Buffer)
	d.outputs = append(d.outputs, outputEntry[T]{
		ch:      ch,
		matcher: matcher,
	})

	return ch, nil
}

// Distribute starts routing input to outputs. Returns done channel.
func (d *Distributor[T]) Distribute(ctx context.Context, input <-chan T) (<-chan struct{}, error) {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return nil, ErrAlreadyStarted
	}
	d.started = true
	d.mu.Unlock()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer close(d.inputDone)

		for {
			select {
			case <-d.done:
				return
			case msg, ok := <-input:
				if !ok {
					return
				}
				d.route(ctx, msg)
			}
		}
	}()

	go func() {
		// Wait for either context cancellation or input completion
		select {
		case <-ctx.Done():
			// Early shutdown requested
			d.mu.Lock()
			d.closed = true
			d.mu.Unlock()

			if d.config.ShutdownTimeout > 0 {
				// Give time for in-flight messages, then force close
				select {
				case <-d.inputDone:
				case <-time.After(d.config.ShutdownTimeout):
					close(d.done)
				}
			}
		case <-d.inputDone:
			// Input completed naturally
			d.mu.Lock()
			d.closed = true
			d.mu.Unlock()
		}

		d.wg.Wait()

		d.mu.RLock()
		for _, out := range d.outputs {
			close(out.ch)
		}
		d.mu.RUnlock()
	}()

	return d.inputDone, nil
}

func (d *Distributor[T]) route(ctx context.Context, msg T) {
	d.mu.RLock()
	outputs := d.outputs
	d.mu.RUnlock()

	for _, out := range outputs {
		if out.matcher == nil || out.matcher(msg) {
			select {
			case out.ch <- msg:
			case <-d.done:
			case <-ctx.Done():
			}
			return
		}
	}

	// No match
	if d.config.NoMatchHandler != nil {
		d.config.NoMatchHandler(msg)
	}
}

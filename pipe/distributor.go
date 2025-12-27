package pipe

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Distributor routes messages from a single input to multiple output channels.
// It is the inverse of Merger: one input → many outputs.
//
// Key features:
//   - First-match-wins routing with optional matcher functions
//   - Dynamic AddOutput() during runtime (concurrent-safe)
//   - NoMatchHandler for unmatched messages
//   - Graceful shutdown with configurable timeout
//
// Example:
//
//	dist := pipe.NewDistributor[int](pipe.DistributorConfig[int]{Buffer: 10})
//	evens, _ := dist.AddOutput(func(v int) bool { return v%2 == 0 })
//	odds, _ := dist.AddOutput(func(v int) bool { return v%2 != 0 })
//	done, _ := dist.Distribute(ctx, input)
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
	// Buffer size for output channels
	Buffer int
	// ShutdownTimeout is the max time to wait for outputs to drain.
	// If 0, waits indefinitely for clean shutdown.
	ShutdownTimeout time.Duration
	// NoMatchHandler is called when no output matches a message.
	// If nil, unmatched messages are silently dropped.
	NoMatchHandler func(T)
}

// NewDistributor creates a new Distributor instance.
// Add output channels with AddOutput(), then call Distribute() exactly once.
func NewDistributor[T any](config DistributorConfig[T]) *Distributor[T] {
	return &Distributor[T]{
		done:      make(chan struct{}),
		inputDone: make(chan struct{}),
		config:    config,
		outputs:   make([]outputEntry[T], 0),
	}
}

// AddOutput registers an output channel with an optional matcher.
// If matcher is nil, the output matches all messages.
// Returns the output channel which closes when the distributor stops.
// Safe to call concurrently and after Distribute() has been called.
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

// Distribute begins routing messages from input to outputs.
// Messages are sent to the first output whose matcher returns true.
// Returns a channel that closes when input processing completes.
// Output channels close when the context is cancelled and input is drained.
// Returns ErrAlreadyStarted if called multiple times.
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

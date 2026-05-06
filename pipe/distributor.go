package pipe

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Distributor routes messages from a single input to multiple output channels.
// Inverse of Merger. First-match-wins routing. Supports dynamic AddOutput() after Distribute().
type Distributor[T any] struct {
	mu        sync.RWMutex
	wg        sync.WaitGroup
	done      chan struct{}
	cfg       DistributorConfig[T]
	closed    bool
	started   bool
	outputs   []outputEntry[T]
	inputDone chan struct{}
	allDone   chan struct{}
}

type outputEntry[T any] struct {
	ch      chan T
	matcher func(T) bool
}

// DistributorConfig configures Distributor behavior.
type DistributorConfig[T any] struct {
	// Buffer is the output channel buffer size.
	Buffer int
	// ShutdownTimeout controls shutdown behavior on context cancellation.
	// If <= 0, forces immediate shutdown (no grace period).
	// If > 0, waits up to this duration for natural completion, then forces shutdown.
	// On forced shutdown:
	//   - Worker stops forwarding (escapes blocked sends)
	//   - Worker drains remaining input, calling ErrorHandler for each
	//   - Worker exits when input closes
	ShutdownTimeout time.Duration
	// ErrorHandler is called when a message cannot be delivered.
	// Called with ErrNoMatchingOutput when no output matches.
	// Called with ErrShutdownDropped when dropped due to shutdown.
	// Default logs via slog.Error.
	ErrorHandler func(in any, err error)

	// Metrics receives observability events (optional, nil = no overhead).
	Metrics Metrics

	// Labels provides static identifiers attached to every metrics event.
	// Common keys: "stage", "pipeline", "handler", "service"
	Labels map[string]string

	// LabelFunc extracts dynamic labels from each routed value.
	// Called before RecordWait (WaitOpSend). Returned labels are merged with Labels,
	// with LabelFunc values taking precedence on conflicts.
	// Only called when Metrics is non-nil. Nil means no dynamic labels.
	LabelFunc func(val any) map[string]string
}

func (c DistributorConfig[T]) parse() DistributorConfig[T] {
	if c.ErrorHandler == nil {
		c.ErrorHandler = func(in any, err error) {
			slog.Error("[GOPIPE] Distributor error", slog.Any("input", in), slog.Any("error", err))
		}
	}
	return c
}

// NewDistributor creates a new Distributor.
func NewDistributor[T any](cfg DistributorConfig[T]) *Distributor[T] {
	cfg = cfg.parse()
	return &Distributor[T]{
		done:      make(chan struct{}),
		inputDone: make(chan struct{}),
		allDone:   make(chan struct{}),
		cfg:       cfg,
		outputs:   make([]outputEntry[T], 0),
	}
}

// AddOutput registers an output with a matcher. Safe to call after Distribute().
func (d *Distributor[T]) AddOutput(matcher func(T) bool) (<-chan T, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, errors.New("distributor: closed")
	}

	ch := make(chan T, d.cfg.Buffer)
	d.outputs = append(d.outputs, outputEntry[T]{
		ch:      ch,
		matcher: matcher,
	})

	return ch, nil
}

// Distribute starts routing input to outputs. Returns a done channel that
// closes after all output channels have been closed.
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
				// Forced shutdown - exit immediately without draining
				// (draining blocks if input channel is still open)
				return
			case msg, ok := <-input:
				if !ok {
					return
				}
				d.route(msg)
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

			if d.cfg.ShutdownTimeout > 0 {
				// Grace period - wait for natural completion or timeout
				select {
				case <-d.inputDone:
					// Input finished naturally within grace period
				case <-time.After(d.cfg.ShutdownTimeout):
					// Force shutdown after grace period
					close(d.done)
				}
			} else {
				// No grace period - force shutdown immediately
				close(d.done)
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

		close(d.allDone)
	}()

	return d.allDone, nil
}

// Stats returns an aggregate point-in-time snapshot across all outputs.
// Depth is the sum of all output buffer depths; Capacity is the per-output
// buffer size (all outputs share the same capacity set in DistributorConfig.Buffer).
// Use with a pull-based metrics backend (e.g. OTel observable gauge) to avoid
// recording on every send. For per-output detail use [Distributor.OutputStats].
func (d *Distributor[T]) Stats() Stats {
	d.mu.RLock()
	outputs := d.outputs
	d.mu.RUnlock()
	var totalDepth int
	for _, out := range outputs {
		totalDepth += len(out.ch)
	}
	return Stats{
		Depth:    totalDepth,
		Capacity: d.cfg.Buffer,
		Labels:   d.cfg.Labels,
	}
}

// OutputStats returns a point-in-time snapshot for each output buffer,
// in the order outputs were registered via AddOutput.
// Use with a pull-based metrics backend (e.g. OTel observable gauge) to avoid
// recording on every send. For an aggregate view use [Distributor.Stats].
func (d *Distributor[T]) OutputStats() []Stats {
	d.mu.RLock()
	outputs := d.outputs
	d.mu.RUnlock()
	stats := make([]Stats, len(outputs))
	for i, out := range outputs {
		stats[i] = Stats{
			Depth:    len(out.ch),
			Capacity: cap(out.ch),
		}
	}
	return stats
}

func (d *Distributor[T]) route(in T) {
	d.mu.RLock()
	outputs := d.outputs
	d.mu.RUnlock()

	for _, out := range outputs {
		if out.matcher == nil || out.matcher(in) {
			var sendStart time.Time
			if d.cfg.Metrics != nil {
				sendStart = time.Now()
			}

			labels := mergeLabels(d.cfg.Labels, d.cfg.LabelFunc, in)

			select {
			case out.ch <- in:
				if d.cfg.Metrics != nil {
					d.cfg.Metrics.RecordWait(context.Background(), WaitInfo{
						Labels:    labels,
						Operation: WaitOpSend,
						Duration:  time.Since(sendStart),
					})
				}
			case <-d.done:
				d.cfg.ErrorHandler(in, ErrShutdownDropped)
			}
			return
		}
	}

	// No match
	d.cfg.ErrorHandler(in, ErrNoMatchingOutput)
}

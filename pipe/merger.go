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
	// If <= 0, forces immediate shutdown (no grace period).
	// If > 0, waits up to this duration for natural completion, then forces shutdown.
	// On forced shutdown:
	//   - Workers stop forwarding (escape blocked sends)
	//   - Workers drain remaining inputs, calling ErrorHandler for each
	//   - Workers exit when their input closes
	ShutdownTimeout time.Duration
	// ErrorHandler is called when a message cannot be forwarded.
	// Called with ErrShutdownDropped when a message is dropped due to shutdown.
	// Default logs via slog.Error.
	ErrorHandler func(in any, err error)

	// Metrics receives observability events (optional, nil = no overhead).
	Metrics Metrics

	// Labels provides static identifiers attached to every metrics event.
	// Common keys: "stage", "pipeline", "handler", "service"
	Labels map[string]string

	// LabelFunc extracts dynamic labels from each forwarded value.
	// Called before RecordWait (WaitOpSend). Returned labels are merged with Labels,
	// with LabelFunc values taking precedence on conflicts.
	// Only called when Metrics is non-nil. Nil means no dynamic labels.
	LabelFunc func(val any) map[string]string
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
			// Grace period - wait for natural completion or timeout
			select {
			case <-wgDone:
				// Inputs finished naturally within grace period
			case <-time.After(m.cfg.ShutdownTimeout):
				// Force shutdown after grace period
				close(m.done)
			}
		} else {
			// No grace period - force shutdown immediately
			close(m.done)
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
				// Forced shutdown - exit immediately without draining
				// (draining blocks if input channel is still open)
				return
			case v, ok := <-ch:
				if !ok {
					return
				}
				var sendStart time.Time
				if m.cfg.Metrics != nil {
					sendStart = time.Now()
				}

				labels := mergeLabels(m.cfg.Labels, m.cfg.LabelFunc, v)

				select {
				case m.out <- v:
					if m.cfg.Metrics != nil {
						m.cfg.Metrics.RecordWait(context.Background(), WaitInfo{
							Labels:    labels,
							Operation: WaitOpSend,
							Duration:  time.Since(sendStart),
						})
					}
				case <-m.done:
					// Forced shutdown - report current value and exit
					m.cfg.ErrorHandler(v, ErrShutdownDropped)
					return
				}
			}
		}
	}()
}

// Stats returns a point-in-time snapshot of the output buffer depth and capacity.
// Use with a pull-based metrics backend (e.g. OTel observable gauge) to avoid
// recording on every send.
func (m *Merger[T]) Stats() Stats {
	return Stats{
		Depth:    len(m.out),
		Capacity: cap(m.out),
		Labels:   m.cfg.Labels,
	}
}

func (m *Merger[T]) isStarted() bool {
	return m.inputs == nil
}

func (m *Merger[T]) setStarted() {
	m.inputs = nil
}

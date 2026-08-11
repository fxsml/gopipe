package message

import (
	"context"
	"log/slog"
	"time"

	"github.com/fxsml/gopipe/pipe"
)

// MergerConfig configures the message merger.
type MergerConfig struct {
	// BufferSize is the output channel buffer size (default: 100).
	BufferSize int
	// ShutdownTimeout controls shutdown behavior on context cancellation.
	// If <= 0, forces immediate shutdown (no grace period).
	// If > 0, waits up to this duration for natural completion, then forces shutdown.
	ShutdownTimeout time.Duration
	// Logger for merger events (default: slog.Default()).
	Logger Logger
	// ErrorHandler is called on merge errors after auto-nack (optional).
	ErrorHandler ErrorHandler
	// Metrics receives observability events from the underlying pipe (optional).
	Metrics pipe.Metrics
	// Labels provides static identifiers attached to every metrics event.
	Labels map[string]string
	// LabelFunc extracts dynamic labels from each forwarded message.
	// Default: extracts "cloudevents.type" from *Message when Metrics is set.
	// Set to nil to disable dynamic label extraction.
	LabelFunc func(val any) map[string]string
}

func (c MergerConfig) parse() MergerConfig {
	if c.BufferSize <= 0 {
		c.BufferSize = 100
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.LabelFunc == nil {
		c.LabelFunc = messageLabelFunc
	}
	return c
}

// Merger combines multiple message input channels into a single output.
// Automatically nacks messages on errors and provides consistent logging.
type Merger struct {
	inner *pipe.Merger[*Message]
	cfg   MergerConfig
}

// NewMerger creates a new message merger.
// Messages are automatically nacked on merge failures (e.g., shutdown timeout).
func NewMerger(cfg MergerConfig) *Merger {
	cfg = cfg.parse()

	m := &Merger{cfg: cfg}
	m.inner = pipe.NewMerger[*Message](pipe.MergerConfig{
		Buffer:          cfg.BufferSize,
		ShutdownTimeout: cfg.ShutdownTimeout,
		Metrics:         cfg.Metrics,
		Labels:          cfg.Labels,
		LabelFunc:       cfg.LabelFunc,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			msg.Nack(err)
			m.cfg.Logger.Warn("Message merge failed",
				"component", "merger",
				"error", err,
				"attributes", msg.Attributes)
			if m.cfg.ErrorHandler != nil {
				m.cfg.ErrorHandler(msg, err)
			}
		},
	})
	return m
}

// AddInput registers an input channel.
// Returns a done channel that closes when the input is fully consumed.
// Can be called before or after Merge().
func (m *Merger) AddInput(ch <-chan *Message) (<-chan struct{}, error) {
	return m.inner.AddInput(ch)
}

// Merge starts merging all registered inputs into a single output channel.
// The output channel closes when all inputs are closed (or shutdown timeout).
func (m *Merger) Merge(ctx context.Context) (<-chan *Message, error) {
	return m.inner.Merge(ctx)
}

// Stats returns a point-in-time snapshot of the output buffer depth and capacity.
// Use with a pull-based metrics backend (e.g. OTel observable gauge).
func (m *Merger) Stats() pipe.Stats {
	return m.inner.Stats()
}

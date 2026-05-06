package message

import (
	"context"
	"log/slog"
	"time"

	"github.com/fxsml/gopipe/pipe"
)

// DistributorConfig configures the message distributor.
type DistributorConfig struct {
	// BufferSize is the per-output channel buffer size (default: 100).
	BufferSize int
	// ShutdownTimeout controls shutdown behavior on context cancellation.
	// If <= 0, forces immediate shutdown (no grace period).
	// If > 0, waits up to this duration for natural completion, then forces shutdown.
	ShutdownTimeout time.Duration
	// Logger for distributor events (default: slog.Default()).
	Logger Logger
	// ErrorHandler is called on distribution errors after auto-nack (optional).
	ErrorHandler ErrorHandler
	// Metrics receives observability events from the underlying pipe (optional).
	Metrics pipe.Metrics
	// Labels provides static identifiers attached to every metrics event.
	Labels map[string]string
	// LabelFunc extracts dynamic labels from each routed message.
	// Default: extracts "cloudevents.type" from *Message when Metrics is set.
	// Set to nil to disable dynamic label extraction.
	LabelFunc func(val any) map[string]string
}

func (c DistributorConfig) parse() DistributorConfig {
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

// Distributor routes messages to multiple outputs based on matchers.
// Automatically nacks messages on errors and provides consistent logging.
type Distributor struct {
	inner *pipe.Distributor[*Message]
	cfg   DistributorConfig
}

// NewDistributor creates a new message distributor.
// Messages are automatically nacked on distribution failures.
func NewDistributor(cfg DistributorConfig) *Distributor {
	cfg = cfg.parse()

	d := &Distributor{cfg: cfg}
	d.inner = pipe.NewDistributor(pipe.DistributorConfig[*Message]{
		Buffer:          cfg.BufferSize,
		ShutdownTimeout: cfg.ShutdownTimeout,
		Metrics:         cfg.Metrics,
		Labels:          cfg.Labels,
		LabelFunc:       cfg.LabelFunc,
		ErrorHandler: func(in any, err error) {
			msg := in.(*Message)
			msg.Nack(err)
			d.cfg.Logger.Warn("Message distribution failed",
				"component", "distributor",
				"error", err,
				"attributes", msg.Attributes)
			if d.cfg.ErrorHandler != nil {
				d.cfg.ErrorHandler(msg, err)
			}
		},
	})
	return d
}

// AddOutput registers an output with the given matcher.
// Returns a channel that receives messages matching the criteria.
// Can be called before or after Distribute().
func (d *Distributor) AddOutput(matcher Matcher) (<-chan *Message, error) {
	return d.inner.AddOutput(func(msg *Message) bool {
		return matcher == nil || matcher.Match(msg.Attributes)
	})
}

// Distribute starts distributing messages from the input to registered outputs.
// Returns a done channel that closes when distribution is complete.
func (d *Distributor) Distribute(ctx context.Context, in <-chan *Message) (<-chan struct{}, error) {
	return d.inner.Distribute(ctx, in)
}

// Stats returns an aggregate point-in-time snapshot across all outputs.
// Use with a pull-based metrics backend (e.g. OTel observable gauge).
// For per-output detail use [Distributor.OutputStats].
func (d *Distributor) Stats() pipe.Stats {
	return d.inner.Stats()
}

// OutputStats returns a point-in-time snapshot for each output buffer,
// in the order outputs were registered via AddOutput.
func (d *Distributor) OutputStats() []pipe.Stats {
	return d.inner.OutputStats()
}

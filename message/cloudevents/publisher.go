package cloudevents

import (
	"context"
	"log/slog"

	"github.com/cloudevents/sdk-go/v2/binding"
	"github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// PublisherConfig configures a Publisher.
type PublisherConfig struct {
	// Concurrency is the number of send goroutines (default: 1).
	Concurrency int
	// ErrorHandler is called on conversion/send errors (default: no-op).
	ErrorHandler func(raw *message.RawMessage, err error)
	// Logger is used for logging (default: slog.Default()).
	Logger message.Logger
}

// Publisher wraps a CloudEvents protocol.Sender as a gopipe output sink.
// It receives RawMessages from a channel, converts them to CloudEvents,
// and sends them via the protocol.Sender.
type Publisher struct {
	sender protocol.Sender
	cfg    PublisherConfig
	logger message.Logger
	sink   *pipe.ProcessPipe[*message.RawMessage, struct{}]
}

// NewPublisher creates a new Publisher wrapping the given sender.
func NewPublisher(sender protocol.Sender, cfg PublisherConfig) *Publisher {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	p := &Publisher{
		sender: sender,
		cfg:    cfg,
		logger: logger,
	}

	p.sink = pipe.NewSinkPipe(p.send, pipe.Config{
		Concurrency: cfg.Concurrency,
		ErrorHandler: func(in any, err error) {
			raw, _ := in.(*message.RawMessage)
			logger.Error("Message send failed",
				"component", "publisher",
				"error", err,
				"attributes", raw.Attributes)
			if cfg.ErrorHandler != nil {
				cfg.ErrorHandler(raw, err)
			}
		},
	})

	return p
}

// Use adds middleware to the publisher's internal pipe. Middleware wraps the
// send function, enabling retry logic, circuit breaking, or backoff on errors.
// Must be called before Publish. Returns ErrAlreadyStarted if called after.
func (p *Publisher) Use(mw ...middleware.Middleware[*message.RawMessage, struct{}]) error {
	return p.sink.Use(mw...)
}

// Publish starts consuming messages from the channel and sending them via CloudEvents.
// Spawns goroutines (based on Concurrency) that run until the input channel is closed
// or context is cancelled. Returns a done channel that closes when publishing completes.
// For each message:
//   - Successful send: calls msg.Ack()
//   - Failed send: calls msg.Nack(err)
//
// Returns ErrAlreadyStarted if Publish has already been called.
func (p *Publisher) Publish(ctx context.Context, ch <-chan *message.RawMessage) (<-chan struct{}, error) {
	return p.sink.Pipe(ctx, ch)
}

func (p *Publisher) send(ctx context.Context, raw *message.RawMessage) error {
	// Convert RawMessage to CloudEvents Event
	event, err := ToCloudEvent(raw)
	if err != nil {
		raw.Nack(err)
		return err
	}

	// Send via CloudEvents protocol
	ceMsg := binding.ToMessage(event)
	result := p.sender.Send(ctx, ceMsg)

	// Handle result
	if protocol.IsACK(result) {
		raw.Ack()
		return nil
	}

	raw.Nack(result)
	return result
}

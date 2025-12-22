package message

import (
	"context"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/pipe/middleware"
)

// Publisher provides channel-based message publishing with batching and routing.
type Publisher struct {
	sender Sender
	config PublisherConfig
}

// Publish consumes messages from the input channel, batches them by topic,
// and sends them via the Sender. Returns a channel that closes when publishing completes.
func (p *Publisher) Publish(ctx context.Context, msgs <-chan *Message) (<-chan struct{}, error) {
	// Extract topic from message properties, defaulting to empty string
	groupBy := func(msg *Message) string {
		topic, _ := msg.Attributes.Topic()
		return topic
	}
	group := channel.GroupBy(msgs, groupBy, channel.GroupByConfig{
		MaxBatchSize: p.config.MaxBatchSize,
		MaxDuration:  p.config.MaxDuration,
	})

	handle := func(ctx context.Context, group channel.Group[string, *Message]) ([]struct{}, error) {
		return nil, p.sender.Send(ctx, group.Key, group.Items)
	}

	// Build middleware chain
	var mw []middleware.Middleware[channel.Group[string, *Message], struct{}]

	mw = append(mw, middleware.Log[channel.Group[string, *Message], struct{}](middleware.LogConfig{
		MessageSuccess: "Published messages",
		MessageFailure: "Failed to publish messages",
		MessageCancel:  "Canceled publishing messages",
	}))

	if p.config.Retry != nil {
		mw = append(mw, middleware.Retry[channel.Group[string, *Message], struct{}](*p.config.Retry))
	}
	if p.config.Timeout > 0 {
		mw = append(mw, middleware.Context[channel.Group[string, *Message], struct{}](middleware.ContextConfig{
			Timeout:    p.config.Timeout,
			Background: true,
		}))
	}
	if p.config.Recover {
		mw = append(mw, middleware.Recover[channel.Group[string, *Message], struct{}]())
	}

	cfg := pipe.Config{
		Concurrency: p.config.Concurrency,
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}

	pp := pipe.NewProcessPipe(handle, cfg)
	if err := pp.ApplyMiddleware(mw...); err != nil {
		return nil, err
	}
	return pp.Pipe(ctx, group)
}

// PublisherConfig configures the Publisher behavior.
type PublisherConfig struct {
	// MaxBatchSize is the maximum number of messages per batch. Default: unlimited.
	MaxBatchSize int
	// MaxDuration is the maximum time to wait before flushing a batch.
	MaxDuration time.Duration
	// Concurrency is the number of concurrent send operations. Default: 1.
	Concurrency int
	// Timeout is the maximum duration for each send operation.
	Timeout time.Duration
	// Retry configures automatic retry on failures.
	Retry *middleware.RetryConfig
	// Recover enables panic recovery in send operations.
	Recover bool
}

// NewPublisher creates a Publisher that wraps a Sender with batching and gopipe processing.
// Messages are grouped by topic and sent in batches. The topic is determined by the
// AttrTopic attribute. If not set, empty string is used as the default topic.
//
// Note: Empty string is a valid topic representing the default topic. Senders should handle
// this appropriately, either by configuring a default topic name or logging errors when
// messages are sent to the default topic without proper configuration.
//
// Panics if sender is nil.
func NewPublisher(
	sender Sender,
	config PublisherConfig,
) *Publisher {
	if sender == nil {
		panic("message: sender cannot be nil")
	}

	return &Publisher{
		sender: sender,
		config: config,
	}
}

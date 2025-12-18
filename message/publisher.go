package message

import (
	"context"
	"time"

	"github.com/fxsml/gopipe/pipe"
	"github.com/fxsml/gopipe/channel"
)

// Publisher provides channel-based message publishing with batching and routing.
type Publisher struct {
	sender Sender
	config PublisherConfig
	proc   pipe.Processor[channel.Group[string, *Message], struct{}]
	opts   []pipe.Option[channel.Group[string, *Message], struct{}]
}

// Publish consumes messages from the input channel, batches them by topic,
// and sends them via the Sender. Returns a channel that closes when publishing completes.
func (p *Publisher) Publish(ctx context.Context, msgs <-chan *Message) <-chan struct{} {
	// Extract topic from message properties, defaulting to empty string
	groupBy := func(msg *Message) string {
		topic, _ := msg.Attributes.Topic()
		return topic
	}
	group := channel.GroupBy(msgs, groupBy, channel.GroupByConfig{
		MaxBatchSize: p.config.MaxBatchSize,
		MaxDuration:  p.config.MaxDuration,
	})
	return pipe.StartProcessor(ctx, group, p.proc, p.opts...)
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
	Retry *pipe.RetryConfig
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
	proc := pipe.NewProcessor(func(ctx context.Context, group channel.Group[string, *Message]) ([]struct{}, error) {
		return nil, sender.Send(ctx, group.Key, group.Items)
	}, nil)

	opts := []pipe.Option[channel.Group[string, *Message], struct{}]{
		pipe.WithLogConfig[channel.Group[string, *Message], struct{}](pipe.LogConfig{
			MessageSuccess: "Published messages",
			MessageFailure: "Failed to publish messages",
			MessageCancel:  "Canceled publishing messages",
		}),
	}
	if config.Recover {
		opts = append(opts, pipe.WithRecover[channel.Group[string, *Message], struct{}]())
	}
	if config.Concurrency > 0 {
		opts = append(opts, pipe.WithConcurrency[channel.Group[string, *Message], struct{}](config.Concurrency))
	}
	if config.Timeout > 0 {
		opts = append(opts, pipe.WithTimeout[channel.Group[string, *Message], struct{}](config.Timeout))
	}
	if config.Retry != nil {
		opts = append(opts, pipe.WithRetryConfig[channel.Group[string, *Message], struct{}](*config.Retry))
	}

	return &Publisher{
		sender: sender,
		config: config,
		proc:   proc,
		opts:   opts,
	}
}

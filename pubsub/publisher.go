package pubsub

import (
	"context"
	"fmt"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

// RouteFunc determines the routing key (topic) for a message based on its properties.
type RouteFunc func(message.Attributes) string

// ============================================================================
// Routing Key Helpers
// ============================================================================

// RouteBySubject returns a RouteFunc that routes by the message subject property.
func RouteBySubject() RouteFunc {
	return func(props message.Attributes) string {
		subject, _ := props.Subject()
		return subject
	}
}

// RouteByProperty returns a RouteFunc that routes by a specific message property.
func RouteByProperty(key string) RouteFunc {
	return func(props message.Attributes) string {
		value, ok := props[key].(string)
		if !ok {
			return ""
		}
		return value
	}
}

// RouteStatic returns a RouteFunc that always routes to the same topic.
func RouteStatic(topic string) RouteFunc {
	return func(props message.Attributes) string {
		return topic
	}
}

// RouteByFormat returns a RouteFunc that formats the topic using fmt.Sprintf with property values.
func RouteByFormat(format string, keys ...string) RouteFunc {
	return func(props message.Attributes) string {
		values := make([]any, len(keys))
		for i, key := range keys {
			values[i] = props[key]
		}
		return fmt.Sprintf(format, values...)
	}
}

// Publisher provides channel-based message publishing with batching and routing.
type Publisher struct {
	sender Sender
	config PublisherConfig
	proc   gopipe.Processor[channel.Group[string, *message.Message], struct{}]
	opts   []gopipe.Option[channel.Group[string, *message.Message], struct{}]
}

// Publish consumes messages from the input channel, batches them by topic,
// and sends them via the Sender. Returns a channel that closes when publishing completes.
func (p *Publisher) Publish(ctx context.Context, msgs <-chan *message.Message) <-chan struct{} {
	// Extract topic from message properties, defaulting to empty string
	groupBy := func(msg *message.Message) string {
		topic, _ := msg.Attributes.Topic()
		return topic
	}
	group := channel.GroupBy(msgs, groupBy, channel.GroupByConfig{
		MaxBatchSize: p.config.MaxBatchSize,
		MaxDuration:  p.config.MaxDuration,
	})
	return gopipe.StartProcessor(ctx, group, p.proc, p.opts...)
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
	Retry *gopipe.RetryConfig
	// Recover enables panic recovery in send operations.
	Recover bool
}

// NewPublisher creates a Publisher that wraps a Sender with batching and gopipe processing.
// Messages are grouped by topic and sent in batches. The topic is determined by the
// message.AttrTopic property. If not set, empty string is used as the default topic.
//
// Note: Empty string is a valid topic representing the default topic. Senders should handle
// this appropriately, either by configuring a default topic name or logging errors when
// messages are sent to the default topic without proper configuration.
func NewPublisher(
	sender Sender,
	config PublisherConfig,
) *Publisher {
	proc := gopipe.NewProcessor(func(ctx context.Context, group channel.Group[string, *message.Message]) ([]struct{}, error) {
		return nil, sender.Send(ctx, group.Key, group.Items)
	}, nil)

	opts := []gopipe.Option[channel.Group[string, *message.Message], struct{}]{
		gopipe.WithLogConfig[channel.Group[string, *message.Message], struct{}](gopipe.LogConfig{
			MessageSuccess: "Published messages",
			MessageFailure: "Failed to publish messages",
			MessageCancel:  "Canceled publishing messages",
		}),
	}
	if config.Recover {
		opts = append(opts, gopipe.WithRecover[channel.Group[string, *message.Message], struct{}]())
	}
	if config.Concurrency > 0 {
		opts = append(opts, gopipe.WithConcurrency[channel.Group[string, *message.Message], struct{}](config.Concurrency))
	}
	if config.Timeout > 0 {
		opts = append(opts, gopipe.WithTimeout[channel.Group[string, *message.Message], struct{}](config.Timeout))
	}
	if config.Retry != nil {
		opts = append(opts, gopipe.WithRetryConfig[channel.Group[string, *message.Message], struct{}](*config.Retry))
	}

	return &Publisher{
		sender: sender,
		config: config,
		proc:   proc,
		opts:   opts,
	}
}

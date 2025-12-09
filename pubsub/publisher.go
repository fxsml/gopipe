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
type RouteFunc func(message.Properties) string

// ============================================================================
// Routing Key Helpers
// ============================================================================

// RouteBySubject returns a RouteFunc that routes by the message subject property.
func RouteBySubject() RouteFunc {
	return func(props message.Properties) string {
		subject, _ := props.Subject()
		return subject
	}
}

// RouteByProperty returns a RouteFunc that routes by a specific message property.
func RouteByProperty(key string) RouteFunc {
	return func(props message.Properties) string {
		value, ok := props[key].(string)
		if !ok {
			return ""
		}
		return value
	}
}

// RouteStatic returns a RouteFunc that always routes to the same topic.
func RouteStatic(topic string) RouteFunc {
	return func(props message.Properties) string {
		return topic
	}
}

// RouteByFormat returns a RouteFunc that formats the topic using fmt.Sprintf with property values.
func RouteByFormat(format string, keys ...string) RouteFunc {
	return func(props message.Properties) string {
		values := make([]any, len(keys))
		for i, key := range keys {
			values[i] = props[key]
		}
		return fmt.Sprintf(format, values...)
	}
}

// Publisher provides channel-based message publishing with batching and routing.
type Publisher interface {
	// Publish consumes messages from the input channel, batches them by routing key,
	// and sends them via the Sender. Returns a channel that closes when publishing completes.
	Publish(ctx context.Context, msgs <-chan *message.Message) <-chan struct{}
}

type publisher struct {
	publish func(ctx context.Context, msgs <-chan *message.Message) <-chan struct{}
}

func (p *publisher) Publish(ctx context.Context, msgs <-chan *message.Message) <-chan struct{} {
	return p.publish(ctx, msgs)
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
// Messages are grouped by the routing key returned by route, then sent in batches.
func NewPublisher(
	sender Sender,
	route RouteFunc,
	config PublisherConfig,
) Publisher {
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

	return &publisher{
		publish: func(ctx context.Context, msgs <-chan *message.Message) <-chan struct{} {
			// Wrap RouteFunc to work with GroupBy's signature
			groupBy := func(msg *message.Message) string {
				return route(msg.Properties)
			}
			group := channel.GroupBy(msgs, groupBy, channel.GroupByConfig{
				MaxBatchSize: config.MaxBatchSize,
				MaxDuration:  config.MaxDuration,
			})
			return gopipe.StartProcessor(ctx, group, proc, opts...)
		},
	}
}

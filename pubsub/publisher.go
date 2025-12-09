package pubsub

import (
	"context"
	"fmt"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

// RouteFunc is a function that determines the routing key (topic/queue) for a message
// based on its properties. This is used by Publisher to route messages to different topics.
//
// Example:
//
//	// Route by subject
//	route := RouteBySubject()
//
//	// Route by custom property
//	route := RouteByProperty("tenant-id")
type RouteFunc func(message.Properties) string

// ============================================================================
// Routing Key Helpers
// ============================================================================

// RouteBySubject returns a RouteFunc that routes messages by their subject property.
// If the subject is not present, returns an empty string.
//
// Example:
//
//	publisher := pubsub.NewPublisher(
//	    sender,
//	    pubsub.RouteBySubject(),
//	    config,
//	)
func RouteBySubject() RouteFunc {
	return func(props message.Properties) string {
		subject, _ := props.Subject()
		return subject
	}
}

// RouteByProperty returns a RouteFunc that routes messages by a specific property.
// If the property is not present or not a string, returns an empty string.
//
// Example:
//
//	publisher := pubsub.NewPublisher(
//	    sender,
//	    pubsub.RouteByProperty("tenant-id"),
//	    config,
//	)
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
//
// Example:
//
//	publisher := pubsub.NewPublisher(
//	    sender,
//	    pubsub.RouteStatic("events"),
//	    config,
//	)
func RouteStatic(topic string) RouteFunc {
	return func(props message.Properties) string {
		return topic
	}
}

// RouteByFormat returns a RouteFunc that formats the routing key using property values.
// Use Go's fmt.Sprintf format string with property keys as arguments.
//
// Example:
//
//	// Route to "tenant-123-events"
//	publisher := pubsub.NewPublisher(
//	    sender,
//	    pubsub.RouteByFormat("tenant-%s-events", "tenant-id"),
//	    config,
//	)
func RouteByFormat(format string, keys ...string) RouteFunc {
	return func(props message.Properties) string {
		values := make([]any, len(keys))
		for i, key := range keys {
			values[i] = props[key]
		}
		return fmt.Sprintf(format, values...)
	}
}

type Sender interface {
	Send(ctx context.Context, topic string, msgs []*message.Message) error
}

type Receiver interface {
	Receive(ctx context.Context, topic string) ([]*message.Message, error)
}

type Broker interface {
	Sender
	Receiver
}

type Publisher interface {
	Publish(ctx context.Context, msgs <-chan *message.Message) <-chan struct{}
}

type Subscriber interface {
	Subscribe(ctx context.Context, topic string) <-chan *message.Message
}

type publisher struct {
	publish func(ctx context.Context, msgs <-chan *message.Message) <-chan struct{}
}

func (p *publisher) Publish(ctx context.Context, msgs <-chan *message.Message) <-chan struct{} {
	return p.publish(ctx, msgs)
}

type subsciber struct {
	subscribe func(ctx context.Context, topic string) <-chan *message.Message
}

func (s *subsciber) Subscribe(ctx context.Context, topic string) <-chan *message.Message {
	return s.subscribe(ctx, topic)
}

type PublisherConfig struct {
	MaxBatchSize int
	MaxDuration  time.Duration
	Concurrency  int
	Timeout      time.Duration
	Retry        *gopipe.RetryConfig
	Recover      bool
}

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

type SubscriberConfig struct {
	Concurrency int
	Timeout     time.Duration
	Retry       *gopipe.RetryConfig
	Recover     bool
}

func NewSubscriber(
	receiver Receiver,
	config SubscriberConfig,
) Subscriber {
	opts := []gopipe.Option[struct{}, *message.Message]{
		gopipe.WithLogConfig[struct{}, *message.Message](gopipe.LogConfig{
			MessageSuccess: "Received messages",
			MessageFailure: "Failed to receive messages",
			MessageCancel:  "Canceled receiving messages",
		}),
	}
	if config.Recover {
		opts = append(opts, gopipe.WithRecover[struct{}, *message.Message]())
	}
	if config.Concurrency > 0 {
		opts = append(opts, gopipe.WithConcurrency[struct{}, *message.Message](config.Concurrency))
	}
	if config.Timeout > 0 {
		opts = append(opts, gopipe.WithTimeout[struct{}, *message.Message](config.Timeout))
	}
	if config.Retry != nil {
		opts = append(opts, gopipe.WithRetryConfig[struct{}, *message.Message](*config.Retry))
	}

	return &subsciber{
		subscribe: func(ctx context.Context, topic string) <-chan *message.Message {
			return gopipe.NewGenerator(func(ctx context.Context) ([]*message.Message, error) {
				return receiver.Receive(ctx, topic)
			}, opts...).Generate(ctx)
		},
	}
}

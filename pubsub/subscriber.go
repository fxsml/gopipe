package pubsub

import (
	"context"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
)

// Subscriber provides channel-based message consumption from a Receiver.
// Topics must be added via AddTopic before calling Subscribe.
type Subscriber interface {
	// AddTopic registers a topic to subscribe to.
	// Must be called before Subscribe. Can be called multiple times
	// to subscribe to multiple topics.
	AddTopic(topic string)

	// Subscribe starts polling all registered topics and returns a channel
	// that emits messages from all topics. Each topic is polled in a separate
	// goroutine and messages are merged into a single output channel.
	// The channel is closed when the context is canceled.
	Subscribe(ctx context.Context) <-chan *message.Message
}

type subscriber struct {
	receiver Receiver
	topics   []string
	opts     []gopipe.Option[struct{}, *message.Message]
}

// AddTopic registers a topic to subscribe to.
func (s *subscriber) AddTopic(topic string) {
	s.topics = append(s.topics, topic)
}

// Subscribe starts polling all registered topics concurrently.
// Messages from all topics are merged into a single output channel.
func (s *subscriber) Subscribe(ctx context.Context) <-chan *message.Message {
	if len(s.topics) == 0 {
		// No topics registered, return closed channel
		out := make(chan *message.Message)
		close(out)
		return out
	}

	if len(s.topics) == 1 {
		// Single topic, no need for merging
		return s.subscribeToTopic(ctx, s.topics[0])
	}

	// Multiple topics: start a goroutine for each and merge outputs
	outputs := make([]<-chan *message.Message, len(s.topics))
	for i, topic := range s.topics {
		outputs[i] = s.subscribeToTopic(ctx, topic)
	}

	return channel.Merge(outputs...)
}

// subscribeToTopic creates a channel that polls a single topic.
func (s *subscriber) subscribeToTopic(ctx context.Context, topic string) <-chan *message.Message {
	return gopipe.NewGenerator(func(ctx context.Context) ([]*message.Message, error) {
		return s.receiver.Receive(ctx, topic)
	}, s.opts...).Generate(ctx)
}

// SubscriberConfig configures the Subscriber behavior.
type SubscriberConfig struct {
	// Concurrency is the number of concurrent receive operations per topic. Default: 1.
	Concurrency int
	// Timeout is the maximum duration for each receive operation.
	Timeout time.Duration
	// Retry configures automatic retry on failures.
	Retry *gopipe.RetryConfig
	// Recover enables panic recovery in receive operations.
	Recover bool
}

// NewSubscriber creates a Subscriber that wraps a Receiver with gopipe processing.
// Topics must be added via AddTopic before calling Subscribe.
//
// Example:
//
//	subscriber := pubsub.NewSubscriber(broker, pubsub.SubscriberConfig{})
//	subscriber.AddTopic("orders.created")
//	subscriber.AddTopic("orders.updated")
//	msgs := subscriber.Subscribe(ctx)
//	for msg := range msgs {
//	    // Process messages from both topics
//	}
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

	return &subscriber{
		receiver: receiver,
		opts:     opts,
	}
}

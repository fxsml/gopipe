package pubsub

import (
	"context"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

// Subscriber provides channel-based message consumption from a Receiver.
type Subscriber struct {
	receiver Receiver
	opts     []gopipe.Option[struct{}, *message.Message]
}

// Subscribe creates a channel that polls the specified topic and emits received messages.
// The channel is closed when the context is canceled.
//
// Subscribe can be called multiple times with the same or different topics. Each call
// creates an independent subscription with its own polling goroutine and message channel.
// The behavior of subscribing to the same topic multiple times depends on the underlying
// Receiver implementation - the Subscriber does not prevent or deduplicate multiple
// subscriptions to the same topic.
func (s *Subscriber) Subscribe(ctx context.Context, topic string) <-chan *message.Message {
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
//
// Example:
//
//	subscriber := pubsub.NewSubscriber(broker, pubsub.SubscriberConfig{})
//	msgs := subscriber.Subscribe(ctx, "orders.created")
//	for msg := range msgs {
//	    // Process messages from the topic
//	}
//
// To subscribe to multiple topics, call Subscribe multiple times:
//
//	orders := subscriber.Subscribe(ctx, "orders.created")
//	payments := subscriber.Subscribe(ctx, "payments.completed")
//	merged := channel.Merge(orders, payments)
//
// Panics if receiver is nil.
func NewSubscriber(
	receiver Receiver,
	config SubscriberConfig,
) *Subscriber {
	if receiver == nil {
		panic("pubsub: receiver cannot be nil")
	}
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

	return &Subscriber{
		receiver: receiver,
		opts:     opts,
	}
}

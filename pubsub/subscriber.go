package pubsub

import (
	"context"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

// Subscriber provides channel-based message consumption from a Receiver.
type Subscriber interface {
	// Subscribe returns a channel that emits messages from the specified topic.
	// The channel is closed when the context is canceled.
	Subscribe(ctx context.Context, topic string) <-chan *message.Message
}

type subscriber struct {
	subscribe func(ctx context.Context, topic string) <-chan *message.Message
}

func (s *subscriber) Subscribe(ctx context.Context, topic string) <-chan *message.Message {
	return s.subscribe(ctx, topic)
}

// SubscriberConfig configures the Subscriber behavior.
type SubscriberConfig struct {
	// Concurrency is the number of concurrent receive operations. Default: 1.
	Concurrency int
	// Timeout is the maximum duration for each receive operation.
	Timeout time.Duration
	// Retry configures automatic retry on failures.
	Retry *gopipe.RetryConfig
	// Recover enables panic recovery in receive operations.
	Recover bool
}

// NewSubscriber creates a Subscriber that wraps a Receiver with gopipe processing.
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
		subscribe: func(ctx context.Context, topic string) <-chan *message.Message {
			return gopipe.NewGenerator(func(ctx context.Context) ([]*message.Message, error) {
				return receiver.Receive(ctx, topic)
			}, opts...).Generate(ctx)
		},
	}
}

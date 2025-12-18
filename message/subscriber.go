package message

import (
	"context"
	"time"

	"github.com/fxsml/gopipe/pipe"
)

// Subscriber provides channel-based message consumption from a Receiver.
type Subscriber struct {
	receiver Receiver
	opts     []pipe.Option[struct{}, *Message]
}

// Subscribe creates a channel that polls the specified topic and emits received messages.
// The channel is closed when the context is canceled.
//
// Subscribe can be called multiple times with the same or different topics. Each call
// creates an independent subscription with its own polling goroutine and message channel.
// The behavior of subscribing to the same topic multiple times depends on the underlying
// Receiver implementation - the Subscriber does not prevent or deduplicate multiple
// subscriptions to the same topic.
func (s *Subscriber) Subscribe(ctx context.Context, topic string) <-chan *Message {
	return pipe.NewGenerator(func(ctx context.Context) ([]*Message, error) {
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
	Retry *pipe.RetryConfig
	// Recover enables panic recovery in receive operations.
	Recover bool
}

// NewSubscriber creates a Subscriber that wraps a Receiver with gopipe processing.
// The subscriber polls the receiver for messages and emits them on a channel.
// Each call to Subscribe creates an independent subscription with its own polling goroutine.
//
// Note: Multiple subscriptions to the same topic are allowed. The behavior depends on the
// underlying Receiver implementation - some may support competing consumers while others
// may deliver duplicate messages.
//
// Panics if receiver is nil.
func NewSubscriber(
	receiver Receiver,
	config SubscriberConfig,
) *Subscriber {
	if receiver == nil {
		panic("message: receiver cannot be nil")
	}
	opts := []pipe.Option[struct{}, *Message]{
		pipe.WithLogConfig[struct{}, *Message](pipe.LogConfig{
			MessageSuccess: "Received messages",
			MessageFailure: "Failed to receive messages",
			MessageCancel:  "Canceled receiving messages",
		}),
	}
	if config.Recover {
		opts = append(opts, pipe.WithRecover[struct{}, *Message]())
	}
	if config.Concurrency > 0 {
		opts = append(opts, pipe.WithConcurrency[struct{}, *Message](config.Concurrency))
	}
	if config.Timeout > 0 {
		opts = append(opts, pipe.WithTimeout[struct{}, *Message](config.Timeout))
	}
	if config.Retry != nil {
		opts = append(opts, pipe.WithRetryConfig[struct{}, *Message](*config.Retry))
	}

	return &Subscriber{
		receiver: receiver,
		opts:     opts,
	}
}

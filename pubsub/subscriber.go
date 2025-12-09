package pubsub

import (
	"context"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/message"
)

type Subscriber interface {
	Subscribe(ctx context.Context, topic string) <-chan *message.Message
}

type subsciber struct {
	subscribe func(ctx context.Context, topic string) <-chan *message.Message
}

func (s *subsciber) Subscribe(ctx context.Context, topic string) <-chan *message.Message {
	return s.subscribe(ctx, topic)
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

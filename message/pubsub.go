package message

import (
	"context"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
)

type Sender interface {
	Send(ctx context.Context, topic string, msgs []*Message) error
}

type Receiver interface {
	Receive(ctx context.Context, topic string) ([]*Message, error)
}

type Broker interface {
	Sender
	Receiver
}

type Publisher interface {
	Publish(ctx context.Context, msgs <-chan *Message) <-chan struct{}
}

type Subscriber interface {
	Subscribe(ctx context.Context, topic string) <-chan *Message
}

type publisher struct {
	publish func(ctx context.Context, msgs <-chan *Message) <-chan struct{}
}

func (p *publisher) Publish(ctx context.Context, msgs <-chan *Message) <-chan struct{} {
	return p.publish(ctx, msgs)
}

type subsciber struct {
	subscribe func(ctx context.Context, topic string) <-chan *Message
}

func (s *subsciber) Subscribe(ctx context.Context, topic string) <-chan *Message {
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
	route func(msg *Message) string,
	config PublisherConfig,
) Publisher {
	proc := gopipe.NewProcessor(func(ctx context.Context, group channel.Group[string, *Message]) ([]struct{}, error) {
		return nil, sender.Send(ctx, group.Key, group.Items)
	}, nil)

	opts := []gopipe.Option[channel.Group[string, *Message], struct{}]{
		gopipe.WithLogConfig[channel.Group[string, *Message], struct{}](gopipe.LogConfig{
			MessageSuccess: "Published messages",
			MessageFailure: "Failed to publish messages",
			MessageCancel:  "Canceled publishing messages",
		}),
	}
	if config.Recover {
		opts = append(opts, gopipe.WithRecover[channel.Group[string, *Message], struct{}]())
	}
	if config.Concurrency > 0 {
		opts = append(opts, gopipe.WithConcurrency[channel.Group[string, *Message], struct{}](config.Concurrency))
	}
	if config.Timeout > 0 {
		opts = append(opts, gopipe.WithTimeout[channel.Group[string, *Message], struct{}](config.Timeout))
	}
	if config.Retry != nil {
		opts = append(opts, gopipe.WithRetryConfig[channel.Group[string, *Message], struct{}](*config.Retry))
	}

	return &publisher{
		publish: func(ctx context.Context, msgs <-chan *Message) <-chan struct{} {
			group := channel.GroupBy(msgs, route, channel.GroupByConfig{
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
	opts := []gopipe.Option[struct{}, *Message]{
		gopipe.WithLogConfig[struct{}, *Message](gopipe.LogConfig{
			MessageSuccess: "Received messages",
			MessageFailure: "Failed to receive messages",
			MessageCancel:  "Canceled receiving messages",
		}),
	}
	if config.Recover {
		opts = append(opts, gopipe.WithRecover[struct{}, *Message]())
	}
	if config.Concurrency > 0 {
		opts = append(opts, gopipe.WithConcurrency[struct{}, *Message](config.Concurrency))
	}
	if config.Timeout > 0 {
		opts = append(opts, gopipe.WithTimeout[struct{}, *Message](config.Timeout))
	}
	if config.Retry != nil {
		opts = append(opts, gopipe.WithRetryConfig[struct{}, *Message](*config.Retry))
	}

	return &subsciber{
		subscribe: func(ctx context.Context, topic string) <-chan *Message {
			return gopipe.NewGenerator(func(ctx context.Context) ([]*Message, error) {
				return receiver.Receive(ctx, topic)
			}, opts...).Generate(ctx)
		},
	}
}

package message

import (
	"context"
	"time"

	"github.com/fxsml/gopipe"
	"github.com/fxsml/gopipe/channel"
)

type Sender interface {
	Send(ctx context.Context, topic string, msgs []*Message[[]byte]) error
}

type Receiver interface {
	Receive(ctx context.Context, topic string) ([]*Message[[]byte], error)
}

type Publisher interface {
	Publish(ctx context.Context, msgs <-chan *Message[[]byte]) <-chan struct{}
}

type Subscriber interface {
	Subscribe(ctx context.Context, topic string) <-chan *Message[[]byte]
}

type publisher struct {
	publish func(ctx context.Context, msgs <-chan *Message[[]byte]) <-chan struct{}
}

func (p *publisher) Publish(ctx context.Context, msgs <-chan *Message[[]byte]) <-chan struct{} {
	return p.publish(ctx, msgs)
}

type subsciber struct {
	subscribe func(ctx context.Context, topic string) <-chan *Message[[]byte]
}

func (s *subsciber) Subscribe(ctx context.Context, topic string) <-chan *Message[[]byte] {
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
	route func(msg *Message[[]byte]) string,
	config PublisherConfig,
) Publisher {
	proc := gopipe.NewProcessor(func(ctx context.Context, group channel.Group[string, *Message[[]byte]]) ([]struct{}, error) {
		return nil, sender.Send(ctx, group.Key, group.Items)
	}, nil)

	opts := []gopipe.Option[channel.Group[string, *Message[[]byte]], struct{}]{
		gopipe.WithLogConfig[channel.Group[string, *Message[[]byte]], struct{}](gopipe.LogConfig{
			MessageSuccess: "Published messages",
			MessageFailure: "Failed to publish messages",
			MessageCancel:  "Canceled publishing messages",
		}),
	}
	if config.Recover {
		opts = append(opts, gopipe.WithRecover[channel.Group[string, *Message[[]byte]], struct{}]())
	}
	if config.Concurrency > 0 {
		opts = append(opts, gopipe.WithConcurrency[channel.Group[string, *Message[[]byte]], struct{}](config.Concurrency))
	}
	if config.Timeout > 0 {
		opts = append(opts, gopipe.WithTimeout[channel.Group[string, *Message[[]byte]], struct{}](config.Timeout))
	}
	if config.Retry != nil {
		opts = append(opts, gopipe.WithRetryConfig[channel.Group[string, *Message[[]byte]], struct{}](*config.Retry))
	}

	return &publisher{
		publish: func(ctx context.Context, msgs <-chan *Message[[]byte]) <-chan struct{} {
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
	opts := []gopipe.Option[struct{}, *Message[[]byte]]{
		gopipe.WithLogConfig[struct{}, *Message[[]byte]](gopipe.LogConfig{
			MessageSuccess: "Received messages",
			MessageFailure: "Failed to receive messages",
			MessageCancel:  "Canceled receiving messages",
		}),
	}
	if config.Recover {
		opts = append(opts, gopipe.WithRecover[struct{}, *Message[[]byte]]())
	}
	if config.Concurrency > 0 {
		opts = append(opts, gopipe.WithConcurrency[struct{}, *Message[[]byte]](config.Concurrency))
	}
	if config.Timeout > 0 {
		opts = append(opts, gopipe.WithTimeout[struct{}, *Message[[]byte]](config.Timeout))
	}
	if config.Retry != nil {
		opts = append(opts, gopipe.WithRetryConfig[struct{}, *Message[[]byte]](*config.Retry))
	}

	return &subsciber{
		subscribe: func(ctx context.Context, topic string) <-chan *Message[[]byte] {
			return gopipe.NewGenerator(func(ctx context.Context) ([]*Message[[]byte], error) {
				return receiver.Receive(ctx, topic)
			}, opts...).Generate(ctx)
		},
	}
}

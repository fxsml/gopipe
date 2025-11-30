package gopipe

import (
	"context"
	"time"

	"github.com/fxsml/gopipe/channel"
)

type Sender[Message any, Topic comparable] interface {
	Send(ctx context.Context, topic Topic, msgs []Message) error
}

type Receiver[Message any, Topic comparable] interface {
	Receive(ctx context.Context, topic Topic) ([]Message, error)
}

func NewPublisher[Message any, Topic comparable](
	sender Sender[Message, Topic],
	route func(msg Message) Topic,
	maxSize int,
	maxDuration time.Duration,
	opts ...Option[channel.Group[Topic, Message], struct{}],
) Pipe[Message, struct{}] {
	preProc := func(in <-chan Message) <-chan channel.Group[Topic, Message] {
		return channel.GroupBy(in, route, channel.GroupByConfig{
			MaxBatchSize: maxSize,
			MaxDuration:  maxDuration,
		})
	}
	proc := NewProcessor(func(ctx context.Context, batch channel.Group[Topic, Message]) ([]struct{}, error) {
		return nil, sender.Send(ctx, batch.Key, batch.Items)
	}, nil)
	return newPipe(preProc, proc, opts...)
}

func NewSubscriber[Message any, Topic comparable](
	receiver Receiver[Message, Topic],
	topic Topic,
	opts ...Option[struct{}, Message],
) Generator[Message] {
	return NewGenerator(func(ctx context.Context) ([]Message, error) {
		return receiver.Receive(ctx, topic)
	}, opts...)
}

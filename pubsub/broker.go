package pubsub

import (
	"context"

	"github.com/fxsml/gopipe/message"
)

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

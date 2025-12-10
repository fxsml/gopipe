package pubsub

import (
	"context"

	"github.com/fxsml/gopipe/message"
)

// Sender publishes messages to topics.
type Sender interface {
	// Send publishes messages to the specified topic.
	// Returns an error if the operation fails or context is canceled.
	Send(ctx context.Context, topic string, msgs []*message.Message) error
}

// Receiver consumes messages from topics.
type Receiver interface {
	// Receive retrieves messages from the specified topic.
	// Behavior varies by implementation: may block, poll, or return buffered messages.
	Receive(ctx context.Context, topic string) ([]*message.Message, error)
}

// Package pubsub provides publish-subscribe messaging abstractions and implementations.
//
// The package defines core interfaces (Sender, Receiver, Broker) and provides
// multiple implementations: in-memory, channel-based, IO streams, and HTTP.
//
// Topic naming uses "/" as separator (e.g., "orders/created"). Pattern matching
// utilities support wildcards: "+" matches one segment, "#" matches zero or more.
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

// Broker combines Sender and Receiver for bidirectional messaging.
type Broker interface {
	Sender
	Receiver
}

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
	//
	// The topic parameter is the routing destination for the messages. The message.PropTopic
	// property should be ignored entirely by Sender implementations and should not be forwarded
	// to the underlying broker's message properties, as it has already been used for routing.
	Send(ctx context.Context, topic string, msgs []*message.Message) error
}

// Receiver consumes messages from topics.
type Receiver interface {
	// Receive retrieves messages from the specified topic.
	// Behavior varies by implementation: may block, poll, or return buffered messages.
	//
	// Implementations should set the message.PropTopic property on received messages to
	// indicate the topic name from which the message was received. This helps consumers
	// identify the source topic, especially when using wildcard subscriptions or multiplexing.
	Receive(ctx context.Context, topic string) ([]*message.Message, error)
}

// Broker combines Sender and Receiver for bidirectional messaging.
type Broker interface {
	Sender
	Receiver
}

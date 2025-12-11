package multiplex

import (
	"context"
	"strings"

	"github.com/fxsml/gopipe/message"
)

// SenderSelector determines which Sender to use for a given topic.
// Returns the selected Sender or nil if no match (triggers fallback).
type SenderSelector func(topic string) message.Sender

// ReceiverSelector determines which Receiver to use for a given topic.
// Returns the selected Receiver or nil if no match (triggers fallback).
type ReceiverSelector func(topic string) message.Receiver

// Sender routes Send calls to different Senders based on topic.
type Sender struct {
	selector SenderSelector
	fallback message.Sender
}

// Compile-time interface assertion
var _ message.Sender = (*Sender)(nil)

// NewSender creates a multiplexing Sender.
// Panics if fallback is nil.
func NewSender(selector SenderSelector, fallback message.Sender) *Sender {
	if fallback == nil {
		panic("multiplex sender requires non-nil fallback")
	}
	return &Sender{
		selector: selector,
		fallback: fallback,
	}
}

// Send routes the message to the appropriate Sender based on topic.
func (m *Sender) Send(ctx context.Context, topic string, msgs []*message.Message) error {
	sender := m.selector(topic)
	if sender == nil {
		sender = m.fallback
	}
	return sender.Send(ctx, topic, msgs)
}

// Receiver routes Receive calls to different Receivers based on topic.
type Receiver struct {
	selector ReceiverSelector
	fallback message.Receiver
}

// Compile-time interface assertion
var _ message.Receiver = (*Receiver)(nil)

// NewReceiver creates a multiplexing Receiver.
// Panics if fallback is nil.
func NewReceiver(selector ReceiverSelector, fallback message.Receiver) *Receiver {
	if fallback == nil {
		panic("multiplex receiver requires non-nil fallback")
	}
	return &Receiver{
		selector: selector,
		fallback: fallback,
	}
}

// Receive routes the subscription to the appropriate Receiver based on topic.
func (m *Receiver) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
	receiver := m.selector(topic)
	if receiver == nil {
		receiver = m.fallback
	}
	return receiver.Receive(ctx, topic)
}

// ============================================================================
// Helper Functions for Common Routing Patterns
// ============================================================================

// TopicSenderRoute defines a topic and its associated Sender.
type TopicSenderRoute struct {
	Topic  string
	Sender message.Sender
}

// NewTopicSenderSelector creates a selector that matches topics exactly.
// First match wins.
func NewTopicSenderSelector(routes []TopicSenderRoute) SenderSelector {
	return func(topic string) message.Sender {
		for _, route := range routes {
			if route.Topic == topic {
				return route.Sender
			}
		}
		return nil
	}
}

// TopicReceiverRoute defines a topic and its associated Receiver.
type TopicReceiverRoute struct {
	Topic    string
	Receiver message.Receiver
}

// NewTopicReceiverSelector creates a selector that matches topics exactly.
// First match wins.
func NewTopicReceiverSelector(routes []TopicReceiverRoute) ReceiverSelector {
	return func(topic string) message.Receiver {
		for _, route := range routes {
			if route.Topic == topic {
				return route.Receiver
			}
		}
		return nil
	}
}

// PrefixSenderSelector creates a selector that routes topics starting with prefix.
// Topics use "/" as separator (e.g., prefix "orders" matches "orders/created").
func PrefixSenderSelector(prefix string, sender message.Sender) SenderSelector {
	prefixWithSep := prefix + "/"
	return func(topic string) message.Sender {
		if topic == prefix || strings.HasPrefix(topic, prefixWithSep) {
			return sender
		}
		return nil
	}
}

// PrefixReceiverSelector creates a selector based on topic prefix.
// Topics use "/" as separator (e.g., prefix "orders" matches "orders/created").
func PrefixReceiverSelector(prefix string, receiver message.Receiver) ReceiverSelector {
	prefixWithSep := prefix + "/"
	return func(topic string) message.Receiver {
		if topic == prefix || strings.HasPrefix(topic, prefixWithSep) {
			return receiver
		}
		return nil
	}
}

// ChainSenderSelectors combines multiple selectors; first match wins.
func ChainSenderSelectors(selectors ...SenderSelector) SenderSelector {
	return func(topic string) message.Sender {
		for _, sel := range selectors {
			if sender := sel(topic); sender != nil {
				return sender
			}
		}
		return nil
	}
}

// ChainReceiverSelectors combines multiple selectors (first match wins).
func ChainReceiverSelectors(selectors ...ReceiverSelector) ReceiverSelector {
	return func(topic string) message.Receiver {
		for _, sel := range selectors {
			if receiver := sel(topic); receiver != nil {
				return receiver
			}
		}
		return nil
	}
}

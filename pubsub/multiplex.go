package pubsub

import (
	"context"
	"strings"

	"github.com/fxsml/gopipe/message"
)

// SenderSelector determines which Sender to use for a given topic.
// Returns the selected Sender or nil if no match (triggers fallback).
type SenderSelector func(topic string) Sender

// ReceiverSelector determines which Receiver to use for a given topic.
// Returns the selected Receiver or nil if no match (triggers fallback).
type ReceiverSelector func(topic string) Receiver

// MultiplexSender routes Send calls to different Senders based on topic.
type MultiplexSender struct {
	selector SenderSelector
	fallback Sender
}

// NewMultiplexSender creates a multiplexing Sender.
// Panics if fallback is nil.
func NewMultiplexSender(selector SenderSelector, fallback Sender) *MultiplexSender {
	if fallback == nil {
		panic("multiplex sender requires non-nil fallback")
	}
	return &MultiplexSender{
		selector: selector,
		fallback: fallback,
	}
}

// Send routes the message to the appropriate Sender based on topic.
func (m *MultiplexSender) Send(ctx context.Context, topic string, msgs []*message.Message) error {
	sender := m.selector(topic)
	if sender == nil {
		sender = m.fallback
	}
	return sender.Send(ctx, topic, msgs)
}

// MultiplexReceiver routes Receive calls to different Receivers based on topic.
type MultiplexReceiver struct {
	selector ReceiverSelector
	fallback Receiver
}

// NewMultiplexReceiver creates a multiplexing Receiver.
// Panics if fallback is nil.
func NewMultiplexReceiver(selector ReceiverSelector, fallback Receiver) *MultiplexReceiver {
	if fallback == nil {
		panic("multiplex receiver requires non-nil fallback")
	}
	return &MultiplexReceiver{
		selector: selector,
		fallback: fallback,
	}
}

// Receive routes the subscription to the appropriate Receiver based on topic.
func (m *MultiplexReceiver) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
	receiver := m.selector(topic)
	if receiver == nil {
		receiver = m.fallback
	}
	return receiver.Receive(ctx, topic)
}

// ============================================================================
// Helper Functions for Common Routing Patterns
// ============================================================================

// TopicSenderRoute defines a topic pattern and its associated Sender.
type TopicSenderRoute struct {
	Pattern string
	Sender  Sender
}

// NewTopicSenderSelector creates a selector that matches topics against dot-separated patterns.
// Patterns: "*" matches one segment, "**" matches multiple. First match wins.
func NewTopicSenderSelector(routes []TopicSenderRoute) SenderSelector {
	return func(topic string) Sender {
		for _, route := range routes {
			if matchTopicPattern(route.Pattern, topic) {
				return route.Sender
			}
		}
		return nil // No match, use fallback
	}
}

// TopicReceiverRoute defines a topic pattern and its associated Receiver.
type TopicReceiverRoute struct {
	Pattern  string
	Receiver Receiver
}

// NewTopicReceiverSelector creates a selector that matches topics against patterns.
// See NewTopicSenderSelector for pattern syntax.
func NewTopicReceiverSelector(routes []TopicReceiverRoute) ReceiverSelector {
	return func(topic string) Receiver {
		for _, route := range routes {
			if matchTopicPattern(route.Pattern, topic) {
				return route.Receiver
			}
		}
		return nil // No match, use fallback
	}
}

// PrefixSenderSelector creates a selector that routes topics starting with prefix.
func PrefixSenderSelector(prefix string, sender Sender) SenderSelector {
	return func(topic string) Sender {
		if strings.HasPrefix(topic, prefix) {
			return sender
		}
		return nil
	}
}

// PrefixReceiverSelector creates a selector based on topic prefix.
func PrefixReceiverSelector(prefix string, receiver Receiver) ReceiverSelector {
	return func(topic string) Receiver {
		if strings.HasPrefix(topic, prefix) {
			return receiver
		}
		return nil
	}
}

// ChainSenderSelectors combines multiple selectors; first match wins.
func ChainSenderSelectors(selectors ...SenderSelector) SenderSelector {
	return func(topic string) Sender {
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
	return func(topic string) Receiver {
		for _, sel := range selectors {
			if receiver := sel(topic); receiver != nil {
				return receiver
			}
		}
		return nil
	}
}

// ============================================================================
// Pattern Matching
// ============================================================================

// matchTopicPattern matches a topic against a dot-separated pattern.
// "*" matches one segment, "**" matches zero or more segments.
func matchTopicPattern(pattern, topic string) bool {
	// Fast path: exact match
	if pattern == topic {
		return true
	}

	// Fast path: no wildcards
	if !strings.Contains(pattern, "*") {
		return false
	}

	patternParts := strings.Split(pattern, ".")
	topicParts := strings.Split(topic, ".")

	return matchParts(patternParts, topicParts)
}

func matchParts(patternParts, topicParts []string) bool {
	pIdx := 0 // pattern index
	tIdx := 0 // topic index

	for pIdx < len(patternParts) && tIdx < len(topicParts) {
		patternPart := patternParts[pIdx]

		if patternPart == "**" {
			// Multi-segment wildcard
			// If this is the last pattern part, it matches everything remaining
			if pIdx == len(patternParts)-1 {
				return true
			}

			// Try to match the rest of the pattern starting from each remaining topic segment
			for i := tIdx; i < len(topicParts); i++ {
				if matchParts(patternParts[pIdx+1:], topicParts[i:]) {
					return true
				}
			}
			return false

		} else if patternPart == "*" {
			// Single segment wildcard - matches exactly one segment
			pIdx++
			tIdx++

		} else {
			// Exact match required
			if patternPart != topicParts[tIdx] {
				return false
			}
			pIdx++
			tIdx++
		}
	}

	// Both must be exhausted for a match (unless pattern ends with **)
	return pIdx == len(patternParts) && tIdx == len(topicParts)
}

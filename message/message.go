package message

import (
	"sync"
)

type ackType byte

const (
	ackTypeNone ackType = iota
	ackTypeAck
	ackTypeNack
)

type acking struct {
	mu               sync.Mutex
	ack              func()
	nack             func(error)
	ackType          ackType
	ackCount         int
	expectedAckCount int
}

// Properties is a map of message properties.
type Properties map[string]any

// TypedMessage wraps a typed payload with properties and acknowledgment callbacks.
// Use this for type-safe pipelines where you need compile-time guarantees.
// Payload and Properties are public for direct access.
// Ack/Nack operations are mutually exclusive and idempotent.
type TypedMessage[T any] struct {
	Payload    T
	Properties Properties

	a *acking
}

// Message is a non-generic message with []byte payload for pub/sub patterns.
// This is an alias for TypedMessage[[]byte], providing a simpler API for
// message broker integrations (Kafka, RabbitMQ, NATS, etc.).
type Message = TypedMessage[[]byte]

// New creates a new typed message with the given payload and properties.
// Pass nil for properties if no properties are needed.
// For messages requiring acknowledgment, use NewWithAcking instead.
func New[T any](payload T, props Properties) *TypedMessage[T] {
	if props == nil {
		props = make(Properties)
	}
	return &TypedMessage[T]{
		Payload:    payload,
		Properties: props,
		a:          nil,
	}
}

// NewWithAcking creates a new typed message with the given payload, properties, and acknowledgment callbacks.
// Both ack and nack callbacks must be provided (not nil).
func NewWithAcking[T any](payload T, props Properties, ack func(), nack func(error)) *TypedMessage[T] {
	if props == nil {
		props = make(Properties)
	}
	var a *acking
	if ack != nil && nack != nil {
		a = &acking{
			ack:              ack,
			nack:             nack,
			expectedAckCount: 1,
		}
	}
	return &TypedMessage[T]{
		Payload:    payload,
		Properties: props,
		a:          a,
	}
}

// SetExpectedAckCount sets the number of acknowledgments required before invoking the ack callback.
// This must be called before any stage calls Ack() to enable multi-stage pipeline coordination.
// Returns true if the count was successfully set.
// Returns false if no ack callbacks were provided, count is invalid (<=0), or if acking has already started.
// Thread-safe.
func (m *TypedMessage[T]) SetExpectedAckCount(count int) bool {
	if m.a == nil || count <= 0 {
		return false
	}
	m.a.mu.Lock()
	defer m.a.mu.Unlock()
	if m.a.ackCount > 0 {
		return false
	}
	m.a.expectedAckCount = count
	return true
}

// Ack acknowledges successful processing of the message.
// Returns true if acknowledgment succeeded or was already performed.
// Returns false if no ack callback was provided, if the message was already nacked, or if expectedAckCount <= 0.
// The ack callback is invoked at most once when all stages have acked. Thread-safe.
func (m *TypedMessage[T]) Ack() bool {
	if m.a == nil {
		return false
	}
	m.a.mu.Lock()
	defer m.a.mu.Unlock()

	switch m.a.ackType {
	case ackTypeAck:
		return true
	case ackTypeNack:
		return false
	default:
	}

	m.a.ackCount++
	if m.a.ackCount < m.a.expectedAckCount {
		return true
	}

	m.a.ack()
	m.a.ackType = ackTypeAck
	return true
}

// Nack negatively acknowledges the message due to a processing error.
// Returns true if negative acknowledgment succeeded or was already performed.
// Returns false if no nack callback was provided or if the message was already acked.
// The nack callback is invoked immediately with the first error, permanently blocking all further acks.
// Thread-safe.
func (m *TypedMessage[T]) Nack(err error) bool {
	if m.a == nil {
		return false
	}
	m.a.mu.Lock()
	defer m.a.mu.Unlock()

	switch m.a.ackType {
	case ackTypeAck:
		return false
	case ackTypeNack:
		return true
	default:
	}

	m.a.nack(err)
	m.a.ackType = ackTypeNack
	return true
}

// Copy creates a new message with a different payload while preserving
// properties and acknowledgment callbacks.
func Copy[In, Out any](msg *TypedMessage[In], payload Out) *TypedMessage[Out] {
	return &TypedMessage[Out]{
		Payload:    payload,
		Properties: msg.Properties,
		a:          msg.a,
	}
}

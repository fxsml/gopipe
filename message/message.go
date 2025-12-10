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

// Attributes is a map of message context attributes per CloudEvents spec.
// CloudEvents defines attributes as the metadata that describes the event.
type Attributes map[string]any

// TypedMessage wraps a typed data payload with attributes and acknowledgment callbacks.
// Use this for type-safe pipelines where you need compile-time guarantees.
// Data and Attributes are public for direct access.
// Ack/Nack operations are mutually exclusive and idempotent.
type TypedMessage[T any] struct {
	// Data is the event payload per CloudEvents spec.
	// For Message ([]byte), this contains the serialized event data.
	Data T

	// Attributes contains the context attributes per CloudEvents spec.
	Attributes Attributes

	a *acking
}

// Message is a non-generic message with []byte data for pub/sub patterns.
// This is an alias for TypedMessage[[]byte], providing a simpler API for
// message broker integrations (Kafka, RabbitMQ, NATS, etc.).
type Message = TypedMessage[[]byte]

// New creates a new typed message with the given data and attributes.
// Pass nil for attributes if no attributes are needed.
// For messages requiring acknowledgment, use NewWithAcking instead.
func New[T any](data T, attrs Attributes) *TypedMessage[T] {
	if attrs == nil {
		attrs = make(Attributes)
	}
	return &TypedMessage[T]{
		Data:       data,
		Attributes: attrs,
		a:          nil,
	}
}

// NewWithAcking creates a new typed message with the given data, attributes, and acknowledgment callbacks.
// Both ack and nack callbacks must be provided (not nil).
func NewWithAcking[T any](data T, attrs Attributes, ack func(), nack func(error)) *TypedMessage[T] {
	if attrs == nil {
		attrs = make(Attributes)
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
		Data:       data,
		Attributes: attrs,
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

// Copy creates a new message with different data while preserving
// attributes and acknowledgment callbacks.
func Copy[In, Out any](msg *TypedMessage[In], data Out) *TypedMessage[Out] {
	return &TypedMessage[Out]{
		Data:       data,
		Attributes: msg.Attributes,
		a:          msg.a,
	}
}

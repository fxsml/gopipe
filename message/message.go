package message

import (
	"maps"
	"sync"
	"time"
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

// Message wraps a payload with properties and acknowledgment callbacks.
// Payload and Properties are public for direct access.
// Ack/Nack operations are mutually exclusive and idempotent.
type Message[T any] struct {
	Payload    T
	Properties map[string]any

	a *acking
}

// Option is a functional option for configuring a Message.
type Option[T any] func(*Message[T])

// New creates a new message with the given payload and options.
func New[T any](payload T, opts ...Option[T]) *Message[T] {
	m := &Message[T]{
		Payload:    payload,
		Properties: make(map[string]any),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// WithDeadline sets the message processing deadline.
func WithDeadline[T any](deadline time.Time) Option[T] {
	return func(m *Message[T]) {
		if !deadline.IsZero() {
			m.Properties[PropDeadline] = deadline
		}
	}
}

// WithAcking configures acknowledgment callbacks for the message.
// Both ack and nack callbacks must be provided (not nil) for acknowledgment to be enabled.
func WithAcking[T any](ack func(), nack func(error)) Option[T] {
	return func(m *Message[T]) {
		if ack != nil && nack != nil {
			m.a = &acking{
				ack:              ack,
				nack:             nack,
				expectedAckCount: 1,
			}
		}
	}
}

// WithProperty sets a custom property on the message.
func WithProperty[T any](key string, value any) Option[T] {
	return func(m *Message[T]) {
		m.Properties[key] = value
	}
}

// WithProperties sets message properties using a map of key-value pairs.
func WithProperties[T any](props map[string]any) Option[T] {
	return func(m *Message[T]) {
		if props != nil {
			maps.Copy(m.Properties, props)
		}
	}
}

// WithID sets the message ID.
func WithID[T any](id string) Option[T] {
	return func(m *Message[T]) {
		m.Properties[PropID] = id
	}
}

// WithCorrelationID sets the correlation ID for tracking related messages.
func WithCorrelationID[T any](correlationID string) Option[T] {
	return func(m *Message[T]) {
		m.Properties[PropCorrelationID] = correlationID
	}
}

// WithCreatedAt sets the creation timestamp of the message.
func WithCreatedAt[T any](createdAt time.Time) Option[T] {
	return func(m *Message[T]) {
		m.Properties[PropCreatedAt] = createdAt
	}
}

// WithSubject sets the subject of the message.
func WithSubject[T any](subject string) Option[T] {
	return func(m *Message[T]) {
		m.Properties[PropSubject] = subject
	}
}

// WithContentType sets the content type of the message.
func WithContentType[T any](ct string) Option[T] {
	return func(m *Message[T]) {
		m.Properties[PropContentType] = ct
	}
}

// IDProps returns the message ID from properties.
func IDProps(m map[string]any) (string, bool) {
	if v, ok := m[PropID]; ok {
		if id, ok := v.(string); ok {
			return id, true
		}
	}
	return "", false
}

// CorrelationIDProps returns the correlation ID from properties.
func CorrelationIDProps(m map[string]any) (string, bool) {
	if v, ok := m[PropCorrelationID]; ok {
		if id, ok := v.(string); ok {
			return id, true
		}
	}
	return "", false
}

// CreatedAtProps returns the created timestamp from properties.
func CreatedAtProps(m map[string]any) (time.Time, bool) {
	if v, ok := m[PropCreatedAt]; ok {
		if t, ok := v.(time.Time); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

// SubjectProps returns the subject from properties.
func SubjectProps(m map[string]any) (string, bool) {
	if v, ok := m[PropSubject]; ok {
		if s, ok := v.(string); ok {
			return s, true
		}
	}
	return "", false
}

// ContentTypeProps returns the content type from properties.
func ContentTypeProps(m map[string]any) (string, bool) {
	if v, ok := m[PropContentType]; ok {
		if ct, ok := v.(string); ok {
			return ct, true
		}
	}
	return "", false
}

// SetExpectedAckCount sets the number of acknowledgments required before invoking the ack callback.
// This must be called before any stage calls Ack() to enable multi-stage pipeline coordination.
// Returns true if the count was successfully set.
// Returns false if no ack callbacks were provided, count is invalid (<=0), or if acking has already started.
// Thread-safe.
func (m *Message[T]) SetExpectedAckCount(count int) bool {
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

// Deadline returns the deadline for processing this message.
func (m *Message[T]) Deadline() (time.Time, bool) {
	if v, ok := m.Properties[PropDeadline]; ok {
		if deadline, ok := v.(time.Time); ok {
			return deadline, true
		}
	}
	return time.Time{}, false
}

// Ack acknowledges successful processing of the message.
// Returns true if acknowledgment succeeded or was already performed.
// Returns false if no ack callback was provided, if the message was already nacked, or if expectedAckCount <= 0.
// The ack callback is invoked at most once when all stages have acked. Thread-safe.
func (m *Message[T]) Ack() bool {
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
func (m *Message[T]) Nack(err error) bool {
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
func Copy[In, Out any](msg *Message[In], payload Out) *Message[Out] {
	return &Message[Out]{
		Payload:    payload,
		Properties: msg.Properties,
		a:          msg.a,
	}
}

package message

import (
	"context"
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

// Message wraps a payload with properties, context, and acknowledgment callbacks.
// It provides thread-safe, mutually exclusive ack/nack operations for reliable message processing.
// Once acknowledged (ack or nack), subsequent calls return the existing state without re-executing callbacks.
// The context (stored in properties under PropContext) is the source of truth for deadlines and cancellation.
type Message[T any] struct {
	// payload is the actual message data of any type.
	payload T

	properties *Properties
	a          *acking
}

// Properties returns the message properties for thread-safe access to properties.
func (m *Message[T]) Properties() *Properties {
	return m.properties
}

// Option is a functional option for configuring a Message.
type Option[T any] func(*Message[T])

// New creates a new message with the given payload and options.
// This is the recommended constructor for creating messages.
func New[T any](payload T, opts ...Option[T]) *Message[T] {
	m := &Message[T]{
		properties: NewProperties(nil),
		payload:    payload,
	}

	// Apply options
	for _, opt := range opts {
		opt(m)
	}

	return m
}

// WithContext sets the message context.
func WithContext[T any](ctx context.Context) Option[T] {
	return func(m *Message[T]) {
		if ctx != nil {
			m.properties.Set(PropContext, ctx)
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
		m.properties.Set(key, value)
	}
}

// WithProperties sets message properties using a map of key-value pairs.
func WithProperties[T any](props map[string]any) Option[T] {
	return func(m *Message[T]) {
		if props != nil {
			maps.Copy(m.properties.m, props)
		}
	}
}

// WithID sets the message ID.
func WithID[T any](id string) Option[T] {
	return func(m *Message[T]) {
		m.properties.Set(PropID, id)
	}
}

// WithCorrelationID sets the correlation ID for tracking related messages.
func WithCorrelationID[T any](correlationID string) Option[T] {
	return func(m *Message[T]) {
		m.properties.Set(PropCorrelationID, correlationID)
	}
}

// WithCreatedAt sets the creation timestamp of the message.
func WithCreatedAt[T any](createdAt time.Time) Option[T] {
	return func(m *Message[T]) {
		m.properties.Set(PropCreatedAt, createdAt)
	}
}

// Payload returns the message payload.
func (m *Message[T]) Payload() T {
	return m.payload
}

// Context returns the message context. If no context is stored, returns context.Background().
func (m *Message[T]) Context() context.Context {
	if m.properties != nil {
		if v, ok := m.properties.Get(PropContext); ok {
			if ctx, ok := v.(context.Context); ok {
				return ctx
			}
		}
	}
	return context.Background()
}

// ID returns the message ID. This is a convenience shortcut for m.Properties().ID().
func (m *Message[T]) ID() string {
	if m.properties == nil {
		return ""
	}
	return m.properties.ID()
}

// CorrelationID returns the correlation ID. This is a convenience shortcut for m.Properties().CorrelationID().
func (m *Message[T]) CorrelationID() string {
	if m.properties == nil {
		return ""
	}
	return m.properties.CorrelationID()
}

// CreatedAt returns when the message was created. This is a convenience shortcut for m.Properties().CreatedAt().
func (m *Message[T]) CreatedAt() time.Time {
	if m.properties == nil {
		return time.Time{}
	}
	return m.properties.CreatedAt()
}

// RetryCount returns the retry count. This is a convenience shortcut for m.Properties().RetryCount().
func (m *Message[T]) RetryCount() int {
	if m.properties == nil {
		return 0
	}
	return m.properties.RetryCount()
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

// Deadline returns the deadline for processing this message, extracted from the context.
// Returns the deadline and true if a deadline is set, otherwise returns zero time and false.
func (m *Message[T]) Deadline() (time.Time, bool) {
	return m.Context().Deadline()
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

// Copy creates a new message with a different payload while preserving the original message's
// properties (including context) and acknowledgment callbacks. This allows output messages to share
// the same acknowledgment state as their input message. Thread-safe.
func Copy[In, Out any](msg *Message[In], payload Out) *Message[Out] {
	return &Message[Out]{
		properties: msg.properties,
		payload:    payload,
		a:          msg.a,
	}
}

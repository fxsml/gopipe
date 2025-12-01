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
// It provides thread-safe, mutually exclusive ack/nack operations for reliable message processing.
// Once acknowledged (ack or nack), subsequent calls return the existing state without re-executing callbacks.
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

// WithDeadline sets the message processing deadline.
func WithDeadline[T any](deadline time.Time) Option[T] {
	return func(m *Message[T]) {
		if !deadline.IsZero() {
			m.properties.Set(PropDeadline, deadline)
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

// WithReplyTo sets the reply-to address.
func WithReplyTo[T any](addr string) Option[T] {
	return func(m *Message[T]) {
		m.properties.Set(PropReplyTo, addr)
	}
}

// WithSequenceNumber sets the sequence number of the message.
func WithSequenceNumber[T any](seq int64) Option[T] {
	return func(m *Message[T]) {
		m.properties.Set(PropSequenceNumber, seq)
	}
}

// WithPartitionKey sets the partition key.
func WithPartitionKey[T any](key string) Option[T] {
	return func(m *Message[T]) {
		m.properties.Set(PropPartitionKey, key)
	}
}

// WithPartitionOffset sets the offset within the partition.
func WithPartitionOffset[T any](offset int64) Option[T] {
	return func(m *Message[T]) {
		m.properties.Set(PropPartitionOffset, offset)
	}
}

// WithTTL sets the time-to-live of the message.
func WithTTL[T any](ttl time.Duration) Option[T] {
	return func(m *Message[T]) {
		m.properties.Set(PropTTL, ttl)
	}
}

// WithSubject sets the subject of the message.
func WithSubject[T any](subject string) Option[T] {
	return func(m *Message[T]) {
		m.properties.Set(PropSubject, subject)
	}
}

// WithContentType sets the content type of the message.
func WithContentType[T any](ct string) Option[T] {
	return func(m *Message[T]) {
		m.properties.Set(PropContentType, ct)
	}
}

// Payload returns the message payload.
func (m *Message[T]) Payload() T {
	return m.payload
}

// ID returns the message ID. This is a convenience shortcut for m.Properties().ID().
func (m *Message[T]) ID() (string, bool) {
	return m.properties.ID()
}

// CorrelationID returns the correlation ID. This is a convenience shortcut for m.Properties().CorrelationID().
func (m *Message[T]) CorrelationID() (string, bool) {
	return m.properties.CorrelationID()
}

// CreatedAt returns when the message was created. This is a convenience shortcut for m.Properties().CreatedAt().
func (m *Message[T]) CreatedAt() (time.Time, bool) {
	return m.properties.CreatedAt()
}

// DeliveryCount returns the delivery count. This is a convenience shortcut for m.Properties().DeliveryCount().
func (m *Message[T]) DeliveryCount() (int, bool) {
	return m.properties.DeliveryCount()
}

// ReplyTo returns the reply-to address. This is a convenience shortcut for m.Properties().ReplyTo().
func (m *Message[T]) ReplyTo() (string, bool) {
	return m.properties.ReplyTo()
}

// SequenceNumber returns the sequence number. This is a convenience shortcut for m.Properties().SequenceNumber().
func (m *Message[T]) SequenceNumber() (int64, bool) {
	return m.properties.SequenceNumber()
}

// PartitionKey returns the partition key. This is a convenience shortcut for m.Properties().PartitionKey().
func (m *Message[T]) PartitionKey() (string, bool) {
	return m.properties.PartitionKey()
}

// PartitionOffset returns the partition offset. This is a convenience shortcut for m.Properties().PartitionOffset().
func (m *Message[T]) PartitionOffset() (int64, bool) {
	return m.properties.PartitionOffset()
}

// TTL returns the time-to-live. This is a convenience shortcut for m.Properties().TTL().
func (m *Message[T]) TTL() (time.Duration, bool) {
	return m.properties.TTL()
}

// Subject returns the subject. This is a convenience shortcut for m.Properties().Subject().
func (m *Message[T]) Subject() (string, bool) {
	return m.properties.Subject()
}

// ContentType returns the content type. This is a convenience shortcut for m.Properties().ContentType().
func (m *Message[T]) ContentType() (string, bool) {
	return m.properties.ContentType()
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
// Returns the deadline and true if a deadline is set, otherwise returns zero time and false.
func (m *Message[T]) Deadline() (time.Time, bool) {
	if m.properties != nil {
		if v, ok := m.properties.Get(PropDeadline); ok {
			if deadline, ok := v.(time.Time); ok {
				return deadline, true
			}
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

// Copy creates a new message with a different payload while preserving the original message's
// properties and acknowledgment callbacks. This allows output messages to share
// the same acknowledgment state as their input message. Thread-safe.
func Copy[In, Out any](msg *Message[In], payload Out) *Message[Out] {
	return &Message[Out]{
		properties: msg.properties,
		payload:    payload,
		a:          msg.a,
	}
}

package gopipe

import (
	"context"
	"sync"
	"time"
)

// MessageProperties provides thread-safe access to message properties using sync.Map.
type MessageProperties struct {
	m sync.Map
}

// Get retrieves a value from the properties.
func (mp *MessageProperties) Get(key string) (any, bool) {
	if mp == nil {
		return nil, false
	}
	return mp.m.Load(key)
}

// Set stores a key-value pair in the properties.
func (mp *MessageProperties) Set(key string, value any) {
	if mp == nil {
		return
	}
	mp.m.Store(key, value)
}

// Delete removes a key from the properties.
func (mp *MessageProperties) Delete(key string) {
	if mp == nil {
		return
	}
	mp.m.Delete(key)
}

// Range iterates over all key-value pairs in the properties.
// The iteration stops if f returns false.
func (mp *MessageProperties) Range(f func(key string, value any) bool) {
	if mp == nil {
		return
	}
	mp.m.Range(func(k, v any) bool {
		return f(k.(string), v)
	})
}

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

// Message wraps a payload with properties, deadline, and acknowledgment callbacks.
// It provides thread-safe, mutually exclusive ack/nack operations for reliable message processing.
// Once acknowledged (ack or nack), subsequent calls return the existing state without re-executing callbacks.
type Message[T any] struct {
	// Payload is the actual message data.
	Payload T

	properties *MessageProperties
	deadline   time.Time
	a          *acking
}

// Properties returns the message properties for thread-safe access to properties.
func (m *Message[T]) Properties() *MessageProperties {
	return m.properties
}

// NewMessage creates a new message with the specified properties, payload, deadline, and acknowledgment callbacks.
// The ack and nack callbacks are optional (can be nil) and will be called at most once when the message is acknowledged.
// If deadline is non-zero, it will be enforced when processing the message through NewMessagePipe.
func NewMessage[T any](
	properties *MessageProperties,
	payload T,
	deadline time.Time,
	ack func(),
	nack func(error),
) *Message[T] {
	var a *acking
	if ack != nil && nack != nil {
		a = &acking{
			ack:              ack,
			nack:             nack,
			expectedAckCount: 1,
		}
	}

	return &Message[T]{
		properties: properties,
		Payload:    payload,
		deadline:   deadline,
		a:          a,
	}
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
// If the deadline is zero, no deadline is enforced.
func (m *Message[T]) Deadline() time.Time {
	return m.deadline
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
	}

	m.a.nack(err)
	m.a.ackType = ackTypeNack
	return true
}

// CopyMessage creates a new message with a different payload type while preserving the original message's
// properties, deadline, and acknowledgment callbacks. This allows output messages to share the same
// acknowledgment state as their input message. Thread-safe.
func CopyMessage[In, Out any](msg *Message[In], payload Out) *Message[Out] {
	// Create a new MessageProperties and copy all entries from the original
	newProps := &MessageProperties{}
	if msg.properties != nil {
		msg.properties.Range(func(key string, value any) bool {
			newProps.Set(key, value)
			return true
		})
	}

	return &Message[Out]{
		properties: newProps,
		Payload:    payload,
		deadline:   msg.deadline,
		a:          msg.a,
	}
}

// NewMessagePipe creates a pipe that processes messages with automatic acknowledgment.
// The handler function receives the unwrapped payload and returns results.
// On success, the input message is automatically acked and output messages are created using CopyMessage.
// On failure, the input message is automatically nacked with the error via the WithCancel option.
// Message deadlines are enforced using context.WithDeadline. Supports all standard pipe options.
func NewMessagePipe[In, Out any](
	handle func(context.Context, In) ([]Out, error),
	opts ...Option[*Message[In], *Message[Out]],
) Pipe[*Message[In], *Message[Out]] {
	opts = append([]Option[*Message[In], *Message[Out]]{
		WithCancel[*Message[In], *Message[Out]](func(msg *Message[In], err error) {
			msg.Nack(err)
		}),
	}, opts...)

	proc := NewProcessor(
		func(ctx context.Context, msg *Message[In]) ([]*Message[Out], error) {
			if !msg.deadline.IsZero() {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, msg.deadline)
				defer cancel()
			}

			results, err := handle(ctx, msg.Payload)
			if err != nil {
				return nil, err
			}

			msg.Ack()

			var messages []*Message[Out]
			for _, result := range results {
				messages = append(messages, CopyMessage(msg, result))
			}
			return messages, nil
		}, nil)

	return newPipe(noopPreProcessorFunc[*Message[In]], proc, opts...)
}

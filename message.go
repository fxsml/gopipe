package gopipe

import (
	"context"
	"sync"
	"time"
)

type ackType byte

const (
	ackTypeNone ackType = iota
	ackTypeAck
	ackTypeNack
)

// Message wraps a payload with metadata, deadline, and acknowledgment callbacks.
// It provides thread-safe, mutually exclusive ack/nack operations for reliable message processing.
// Once acknowledged (ack or nack), subsequent calls return the existing state without re-executing callbacks.
type Message[T any] struct {
	// Metadata contains additional contextual information about the message.
	Metadata Metadata
	// Payload is the actual message data.
	Payload  T
	deadline time.Time

	mu   sync.Mutex
	ack  func()
	nack func(error)

	ackType ackType
}

// NewMessage creates a new message with the specified metadata, payload, deadline, and acknowledgment callbacks.
// The ack and nack callbacks are optional (can be nil) and will be called at most once when the message is acknowledged.
// If deadline is non-zero, it will be enforced when processing the message through NewMessagePipe.
func NewMessage[T any](
	metadata Metadata,
	payload T,
	deadline time.Time,
	ack func(),
	nack func(error),
) *Message[T] {
	return &Message[T]{
		Metadata: metadata,
		Payload:  payload,
		deadline: deadline,
		ack:      ack,
		nack:     nack,
	}
}

// Deadline returns the deadline for processing this message.
// If the deadline is zero, no deadline is enforced.
func (m *Message[T]) Deadline() time.Time {
	return m.deadline
}

// Ack acknowledges successful processing of the message.
// Returns true if acknowledgment succeeded or was already performed.
// Returns false if no ack callback was provided or if the message was already nacked.
// The ack callback is invoked at most once. Thread-safe.
func (m *Message[T]) Ack() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ack == nil || m.ackType == ackTypeNack {
		return false
	}
	if m.ackType == ackTypeAck {
		return true
	}
	m.ack()
	m.ackType = ackTypeAck
	return true
}

// Nack negatively acknowledges the message due to a processing error.
// Returns true if negative acknowledgment succeeded or was already performed.
// Returns false if no nack callback was provided or if the message was already acked.
// The nack callback is invoked at most once with the provided error. Thread-safe.
func (m *Message[T]) Nack(err error) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nack == nil || m.ackType == ackTypeAck {
		return false
	}
	if m.ackType == ackTypeNack {
		return true
	}
	m.nack(err)
	m.ackType = ackTypeNack
	return true
}

// CopyMessage creates a new message with a different payload type while preserving the original message's
// metadata, deadline, and acknowledgment callbacks. This allows output messages to share the same
// acknowledgment state as their input message. Thread-safe.
func CopyMessage[In, Out any](msg *Message[In], payload Out) *Message[Out] {
	return &Message[Out]{
		Metadata: msg.Metadata,
		Payload:  payload,
		deadline: msg.deadline,
		ack:      msg.ack,
		nack:     msg.nack,
		ackType:  msg.ackType,
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
		WithMetadataProvider[*Message[In], *Message[Out]](
			func(msg *Message[In]) Metadata {
				return msg.Metadata
			},
		),
	}, opts...)

	return NewProcessPipe(
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
		},
		opts...,
	)
}

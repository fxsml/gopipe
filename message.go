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

type Message[T any] struct {
	Metadata Metadata
	Payload  T
	deadline time.Time

	mu   sync.Mutex
	ack  func()
	nack func(error)

	ackType ackType
}

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

func (m *Message[T]) Deadline() time.Time {
	return m.deadline
}

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

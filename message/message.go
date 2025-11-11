package message

import (
	"sync"
	"time"

	"github.com/fxsml/gopipe"
)

type ackType byte

const (
	ackTypeNone ackType = iota
	ackTypeAck
	ackTypeNack
)

type Message[T any] struct {
	ID       string
	Metadata gopipe.Metadata
	Payload  T
	deadline time.Time

	mu   sync.Mutex
	ack  func()
	nack func(error)

	ackType ackType
}

func NewMessage[T any](
	id string,
	payload T,
) *Message[T] {
	return &Message[T]{
		ID:       id,
		Payload:  payload,
		Metadata: make(gopipe.Metadata),
	}
}

func NewMessageWithAck[T any](
	id string,
	payload T,
	ack func(),
	nack func(error),
) *Message[T] {
	m := NewMessage(id, payload)
	m.ack = ack
	m.nack = nack
	return m
}

func (m *Message[T]) SetTimeout(d time.Duration) {
	m.deadline = time.Now().Add(d)
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

// Package pubsub provides publish-subscribe messaging abstractions and implementations.
//
// The package defines core interfaces (Sender, Receiver) and provides
// implementations: in-process channel-based, IO streams, and HTTP.
//
// Topic naming uses "/" as separator (e.g., "orders/created").
package pubsub

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fxsml/gopipe/message"
)

// Sender publishes messages to topics.
type Sender interface {
	// Send publishes messages to the specified topic.
	// Returns an error if the operation fails or context is canceled.
	Send(ctx context.Context, topic string, msgs []*message.Message) error
}

// Receiver consumes messages from topics.
type Receiver interface {
	// Receive retrieves messages from the specified topic.
	// Behavior varies by implementation: may block, poll, or return buffered messages.
	Receive(ctx context.Context, topic string) ([]*message.Message, error)
}

var (
	// ErrBrokerClosed is returned when operations are attempted on a closed broker.
	ErrBrokerClosed = errors.New("broker is closed")
	// ErrSendTimeout is returned when a send operation times out.
	ErrSendTimeout = errors.New("send timeout")
)

// BrokerConfig configures the in-process broker.
type BrokerConfig struct {
	// BufferSize is the channel buffer size for subscriptions.
	// Default: 100.
	BufferSize int

	// SendTimeout is the maximum duration for a send operation.
	// Zero means no timeout (blocks until delivered or context canceled).
	SendTimeout time.Duration

	// CloseTimeout is the maximum duration to wait for graceful shutdown.
	// Default: 5 seconds.
	CloseTimeout time.Duration
}

func (c BrokerConfig) defaults() BrokerConfig {
	cfg := c
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 100
	}
	if cfg.CloseTimeout == 0 {
		cfg.CloseTimeout = 5 * time.Second
	}
	return cfg
}

// subscription represents an active subscription to a topic.
type subscription struct {
	id    string
	topic string
	ch    chan *message.Message
}

// Broker is an in-process message broker using Go channels.
// It supports both Subscribe (push) and Receive (pull) patterns.
type Broker struct {
	config BrokerConfig

	mu     sync.RWMutex
	subs   map[string]*subscription // keyed by subscription ID
	nextID uint64
	closed bool
}

// Compile-time interface assertions
var (
	_ Sender   = (*Broker)(nil)
	_ Receiver = (*Broker)(nil)
)

// NewBroker creates a new in-process message broker.
func NewBroker(config BrokerConfig) *Broker {
	cfg := config.defaults()
	return &Broker{
		config: cfg,
		subs:   make(map[string]*subscription),
	}
}

// Subscribe creates a subscription to the specified topic.
// Returns a channel that receives messages sent to that topic.
// The channel is closed when the context is canceled or the broker is closed.
func (b *Broker) Subscribe(ctx context.Context, topic string) <-chan *message.Message {
	out := make(chan *message.Message, b.config.BufferSize)

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(out)
		return out
	}

	id := b.nextSubID()
	sub := &subscription{
		id:    id,
		topic: topic,
		ch:    make(chan *message.Message, b.config.BufferSize),
	}
	b.subs[id] = sub
	b.mu.Unlock()

	// Forward messages from internal channel to output channel
	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				// Context canceled - remove subscription
				b.mu.Lock()
				if _, exists := b.subs[id]; exists {
					delete(b.subs, id)
					close(sub.ch)
				}
				b.mu.Unlock()
				return
			case msg, ok := <-sub.ch:
				if !ok {
					// Channel closed by broker.Close() - subscription already removed
					return
				}
				select {
				case <-ctx.Done():
					return
				case out <- msg:
				}
			}
		}
	}()

	return out
}

// Send publishes messages to all subscribers of the specified topic.
// Messages are delivered to subscribers with exact topic match only.
func (b *Broker) Send(ctx context.Context, topic string, msgs []*message.Message) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return ErrBrokerClosed
	}

	// Get matching subscriptions (exact match only)
	var matching []*subscription
	for _, sub := range b.subs {
		if sub.topic == topic {
			matching = append(matching, sub)
		}
	}
	b.mu.RUnlock()

	// Apply send timeout if configured
	if b.config.SendTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.config.SendTimeout)
		defer cancel()
	}

	// Send to all matching subscriptions
	for _, msg := range msgs {
		for _, sub := range matching {
			select {
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					return ErrSendTimeout
				}
				return ctx.Err()
			case sub.ch <- msg:
				// Message sent
			default:
				// Channel full, block with context
				select {
				case <-ctx.Done():
					if ctx.Err() == context.DeadlineExceeded {
						return ErrSendTimeout
					}
					return ctx.Err()
				case sub.ch <- msg:
					// Message sent
				}
			}
		}
	}

	return nil
}

// Receive polls for messages from the specified topic.
// Creates a temporary subscription, collects available messages, and returns.
// For continuous message consumption, use Subscribe instead.
func (b *Broker) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrBrokerClosed
	}

	id := b.nextSubID()
	sub := &subscription{
		id:    id,
		topic: topic,
		ch:    make(chan *message.Message, b.config.BufferSize),
	}
	b.subs[id] = sub
	b.mu.Unlock()

	// Collect messages with short timeout
	var result []*message.Message
	timeout := 100 * time.Millisecond
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	defer func() {
		b.mu.Lock()
		if _, exists := b.subs[id]; exists {
			delete(b.subs, id)
			close(sub.ch)
		}
		b.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-timer.C:
			return result, nil
		case msg := <-sub.ch:
			result = append(result, msg)
			timer.Reset(10 * time.Millisecond)
		}
	}
}

// Close gracefully shuts down the broker.
func (b *Broker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return ErrBrokerClosed
	}
	b.closed = true

	// Close all subscription channels
	for _, sub := range b.subs {
		close(sub.ch)
	}
	b.subs = nil

	return nil
}

func (b *Broker) nextSubID() string {
	id := atomic.AddUint64(&b.nextID, 1)
	return string(rune(id)) + "-sub"
}

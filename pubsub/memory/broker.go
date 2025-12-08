// Package memory provides in-memory implementation for message brokering.
package memory

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/pubsub"
)

var (
	// ErrBrokerClosed is returned when operations are attempted on a closed broker.
	ErrBrokerClosed = errors.New("broker is closed")
	// ErrSendTimeout is returned when a send operation times out.
	ErrSendTimeout = errors.New("send timeout")
	// ErrReceiveTimeout is returned when a receive operation times out.
	ErrReceiveTimeout = errors.New("receive timeout")
)

// Config configures the broker behavior.
type Config struct {
	// CloseTimeout is the maximum duration to wait for graceful shutdown.
	// Default: 5 seconds.
	CloseTimeout time.Duration

	// SendTimeout is the maximum duration for a send operation.
	// Zero means no timeout (blocks until delivered or context canceled).
	SendTimeout time.Duration

	// ReceiveTimeout is the maximum duration to wait for a message when receiving.
	// Zero means no timeout (blocks until message available or context canceled).
	ReceiveTimeout time.Duration

	// BufferSize is the channel buffer size for each topic.
	// Default: 100.
	BufferSize int
}

func (c *Config) defaults() Config {
	cfg := *c
	if cfg.CloseTimeout == 0 {
		cfg.CloseTimeout = 5 * time.Second
	}
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 100
	}
	return cfg
}

// topic represents a single topic with buffered messages.
type topic struct {
	mu       sync.RWMutex
	messages []*message.Message
}

func newTopic() *topic {
	return &topic{
		messages: make([]*message.Message, 0),
	}
}

func (t *topic) addMessage(msg *message.Message) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messages = append(t.messages, msg)
}

func (t *topic) getMessages() []*message.Message {
	t.mu.RLock()
	defer t.mu.RUnlock()
	// Return a copy of messages
	result := make([]*message.Message, len(t.messages))
	copy(result, t.messages)
	return result
}

func (t *topic) clearMessages() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messages = nil
}

// memoryBroker is an in-memory implementation of Broker.
type memoryBroker struct {
	config Config

	mu     sync.RWMutex
	topics map[string]*topic
	closed bool
	wg     sync.WaitGroup
}

// NewBroker creates a new in-memory broker with the given configuration.
func NewBroker(config Config) pubsub.Broker {
	cfg := config.defaults()
	return &memoryBroker{
		config: cfg,
		topics: make(map[string]*topic),
	}
}

// NewSender returns a Sender backed by the given broker.
// This allows using the broker as a Sender in components that only need send capability.
func NewSender(b pubsub.Broker) pubsub.Sender {
	return b
}

// NewReceiver returns a Receiver backed by the given broker.
// This allows using the broker as a Receiver in components that only need receive capability.
func NewReceiver(b pubsub.Broker) pubsub.Receiver {
	return b
}

// getOrCreateTopic returns the topic for the given name, creating it if necessary.
func (b *memoryBroker) getOrCreateTopic(name string) *topic {
	b.mu.RLock()
	t, ok := b.topics[name]
	b.mu.RUnlock()
	if ok {
		return t
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	// Double-check after acquiring write lock
	if t, ok = b.topics[name]; ok {
		return t
	}
	t = newTopic()
	b.topics[name] = t
	return t
}

// Send sends messages to the specified topic.
func (b *memoryBroker) Send(ctx context.Context, topicName string, msgs []*message.Message) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return ErrBrokerClosed
	}
	b.mu.RUnlock()

	// Apply send timeout if configured
	if b.config.SendTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.config.SendTimeout)
		defer cancel()
	}

	t := b.getOrCreateTopic(topicName)

	for _, msg := range msgs {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return ErrSendTimeout
			}
			return ctx.Err()
		default:
			t.addMessage(msg)
		}
	}

	return nil
}

// Receive returns a channel that emits messages from the specified topic.
func (b *memoryBroker) Receive(ctx context.Context, topicName string) ([]*message.Message, error) {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return nil, ErrBrokerClosed
	}
	b.mu.RUnlock()

	t := b.getOrCreateTopic(topicName)

	// Return all messages from the topic
	return t.getMessages(), nil
}

// Close gracefully shuts down the broker.
func (b *memoryBroker) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrBrokerClosed
	}
	b.closed = true

	// Clear all topic messages
	for _, t := range b.topics {
		t.clearMessages()
	}
	b.mu.Unlock()

	return nil
}

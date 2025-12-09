package pubsub

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/fxsml/gopipe/message"
)

var (
	// ErrBrokerClosed is returned when operations are attempted on a closed broker.
	ErrBrokerClosed = errors.New("broker is closed")
	// ErrSendTimeout is returned when a send operation times out.
	ErrSendTimeout = errors.New("send timeout")
	// ErrReceiveTimeout is returned when a receive operation times out.
	ErrReceiveTimeout = errors.New("receive timeout")
)

// InMemoryConfig configures the in-memory broker behavior.
type InMemoryConfig struct {
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

func (c InMemoryConfig) defaults() InMemoryConfig {
	cfg := c
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

// inMemoryBroker is an in-memory implementation of Broker.
type inMemoryBroker struct {
	config InMemoryConfig

	mu     sync.RWMutex
	topics map[string]*topic
	closed bool
	wg     sync.WaitGroup
}

// NewInMemoryBroker creates a new in-memory broker with the given configuration.
// This broker stores messages in memory and is suitable for testing and simple use cases.
func NewInMemoryBroker(config InMemoryConfig) Broker {
	cfg := config.defaults()
	return &inMemoryBroker{
		config: cfg,
		topics: make(map[string]*topic),
	}
}

// getOrCreateTopic returns the topic for the given name, creating it if necessary.
func (b *inMemoryBroker) getOrCreateTopic(name string) *topic {
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
func (b *inMemoryBroker) Send(ctx context.Context, topicName string, msgs []*message.Message) error {
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

// Receive returns all messages from the specified topic.
func (b *inMemoryBroker) Receive(ctx context.Context, topicName string) ([]*message.Message, error) {
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
func (b *inMemoryBroker) Close() error {
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

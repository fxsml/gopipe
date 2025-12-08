// Package broker provides interfaces and in-memory implementation for message brokering.
// It supports topic-based publish/subscribe patterns with "/" separated topic hierarchies.
package broker

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

// Sender sends messages to a topic.
type Sender[T any] interface {
	// Send sends messages to the specified topic.
	// Returns an error if the send fails or times out.
	Send(ctx context.Context, topic string, msgs ...*message.Message[T]) error
}

// Receiver receives messages from a topic.
type Receiver[T any] interface {
	// Receive returns a channel that emits messages from the specified topic.
	// The channel is closed when the context is canceled or the broker is closed.
	Receive(ctx context.Context, topic string) <-chan *message.Message[T]
}

// Broker combines Sender and Receiver capabilities.
type Broker[T any] interface {
	Sender[T]
	Receiver[T]
	// Close gracefully shuts down the broker.
	Close() error
}

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

// topic represents a single topic with its subscribers.
type topic[T any] struct {
	mu          sync.RWMutex
	subscribers map[*subscriber[T]]struct{}
}

func newTopic[T any]() *topic[T] {
	return &topic[T]{
		subscribers: make(map[*subscriber[T]]struct{}),
	}
}

func (t *topic[T]) addSubscriber(s *subscriber[T]) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.subscribers[s] = struct{}{}
}

func (t *topic[T]) removeSubscriber(s *subscriber[T]) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.subscribers, s)
}

func (t *topic[T]) broadcast(msg *message.Message[T]) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for s := range t.subscribers {
		select {
		case s.ch <- msg:
		default:
			// Drop message if subscriber is slow (non-blocking)
		}
	}
}

// subscriber represents a subscription to a topic.
type subscriber[T any] struct {
	ch chan *message.Message[T]
}

// memoryBroker is an in-memory implementation of Broker.
type memoryBroker[T any] struct {
	config Config

	mu     sync.RWMutex
	topics map[string]*topic[T]
	closed bool
	wg     sync.WaitGroup
}

// NewBroker creates a new in-memory broker with the given configuration.
func NewBroker[T any](config Config) Broker[T] {
	cfg := config.defaults()
	return &memoryBroker[T]{
		config: cfg,
		topics: make(map[string]*topic[T]),
	}
}

// NewSender returns a Sender backed by the given broker.
// This allows using the broker as a Sender in components that only need send capability.
func NewSender[T any](b Broker[T]) Sender[T] {
	return b
}

// NewReceiver returns a Receiver backed by the given broker.
// This allows using the broker as a Receiver in components that only need receive capability.
func NewReceiver[T any](b Broker[T]) Receiver[T] {
	return b
}

// getOrCreateTopic returns the topic for the given name, creating it if necessary.
func (b *memoryBroker[T]) getOrCreateTopic(name string) *topic[T] {
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
	t = newTopic[T]()
	b.topics[name] = t
	return t
}

// Send sends messages to the specified topic.
func (b *memoryBroker[T]) Send(ctx context.Context, topicName string, msgs ...*message.Message[T]) error {
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
			t.broadcast(msg)
		}
	}

	return nil
}

// Receive returns a channel that emits messages from the specified topic.
func (b *memoryBroker[T]) Receive(ctx context.Context, topicName string) <-chan *message.Message[T] {
	out := make(chan *message.Message[T])

	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		close(out)
		return out
	}
	b.mu.RUnlock()

	t := b.getOrCreateTopic(topicName)
	s := &subscriber[T]{
		ch: make(chan *message.Message[T], b.config.BufferSize),
	}
	t.addSubscriber(s)

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer close(out)
		defer t.removeSubscriber(s)

		for {
			// Apply receive timeout if configured
			var timeoutCh <-chan time.Time
			if b.config.ReceiveTimeout > 0 {
				timer := time.NewTimer(b.config.ReceiveTimeout)
				timeoutCh = timer.C
				defer timer.Stop()
			}

			select {
			case <-ctx.Done():
				return
			case msg, ok := <-s.ch:
				if !ok {
					return
				}
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			case <-timeoutCh:
				// Timeout on receive, continue waiting
				continue
			}
		}
	}()

	return out
}

// Close gracefully shuts down the broker.
func (b *memoryBroker[T]) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrBrokerClosed
	}
	b.closed = true

	// Close all subscriber channels
	for _, t := range b.topics {
		t.mu.Lock()
		for s := range t.subscribers {
			close(s.ch)
		}
		t.mu.Unlock()
	}
	b.mu.Unlock()

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(b.config.CloseTimeout):
		return context.DeadlineExceeded
	}
}

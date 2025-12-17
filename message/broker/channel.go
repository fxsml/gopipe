package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fxsml/gopipe/message"
)

var (
	// ErrBrokerClosed is returned when operations are attempted on a closed broker.
	ErrBrokerClosed = errors.New("broker is closed")
	// ErrSendTimeout is returned when a send operation times out.
	ErrSendTimeout = errors.New("send timeout")
)

// ChannelBrokerConfig configures the in-process channel broker.
type ChannelBrokerConfig struct {
	// BufferSize is the channel buffer size for subscriptions.
	// Default: 100.
	BufferSize int

	// SendTimeout is the maximum duration for a send operation.
	// Zero means no timeout (blocks until delivered or context canceled).
	SendTimeout time.Duration

	// ReceiveTimeout is the maximum duration to wait for messages in Receive.
	// Zero means block indefinitely until a message arrives or context is canceled.
	// Default: 0 (blocking).
	ReceiveTimeout time.Duration

	// CloseTimeout is the maximum duration to wait for graceful shutdown.
	// Default: 5 seconds.
	CloseTimeout time.Duration
}

func (c ChannelBrokerConfig) defaults() ChannelBrokerConfig {
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

// ChannelBroker is an in-process message broker using Go channels.
// It supports both Subscribe (push) and Receive (pull) patterns.
type ChannelBroker struct {
	config ChannelBrokerConfig

	mu     sync.RWMutex
	subs   map[string]*subscription // keyed by subscription ID
	nextID uint64
	closed bool
}

// Compile-time interface assertions
var (
	_ message.Sender   = (*ChannelBroker)(nil)
	_ message.Receiver = (*ChannelBroker)(nil)
)

// NewChannelBroker creates a new in-process message broker.
func NewChannelBroker(config ChannelBrokerConfig) *ChannelBroker {
	cfg := config.defaults()
	return &ChannelBroker{
		config: cfg,
		subs:   make(map[string]*subscription),
	}
}

// Subscribe creates a subscription to the specified topic.
// Returns a channel that receives messages sent to that topic.
// The channel is closed when the context is canceled or the broker is closed.
func (b *ChannelBroker) Subscribe(ctx context.Context, topic string) <-chan *message.Message {
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
func (b *ChannelBroker) Send(ctx context.Context, topic string, msgs []*message.Message) error {
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

// Receive waits for messages from the specified topic.
// If ReceiveTimeout is configured, returns after timeout even if no messages received.
// If ReceiveTimeout is zero (default), blocks until at least one message arrives.
// After receiving the first message, collects any additional immediately available messages.
// Returns when context is canceled, broker is closed, or timeout is reached.
func (b *ChannelBroker) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
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

	defer func() {
		b.mu.Lock()
		if _, exists := b.subs[id]; exists {
			delete(b.subs, id)
			close(sub.ch)
		}
		b.mu.Unlock()
	}()

	var result []*message.Message

	// Wait for first message with optional timeout
	if b.config.ReceiveTimeout > 0 {
		timer := time.NewTimer(b.config.ReceiveTimeout)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, nil // Timeout, return empty (no error)
		case msg, ok := <-sub.ch:
			if !ok {
				return nil, ErrBrokerClosed
			}
			result = append(result, msg)
		}
	} else {
		// Block indefinitely until message arrives
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case msg, ok := <-sub.ch:
			if !ok {
				return nil, ErrBrokerClosed
			}
			result = append(result, msg)
		}
	}

	// Collect any additional immediately available messages (non-blocking)
	for {
		select {
		case msg, ok := <-sub.ch:
			if !ok {
				return result, nil
			}
			result = append(result, msg)
		default:
			// No more messages immediately available
			return result, nil
		}
	}
}

// Close gracefully shuts down the broker.
func (b *ChannelBroker) Close() error {
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

func (b *ChannelBroker) nextSubID() string {
	id := atomic.AddUint64(&b.nextID, 1)
	return fmt.Sprintf("sub-%d", id)
}

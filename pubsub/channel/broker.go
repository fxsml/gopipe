// Package channel provides a channel-based broker for pub/sub messaging.
package channel

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

// Config configures the channel-based broker.
type Config struct {
	// BufferSize is the channel buffer size for each topic.
	// Default: 100.
	BufferSize int

	// SendTimeout is the maximum duration for a send operation.
	// Zero means no timeout (blocks until delivered or context canceled).
	SendTimeout time.Duration

	// ReceiveTimeout is the maximum duration to wait for messages when receiving.
	// Zero means no timeout (blocks until message available or context canceled).
	ReceiveTimeout time.Duration

	// CloseTimeout is the maximum duration to wait for graceful shutdown.
	// Default: 5 seconds.
	CloseTimeout time.Duration
}

func (c *Config) defaults() Config {
	cfg := *c
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 100
	}
	if cfg.CloseTimeout == 0 {
		cfg.CloseTimeout = 5 * time.Second
	}
	return cfg
}

// topicMessage pairs a topic with a message.
type topicMessage struct {
	topic string
	msg   *message.Message
}

// subscription represents a single subscription to messages.
type subscription struct {
	pattern string
	ch      chan topicMessage
}

// Broker implements a channel-based pub/sub broker.
// Messages are distributed to subscribers using Go channels.
type Broker struct {
	config Config

	mu            sync.RWMutex
	subscriptions []*subscription
	closed        bool
}

// NewBroker creates a new channel-based broker.
// This broker uses Go channels for message distribution,
// making it suitable for in-process concurrent message passing.
func NewBroker(config Config) pubsub.Broker {
	cfg := config.defaults()
	return &Broker{
		config:        cfg,
		subscriptions: make([]*subscription, 0),
	}
}

// Send sends messages to all subscribers whose topic patterns match.
func (b *Broker) Send(ctx context.Context, topic string, msgs []*message.Message) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return ErrBrokerClosed
	}

	// Get snapshot of subscriptions
	subs := make([]*subscription, len(b.subscriptions))
	copy(subs, b.subscriptions)
	b.mu.RUnlock()

	// Apply send timeout if configured
	if b.config.SendTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.config.SendTimeout)
		defer cancel()
	}

	// Send to all matching subscriptions
	for _, msg := range msgs {
		tm := topicMessage{topic: topic, msg: msg}

		for _, sub := range subs {
			// Check if topic matches subscription pattern
			matcher := pubsub.NewTopicMatcher(sub.pattern)
			if !matcher.Matches(topic) {
				continue
			}

			select {
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					return ErrSendTimeout
				}
				return ctx.Err()
			case sub.ch <- tm:
				// Message sent successfully
			default:
				// Channel full, try with timeout
				select {
				case <-ctx.Done():
					if ctx.Err() == context.DeadlineExceeded {
						return ErrSendTimeout
					}
					return ctx.Err()
				case sub.ch <- tm:
					// Message sent successfully
				}
			}
		}
	}

	return nil
}

// Receive receives messages matching the topic pattern.
// It creates a temporary subscription and collects messages until
// the receive timeout or context cancellation.
func (b *Broker) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrBrokerClosed
	}

	// Create a subscription channel
	sub := &subscription{
		pattern: topic,
		ch:      make(chan topicMessage, b.config.BufferSize),
	}
	b.subscriptions = append(b.subscriptions, sub)
	subIndex := len(b.subscriptions) - 1
	b.mu.Unlock()

	// Collect messages
	var result []*message.Message
	timeout := b.config.ReceiveTimeout
	if timeout == 0 {
		timeout = 100 * time.Millisecond
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	// Remove subscription when done
	defer func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		// Remove the subscription by index
		if subIndex < len(b.subscriptions) {
			b.subscriptions = append(b.subscriptions[:subIndex], b.subscriptions[subIndex+1:]...)
		}
		close(sub.ch)
	}()

	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-timer.C:
			return result, nil
		case tm := <-sub.ch:
			result = append(result, tm.msg)
			// Reset timer to collect more messages if they arrive quickly
			timer.Reset(10 * time.Millisecond)
		}
	}
}

// Subscribe creates a long-lived subscription to messages matching the topic pattern.
// Returns a channel that emits messages. The channel is closed when the broker closes
// or the context is canceled.
func (b *Broker) Subscribe(ctx context.Context, topic string) <-chan *message.Message {
	out := make(chan *message.Message, b.config.BufferSize)

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(out)
		return out
	}

	sub := &subscription{
		pattern: topic,
		ch:      make(chan topicMessage, b.config.BufferSize),
	}
	b.subscriptions = append(b.subscriptions, sub)
	subIndex := len(b.subscriptions) - 1
	b.mu.Unlock()

	// Forward messages from internal channel to output channel
	go func() {
		defer func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			// Remove subscription
			if subIndex < len(b.subscriptions) {
				b.subscriptions = append(b.subscriptions[:subIndex], b.subscriptions[subIndex+1:]...)
			}
			close(sub.ch)
			close(out)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case tm, ok := <-sub.ch:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return
				case out <- tm.msg:
				}
			}
		}
	}()

	return out
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
	for _, sub := range b.subscriptions {
		close(sub.ch)
	}
	b.subscriptions = nil

	return nil
}

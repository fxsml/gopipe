package pubsub

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/fxsml/gopipe/message"
)

var (
	// ErrChannelBrokerClosed is returned when operations are attempted on a closed channel broker.
	ErrChannelBrokerClosed = errors.New("channel broker is closed")
	// ErrChannelSendTimeout is returned when a send operation times out.
	ErrChannelSendTimeout = errors.New("channel send timeout")
	// ErrChannelReceiveTimeout is returned when a receive operation times out.
	ErrChannelReceiveTimeout = errors.New("channel receive timeout")
)

// ChannelConfig configures the channel-based broker.
type ChannelConfig struct {
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

func (c ChannelConfig) defaults() ChannelConfig {
	cfg := c
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 100
	}
	if cfg.CloseTimeout == 0 {
		cfg.CloseTimeout = 5 * time.Second
	}
	return cfg
}

// topicChannelMessage pairs a topic with a message.
type topicChannelMessage struct {
	topic string
	msg   *message.Message
}

// channelSubscription represents a single subscription to messages.
type channelSubscription struct {
	pattern string
	ch      chan topicChannelMessage
}

// channelBroker implements a channel-based pub/sub broker.
// Messages are distributed to subscribers using Go channels.
type channelBroker struct {
	config ChannelConfig

	mu            sync.RWMutex
	subscriptions []*channelSubscription
	closed        bool
}

// NewChannelBroker creates a new channel-based broker.
// This broker uses Go channels for message distribution,
// making it suitable for in-process concurrent message passing.
func NewChannelBroker(config ChannelConfig) Broker {
	cfg := config.defaults()
	return &channelBroker{
		config:        cfg,
		subscriptions: make([]*channelSubscription, 0),
	}
}

// Send sends messages to all subscribers whose topic patterns match.
func (b *channelBroker) Send(ctx context.Context, topic string, msgs []*message.Message) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return ErrChannelBrokerClosed
	}

	// Get snapshot of subscriptions
	subs := make([]*channelSubscription, len(b.subscriptions))
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
		tm := topicChannelMessage{topic: topic, msg: msg}

		for _, sub := range subs {
			// Check if topic matches subscription pattern
			matcher := NewTopicMatcher(sub.pattern)
			if !matcher.Matches(topic) {
				continue
			}

			select {
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					return ErrChannelSendTimeout
				}
				return ctx.Err()
			case sub.ch <- tm:
				// Message sent successfully
			default:
				// Channel full, try with timeout
				select {
				case <-ctx.Done():
					if ctx.Err() == context.DeadlineExceeded {
						return ErrChannelSendTimeout
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
func (b *channelBroker) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrChannelBrokerClosed
	}

	// Create a subscription channel
	sub := &channelSubscription{
		pattern: topic,
		ch:      make(chan topicChannelMessage, b.config.BufferSize),
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
func (b *channelBroker) Subscribe(ctx context.Context, topic string) <-chan *message.Message {
	out := make(chan *message.Message, b.config.BufferSize)

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(out)
		return out
	}

	sub := &channelSubscription{
		pattern: topic,
		ch:      make(chan topicChannelMessage, b.config.BufferSize),
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
func (b *channelBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return ErrChannelBrokerClosed
	}
	b.closed = true

	// Close all subscription channels
	for _, sub := range b.subscriptions {
		close(sub.ch)
	}
	b.subscriptions = nil

	return nil
}

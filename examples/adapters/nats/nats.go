// Package nats provides a NATS adapter for gopipe message streaming.
//
// This adapter demonstrates the Subscriber[*Message] pattern without using
// the deprecated Sender/Receiver interfaces. It shows how broker-specific
// concepts (subjects, wildcards, JetStream) map to gopipe patterns.
//
// # Topic Semantics
//
// NATS uses "subjects" as topics with hierarchical wildcard support:
//   - Exact: "orders.created" matches only "orders.created"
//   - Single wildcard: "orders.*" matches "orders.created", "orders.updated"
//   - Multi wildcard: "orders.>" matches "orders.created", "orders.us.created"
//
// This is fundamentally different from Kafka's partition model or RabbitMQ's
// exchange/queue/binding model, which is why gopipe doesn't force a single
// "topic" abstraction.
//
// # Usage
//
//	// Subscriber - directly implements Subscriber[*Message]
//	sub := nats.NewSubscriber(nats.SubscriberConfig{
//	    URL:     "nats://localhost:4222",
//	    Subject: "orders.>",  // NATS-specific: wildcard subscription
//	})
//	msgs := sub.Subscribe(ctx)
//
//	// Publisher - uses channel.GroupBy for batching
//	pub := nats.NewPublisher(nats.PublisherConfig{
//	    URL: "nats://localhost:4222",
//	})
//	groups := channel.GroupBy(processed, topicFunc, config)
//	pub.PublishBatches(ctx, groups)
package nats

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
	"github.com/nats-io/nats.go"
)

// SubscriberConfig configures the NATS subscriber.
type SubscriberConfig struct {
	// URL is the NATS server URL (e.g., "nats://localhost:4222").
	URL string

	// Subject is the NATS subject to subscribe to.
	// Supports wildcards: "*" (single token), ">" (multiple tokens).
	// Examples: "orders.created", "orders.*", "orders.>"
	Subject string

	// Queue is the optional queue group name for load balancing.
	// When set, only one subscriber in the queue group receives each message.
	Queue string

	// BufferSize is the channel buffer size for received messages.
	// Default is 256.
	BufferSize int

	// ConnectTimeout is the timeout for initial connection.
	// Default is 5 seconds.
	ConnectTimeout time.Duration

	// Logger for operational logging. If nil, uses slog.Default().
	Logger *slog.Logger
}

func (c SubscriberConfig) applyDefaults() SubscriberConfig {
	if c.BufferSize <= 0 {
		c.BufferSize = 256
	}
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 5 * time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

// Subscriber implements Subscriber[*message.Message] for NATS.
// It directly provides a channel of messages without the deprecated
// Receiver interface abstraction.
type Subscriber struct {
	config SubscriberConfig
	conn   *nats.Conn
	mu     sync.Mutex
}

// NewSubscriber creates a new NATS subscriber.
func NewSubscriber(config SubscriberConfig) *Subscriber {
	return &Subscriber{
		config: config.applyDefaults(),
	}
}

// Subscribe returns a channel of messages from the configured NATS subject.
// The channel closes when the context is canceled or an unrecoverable error occurs.
//
// This method implements the Subscriber[*Message] pattern directly,
// bypassing the deprecated Receiver interface.
func (s *Subscriber) Subscribe(ctx context.Context) <-chan *message.Message {
	out := make(chan *message.Message, s.config.BufferSize)

	go func() {
		defer close(out)

		// Connect to NATS
		conn, err := nats.Connect(
			s.config.URL,
			nats.Timeout(s.config.ConnectTimeout),
			nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
				if err != nil {
					s.config.Logger.Warn("NATS disconnected", "error", err)
				}
			}),
			nats.ReconnectHandler(func(_ *nats.Conn) {
				s.config.Logger.Info("NATS reconnected")
			}),
		)
		if err != nil {
			s.config.Logger.Error("Failed to connect to NATS", "error", err, "url", s.config.URL)
			return
		}

		s.mu.Lock()
		s.conn = conn
		s.mu.Unlock()

		defer func() {
			s.mu.Lock()
			if s.conn != nil {
				s.conn.Close()
				s.conn = nil
			}
			s.mu.Unlock()
		}()

		// Create message channel for NATS subscription
		msgCh := make(chan *nats.Msg, s.config.BufferSize)

		// Subscribe based on whether queue group is specified
		var sub *nats.Subscription
		if s.config.Queue != "" {
			sub, err = conn.QueueSubscribeSyncWithChan(s.config.Subject, s.config.Queue, msgCh)
		} else {
			sub, err = conn.SubscribeSyncWithChan(s.config.Subject, msgCh)
		}
		if err != nil {
			s.config.Logger.Error("Failed to subscribe", "error", err, "subject", s.config.Subject)
			return
		}
		defer sub.Unsubscribe()

		s.config.Logger.Info("NATS subscription started",
			"subject", s.config.Subject,
			"queue", s.config.Queue,
		)

		// Convert NATS messages to gopipe messages
		for {
			select {
			case <-ctx.Done():
				s.config.Logger.Debug("Context canceled, closing subscription")
				return

			case natsMsg, ok := <-msgCh:
				if !ok {
					s.config.Logger.Debug("NATS message channel closed")
					return
				}

				// Convert to gopipe message with NATS-specific attributes
				msg := message.New(natsMsg.Data, message.Attributes{
					// CloudEvents-style attributes
					"subject": natsMsg.Subject,
					"source":  s.config.URL,
					// NATS-specific attributes
					"nats.subject": natsMsg.Subject,
					"nats.reply":   natsMsg.Reply,
				})

				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out
}

// Close closes the NATS connection.
func (s *Subscriber) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	return nil
}

// PublisherConfig configures the NATS publisher.
type PublisherConfig struct {
	// URL is the NATS server URL.
	URL string

	// ConnectTimeout is the timeout for initial connection.
	// Default is 5 seconds.
	ConnectTimeout time.Duration

	// FlushTimeout is the timeout for flushing pending messages.
	// Default is 1 second.
	FlushTimeout time.Duration

	// Logger for operational logging.
	Logger *slog.Logger
}

func (c PublisherConfig) applyDefaults() PublisherConfig {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 5 * time.Second
	}
	if c.FlushTimeout <= 0 {
		c.FlushTimeout = time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

// Publisher publishes messages to NATS subjects.
// It works with channel.GroupBy for batching messages by subject.
type Publisher struct {
	config PublisherConfig
	conn   *nats.Conn
	mu     sync.Mutex
}

// NewPublisher creates a new NATS publisher.
func NewPublisher(config PublisherConfig) *Publisher {
	return &Publisher{
		config: config.applyDefaults(),
	}
}

// Connect establishes the NATS connection.
func (p *Publisher) Connect(ctx context.Context) error {
	conn, err := nats.Connect(
		p.config.URL,
		nats.Timeout(p.config.ConnectTimeout),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	p.mu.Lock()
	p.conn = conn
	p.mu.Unlock()

	return nil
}

// Publish publishes a single message to the specified subject.
func (p *Publisher) Publish(ctx context.Context, subject string, msg *message.Message) error {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected to NATS")
	}

	if err := conn.Publish(subject, msg.Data); err != nil {
		return fmt.Errorf("failed to publish to %s: %w", subject, err)
	}

	return nil
}

// PublishBatch publishes a batch of messages to the specified subject.
func (p *Publisher) PublishBatch(ctx context.Context, subject string, msgs []*message.Message) error {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected to NATS")
	}

	for _, msg := range msgs {
		if err := conn.Publish(subject, msg.Data); err != nil {
			return fmt.Errorf("failed to publish to %s: %w", subject, err)
		}
	}

	// Flush to ensure messages are sent
	if err := conn.FlushTimeout(p.config.FlushTimeout); err != nil {
		return fmt.Errorf("failed to flush: %w", err)
	}

	return nil
}

// PublishBatches consumes groups from channel.GroupBy and publishes them.
// This is the recommended pattern for publishing with gopipe.
//
// Example:
//
//	groups := channel.GroupBy(msgs, topicFunc, channel.GroupByConfig{})
//	if err := pub.PublishBatches(ctx, groups); err != nil {
//	    log.Fatal(err)
//	}
func (p *Publisher) PublishBatches(ctx context.Context, groups <-chan channel.Group[string, *message.Message]) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case group, ok := <-groups:
			if !ok {
				return nil // Channel closed, done
			}

			if err := p.PublishBatch(ctx, group.Key, group.Items); err != nil {
				p.config.Logger.Error("Failed to publish batch",
					"subject", group.Key,
					"count", len(group.Items),
					"error", err,
				)
				// Continue processing other groups
				continue
			}

			p.config.Logger.Debug("Published batch",
				"subject", group.Key,
				"count", len(group.Items),
			)
		}
	}
}

// Close closes the NATS connection.
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}
	return nil
}

// TopicFromAttribute returns a function that extracts the topic from message attributes.
// This is useful with channel.GroupBy for routing messages to different subjects.
//
// Example:
//
//	groups := channel.GroupBy(msgs, nats.TopicFromAttribute("destination"), config)
func TopicFromAttribute(attr string) func(*message.Message) string {
	return func(msg *message.Message) string {
		if v, ok := msg.Attributes[attr].(string); ok {
			return v
		}
		return "default"
	}
}

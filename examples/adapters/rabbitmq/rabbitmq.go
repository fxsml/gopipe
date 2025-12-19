// Package rabbitmq provides a RabbitMQ adapter for gopipe message streaming.
//
// This adapter demonstrates the Subscriber[*Message] pattern without using
// the deprecated Sender/Receiver interfaces. It shows how RabbitMQ's exchange/
// queue/binding model requires broker-specific configuration that doesn't fit
// a generic "topic" abstraction.
//
// # Topic Semantics
//
// RabbitMQ has the most complex routing model:
//   - Exchanges: receive messages from publishers (direct, topic, fanout, headers)
//   - Queues: store messages for consumers
//   - Bindings: connect exchanges to queues with routing keys
//   - Routing keys: patterns for message routing ("orders.*", "orders.#")
//
// What other brokers call "topic" is actually:
//   - Exchange name (where to send)
//   - Routing key (how to route)
//   - Queue name (where to consume from)
//   - Binding pattern (what to receive)
//
// This complexity cannot be captured in a simple `topic string` parameter.
//
// # Usage
//
//	// Subscriber - directly implements Subscriber[*Message]
//	sub := rabbitmq.NewSubscriber(rabbitmq.SubscriberConfig{
//	    URL:         "amqp://guest:guest@localhost:5672/",
//	    Exchange:    "orders",           // RabbitMQ-specific
//	    ExchangeType: "topic",           // RabbitMQ-specific
//	    Queue:       "order-processor",  // RabbitMQ-specific
//	    BindingKey:  "orders.#",         // RabbitMQ-specific: routing pattern
//	})
//	msgs := sub.Subscribe(ctx)
//
//	// Publisher - uses channel.GroupBy for batching
//	pub := rabbitmq.NewPublisher(rabbitmq.PublisherConfig{
//	    URL:      "amqp://guest:guest@localhost:5672/",
//	    Exchange: "orders",
//	})
//	groups := channel.GroupBy(processed, routingKeyFunc, config)
//	pub.PublishBatches(ctx, groups)
package rabbitmq

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
	amqp "github.com/rabbitmq/amqp091-go"
)

// SubscriberConfig configures the RabbitMQ subscriber.
type SubscriberConfig struct {
	// URL is the AMQP connection URL.
	// Format: amqp://user:pass@host:port/vhost
	URL string

	// Exchange is the exchange to bind to.
	// If empty and ExchangeType is set, the exchange will be declared.
	Exchange string

	// ExchangeType is the type of exchange to declare.
	// Valid types: "direct", "topic", "fanout", "headers".
	// Default is "topic" for flexible routing.
	ExchangeType string

	// Queue is the queue name.
	// If empty, a unique queue will be created and auto-deleted.
	Queue string

	// BindingKey is the routing key pattern for the queue binding.
	// For topic exchanges: "*" matches one word, "#" matches zero or more.
	// Examples: "orders.*", "orders.#", "orders.created"
	BindingKey string

	// Durable determines if the queue survives broker restart.
	// Default is true for production use.
	Durable bool

	// AutoAck determines if messages are automatically acknowledged.
	// When false (default), you must call msg.Ack() or msg.Nack().
	AutoAck bool

	// PrefetchCount is the number of messages to prefetch.
	// Default is 10.
	PrefetchCount int

	// BufferSize is the channel buffer size.
	// Default is 256.
	BufferSize int

	// Logger for operational logging.
	Logger *slog.Logger
}

func (c SubscriberConfig) applyDefaults() SubscriberConfig {
	if c.ExchangeType == "" {
		c.ExchangeType = "topic"
	}
	if c.PrefetchCount <= 0 {
		c.PrefetchCount = 10
	}
	if c.BufferSize <= 0 {
		c.BufferSize = 256
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	// Default to durable for production safety
	// (Durable: false is explicit opt-out)
	return c
}

// Subscriber implements Subscriber[*message.Message] for RabbitMQ.
// It handles exchange declaration, queue creation, and message acknowledgment.
type Subscriber struct {
	config SubscriberConfig
	conn   *amqp.Connection
	ch     *amqp.Channel
	mu     sync.Mutex
}

// NewSubscriber creates a new RabbitMQ subscriber.
func NewSubscriber(config SubscriberConfig) *Subscriber {
	return &Subscriber{
		config: config.applyDefaults(),
	}
}

// Subscribe returns a channel of messages from the configured RabbitMQ queue.
// The channel closes when the context is canceled or an unrecoverable error occurs.
//
// This method:
//   - Declares the exchange if Exchange and ExchangeType are set
//   - Creates or uses the specified queue
//   - Binds the queue to the exchange with the binding key
//   - Starts consuming with the configured acknowledgment mode
func (s *Subscriber) Subscribe(ctx context.Context) <-chan *message.Message {
	out := make(chan *message.Message, s.config.BufferSize)

	go func() {
		defer close(out)

		// Connect to RabbitMQ
		conn, err := amqp.Dial(s.config.URL)
		if err != nil {
			s.config.Logger.Error("Failed to connect to RabbitMQ", "error", err)
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

		// Create channel
		ch, err := conn.Channel()
		if err != nil {
			s.config.Logger.Error("Failed to create channel", "error", err)
			return
		}

		s.mu.Lock()
		s.ch = ch
		s.mu.Unlock()

		defer func() {
			s.mu.Lock()
			if s.ch != nil {
				s.ch.Close()
				s.ch = nil
			}
			s.mu.Unlock()
		}()

		// Set QoS (prefetch)
		if err := ch.Qos(s.config.PrefetchCount, 0, false); err != nil {
			s.config.Logger.Error("Failed to set QoS", "error", err)
			return
		}

		// Declare exchange if configured
		if s.config.Exchange != "" && s.config.ExchangeType != "" {
			err := ch.ExchangeDeclare(
				s.config.Exchange,     // name
				s.config.ExchangeType, // type
				s.config.Durable,      // durable
				false,                 // auto-deleted
				false,                 // internal
				false,                 // no-wait
				nil,                   // arguments
			)
			if err != nil {
				s.config.Logger.Error("Failed to declare exchange",
					"exchange", s.config.Exchange,
					"type", s.config.ExchangeType,
					"error", err,
				)
				return
			}
		}

		// Declare queue
		queueName := s.config.Queue
		exclusive := false
		autoDelete := false

		if queueName == "" {
			// Anonymous queue - exclusive and auto-deleted
			exclusive = true
			autoDelete = true
		}

		q, err := ch.QueueDeclare(
			queueName,        // name (empty for server-generated name)
			s.config.Durable, // durable
			autoDelete,       // auto-delete when unused
			exclusive,        // exclusive
			false,            // no-wait
			nil,              // arguments
		)
		if err != nil {
			s.config.Logger.Error("Failed to declare queue", "error", err)
			return
		}

		// Bind queue to exchange if configured
		if s.config.Exchange != "" && s.config.BindingKey != "" {
			err := ch.QueueBind(
				q.Name,            // queue name
				s.config.BindingKey, // routing key
				s.config.Exchange, // exchange
				false,             // no-wait
				nil,               // arguments
			)
			if err != nil {
				s.config.Logger.Error("Failed to bind queue",
					"queue", q.Name,
					"exchange", s.config.Exchange,
					"binding", s.config.BindingKey,
					"error", err,
				)
				return
			}
		}

		// Start consuming
		deliveries, err := ch.Consume(
			q.Name,          // queue
			"",              // consumer tag (auto-generated)
			s.config.AutoAck, // auto-ack
			exclusive,       // exclusive
			false,           // no-local
			false,           // no-wait
			nil,             // args
		)
		if err != nil {
			s.config.Logger.Error("Failed to start consuming", "error", err)
			return
		}

		s.config.Logger.Info("RabbitMQ subscription started",
			"queue", q.Name,
			"exchange", s.config.Exchange,
			"binding", s.config.BindingKey,
		)

		// Convert RabbitMQ deliveries to gopipe messages
		for {
			select {
			case <-ctx.Done():
				s.config.Logger.Debug("Context canceled, closing subscription")
				return

			case delivery, ok := <-deliveries:
				if !ok {
					s.config.Logger.Debug("Delivery channel closed")
					return
				}

				// Create ack/nack callbacks for manual acknowledgment
				deliveryCopy := delivery // Capture for closure
				ack := func() {
					if !s.config.AutoAck {
						if err := deliveryCopy.Ack(false); err != nil {
							s.config.Logger.Error("Failed to ack message",
								"delivery_tag", deliveryCopy.DeliveryTag,
								"error", err,
							)
						}
					}
				}
				nack := func(err error) {
					if !s.config.AutoAck {
						s.config.Logger.Warn("Message nacked",
							"delivery_tag", deliveryCopy.DeliveryTag,
							"error", err,
						)
						// Requeue the message
						deliveryCopy.Nack(false, true)
					}
				}

				// Convert to gopipe message with RabbitMQ-specific attributes
				var msg *message.Message
				if s.config.AutoAck {
					msg = message.New(delivery.Body, nil)
				} else {
					msg = message.NewWithAcking(delivery.Body, nil, ack, nack)
				}

				// Set CloudEvents-style attributes
				msg.Attributes["source"] = s.config.URL
				msg.Attributes["topic"] = delivery.RoutingKey
				msg.Attributes["type"] = delivery.Type
				if delivery.MessageId != "" {
					msg.Attributes["id"] = delivery.MessageId
				}
				if !delivery.Timestamp.IsZero() {
					msg.Attributes["time"] = delivery.Timestamp
				}
				if delivery.ContentType != "" {
					msg.Attributes["datacontenttype"] = delivery.ContentType
				}

				// Set RabbitMQ-specific attributes
				msg.Attributes["rabbitmq.exchange"] = delivery.Exchange
				msg.Attributes["rabbitmq.routing_key"] = delivery.RoutingKey
				msg.Attributes["rabbitmq.delivery_tag"] = delivery.DeliveryTag
				msg.Attributes["rabbitmq.redelivered"] = delivery.Redelivered

				// Copy headers to attributes
				for k, v := range delivery.Headers {
					if str, ok := v.(string); ok {
						msg.Attributes["rabbitmq.header."+k] = str
					}
				}

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

// Close closes the RabbitMQ connection.
func (s *Subscriber) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var lastErr error
	if s.ch != nil {
		if err := s.ch.Close(); err != nil {
			lastErr = err
		}
		s.ch = nil
	}
	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			lastErr = err
		}
		s.conn = nil
	}
	return lastErr
}

// PublisherConfig configures the RabbitMQ publisher.
type PublisherConfig struct {
	// URL is the AMQP connection URL.
	URL string

	// Exchange is the default exchange to publish to.
	Exchange string

	// ExchangeType is the type of exchange to declare.
	// If set with Exchange, the exchange will be declared on connect.
	ExchangeType string

	// Durable determines if declared exchanges survive broker restart.
	Durable bool

	// Mandatory makes the server return unroutable messages.
	// When true, messages that can't be routed will be returned.
	Mandatory bool

	// Immediate is deprecated in RabbitMQ 3.0+.
	// Left here for documentation purposes.
	// Immediate bool

	// DeliveryMode controls message persistence.
	// 1 = non-persistent, 2 = persistent.
	// Default is 2 (persistent).
	DeliveryMode uint8

	// Logger for operational logging.
	Logger *slog.Logger
}

func (c PublisherConfig) applyDefaults() PublisherConfig {
	if c.DeliveryMode == 0 {
		c.DeliveryMode = 2 // Persistent
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

// Publisher publishes messages to RabbitMQ exchanges.
// In RabbitMQ, the "topic" from GroupBy maps to routing keys.
type Publisher struct {
	config PublisherConfig
	conn   *amqp.Connection
	ch     *amqp.Channel
	mu     sync.Mutex
}

// NewPublisher creates a new RabbitMQ publisher.
func NewPublisher(config PublisherConfig) *Publisher {
	return &Publisher{
		config: config.applyDefaults(),
	}
}

// Connect establishes the RabbitMQ connection.
func (p *Publisher) Connect(ctx context.Context) error {
	conn, err := amqp.Dial(p.config.URL)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create channel: %w", err)
	}

	// Declare exchange if configured
	if p.config.Exchange != "" && p.config.ExchangeType != "" {
		err := ch.ExchangeDeclare(
			p.config.Exchange,
			p.config.ExchangeType,
			p.config.Durable,
			false, // auto-delete
			false, // internal
			false, // no-wait
			nil,   // args
		)
		if err != nil {
			ch.Close()
			conn.Close()
			return fmt.Errorf("failed to declare exchange: %w", err)
		}
	}

	p.mu.Lock()
	p.conn = conn
	p.ch = ch
	p.mu.Unlock()

	return nil
}

// Publish publishes a single message with the specified routing key.
// In RabbitMQ, what other brokers call "topic" is the routing key.
func (p *Publisher) Publish(ctx context.Context, routingKey string, msg *message.Message) error {
	p.mu.Lock()
	ch := p.ch
	p.mu.Unlock()

	if ch == nil {
		return fmt.Errorf("not connected to RabbitMQ")
	}

	amqpMsg := amqp.Publishing{
		DeliveryMode: p.config.DeliveryMode,
		Timestamp:    time.Now(),
		Body:         msg.Data,
	}

	// Set message properties from attributes
	if id, ok := msg.Attributes["id"].(string); ok {
		amqpMsg.MessageId = id
	}
	if typ, ok := msg.Attributes["type"].(string); ok {
		amqpMsg.Type = typ
	}
	if ct, ok := msg.Attributes["datacontenttype"].(string); ok {
		amqpMsg.ContentType = ct
	}

	err := ch.PublishWithContext(
		ctx,
		p.config.Exchange, // exchange
		routingKey,        // routing key
		p.config.Mandatory,
		false, // immediate (deprecated)
		amqpMsg,
	)
	if err != nil {
		return fmt.Errorf("failed to publish to %s: %w", routingKey, err)
	}

	return nil
}

// PublishBatch publishes a batch of messages with the specified routing key.
func (p *Publisher) PublishBatch(ctx context.Context, routingKey string, msgs []*message.Message) error {
	for _, msg := range msgs {
		if err := p.Publish(ctx, routingKey, msg); err != nil {
			return err
		}
	}
	return nil
}

// PublishBatches consumes groups from channel.GroupBy and publishes them.
// The group key is used as the routing key.
//
// Important: In RabbitMQ, the GroupBy key maps to routing keys, not topics.
// This enables flexible routing patterns like "orders.created", "orders.updated".
func (p *Publisher) PublishBatches(ctx context.Context, groups <-chan channel.Group[string, *message.Message]) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case group, ok := <-groups:
			if !ok {
				return nil
			}

			if err := p.PublishBatch(ctx, group.Key, group.Items); err != nil {
				p.config.Logger.Error("Failed to publish batch",
					"routing_key", group.Key,
					"count", len(group.Items),
					"error", err,
				)
				continue
			}

			p.config.Logger.Debug("Published batch",
				"routing_key", group.Key,
				"count", len(group.Items),
			)
		}
	}
}

// Close closes the RabbitMQ connection.
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	if p.ch != nil {
		if err := p.ch.Close(); err != nil {
			lastErr = err
		}
		p.ch = nil
	}
	if p.conn != nil {
		if err := p.conn.Close(); err != nil {
			lastErr = err
		}
		p.conn = nil
	}
	return lastErr
}

// RoutingKeyFromAttribute returns a function that extracts the routing key from attributes.
// This is used with channel.GroupBy to route messages to different queues.
func RoutingKeyFromAttribute(attr string) func(*message.Message) string {
	return func(msg *message.Message) string {
		if v, ok := msg.Attributes[attr].(string); ok {
			return v
		}
		return "default"
	}
}

// Package kafka provides a Kafka adapter for gopipe message streaming.
//
// This adapter demonstrates the Subscriber[*Message] pattern without using
// the deprecated Sender/Receiver interfaces. It shows how Kafka's specific
// concepts (partitions, consumer groups, offsets) require broker-specific
// configuration that doesn't fit a generic "topic" abstraction.
//
// # Topic Semantics
//
// Kafka topics are fundamentally different from NATS subjects or RabbitMQ exchanges:
//   - Topics have partitions for parallel processing
//   - Consumer groups enable load balancing across consumers
//   - Each partition has offsets that must be committed
//   - Messages are ordered within a partition, not across partitions
//   - No wildcard subscriptions (must subscribe to explicit topics)
//
// This complexity is why a simple `Receive(topic string)` interface is inadequate.
//
// # Usage
//
//	// Subscriber - directly implements Subscriber[*Message]
//	sub := kafka.NewSubscriber(kafka.SubscriberConfig{
//	    Brokers:       []string{"localhost:9092"},
//	    Topics:        []string{"orders"},  // Kafka-specific: explicit topics
//	    ConsumerGroup: "order-processor",   // Kafka-specific: consumer group
//	})
//	msgs := sub.Subscribe(ctx)
//
//	// Publisher - uses channel.GroupBy for batching
//	pub := kafka.NewPublisher(kafka.PublisherConfig{
//	    Brokers: []string{"localhost:9092"},
//	})
//	groups := channel.GroupBy(processed, topicFunc, config)
//	pub.PublishBatches(ctx, groups)
package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fxsml/gopipe/channel"
	"github.com/fxsml/gopipe/message"
	"github.com/segmentio/kafka-go"
)

// SubscriberConfig configures the Kafka subscriber.
type SubscriberConfig struct {
	// Brokers is the list of Kafka broker addresses.
	Brokers []string

	// Topics is the list of topics to subscribe to.
	// Unlike NATS, Kafka doesn't support wildcards - you must specify exact topics.
	Topics []string

	// ConsumerGroup is the consumer group ID.
	// All consumers in the same group share the partitions of subscribed topics.
	// Required for production use.
	ConsumerGroup string

	// BufferSize is the channel buffer size for received messages.
	// Default is 256.
	BufferSize int

	// StartOffset controls where to start reading when no committed offset exists.
	// Use kafka.FirstOffset (-2) or kafka.LastOffset (-1).
	// Default is kafka.LastOffset (only new messages).
	StartOffset int64

	// CommitInterval is how often to auto-commit offsets.
	// Default is 1 second.
	CommitInterval time.Duration

	// MaxWait is the maximum time to wait for new messages.
	// Default is 1 second.
	MaxWait time.Duration

	// Logger for operational logging.
	Logger *slog.Logger
}

func (c SubscriberConfig) applyDefaults() SubscriberConfig {
	if c.BufferSize <= 0 {
		c.BufferSize = 256
	}
	if c.StartOffset == 0 {
		c.StartOffset = kafka.LastOffset
	}
	if c.CommitInterval <= 0 {
		c.CommitInterval = time.Second
	}
	if c.MaxWait <= 0 {
		c.MaxWait = time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

// Subscriber implements Subscriber[*message.Message] for Kafka.
// It manages consumer group membership, partition assignment, and offset commits.
type Subscriber struct {
	config SubscriberConfig
	reader *kafka.Reader
	mu     sync.Mutex
}

// NewSubscriber creates a new Kafka subscriber.
func NewSubscriber(config SubscriberConfig) *Subscriber {
	return &Subscriber{
		config: config.applyDefaults(),
	}
}

// Subscribe returns a channel of messages from the configured Kafka topics.
// The channel closes when the context is canceled or an unrecoverable error occurs.
//
// Important Kafka-specific behaviors:
//   - Partitions are automatically assigned within the consumer group
//   - Offsets are committed automatically at CommitInterval
//   - Message ordering is guaranteed only within a partition
func (s *Subscriber) Subscribe(ctx context.Context) <-chan *message.Message {
	out := make(chan *message.Message, s.config.BufferSize)

	go func() {
		defer close(out)

		// Create Kafka reader (consumer)
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:        s.config.Brokers,
			GroupID:        s.config.ConsumerGroup,
			GroupTopics:    s.config.Topics,
			StartOffset:    s.config.StartOffset,
			CommitInterval: s.config.CommitInterval,
			MaxWait:        s.config.MaxWait,
		})

		s.mu.Lock()
		s.reader = reader
		s.mu.Unlock()

		defer func() {
			s.mu.Lock()
			if s.reader != nil {
				s.reader.Close()
				s.reader = nil
			}
			s.mu.Unlock()
		}()

		s.config.Logger.Info("Kafka subscription started",
			"topics", s.config.Topics,
			"group", s.config.ConsumerGroup,
			"brokers", s.config.Brokers,
		)

		// Read messages from Kafka
		for {
			kafkaMsg, err := reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					s.config.Logger.Debug("Context canceled, closing subscription")
					return
				}
				s.config.Logger.Error("Failed to fetch message", "error", err)
				continue
			}

			// Create ack/nack callbacks for manual acknowledgment
			// This enables at-least-once delivery with explicit commit
			msgCopy := kafkaMsg // Capture for closure
			ack := func() {
				if err := reader.CommitMessages(ctx, msgCopy); err != nil {
					s.config.Logger.Error("Failed to commit offset",
						"topic", msgCopy.Topic,
						"partition", msgCopy.Partition,
						"offset", msgCopy.Offset,
						"error", err,
					)
				}
			}
			nack := func(err error) {
				s.config.Logger.Warn("Message nacked, will be redelivered",
					"topic", msgCopy.Topic,
					"partition", msgCopy.Partition,
					"offset", msgCopy.Offset,
					"error", err,
				)
				// Don't commit - message will be redelivered
			}

			// Convert to gopipe message with Kafka-specific attributes
			msg := message.NewWithAcking(kafkaMsg.Value, message.Attributes{
				// CloudEvents-style attributes
				"source": fmt.Sprintf("kafka://%s", s.config.Brokers[0]),
				"topic":  kafkaMsg.Topic,
				"key":    string(kafkaMsg.Key),
				"time":   kafkaMsg.Time,
				// Kafka-specific attributes
				"kafka.topic":     kafkaMsg.Topic,
				"kafka.partition": kafkaMsg.Partition,
				"kafka.offset":    kafkaMsg.Offset,
				"kafka.key":       string(kafkaMsg.Key),
			}, ack, nack)

			// Copy headers to attributes
			for _, h := range kafkaMsg.Headers {
				msg.Attributes["kafka.header."+h.Key] = string(h.Value)
			}

			select {
			case out <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// Close closes the Kafka consumer.
func (s *Subscriber) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.reader != nil {
		err := s.reader.Close()
		s.reader = nil
		return err
	}
	return nil
}

// PublisherConfig configures the Kafka publisher.
type PublisherConfig struct {
	// Brokers is the list of Kafka broker addresses.
	Brokers []string

	// BatchSize is the number of messages to batch before sending.
	// Default is 100.
	BatchSize int

	// BatchTimeout is the maximum time to wait for a full batch.
	// Default is 1 second.
	BatchTimeout time.Duration

	// RequiredAcks controls producer acknowledgment.
	// Use kafka.RequireNone (0), kafka.RequireOne (1), or kafka.RequireAll (-1).
	// Default is kafka.RequireAll for durability.
	RequiredAcks kafka.RequiredAcks

	// Async enables asynchronous publishing for higher throughput.
	// When true, messages are batched and sent in the background.
	// Default is false (synchronous).
	Async bool

	// Logger for operational logging.
	Logger *slog.Logger
}

func (c PublisherConfig) applyDefaults() PublisherConfig {
	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}
	if c.BatchTimeout <= 0 {
		c.BatchTimeout = time.Second
	}
	if c.RequiredAcks == 0 {
		c.RequiredAcks = kafka.RequireAll
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return c
}

// Publisher publishes messages to Kafka topics.
// It uses Kafka's built-in batching for efficiency.
type Publisher struct {
	config  PublisherConfig
	writers map[string]*kafka.Writer
	mu      sync.Mutex
}

// NewPublisher creates a new Kafka publisher.
func NewPublisher(config PublisherConfig) *Publisher {
	return &Publisher{
		config:  config.applyDefaults(),
		writers: make(map[string]*kafka.Writer),
	}
}

// getWriter returns or creates a writer for the given topic.
func (p *Publisher) getWriter(topic string) *kafka.Writer {
	p.mu.Lock()
	defer p.mu.Unlock()

	if w, ok := p.writers[topic]; ok {
		return w
	}

	w := &kafka.Writer{
		Addr:         kafka.TCP(p.config.Brokers...),
		Topic:        topic,
		BatchSize:    p.config.BatchSize,
		BatchTimeout: p.config.BatchTimeout,
		RequiredAcks: p.config.RequiredAcks,
		Async:        p.config.Async,
	}
	p.writers[topic] = w
	return w
}

// Publish publishes a single message to the specified topic.
func (p *Publisher) Publish(ctx context.Context, topic string, msg *message.Message) error {
	writer := p.getWriter(topic)

	kafkaMsg := kafka.Message{
		Value: msg.Data,
	}

	// Use message key if provided
	if key, ok := msg.Attributes["key"].(string); ok {
		kafkaMsg.Key = []byte(key)
	}

	// Copy headers from attributes
	for k, v := range msg.Attributes {
		if str, ok := v.(string); ok && len(k) > 0 && k[0] != '_' {
			kafkaMsg.Headers = append(kafkaMsg.Headers, kafka.Header{
				Key:   k,
				Value: []byte(str),
			})
		}
	}

	if err := writer.WriteMessages(ctx, kafkaMsg); err != nil {
		return fmt.Errorf("failed to publish to %s: %w", topic, err)
	}

	return nil
}

// PublishBatch publishes a batch of messages to the specified topic.
func (p *Publisher) PublishBatch(ctx context.Context, topic string, msgs []*message.Message) error {
	writer := p.getWriter(topic)

	kafkaMsgs := make([]kafka.Message, len(msgs))
	for i, msg := range msgs {
		kafkaMsgs[i] = kafka.Message{
			Value: msg.Data,
		}

		// Use message key if provided
		if key, ok := msg.Attributes["key"].(string); ok {
			kafkaMsgs[i].Key = []byte(key)
		}
	}

	if err := writer.WriteMessages(ctx, kafkaMsgs...); err != nil {
		return fmt.Errorf("failed to publish batch to %s: %w", topic, err)
	}

	return nil
}

// PublishBatches consumes groups from channel.GroupBy and publishes them.
// This is the recommended pattern for publishing with gopipe.
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
					"topic", group.Key,
					"count", len(group.Items),
					"error", err,
				)
				continue
			}

			p.config.Logger.Debug("Published batch",
				"topic", group.Key,
				"count", len(group.Items),
			)
		}
	}
}

// Close closes all Kafka writers.
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for topic, w := range p.writers {
		if err := w.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close writer for %s: %w", topic, err)
		}
	}
	p.writers = make(map[string]*kafka.Writer)
	return lastErr
}

// TopicFromAttribute returns a function that extracts the topic from message attributes.
func TopicFromAttribute(attr string) func(*message.Message) string {
	return func(msg *message.Message) string {
		if v, ok := msg.Attributes[attr].(string); ok {
			return v
		}
		return "default"
	}
}

// KeyFromAttribute returns a function that extracts the partition key from attributes.
// This can be used to ensure messages with the same key go to the same partition.
func KeyFromAttribute(attr string) func(*message.Message) string {
	return func(msg *message.Message) string {
		if v, ok := msg.Attributes[attr].(string); ok {
			return v
		}
		return ""
	}
}

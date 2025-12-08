package broker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/fxsml/gopipe/message"
)

var (
	// ErrWriterClosed is returned when writing to a closed writer.
	ErrWriterClosed = errors.New("writer is closed")
	// ErrReaderClosed is returned when reading from a closed reader.
	ErrReaderClosed = errors.New("reader is closed")
	// ErrMarshalFailed is returned when message marshalling fails.
	ErrMarshalFailed = errors.New("marshal failed")
	// ErrUnmarshalFailed is returned when message unmarshalling fails.
	ErrUnmarshalFailed = errors.New("unmarshal failed")
)

// Envelope wraps a message for wire transmission.
// It includes the topic, properties, and JSON-encoded payload.
type Envelope struct {
	Topic      string         `json:"topic"`
	Properties map[string]any `json:"properties,omitempty"`
	Payload    json.RawMessage `json:"payload"`
	Timestamp  time.Time      `json:"timestamp"`
}

// Marshaler handles message serialization.
type Marshaler interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// JSONMarshaler is the default JSON-based marshaler.
type JSONMarshaler struct{}

func (JSONMarshaler) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (JSONMarshaler) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// IOConfig configures IO-based sender/receiver.
type IOConfig struct {
	// SendTimeout is the maximum duration for a write operation.
	SendTimeout time.Duration

	// ReceiveTimeout is the maximum duration for a read operation.
	ReceiveTimeout time.Duration

	// Marshaler for message serialization. Defaults to JSONMarshaler.
	Marshaler Marshaler

	// BufferSize for the reader. Defaults to 64KB.
	BufferSize int
}

func (c *IOConfig) defaults() IOConfig {
	cfg := *c
	if cfg.Marshaler == nil {
		cfg.Marshaler = JSONMarshaler{}
	}
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 64 * 1024
	}
	return cfg
}

// IOSender writes messages to an io.Writer as JSON Lines (JSONL).
type IOSender[T any] struct {
	config    IOConfig
	mu        sync.Mutex
	writer    io.Writer
	encoder   *json.Encoder
	closed    bool
	marshaler Marshaler
}

// NewIOSender creates a sender that writes messages to the given writer.
// Messages are encoded as JSON Lines (JSONL) - one JSON object per line.
func NewIOSender[T any](w io.Writer, config IOConfig) *IOSender[T] {
	cfg := config.defaults()
	return &IOSender[T]{
		config:    cfg,
		writer:    w,
		encoder:   json.NewEncoder(w),
		marshaler: cfg.Marshaler,
	}
}

// Send writes messages to the underlying writer.
func (s *IOSender[T]) Send(ctx context.Context, topic string, msgs ...*message.Message[T]) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrWriterClosed
	}

	// Apply timeout if configured
	if s.config.SendTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.config.SendTimeout)
		defer cancel()
	}

	for _, msg := range msgs {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return ErrSendTimeout
			}
			return ctx.Err()
		default:
		}

		// Marshal the payload
		payloadBytes, err := s.marshaler.Marshal(msg.Payload())
		if err != nil {
			return errors.Join(ErrMarshalFailed, err)
		}

		// Extract properties
		props := make(map[string]any)
		msg.Properties().Range(func(key string, value any) bool {
			props[key] = value
			return true
		})

		env := Envelope{
			Topic:      topic,
			Properties: props,
			Payload:    payloadBytes,
			Timestamp:  time.Now(),
		}

		if err := s.encoder.Encode(env); err != nil {
			return errors.Join(ErrMarshalFailed, err)
		}
	}

	return nil
}

// Close marks the sender as closed. It does not close the underlying writer.
func (s *IOSender[T]) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// IOReceiver reads messages from an io.Reader.
type IOReceiver[T any] struct {
	config    IOConfig
	mu        sync.Mutex
	reader    *bufio.Reader
	closed    bool
	marshaler Marshaler
}

// NewIOReceiver creates a receiver that reads messages from the given reader.
// Messages are decoded from JSON Lines (JSONL) - one JSON object per line.
func NewIOReceiver[T any](r io.Reader, config IOConfig) *IOReceiver[T] {
	cfg := config.defaults()
	return &IOReceiver[T]{
		config:    cfg,
		reader:    bufio.NewReaderSize(r, cfg.BufferSize),
		marshaler: cfg.Marshaler,
	}
}

// Receive returns a channel that emits messages from the specified topic.
// If topic is empty, all messages are emitted regardless of topic.
func (r *IOReceiver[T]) Receive(ctx context.Context, topic string) <-chan *message.Message[T] {
	out := make(chan *message.Message[T])

	go func() {
		defer close(out)

		var matcher *TopicMatcher
		if topic != "" {
			matcher = NewTopicMatcher(topic)
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			msg, msgTopic, err := r.readOne(ctx)
			if err != nil {
				if err == io.EOF || errors.Is(err, ErrReaderClosed) {
					return
				}
				// Skip malformed messages
				continue
			}

			// Filter by topic if specified
			if matcher != nil && !matcher.Matches(msgTopic) {
				continue
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

// readOne reads a single message from the reader.
func (r *IOReceiver[T]) readOne(ctx context.Context) (*message.Message[T], string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, "", ErrReaderClosed
	}

	// Read a line (JSONL format)
	line, err := r.reader.ReadBytes('\n')
	if err != nil {
		return nil, "", err
	}

	// Decode envelope
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return nil, "", errors.Join(ErrUnmarshalFailed, err)
	}

	// Unmarshal payload
	var payload T
	if err := r.marshaler.Unmarshal(env.Payload, &payload); err != nil {
		return nil, "", errors.Join(ErrUnmarshalFailed, err)
	}

	// Build message with properties
	opts := []message.Option[T]{}
	for key, value := range env.Properties {
		opts = append(opts, message.WithProperty[T](key, value))
	}

	msg := message.New(payload, opts...)

	return msg, env.Topic, nil
}

// Close marks the receiver as closed. It does not close the underlying reader.
func (r *IOReceiver[T]) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

// IOBroker combines IOSender and IOReceiver for bidirectional streaming.
type IOBroker[T any] struct {
	sender   *IOSender[T]
	receiver *IOReceiver[T]
}

// NewIOBroker creates a broker that reads from r and writes to w.
// This is useful for pipe-based IPC or file-based messaging.
func NewIOBroker[T any](r io.Reader, w io.Writer, config IOConfig) *IOBroker[T] {
	return &IOBroker[T]{
		sender:   NewIOSender[T](w, config),
		receiver: NewIOReceiver[T](r, config),
	}
}

// Send writes messages to the underlying writer.
func (b *IOBroker[T]) Send(ctx context.Context, topic string, msgs ...*message.Message[T]) error {
	return b.sender.Send(ctx, topic, msgs...)
}

// Receive returns a channel that emits messages from the specified topic.
func (b *IOBroker[T]) Receive(ctx context.Context, topic string) <-chan *message.Message[T] {
	return b.receiver.Receive(ctx, topic)
}

// Close closes both sender and receiver.
func (b *IOBroker[T]) Close() error {
	err1 := b.sender.Close()
	err2 := b.receiver.Close()
	return errors.Join(err1, err2)
}

// Sender returns the underlying IOSender.
func (b *IOBroker[T]) Sender() *IOSender[T] {
	return b.sender
}

// Receiver returns the underlying IOReceiver.
func (b *IOBroker[T]) Receiver() *IOReceiver[T] {
	return b.receiver
}

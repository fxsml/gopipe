package pubsub

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
	Topic      string          `json:"topic"`
	Properties map[string]any  `json:"properties,omitempty"`
	Payload    json.RawMessage `json:"payload"`
	Timestamp  time.Time       `json:"timestamp"`
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

func (c IOConfig) defaults() IOConfig {
	cfg := c
	if cfg.Marshaler == nil {
		cfg.Marshaler = JSONMarshaler{}
	}
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 64 * 1024
	}
	return cfg
}

// ioSender writes messages to an io.Writer as JSON Lines (JSONL).
type ioSender struct {
	config    IOConfig
	mu        sync.Mutex
	writer    io.Writer
	encoder   *json.Encoder
	closed    bool
	marshaler Marshaler
}

// NewIOSender creates a sender that writes messages to the given writer.
// Messages are encoded as JSON Lines (JSONL) - one JSON object per line.
func NewIOSender(w io.Writer, config IOConfig) message.Sender {
	cfg := config.defaults()
	return &ioSender{
		config:    cfg,
		writer:    w,
		encoder:   json.NewEncoder(w),
		marshaler: cfg.Marshaler,
	}
}

// Send writes messages to the underlying writer.
func (s *ioSender) Send(ctx context.Context, topic string, msgs []*message.Message) error {
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

		// Extract properties
		props := make(map[string]any)
		for k, v := range msg.Properties {
			props[k] = v
		}

		env := Envelope{
			Topic:      topic,
			Properties: props,
			Payload:    msg.Payload,
			Timestamp:  time.Now(),
		}

		if err := s.encoder.Encode(env); err != nil {
			return errors.Join(ErrMarshalFailed, err)
		}
	}

	return nil
}

// Close marks the sender as closed. It does not close the underlying writer.
func (s *ioSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// ioReceiver reads messages from an io.Reader.
type ioReceiver struct {
	config    IOConfig
	mu        sync.Mutex
	reader    *bufio.Reader
	closed    bool
	marshaler Marshaler
}

// NewIOReceiver creates a receiver that reads messages from the given reader.
// Messages are decoded from JSON Lines (JSONL) - one JSON object per line.
func NewIOReceiver(r io.Reader, config IOConfig) message.Receiver {
	cfg := config.defaults()
	return &ioReceiver{
		config:    cfg,
		reader:    bufio.NewReaderSize(r, cfg.BufferSize),
		marshaler: cfg.Marshaler,
	}
}

// Receive reads messages from the reader that match the specified topic.
// If topic is empty, all messages are returned.
func (r *ioReceiver) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
	var matcher *TopicMatcher
	if topic != "" {
		matcher = NewTopicMatcher(topic)
	}

	var result []*message.Message
	timeout := r.config.ReceiveTimeout
	if timeout == 0 {
		timeout = 100 * time.Millisecond
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-timer.C:
			return result, nil
		default:
		}

		msg, msgTopic, err := r.readOne(ctx)
		if err != nil {
			if err == io.EOF || errors.Is(err, ErrReaderClosed) {
				return result, nil
			}
			// Skip malformed messages
			continue
		}

		// Filter by topic if specified
		if matcher != nil && !matcher.Matches(msgTopic) {
			continue
		}

		result = append(result, msg)
		timer.Reset(10 * time.Millisecond)
	}
}

// readOne reads a single message from the reader.
func (r *ioReceiver) readOne(ctx context.Context) (*message.Message, string, error) {
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

	// Build message with properties
	props := message.Properties{}
	for key, value := range env.Properties {
		if strVal, ok := value.(string); ok {
			props[key] = strVal
		}
	}

	// Convert json.RawMessage to []byte explicitly
	payload := []byte(env.Payload)
	msg := message.New(payload, props)

	return msg, env.Topic, nil
}

// Close marks the receiver as closed. It does not close the underlying reader.
func (r *ioReceiver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

// ioBroker combines Sender and Receiver for bidirectional streaming.
type ioBroker struct {
	sender   *ioSender
	receiver *ioReceiver
}

// NewIOBroker creates a broker that reads from r and writes to w.
// This is useful for pipe-based IPC or file-based messaging.
func NewIOBroker(r io.Reader, w io.Writer, config IOConfig) message.Broker {
	cfg := config.defaults()
	return &ioBroker{
		sender:   NewIOSender(w, cfg).(*ioSender),
		receiver: NewIOReceiver(r, cfg).(*ioReceiver),
	}
}

// Send writes messages to the underlying writer.
func (b *ioBroker) Send(ctx context.Context, topic string, msgs []*message.Message) error {
	return b.sender.Send(ctx, topic, msgs)
}

// Receive reads messages from the underlying reader.
func (b *ioBroker) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
	return b.receiver.Receive(ctx, topic)
}

// Close closes both sender and receiver.
func (b *ioBroker) Close() error {
	err1 := b.sender.Close()
	err2 := b.receiver.Close()
	return errors.Join(err1, err2)
}

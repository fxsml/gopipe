package broker

// IO Broker - Debug and Management Tool
//
// The IO broker is designed for debugging, logging, and bridging scenarios - not as
// a production message broker. It serializes messages to JSONL (one CloudEvent per line)
// for human-readable output and interoperability with other tools.
//
// Primary use cases:
//   - Debug logging: Subscribe from a real broker, write to file for inspection
//   - Replay testing: Read from file, publish to a real broker for testing
//   - Pipe-based IPC: Simple inter-process communication via stdin/stdout
//   - Format conversion: Bridge between file-based and broker-based systems
//
// Topic handling:
//   - Send: Writes ALL messages regardless of topic. The topic is serialized
//     in the CloudEvent "topic" extension for use by downstream consumers.
//   - Receive: Empty topic ("") returns all messages. Non-empty topic filters
//     messages by exact match on the serialized topic extension.
//
// Example: Debug logging
//
//	// Subscribe from NATS, write to file for debugging
//	natsReceiver := NewNATSReceiver(...)
//	fileWriter := NewIOSender(file, IOConfig{})
//	msgs, _ := natsReceiver.Receive(ctx, "orders.*")
//	fileWriter.Send(ctx, "orders.debug", msgs) // topic preserved in output
//
// Example: Replay testing
//
//	// Read from file, publish to broker for replay
//	fileReader := NewIOReceiver(file, IOConfig{})
//	broker := NewNATSSender(...)
//	msgs, _ := fileReader.Receive(ctx, "") // empty = read all
//	for _, msg := range msgs {
//	    topic, _ := msg.Attributes.Topic()
//	    broker.Send(ctx, topic, []*message.Message{msg})
//	}

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/fxsml/gopipe/message"
	"github.com/fxsml/gopipe/message/cloudevents"
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

// IOConfig configures IO-based sender/receiver for debugging and bridging.
type IOConfig struct {
	// SendTimeout is the maximum duration for a write operation.
	SendTimeout time.Duration

	// ReceiveTimeout is the maximum duration for a read operation.
	// When reading from a file, this controls how long to wait for more data.
	ReceiveTimeout time.Duration

	// Marshaler for message serialization. Defaults to JSONMarshaler.
	Marshaler Marshaler

	// BufferSize for the reader. Defaults to 64KB.
	BufferSize int

	// Source is the CloudEvents source URI for outgoing messages.
	// Defaults to "gopipe://io".
	Source string
}

func (c IOConfig) defaults() IOConfig {
	cfg := c
	if cfg.Marshaler == nil {
		cfg.Marshaler = JSONMarshaler{}
	}
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 64 * 1024
	}
	if cfg.Source == "" {
		cfg.Source = "gopipe://io"
	}
	return cfg
}

// IOSender writes messages to an io.Writer as JSON Lines (JSONL).
type IOSender struct {
	config    IOConfig
	mu        sync.Mutex
	writer    io.Writer
	encoder   *json.Encoder
	closed    bool
	marshaler Marshaler
}

// NewIOSender creates a sender that writes messages to the given writer.
//
// All messages are written regardless of topic. The topic is preserved in the
// CloudEvent "topic" extension so it can be used when reading messages back.
// Messages are encoded as JSON Lines (JSONL) - one CloudEvent per line.
//
// Use for: debug logging, recording messages to file, pipe-based output.
func NewIOSender(w io.Writer, config IOConfig) *IOSender {
	cfg := config.defaults()
	return &IOSender{
		config:    cfg,
		writer:    w,
		encoder:   json.NewEncoder(w),
		marshaler: cfg.Marshaler,
	}
}

// Send writes all messages to the underlying writer in CloudEvents JSON format.
// The topic is stored in the CloudEvent "topic" extension for later filtering.
func (s *IOSender) Send(ctx context.Context, topic string, msgs []*message.Message) error {
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

		// Convert message to CloudEvent
		event := cloudevents.FromMessage(msg, topic, s.config.Source)

		// Encode as JSON line
		if err := s.encoder.Encode(event); err != nil {
			return errors.Join(ErrMarshalFailed, err)
		}
	}

	return nil
}

// Close marks the sender as closed. It does not close the underlying writer.
func (s *IOSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// IOReceiver reads messages from an io.Reader.
type IOReceiver struct {
	config    IOConfig
	mu        sync.Mutex
	reader    *bufio.Reader
	closed    bool
	marshaler Marshaler
}

// NewIOReceiver creates a receiver that reads messages from the given reader.
//
// Topic filtering:
//   - Empty topic (""): Returns all messages from the stream
//   - Non-empty topic: Filters by exact match on the "topic" extension
//
// Messages are decoded from JSON Lines (JSONL) - one CloudEvent per line.
//
// Use for: replay testing, reading debug logs, pipe-based input.
func NewIOReceiver(r io.Reader, config IOConfig) *IOReceiver {
	cfg := config.defaults()
	return &IOReceiver{
		config:    cfg,
		reader:    bufio.NewReaderSize(r, cfg.BufferSize),
		marshaler: cfg.Marshaler,
	}
}

// Receive reads messages from the reader. Empty topic returns all messages;
// non-empty topic filters by exact match on the serialized topic extension.
func (r *IOReceiver) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
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

		// Filter by topic if specified (exact match only)
		if topic != "" && msgTopic != topic {
			continue
		}

		result = append(result, msg)
		timer.Reset(10 * time.Millisecond)
	}
}

// readOne reads a single CloudEvent from the reader.
func (r *IOReceiver) readOne(ctx context.Context) (*message.Message, string, error) {
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

	// Decode CloudEvent
	var event cloudevents.Event
	if err := json.Unmarshal(line, &event); err != nil {
		return nil, "", errors.Join(ErrUnmarshalFailed, err)
	}

	// Convert CloudEvent to message
	msg, topic, err := cloudevents.ToMessage(&event)
	if err != nil {
		return nil, "", errors.Join(ErrUnmarshalFailed, err)
	}

	return msg, topic, nil
}

// Close marks the receiver as closed. It does not close the underlying reader.
func (r *IOReceiver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

// IOBroker combines Sender and Receiver for bidirectional IO streaming.
// Primarily useful for pipe-based IPC (stdin/stdout) or testing scenarios.
type IOBroker struct {
	sender   *IOSender
	receiver *IOReceiver
}

// Compile-time interface assertions
var (
	_ message.Sender   = (*IOSender)(nil)
	_ message.Receiver = (*IOReceiver)(nil)
	_ message.Sender   = (*IOBroker)(nil)
	_ message.Receiver = (*IOBroker)(nil)
)

// NewIOBroker creates a broker that reads from r and writes to w.
//
// Primary use cases:
//   - Pipe-based IPC: Use with os.Stdin/os.Stdout for inter-process messaging
//   - Testing: Use with bytes.Buffer for unit tests
//
// Note: For file-based scenarios, prefer separate NewIOSender and NewIOReceiver
// as files are typically written then read, not both simultaneously.
func NewIOBroker(r io.Reader, w io.Writer, config IOConfig) *IOBroker {
	cfg := config.defaults()
	return &IOBroker{
		sender:   NewIOSender(w, cfg),
		receiver: NewIOReceiver(r, cfg),
	}
}

// Send writes messages to the underlying writer.
func (b *IOBroker) Send(ctx context.Context, topic string, msgs []*message.Message) error {
	return b.sender.Send(ctx, topic, msgs)
}

// Receive reads messages from the underlying reader.
func (b *IOBroker) Receive(ctx context.Context, topic string) ([]*message.Message, error) {
	return b.receiver.Receive(ctx, topic)
}

// Close closes both sender and receiver.
func (b *IOBroker) Close() error {
	err1 := b.sender.Close()
	err2 := b.receiver.Close()
	return errors.Join(err1, err2)
}

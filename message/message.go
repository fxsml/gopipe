package message

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"sync"
)

type ackType byte

const (
	ackTypeNone ackType = iota
	ackTypeAck
	ackTypeNack
)

// Acking coordinates acknowledgment across one or more messages.
// When expectedAckCount messages call Ack(), the ack callback is invoked.
// If any message calls Nack(), the nack callback is invoked immediately.
// Acking is thread-safe and can be shared between multiple messages.
type Acking struct {
	mu               sync.Mutex
	ack              func()
	nack             func(error)
	ackType          ackType
	ackCount         int
	expectedAckCount int
}

// NewAcking creates an Acking that requires expectedCount acknowledgments
// before invoking the ack callback. The nack callback is invoked immediately
// on the first Nack() call from any message sharing this Acking.
//
// Both ack and nack must be non-nil. Returns nil if expectedCount <= 0
// or if either callback is nil.
func NewAcking(ack func(), nack func(error), expectedCount int) *Acking {
	if expectedCount <= 0 || ack == nil || nack == nil {
		return nil
	}
	return &Acking{
		ack:              ack,
		nack:             nack,
		expectedAckCount: expectedCount,
	}
}

// Attributes is a map of message context attributes per CloudEvents spec.
// CloudEvents defines attributes as the metadata that describes the event.
type Attributes map[string]any

// TypedMessage wraps a typed data payload with attributes and acknowledgment callbacks.
// This is the base generic type for all message variants.
// Data and Attributes are public for direct access.
// Ack/Nack operations are mutually exclusive and idempotent.
type TypedMessage[T any] struct {
	// Data is the event payload per CloudEvents spec.
	Data T

	// Attributes contains the context attributes per CloudEvents spec.
	Attributes Attributes

	a *Acking
}

// Message is the internal message type used by handlers and middleware.
// Data holds any typed payload after unmarshaling from RawMessage.
type Message = TypedMessage[any]

// RawMessage is the broker boundary message type with serialized []byte data.
// Used for pub/sub integrations (Kafka, RabbitMQ, NATS, etc.).
type RawMessage = TypedMessage[[]byte]

// New creates a new typed message with the given data and attributes.
// Pass nil for attributes if no attributes are needed.
// For messages requiring acknowledgment, use NewWithAcking instead.
func New[T any](data T, attrs Attributes) *TypedMessage[T] {
	if attrs == nil {
		attrs = make(Attributes)
	}
	return &TypedMessage[T]{
		Data:       data,
		Attributes: attrs,
		a:          nil,
	}
}

// NewWithAcking creates a new typed message with the given data, attributes, and acknowledgment callbacks.
// Both ack and nack callbacks must be provided (not nil).
// For sharing acking across multiple messages, use NewAcking and NewWithSharedAcking instead.
func NewWithAcking[T any](data T, attrs Attributes, ack func(), nack func(error)) *TypedMessage[T] {
	if attrs == nil {
		attrs = make(Attributes)
	}
	return &TypedMessage[T]{
		Data:       data,
		Attributes: attrs,
		a:          NewAcking(ack, nack, 1),
	}
}

// NewWithSharedAcking creates a new typed message with shared acknowledgment coordination.
// Multiple messages can share the same Acking to coordinate acknowledgment (e.g., batch processing).
// Pass nil for acking to create a message without acknowledgment support.
func NewWithSharedAcking[T any](data T, attrs Attributes, acking *Acking) *TypedMessage[T] {
	if attrs == nil {
		attrs = make(Attributes)
	}
	return &TypedMessage[T]{
		Data:       data,
		Attributes: attrs,
		a:          acking,
	}
}

// Ack acknowledges successful processing of the message.
// Returns true if acknowledgment succeeded or was already performed.
// Returns false if no ack callback was provided, if the message was already nacked, or if expectedAckCount <= 0.
// The ack callback is invoked at most once when all stages have acked. Thread-safe.
func (m *TypedMessage[T]) Ack() bool {
	if m.a == nil {
		return false
	}
	m.a.mu.Lock()
	defer m.a.mu.Unlock()

	switch m.a.ackType {
	case ackTypeAck:
		return true
	case ackTypeNack:
		return false
	default:
	}

	m.a.ackCount++
	if m.a.ackCount < m.a.expectedAckCount {
		return true
	}

	m.a.ack()
	m.a.ackType = ackTypeAck
	return true
}

// Nack negatively acknowledges the message due to a processing error.
// Returns true if negative acknowledgment succeeded or was already performed.
// Returns false if no nack callback was provided or if the message was already acked.
// The nack callback is invoked immediately with the first error, permanently blocking all further acks.
// Thread-safe.
func (m *TypedMessage[T]) Nack(err error) bool {
	if m.a == nil {
		return false
	}
	m.a.mu.Lock()
	defer m.a.mu.Unlock()

	switch m.a.ackType {
	case ackTypeAck:
		return false
	case ackTypeNack:
		return true
	default:
	}

	m.a.nack(err)
	m.a.ackType = ackTypeNack
	return true
}

// Copy creates a new message with different data while preserving
// attributes and acknowledgment callbacks.
func Copy[In, Out any](msg *TypedMessage[In], data Out) *TypedMessage[Out] {
	return &TypedMessage[Out]{
		Data:       data,
		Attributes: msg.Attributes,
		a:          msg.a,
	}
}

// cloudEvent returns the message as a CloudEvents structured map.
// Injects specversion "1.0" if not present in attributes.
func (m *TypedMessage[T]) cloudEvent() map[string]any {
	ce := make(map[string]any, len(m.Attributes)+2)
	maps.Copy(ce, m.Attributes)
	if _, ok := ce["specversion"]; !ok {
		ce["specversion"] = "1.0"
	}

	// For []byte data, embed as raw JSON if valid
	if data, ok := any(m.Data).([]byte); ok {
		if json.Valid(data) {
			ce["data"] = json.RawMessage(data)
		} else {
			ce["data"] = data
		}
	} else {
		ce["data"] = m.Data
	}
	return ce
}

// String returns the message in CloudEvents structured JSON format.
// Injects specversion "1.0" if not present in attributes.
func (m *TypedMessage[T]) String() string {
	b, err := json.Marshal(m.cloudEvent())
	if err != nil {
		return fmt.Sprintf("gopipe error: marshaling CloudEvents message: %v", err)
	}
	return string(b)
}

// WriteTo writes the message in CloudEvents structured JSON format to w.
// Implements io.WriterTo for direct streaming to http.ResponseWriter, files, etc.
func (m *TypedMessage[T]) WriteTo(w io.Writer) (int64, error) {
	cw := &countWriter{w: w}
	err := json.NewEncoder(cw).Encode(m.cloudEvent())
	return cw.n, err
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// ParseRawMessage parses CloudEvents structured JSON from r into a RawMessage.
// Extracts "data" as raw JSON bytes and all other fields as attributes.
func ParseRawMessage(r io.Reader) (*RawMessage, error) {
	var ce struct {
		Data json.RawMessage `json:"data"`
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("message: reading body: %w", err)
	}
	if err := json.Unmarshal(body, &ce); err != nil {
		return nil, fmt.Errorf("message: parsing CloudEvents JSON: %w", err)
	}

	var attrs Attributes
	if err := json.Unmarshal(body, &attrs); err != nil {
		return nil, fmt.Errorf("message: parsing attributes: %w", err)
	}
	delete(attrs, "data")

	return &RawMessage{
		Data:       ce.Data,
		Attributes: attrs,
	}, nil
}

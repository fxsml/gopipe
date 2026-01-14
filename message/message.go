package message

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"sync"
	"time"
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

// NewAcking creates an Acking for a single message.
// Returns nil if either callback is nil.
func NewAcking(ack func(), nack func(error)) *Acking {
	if ack == nil || nack == nil {
		return nil
	}
	return &Acking{
		ack:              ack,
		nack:             nack,
		expectedAckCount: 1,
	}
}

// NewSharedAcking creates an Acking shared across multiple messages.
// The ack callback is invoked after expectedCount Ack() calls.
// Returns nil if expectedCount <= 0 or if either callback is nil.
func NewSharedAcking(ack func(), nack func(error), expectedCount int) *Acking {
	if expectedCount <= 0 || ack == nil || nack == nil {
		return nil
	}
	return &Acking{
		ack:              ack,
		nack:             nack,
		expectedAckCount: expectedCount,
	}
}

// TypedMessage wraps a typed data payload with attributes and acknowledgment callbacks.
// This is the base generic type for all message variants.
// Data and Attributes are public for direct access.
// Ack/Nack operations are mutually exclusive and idempotent.
type TypedMessage[T any] struct {
	// Data is the event payload per CloudEvents spec.
	Data T

	// Attributes contains the context attributes per CloudEvents spec.
	Attributes Attributes

	acking *Acking
}

// Message is the internal message type used by handlers and middleware.
// Data holds any typed payload after unmarshaling from RawMessage.
type Message = TypedMessage[any]

// RawMessage is the broker boundary message type with serialized []byte data.
// Used for pub/sub integrations (Kafka, RabbitMQ, NATS, etc.).
type RawMessage = TypedMessage[[]byte]

// New creates a Message for engine input channels.
// Pass nil for attrs or acking if not needed.
func New(data any, attrs Attributes, acking *Acking) *Message {
	if attrs == nil {
		attrs = make(Attributes)
	}
	return &Message{
		Data:       data,
		Attributes: attrs,
		acking:     acking,
	}
}

// NewTyped creates a generic typed message.
// Pass nil for attrs or acking if not needed.
func NewTyped[T any](data T, attrs Attributes, acking *Acking) *TypedMessage[T] {
	if attrs == nil {
		attrs = make(Attributes)
	}
	return &TypedMessage[T]{
		Data:       data,
		Attributes: attrs,
		acking:     acking,
	}
}

// NewRaw creates a RawMessage for broker integration.
// Pass nil for attrs or acking if not needed.
func NewRaw(data []byte, attrs Attributes, acking *Acking) *RawMessage {
	if attrs == nil {
		attrs = make(Attributes)
	}
	return &RawMessage{
		Data:       data,
		Attributes: attrs,
		acking:     acking,
	}
}

// Ack acknowledges successful processing of the message.
// Returns true if acknowledgment succeeded or was already performed.
// Returns false if no ack callback was provided, if the message was already nacked, or if expectedAckCount <= 0.
// The ack callback is invoked at most once when all stages have acked. Thread-safe.
func (m *TypedMessage[T]) Ack() bool {
	if m.acking == nil {
		return false
	}
	m.acking.mu.Lock()
	defer m.acking.mu.Unlock()

	switch m.acking.ackType {
	case ackTypeAck:
		return true
	case ackTypeNack:
		return false
	default:
	}

	m.acking.ackCount++
	if m.acking.ackCount < m.acking.expectedAckCount {
		return true
	}

	m.acking.ack()
	m.acking.ackType = ackTypeAck
	return true
}

// Nack negatively acknowledges the message due to a processing error.
// Returns true if negative acknowledgment succeeded or was already performed.
// Returns false if no nack callback was provided or if the message was already acked.
// The nack callback is invoked immediately with the first error, permanently blocking all further acks.
// Thread-safe.
func (m *TypedMessage[T]) Nack(err error) bool {
	if m.acking == nil {
		return false
	}
	m.acking.mu.Lock()
	defer m.acking.mu.Unlock()

	switch m.acking.ackType {
	case ackTypeAck:
		return false
	case ackTypeNack:
		return true
	default:
	}

	m.acking.nack(err)
	m.acking.ackType = ackTypeNack
	return true
}

// ID returns the event identifier. Returns empty string if not set.
func (m *TypedMessage[T]) ID() string {
	s, _ := m.Attributes[AttrID].(string)
	return s
}

// Type returns the event type. Returns empty string if not set.
func (m *TypedMessage[T]) Type() string {
	s, _ := m.Attributes[AttrType].(string)
	return s
}

// Source returns the event source. Returns empty string if not set.
func (m *TypedMessage[T]) Source() string {
	s, _ := m.Attributes[AttrSource].(string)
	return s
}

// Subject returns the event subject. Returns empty string if not set.
func (m *TypedMessage[T]) Subject() string {
	s, _ := m.Attributes[AttrSubject].(string)
	return s
}

// Time returns the event timestamp. Returns zero time if not set or invalid.
func (m *TypedMessage[T]) Time() time.Time {
	switch v := m.Attributes[AttrTime].(type) {
	case time.Time:
		return v
	case string:
		t, _ := time.Parse(time.RFC3339, v)
		return t
	default:
		return time.Time{}
	}
}

// DataContentType returns the data content type. Returns empty string if not set.
func (m *TypedMessage[T]) DataContentType() string {
	s, _ := m.Attributes[AttrDataContentType].(string)
	return s
}

// DataSchema returns the data schema URI. Returns empty string if not set.
func (m *TypedMessage[T]) DataSchema() string {
	s, _ := m.Attributes[AttrDataSchema].(string)
	return s
}

// SpecVersion returns the CloudEvents spec version. Returns empty string if not set.
func (m *TypedMessage[T]) SpecVersion() string {
	s, _ := m.Attributes[AttrSpecVersion].(string)
	return s
}

// Copy creates a new message with different data while preserving
// attributes (cloned) and acknowledgment callbacks (shared).
func Copy[In, Out any](msg *TypedMessage[In], data Out) *TypedMessage[Out] {
	return &TypedMessage[Out]{
		Data:       data,
		Attributes: maps.Clone(msg.Attributes),
		acking:     msg.acking,
	}
}

// cloudEvent returns the message as a CloudEvents structured map.
// Injects specversion "1.0" if not present in attributes.
// For []byte data: valid JSON goes to "data", binary goes to "data_base64".
func (m *TypedMessage[T]) cloudEvent() map[string]any {
	ce := make(map[string]any, len(m.Attributes)+2)
	maps.Copy(ce, m.Attributes)
	if _, ok := ce[AttrSpecVersion]; !ok {
		ce[AttrSpecVersion] = "1.0"
	}

	// For []byte data, embed as raw JSON if valid, otherwise base64 encode
	if data, ok := any(m.Data).([]byte); ok {
		if len(data) == 0 {
			// Empty data, omit
		} else if json.Valid(data) {
			ce["data"] = json.RawMessage(data)
		} else {
			ce["data_base64"] = base64.StdEncoding.EncodeToString(data)
		}
	} else {
		ce["data"] = m.Data
	}
	return ce
}

// MarshalJSON implements json.Marshaler.
// Returns the message as CloudEvents structured JSON.
func (m *TypedMessage[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.cloudEvent())
}

// String implements fmt.Stringer.
// Returns the message as CloudEvents structured JSON.
func (m *TypedMessage[T]) String() string {
	b, err := m.MarshalJSON()
	if err != nil {
		return fmt.Sprintf("gopipe error: %v", err)
	}
	return string(b)
}

// WriteTo implements io.WriterTo.
// Writes the message as CloudEvents structured JSON.
func (m *TypedMessage[T]) WriteTo(w io.Writer) (int64, error) {
	b, err := m.MarshalJSON()
	if err != nil {
		return 0, err
	}
	n, err := w.Write(b)
	return int64(n), err
}

// ParseRaw parses CloudEvents structured JSON from r into a RawMessage.
// Handles both data and data_base64 fields per CloudEvents spec.
func ParseRaw(r io.Reader) (*RawMessage, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("message: reading body: %w", err)
	}
	return parseRawBytes(b)
}

// parseRawBytes parses CloudEvents structured JSON bytes into a RawMessage.
func parseRawBytes(b []byte) (*RawMessage, error) {
	var ce struct {
		Data       json.RawMessage `json:"data"`
		DataBase64 string          `json:"data_base64"`
	}
	if err := json.Unmarshal(b, &ce); err != nil {
		return nil, fmt.Errorf("message: parsing CloudEvents JSON: %w", err)
	}

	var attrs Attributes
	if err := json.Unmarshal(b, &attrs); err != nil {
		return nil, fmt.Errorf("message: parsing attributes: %w", err)
	}
	delete(attrs, "data")
	delete(attrs, "data_base64")

	// Prefer data_base64 if present (binary data), otherwise use data
	var data []byte
	if ce.DataBase64 != "" {
		var err error
		data, err = base64.StdEncoding.DecodeString(ce.DataBase64)
		if err != nil {
			return nil, fmt.Errorf("message: decoding data_base64: %w", err)
		}
	} else {
		data = ce.Data
	}

	return &RawMessage{
		Data:       data,
		Attributes: attrs,
	}, nil
}

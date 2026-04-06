package message

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"time"
)

// TypedMessage wraps a typed data payload with attributes and acknowledgment callbacks.
// This is the base generic type for all message variants.
// Ack/Nack operations are mutually exclusive and idempotent.
type TypedMessage[T any] struct {
	// Data is the event payload per CloudEvents spec.
	Data T

	// Attributes contains the context attributes per CloudEvents spec.
	Attributes Attributes

	// acking coordinates acknowledgment. Can be shared across messages.
	acking *Acking

	// locals carries in-process, request-scoped values. It is never serialized
	// and does not cross broker boundaries. Values are added via SetLocal
	// and read via Local. These do NOT merge into contexts created by Context(parent).
	locals map[any]any
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
// Returns false if no ack callback was provided or if the message was already nacked.
// The ack callback is invoked at most once when all stages have acked. Thread-safe.
// Callbacks are invoked outside the mutex to prevent deadlocks.
func (m *TypedMessage[T]) Ack() bool {
	return m.acking.ack()
}

// Nack negatively acknowledges the message due to a processing error.
// Returns true if negative acknowledgment succeeded or was already performed.
// Returns false if no nack callback was provided or if the message was already acked.
// The nack callback is invoked immediately with the first error, permanently blocking all further acks.
// Also closes the done channel, allowing sibling messages to detect the settlement.
// Thread-safe. Callbacks are invoked outside the mutex to prevent deadlocks.
func (m *TypedMessage[T]) Nack(err error) bool {
	return m.acking.nack(err)
}

// Err returns the error from Nack, or nil if pending or acked.
// Use with Settled() to check settlement status:
//
//	select {
//	case <-msg.Settled():
//	    if err := msg.Err(); err != nil {
//	        // nacked with error
//	    } else {
//	        // acked successfully
//	    }
//	default:
//	    // still pending
//	}
func (m *TypedMessage[T]) Err() error {
	return m.acking.err()
}

// Settled returns a channel that is closed when the message is settled (acked or nacked).
// Returns nil if no acking is set.
func (m *TypedMessage[T]) Settled() <-chan struct{} {
	return m.acking.done()
}

// SetLocal binds an in-process value to the message under key.
// Locals are NOT serialized and do NOT cross broker boundaries.
// They are decoupled from contexts: use msg.Local(key) to read them,
// not ctx.Value(key).
//
// Not safe for concurrent use. Follows the single-writer assumption: one
// worker processes one message at a time (same as Attributes mutation).
func (m *TypedMessage[T]) SetLocal(key, val any) {
	if m == nil {
		return
	}
	if m.locals == nil {
		m.locals = make(map[any]any)
	}
	m.locals[key] = val
}

// Local returns a value previously bound via SetLocal, or nil if not set.
//
// This inspects only the message's own locals. Locals are decoupled from
// context: they do not appear in contexts created by Context(parent).
// Use typed helpers like TxFromMessage(msg) to read specific locals.
func (m *TypedMessage[T]) Local(key any) any {
	if m == nil {
		return nil
	}
	return m.locals[key]
}

// Context returns a context derived from parent with message-specific behavior.
//
// The context provides:
//   - Deadline from minimum of parent deadline and message ExpiryTime
//   - Message reference via MessageFromContext or RawMessageFromContext
//   - Parent cancellation propagation
//
// Note: This method reports the deadline via ctx.Deadline() but does not
// create timers or enforce the deadline. Use middleware.Deadline() for
// enforcement with proper timer cleanup.
//
// Locals (SetLocal/Local) are decoupled from context: they do NOT appear
// in ctx.Value lookups. Use msg.Local(key) to read locals directly.
//
// For settlement detection (ack/nack), use msg.Settled() directly.
// This keeps context cancellation (lifecycle) separate from message
// settlement (domain logic).
func (m *TypedMessage[T]) Context(parent context.Context) context.Context {
	// Determine what to store as the message reference
	var msg any
	switch v := any(m).(type) {
	case *Message:
		msg = v
	case *RawMessage:
		msg = v
	}

	return &messageContext{
		Context: parent,
		msg:     msg,
		expiry:  m.ExpiryTime(),
	}
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

// CorrelationID returns the correlation ID extension. Returns empty string if not set.
func (m *TypedMessage[T]) CorrelationID() string {
	s, _ := m.Attributes[AttrCorrelationID].(string)
	return s
}

// ExpiryTime returns the expiry time extension. Returns zero time if not set or invalid.
func (m *TypedMessage[T]) ExpiryTime() time.Time {
	switch v := m.Attributes[AttrExpiryTime].(type) {
	case time.Time:
		return v
	case string:
		t, _ := time.Parse(time.RFC3339, v)
		return t
	default:
		return time.Time{}
	}
}

// Copy creates a new message with different data while preserving
// attributes (cloned), acknowledgment callbacks (shared), and locals (cloned).
func Copy[In, Out any](msg *TypedMessage[In], data Out) *TypedMessage[Out] {
	return &TypedMessage[Out]{
		Data:       data,
		Attributes: maps.Clone(msg.Attributes),
		acking:     msg.acking,
		locals:     maps.Clone(msg.locals),
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

package message

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"sync"
	"time"
)

// AckState represents the current acknowledgment state.
type AckState int

const (
	// AckPending indicates the message has not been acked or nacked yet.
	AckPending AckState = iota
	// AckDone indicates the message was successfully acknowledged.
	AckDone
	// AckNacked indicates the message was negatively acknowledged.
	AckNacked
)

// AckStrategy determines how the router handles message acknowledgment.
type AckStrategy int

const (
	// AckManual means the handler is responsible for acking/nacking.
	// This is the default and provides maximum control.
	AckManual AckStrategy = iota

	// AckOnSuccess automatically acks on successful processing
	// and nacks on error. Equivalent to using AutoAck middleware.
	AckOnSuccess

	// AckForward forwards acknowledgment to output messages.
	// The input is acked only when ALL outputs are acked.
	// If ANY output nacks, the input is immediately nacked.
	// Useful for event sourcing where a command should only be
	// acked after all resulting events are processed.
	// Equivalent to using ForwardAck middleware.
	AckForward
)

// Acking coordinates acknowledgment across one or more messages.
// When expectedAckCount messages call Ack(), the ack callback is invoked.
// If any message calls Nack(), the nack callback is invoked immediately,
// and the done channel is closed.
// Acking is thread-safe and can be shared between multiple messages.
type Acking struct {
	mu               sync.Mutex
	ack              func()
	nack             func(error)
	state            AckState
	ackCount         int
	expectedAckCount int
	doneCh chan struct{}
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
		doneCh: make(chan struct{}),
	}
}

// NewSharedAcking creates an Acking shared across multiple messages.
// The ack callback is invoked after expectedCount Ack() calls.
// If any message nacks, all sibling messages' done channels are closed.
// Returns nil if expectedCount <= 0 or if either callback is nil.
func NewSharedAcking(ack func(), nack func(error), expectedCount int) *Acking {
	if expectedCount <= 0 || ack == nil || nack == nil {
		return nil
	}
	return &Acking{
		ack:              ack,
		nack:             nack,
		expectedAckCount: expectedCount,
		doneCh: make(chan struct{}),
	}
}

// State returns the current acknowledgment state.
// Thread-safe.
func (a *Acking) State() AckState {
	if a == nil {
		return AckPending
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

// done returns a channel that is closed when the acking is settled (acked or nacked).
func (a *Acking) done() <-chan struct{} {
	if a == nil {
		return nil
	}
	return a.doneCh
}

// messageContext wraps a parent context with message-specific behavior.
// It combines the parent's deadline with the message's expiry time,
// and cancels when either the parent cancels or the message is settled.
type messageContext[T any] struct {
	parent   context.Context
	msg      *TypedMessage[T]
	done     chan struct{}
	deadline time.Time
	hasDeadline bool
}

// newMessageContext creates a context that combines parent with message settlement.
func newMessageContext[T any](parent context.Context, msg *TypedMessage[T]) *messageContext[T] {
	ctx := &messageContext[T]{
		parent: parent,
		msg:    msg,
	}

	// Compute deadline: minimum of parent deadline and message expiry time
	if parentDeadline, ok := parent.Deadline(); ok {
		ctx.deadline = parentDeadline
		ctx.hasDeadline = true
	}
	if expiry := msg.ExpiryTime(); !expiry.IsZero() {
		if !ctx.hasDeadline || expiry.Before(ctx.deadline) {
			ctx.deadline = expiry
			ctx.hasDeadline = true
		}
	}

	// Optimization: if parent never cancels, reuse acking's done channel directly
	if parent.Done() == nil {
		ctx.done = msg.Acking.doneCh
		return ctx
	}

	// Otherwise, spawn goroutine to merge both cancellation sources
	ctx.done = make(chan struct{})
	go func() {
		select {
		case <-parent.Done():
		case <-msg.Acking.doneCh:
		}
		close(ctx.done)
	}()

	return ctx
}

func (c *messageContext[T]) Deadline() (time.Time, bool) {
	return c.deadline, c.hasDeadline
}

func (c *messageContext[T]) Done() <-chan struct{} {
	return c.done
}

func (c *messageContext[T]) Value(key any) any {
	switch key {
	case messageKey:
		if msg, ok := any(c.msg).(*Message); ok {
			return msg
		}
		return nil
	case rawMessageKey:
		if msg, ok := any(c.msg).(*RawMessage); ok {
			return msg
		}
		return nil
	case attributesKey:
		return c.msg.Attributes
	default:
		return c.parent.Value(key)
	}
}

func (c *messageContext[T]) Err() error {
	select {
	case <-c.done:
		// Check which caused cancellation
		if c.parent.Err() != nil {
			return c.parent.Err()
		}
		return context.Canceled
	default:
		return nil
	}
}

// TypedMessage wraps a typed data payload with attributes and acknowledgment callbacks.
// This is the base generic type for all message variants.
// Data, Attributes, and Acking are public for direct access.
// Ack/Nack operations are mutually exclusive and idempotent.
type TypedMessage[T any] struct {
	// Data is the event payload per CloudEvents spec.
	Data T

	// Attributes contains the context attributes per CloudEvents spec.
	Attributes Attributes

	// Acking coordinates acknowledgment. Can be shared across messages.
	// Set before processing starts; do not modify during processing.
	Acking *Acking
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
		Acking:     acking,
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
		Acking:     acking,
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
		Acking:     acking,
	}
}

// Ack acknowledges successful processing of the message.
// Returns true if acknowledgment succeeded or was already performed.
// Returns false if no ack callback was provided, if the message was already nacked, or if expectedAckCount <= 0.
// The ack callback is invoked at most once when all stages have acked. Thread-safe.
// Callbacks are invoked outside the mutex to prevent deadlocks.
func (m *TypedMessage[T]) Ack() bool {
	if m.Acking == nil {
		return false
	}
	m.Acking.mu.Lock()

	switch m.Acking.state {
	case AckDone:
		m.Acking.mu.Unlock()
		return true
	case AckNacked:
		m.Acking.mu.Unlock()
		return false
	}

	m.Acking.ackCount++
	if m.Acking.ackCount < m.Acking.expectedAckCount {
		m.Acking.mu.Unlock()
		return true
	}

	// Capture callback and done channel before releasing lock
	ackFn := m.Acking.ack
	done := m.Acking.doneCh
	m.Acking.state = AckDone
	m.Acking.mu.Unlock()

	// Call callback and close done channel outside mutex to prevent deadlock
	ackFn()
	close(done)
	return true
}

// Nack negatively acknowledges the message due to a processing error.
// Returns true if negative acknowledgment succeeded or was already performed.
// Returns false if no nack callback was provided or if the message was already acked.
// The nack callback is invoked immediately with the first error, permanently blocking all further acks.
// Also closes the done channel, allowing sibling messages to detect the settlement.
// Thread-safe. Callbacks are invoked outside the mutex to prevent deadlocks.
func (m *TypedMessage[T]) Nack(err error) bool {
	if m.Acking == nil {
		return false
	}
	m.Acking.mu.Lock()

	switch m.Acking.state {
	case AckDone:
		m.Acking.mu.Unlock()
		return false
	case AckNacked:
		m.Acking.mu.Unlock()
		return true
	}

	// Capture callback and done channel before releasing lock
	nackFn := m.Acking.nack
	done := m.Acking.doneCh
	m.Acking.state = AckNacked
	m.Acking.mu.Unlock()

	// Call callback and close done channel outside mutex to prevent deadlock
	nackFn(err)
	close(done)
	return true
}

// AckState returns the current acknowledgment state of the message.
// Returns AckPending if no acking is set.
func (m *TypedMessage[T]) AckState() AckState {
	if m.Acking == nil {
		return AckPending
	}
	return m.Acking.State()
}

// Done returns a channel that is closed when the message is settled (acked or nacked).
// Returns nil if no acking is set.
func (m *TypedMessage[T]) Done() <-chan struct{} {
	if m.Acking == nil {
		return nil
	}
	return m.Acking.doneCh
}

// Context returns a context derived from parent that is cancelled when
// the message (or any sibling sharing the acking) is settled (acked or nacked).
//
// The context provides:
//   - Cancellation when parent cancels OR message settles (whichever first)
//   - Deadline from minimum of parent deadline and message ExpiryTime
//   - Message reference via MessageFromContext or RawMessageFromContext
//   - Attributes via AttributesFromContext
//
// If no acking is set, returns the parent context unchanged.
func (m *TypedMessage[T]) Context(parent context.Context) context.Context {
	if m.Acking == nil {
		return parent
	}
	return newMessageContext(parent, m)
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
// attributes (cloned) and acknowledgment callbacks (shared).
func Copy[In, Out any](msg *TypedMessage[In], data Out) *TypedMessage[Out] {
	return &TypedMessage[Out]{
		Data:       data,
		Attributes: maps.Clone(msg.Attributes),
		Acking:     msg.Acking,
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

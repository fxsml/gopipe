// Package examples demonstrates CloudEvents sdk-go integration with goengine.
//
// This example shows the recommended hybrid approach:
// - sdk-go Event at protocol boundaries
// - Internal typed Message for handler processing
// - SDKMarshaller for conversion between the two
package examples

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	// In real implementation, this would be:
	// cloudevents "github.com/cloudevents/sdk-go/v2"
	// For this example, we provide a minimal mock of the SDK
)

// ============================================================================
// MOCK SDK TYPES (in real code, import github.com/cloudevents/sdk-go/v2)
// ============================================================================

// Event is a minimal mock of cloudevents.Event for demonstration.
// In production, use the real SDK: github.com/cloudevents/sdk-go/v2
type Event struct {
	id              string
	source          string
	specVersion     string
	ceType          string
	subject         string
	time            time.Time
	dataContentType string
	dataSchema      string
	extensions      map[string]any
	dataEncoded     []byte
}

// NewEvent creates a new CloudEvents event (mock of cloudevents.NewEvent)
func NewEvent() Event {
	return Event{
		specVersion: "1.0",
		extensions:  make(map[string]any),
	}
}

// Setters (mock SDK methods)
func (e *Event) SetID(id string)                     { e.id = id }
func (e *Event) SetSource(s string)                  { e.source = s }
func (e *Event) SetType(t string)                    { e.ceType = t }
func (e *Event) SetSubject(s string)                 { e.subject = s }
func (e *Event) SetTime(t time.Time)                 { e.time = t }
func (e *Event) SetDataContentType(ct string)        { e.dataContentType = ct }
func (e *Event) SetDataSchema(s string)              { e.dataSchema = s }
func (e *Event) SetExtension(name string, value any) { e.extensions[name] = value }

func (e *Event) SetData(contentType string, payload any) error {
	e.dataContentType = contentType
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	e.dataEncoded = data
	return nil
}

// Getters (mock SDK methods)
func (e Event) ID() string              { return e.id }
func (e Event) Source() string          { return e.source }
func (e Event) SpecVersion() string     { return e.specVersion }
func (e Event) Type() string            { return e.ceType }
func (e Event) Subject() string         { return e.subject }
func (e Event) Time() time.Time         { return e.time }
func (e Event) DataContentType() string { return e.dataContentType }
func (e Event) DataSchema() string      { return e.dataSchema }
func (e Event) Extensions() map[string]any {
	cp := make(map[string]any, len(e.extensions))
	for k, v := range e.extensions {
		cp[k] = v
	}
	return cp
}
func (e Event) Data() []byte { return e.dataEncoded }

// DataAs decodes the event data into the provided target (mock SDK method)
func (e Event) DataAs(target any) error {
	return json.Unmarshal(e.dataEncoded, target)
}

// Validate checks CloudEvents compliance (mock SDK method)
func (e Event) Validate() error {
	if e.id == "" {
		return fmt.Errorf("id is required")
	}
	if e.source == "" {
		return fmt.Errorf("source is required")
	}
	if e.specVersion == "" {
		return fmt.Errorf("specversion is required")
	}
	if e.ceType == "" {
		return fmt.Errorf("type is required")
	}
	return nil
}

// MarshalJSON for wire format (mock SDK method)
func (e Event) MarshalJSON() ([]byte, error) {
	m := map[string]any{
		"specversion": e.specVersion,
		"id":          e.id,
		"source":      e.source,
		"type":        e.ceType,
	}
	if e.subject != "" {
		m["subject"] = e.subject
	}
	if !e.time.IsZero() {
		m["time"] = e.time.Format(time.RFC3339)
	}
	if e.dataContentType != "" {
		m["datacontenttype"] = e.dataContentType
	}
	if e.dataSchema != "" {
		m["dataschema"] = e.dataSchema
	}
	for k, v := range e.extensions {
		m[k] = v
	}
	if len(e.dataEncoded) > 0 {
		var data any
		json.Unmarshal(e.dataEncoded, &data)
		m["data"] = data
	}
	return json.Marshal(m)
}

const ApplicationJSON = "application/json"

// ============================================================================
// GOENGINE INTERNAL MESSAGE TYPE
// ============================================================================

// Message is goengine's internal typed representation.
// It mirrors CloudEvents attributes but keeps payload as typed `any`.
type Message struct {
	// CloudEvents required attributes
	ID          string
	Source      string
	SpecVersion string
	Type        string

	// CloudEvents optional attributes
	Subject         string
	Time            time.Time
	DataContentType string
	DataSchema      string

	// Extension attributes
	Extensions map[string]any

	// Typed payload - this is the key difference from sdk-go Event
	// Handlers work with actual Go types, not []byte
	Payload any
}

// NewMessage creates a message with typed payload
func NewMessage[T any](payload T) *Message {
	return &Message{
		SpecVersion: "1.0",
		Type:        deriveType(reflect.TypeOf(payload)),
		Payload:     payload,
		Extensions:  make(map[string]any),
	}
}

// Destination returns the routing destination
func (m *Message) Destination() string {
	if dest, ok := m.Extensions["xdestination"].(string); ok {
		return dest
	}
	return ""
}

// SetDestination sets the routing destination
func (m *Message) SetDestination(dest string) *Message {
	if m.Extensions == nil {
		m.Extensions = make(map[string]any)
	}
	m.Extensions["xdestination"] = dest
	return m
}

// IsInternalDestination returns true for gopipe:// destinations
func (m *Message) IsInternalDestination() bool {
	return strings.HasPrefix(m.Destination(), "gopipe://")
}

// ============================================================================
// TYPE REGISTRY
// ============================================================================

// TypeEntry holds registration info for a CloudEvents type
type TypeEntry struct {
	GoType  reflect.Type
	Factory func() any
}

// TypeRegistry maps CloudEvents type → Go type for unmarshalling
type TypeRegistry struct {
	types map[string]TypeEntry
}

// NewTypeRegistry creates an empty registry
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{
		types: make(map[string]TypeEntry),
	}
}

// Register adds a type mapping
func (r *TypeRegistry) Register(ceType string, goType reflect.Type) {
	r.types[ceType] = TypeEntry{
		GoType: goType,
		Factory: func() any {
			return reflect.New(goType).Interface()
		},
	}
}

// RegisterType registers a type using generics
func RegisterType[T any](r *TypeRegistry) {
	var zero T
	t := reflect.TypeOf(zero)
	ceType := deriveType(t)
	r.Register(ceType, t)
}

// Lookup finds the type entry for a CloudEvents type
func (r *TypeRegistry) Lookup(ceType string) (TypeEntry, bool) {
	entry, ok := r.types[ceType]
	return entry, ok
}

// deriveType generates CloudEvents type from Go type
func deriveType(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	name := toSnakeCase(t.Name())
	pkg := t.PkgPath()
	if idx := strings.LastIndex(pkg, "/"); idx != -1 {
		pkg = pkg[idx+1:]
	}
	if pkg == "" || pkg == "examples" {
		return "com.goengine." + name
	}
	return fmt.Sprintf("com.%s.%s", pkg, name)
}

func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

// ============================================================================
// SDK MARSHALLER - THE KEY INTEGRATION POINT
// ============================================================================

// SDKMarshaller converts between sdk-go Event and internal Message.
// This is where the SDK integration happens at the marshalling layer.
type SDKMarshaller struct {
	registry *TypeRegistry
}

// NewSDKMarshaller creates a marshaller with the given registry
func NewSDKMarshaller(registry *TypeRegistry) *SDKMarshaller {
	return &SDKMarshaller{registry: registry}
}

// FromEvent converts a CloudEvents sdk-go Event to internal Message.
// This is called when receiving events from external systems.
func (m *SDKMarshaller) FromEvent(evt Event) (*Message, error) {
	// Validate CloudEvents compliance using SDK
	if err := evt.Validate(); err != nil {
		return nil, fmt.Errorf("invalid CloudEvent: %w", err)
	}

	msg := &Message{
		ID:              evt.ID(),
		Source:          evt.Source(),
		SpecVersion:     evt.SpecVersion(),
		Type:            evt.Type(),
		Subject:         evt.Subject(),
		Time:            evt.Time(),
		DataContentType: evt.DataContentType(),
		DataSchema:      evt.DataSchema(),
		Extensions:      evt.Extensions(),
	}

	// Look up Go type in registry
	entry, ok := m.registry.Lookup(evt.Type())
	if !ok {
		// Unknown type - keep raw bytes for passthrough
		msg.Payload = evt.Data()
		return msg, nil
	}

	// Create typed instance and decode using SDK's DataAs
	target := entry.Factory()
	if err := evt.DataAs(target); err != nil {
		return nil, fmt.Errorf("decode payload for type %s: %w", evt.Type(), err)
	}

	// Dereference pointer to get value type
	v := reflect.ValueOf(target)
	if v.Kind() == reflect.Ptr {
		msg.Payload = v.Elem().Interface()
	} else {
		msg.Payload = target
	}

	return msg, nil
}

// ToEvent converts internal Message to CloudEvents sdk-go Event.
// This is called when publishing events to external systems.
func (m *SDKMarshaller) ToEvent(msg *Message) (Event, error) {
	evt := NewEvent()

	// Set required attributes
	evt.SetID(msg.ID)
	evt.SetSource(msg.Source)
	evt.SetType(msg.Type)

	// Set optional attributes
	if msg.Subject != "" {
		evt.SetSubject(msg.Subject)
	}
	if !msg.Time.IsZero() {
		evt.SetTime(msg.Time)
	}
	if msg.DataSchema != "" {
		evt.SetDataSchema(msg.DataSchema)
	}

	// Set extensions
	for k, v := range msg.Extensions {
		evt.SetExtension(k, v)
	}

	// Encode payload using SDK's SetData
	contentType := msg.DataContentType
	if contentType == "" {
		contentType = ApplicationJSON
	}
	if err := evt.SetData(contentType, msg.Payload); err != nil {
		return evt, fmt.Errorf("encode payload: %w", err)
	}

	return evt, nil
}

// ============================================================================
// DOMAIN TYPES (example business events)
// ============================================================================

// OrderCreated is a domain event
type OrderCreated struct {
	OrderID    string   `json:"order_id"`
	CustomerID string   `json:"customer_id"`
	Items      []string `json:"items"`
	Total      float64  `json:"total"`
}

// PaymentRequested is a domain command
type PaymentRequested struct {
	OrderID string  `json:"order_id"`
	Amount  float64 `json:"amount"`
}

// PaymentCompleted is a domain event
type PaymentCompleted struct {
	OrderID       string `json:"order_id"`
	TransactionID string `json:"transaction_id"`
}

// ============================================================================
// HANDLER EXAMPLE
// ============================================================================

// Handler interface processes typed Messages
type Handler interface {
	Handle(ctx context.Context, msg *Message) ([]*Message, error)
	Match(msg *Message) bool
}

// OrderCreatedHandler processes OrderCreated events with type safety
type OrderCreatedHandler struct{}

func (h *OrderCreatedHandler) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
	// Type-safe access to payload - this is why we use internal Message
	evt, ok := msg.Payload.(OrderCreated)
	if !ok {
		return nil, fmt.Errorf("expected OrderCreated, got %T", msg.Payload)
	}

	fmt.Printf("Processing order %s for customer %s (total: $%.2f)\n",
		evt.OrderID, evt.CustomerID, evt.Total)

	// Create response event with typed payload
	payment := NewMessage(PaymentRequested{
		OrderID: evt.OrderID,
		Amount:  evt.Total,
	})
	payment.ID = "payment-" + evt.OrderID
	payment.Source = "/orders/handler"
	payment.SetDestination("kafka://payments") // External destination

	return []*Message{payment}, nil
}

func (h *OrderCreatedHandler) Match(msg *Message) bool {
	return msg.Type == "com.goengine.order_created"
}

// ============================================================================
// EXAMPLE USAGE
// ============================================================================

// ExampleSDKIntegration demonstrates the full flow
func ExampleSDKIntegration() {
	// 1. Create type registry and register known types
	registry := NewTypeRegistry()
	RegisterType[OrderCreated](registry)
	RegisterType[PaymentRequested](registry)
	RegisterType[PaymentCompleted](registry)

	fmt.Println("=== Type Registry ===")
	fmt.Printf("Registered types: OrderCreated, PaymentRequested, PaymentCompleted\n\n")

	// 2. Create SDK marshaller
	marshaller := NewSDKMarshaller(registry)

	// 3. Simulate receiving a CloudEvent from external system (e.g., Kafka)
	// In real code, this would come from sdk-go protocol binding
	incomingEvent := NewEvent()
	incomingEvent.SetID("evt-12345")
	incomingEvent.SetSource("/orders/api")
	incomingEvent.SetType("com.goengine.order_created")
	incomingEvent.SetTime(time.Now())
	incomingEvent.SetData(ApplicationJSON, OrderCreated{
		OrderID:    "ORD-001",
		CustomerID: "CUST-123",
		Items:      []string{"widget-a", "widget-b"},
		Total:      99.99,
	})

	fmt.Println("=== Incoming CloudEvent (from SDK) ===")
	wireFormat, _ := incomingEvent.MarshalJSON()
	fmt.Printf("%s\n\n", wireFormat)

	// 4. Convert to internal Message using marshaller
	msg, err := marshaller.FromEvent(incomingEvent)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("=== Internal Message (typed payload) ===")
	fmt.Printf("ID: %s\n", msg.ID)
	fmt.Printf("Type: %s\n", msg.Type)
	fmt.Printf("Payload Type: %T\n", msg.Payload)
	fmt.Printf("Payload: %+v\n\n", msg.Payload)

	// 5. Handler processes typed Message
	handler := &OrderCreatedHandler{}
	fmt.Println("=== Handler Processing ===")
	results, err := handler.Handle(context.Background(), msg)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// 6. Convert output Messages back to CloudEvents for publishing
	fmt.Println("\n=== Outgoing CloudEvents (to SDK) ===")
	for _, result := range results {
		outEvent, err := marshaller.ToEvent(result)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		wireFormat, _ := outEvent.MarshalJSON()
		fmt.Printf("Destination: %s\n", result.Destination())
		fmt.Printf("Event: %s\n", wireFormat)
	}
}

// ExampleInternalLoop demonstrates internal routing without SDK conversion
func ExampleInternalLoop() {
	fmt.Println("\n=== Internal Loop (no SDK) ===")

	// Internal messages skip SDK marshalling entirely
	internalMsg := NewMessage(PaymentCompleted{
		OrderID:       "ORD-001",
		TransactionID: "TXN-789",
	})
	internalMsg.ID = "internal-001"
	internalMsg.Source = "/payments/handler"
	internalMsg.SetDestination("gopipe://orders") // Internal destination

	fmt.Printf("Internal Message:\n")
	fmt.Printf("  Destination: %s\n", internalMsg.Destination())
	fmt.Printf("  IsInternal: %v\n", internalMsg.IsInternalDestination())
	fmt.Printf("  Payload Type: %T\n", internalMsg.Payload)
	fmt.Printf("  Payload: %+v\n", internalMsg.Payload)
	fmt.Println("\n  → Stays as typed Message, no SDK conversion needed")
	fmt.Println("  → Zero serialization overhead for internal routing")
}

// Run both examples
func init() {
	ExampleSDKIntegration()
	ExampleInternalLoop()
}

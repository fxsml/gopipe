// Package examples demonstrates the type registry mechanism for goengine.
//
// The type registry provides automatic type discovery and registration
// from handlers, enabling dynamic unmarshalling of CloudEvents payloads.
package examples

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// TypeEntry holds registration information for a CloudEvents type
type TypeEntry struct {
	// GoType is the reflect.Type of the Go struct
	GoType reflect.Type

	// ContentType is the MIME type for serialization
	ContentType string

	// Factory creates a new zero-value instance
	Factory func() any
}

// TypeRegistry maps CloudEvents type strings to Go type information
type TypeRegistry struct {
	mu    sync.RWMutex
	types map[string]TypeEntry
}

// NewTypeRegistry creates an empty registry
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{
		types: make(map[string]TypeEntry),
	}
}

// Register adds a type mapping to the registry
func (r *TypeRegistry) Register(ceType string, entry TypeEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.types[ceType] = entry
}

// Lookup finds the type entry for a CloudEvents type
func (r *TypeRegistry) Lookup(ceType string) (TypeEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.types[ceType]
	return entry, ok
}

// Types returns all registered CloudEvents types
func (r *TypeRegistry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.types))
	for t := range r.types {
		types = append(types, t)
	}
	return types
}

// TypeDescriptor describes a type that a handler can process
type TypeDescriptor struct {
	Type        string       // CloudEvents type (e.g., "com.example.order.created")
	GoType      reflect.Type // Go struct type
	ContentType string       // MIME type (e.g., "application/json")
}

// TypeProvider is implemented by handlers that declare their types
type TypeProvider interface {
	// Types returns all CloudEvents types this handler can process
	Types() []TypeDescriptor
}

// RegisterFromProviders populates the registry from TypeProvider implementations
func (r *TypeRegistry) RegisterFromProviders(providers ...TypeProvider) {
	for _, p := range providers {
		for _, td := range p.Types() {
			r.Register(td.Type, TypeEntry{
				GoType:      td.GoType,
				ContentType: td.ContentType,
				Factory: func(t reflect.Type) func() any {
					return func() any {
						return reflect.New(t).Interface()
					}
				}(td.GoType),
			})
		}
	}
}

// DeriveCloudEventsType generates a CloudEvents type from a Go type
// Convention: package/TypeName → com.package.type_name
func DeriveCloudEventsType(t reflect.Type) string {
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	pkg := t.PkgPath()
	name := toSnakeCase(t.Name())

	// Extract last package component
	if idx := strings.LastIndex(pkg, "/"); idx != -1 {
		pkg = pkg[idx+1:]
	}

	if pkg == "" {
		return name
	}
	return fmt.Sprintf("com.%s.%s", pkg, name)
}

// toSnakeCase converts PascalCase to snake_case
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

// --- Handler Implementation with Type Introspection ---

// Handler processes messages and declares processable types
type Handler interface {
	Handle(ctx context.Context, msg *Message) ([]*Message, error)
	Match(msg *Message) bool
	TypeProvider // Embedded for type introspection
}

// Message is a simplified message type for this example
type Message struct {
	Type        string
	Data        any
	ContentType string
}

// CommandHandler is a generic handler for command messages
type CommandHandler[Cmd any, Event any] struct {
	handler     func(ctx context.Context, cmd Cmd) (Event, error)
	cmdType     string
	eventType   string
	contentType string
}

// NewCommandHandler creates a handler with automatic type registration
func NewCommandHandler[Cmd any, Event any](
	handler func(ctx context.Context, cmd Cmd) (Event, error),
) *CommandHandler[Cmd, Event] {
	var zeroCmd Cmd
	var zeroEvent Event

	return &CommandHandler[Cmd, Event]{
		handler:     handler,
		cmdType:     DeriveCloudEventsType(reflect.TypeOf(zeroCmd)),
		eventType:   DeriveCloudEventsType(reflect.TypeOf(zeroEvent)),
		contentType: "application/json",
	}
}

// Handle processes the command and returns events
func (h *CommandHandler[Cmd, Event]) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
	cmd, ok := msg.Data.(Cmd)
	if !ok {
		return nil, fmt.Errorf("invalid command type: expected %T, got %T", *new(Cmd), msg.Data)
	}

	event, err := h.handler(ctx, cmd)
	if err != nil {
		return nil, err
	}

	return []*Message{{
		Type:        h.eventType,
		Data:        event,
		ContentType: h.contentType,
	}}, nil
}

// Match returns true if message type matches the command type
func (h *CommandHandler[Cmd, Event]) Match(msg *Message) bool {
	return msg.Type == h.cmdType
}

// Types returns the type descriptors for registry auto-generation
func (h *CommandHandler[Cmd, Event]) Types() []TypeDescriptor {
	var zeroCmd Cmd
	return []TypeDescriptor{{
		Type:        h.cmdType,
		GoType:      reflect.TypeOf(zeroCmd),
		ContentType: h.contentType,
	}}
}

// --- Unmarshaller with Registry ---

// Unmarshaller transforms raw bytes to typed messages using the registry
type Unmarshaller struct {
	registry *TypeRegistry
	codecs   map[string]Codec
}

// Codec handles serialization for a content type
type Codec interface {
	Unmarshal(data []byte, v any) error
	Marshal(v any) ([]byte, error)
}

// JSONCodec implements Codec for JSON
type JSONCodec struct{}

func (JSONCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func (JSONCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// NewUnmarshaller creates an unmarshaller with the given registry
func NewUnmarshaller(registry *TypeRegistry) *Unmarshaller {
	return &Unmarshaller{
		registry: registry,
		codecs: map[string]Codec{
			"application/json": JSONCodec{},
		},
	}
}

// Unmarshal converts raw bytes to a typed message
func (u *Unmarshaller) Unmarshal(ceType, contentType string, data []byte) (*Message, error) {
	entry, ok := u.registry.Lookup(ceType)
	if !ok {
		return nil, fmt.Errorf("unknown CloudEvents type: %s", ceType)
	}

	codec, ok := u.codecs[contentType]
	if !ok {
		return nil, fmt.Errorf("unknown content type: %s", contentType)
	}

	// Create new instance using factory
	instance := entry.Factory()

	if err := codec.Unmarshal(data, instance); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	// Dereference pointer for value type
	if reflect.TypeOf(instance).Kind() == reflect.Ptr {
		instance = reflect.ValueOf(instance).Elem().Interface()
	}

	return &Message{
		Type:        ceType,
		Data:        instance,
		ContentType: contentType,
	}, nil
}

// --- Example Usage ---

// Domain types
type CreateOrder struct {
	CustomerID string
	Items      []string
}

type OrderCreated struct {
	OrderID    string
	CustomerID string
}

// ExampleTypeRegistry demonstrates automatic registry population
func ExampleTypeRegistry() {
	// Create handlers
	createOrderHandler := NewCommandHandler(
		func(ctx context.Context, cmd CreateOrder) (OrderCreated, error) {
			return OrderCreated{
				OrderID:    "order-123",
				CustomerID: cmd.CustomerID,
			}, nil
		},
	)

	// Build registry from handlers (engine does this automatically)
	registry := NewTypeRegistry()
	registry.RegisterFromProviders(createOrderHandler)

	// Print registered types
	fmt.Println("Registered types:")
	for _, t := range registry.Types() {
		entry, _ := registry.Lookup(t)
		fmt.Printf("  %s → %s\n", t, entry.GoType.Name())
	}

	// Create unmarshaller with registry
	unmarshaller := NewUnmarshaller(registry)

	// Simulate receiving raw CloudEvent
	rawData := []byte(`{"CustomerID": "cust-456", "Items": ["item-1", "item-2"]}`)
	ceType := "com.examples.create_order"
	contentType := "application/json"

	// Unmarshal using registry
	msg, err := unmarshaller.Unmarshal(ceType, contentType, rawData)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Unmarshalled: %+v\n", msg.Data)

	// Handler can now process the typed message
	results, _ := createOrderHandler.Handle(context.Background(), msg)
	fmt.Printf("Result: %+v\n", results[0].Data)
}

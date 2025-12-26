# Plan 0002: Marshaler Implementation

**Status:** Proposed
**Related ADRs:** [0021](../adr/0021-codec-marshaling-pattern.md), [0022](../adr/0022-message-package-redesign.md)
**Required By:** [Plan 0001](0001-message-engine.md) (Message Engine)

## Overview

Implement a Marshaler with type registry and NamingStrategy for automatic CE type ↔ Go type mapping.

## Type Registry

The Marshaler maintains a bidirectional mapping:

```
┌─────────────────────────────────────────────┐
│              Marshaler Registry              │
│                                              │
│  CE Type → Go Type    │    Go Type → CE Type │
│  ─────────────────    │    ─────────────────  │
│  "order.created" → OrderCreated              │
│  "order.shipped" → OrderShipped              │
│  "create.order"  → CreateOrder               │
└─────────────────────────────────────────────┘
```

- **Unmarshal direction**: CE type string → Go type → instantiate
- **TypeName direction**: Go type → CE type string

## Interface

```go
type Marshaler interface {
    // Register adds a Go type to the registry
    // CE type is derived via NamingStrategy: OrderCreated → "order.created"
    Register(goType reflect.Type)

    // TypeName returns the CE type for a Go type (lookup or derive via NamingStrategy)
    TypeName(goType reflect.Type) string

    // Marshal converts a Go value to []byte
    Marshal(v any) ([]byte, error)

    // Unmarshal converts []byte to a Go value based on CloudEvents type
    // Looks up CE type in registry to find Go type, then instantiates and unmarshals
    Unmarshal(data []byte, ceType string) (any, error)

    // ContentType returns the MIME type (e.g., "application/json")
    ContentType() string
}
```

## JSON Implementation

```go
type JSONMarshaler struct {
    mu     sync.RWMutex
    ceToGo map[string]reflect.Type  // "order.created" → OrderCreated
    goToCE map[reflect.Type]string  // OrderCreated → "order.created"
    naming NamingStrategy
}

type JSONMarshalerConfig struct {
    Naming NamingStrategy  // required (no default - explicit configuration)
}

func NewJSONMarshaler(cfg JSONMarshalerConfig) *JSONMarshaler

func (m *JSONMarshaler) Register(goType reflect.Type)
func (m *JSONMarshaler) TypeName(goType reflect.Type) string
func (m *JSONMarshaler) Marshal(v any) ([]byte, error)
func (m *JSONMarshaler) Unmarshal(data []byte, ceType string) (any, error)
func (m *JSONMarshaler) ContentType() string  // "application/json"
```

## NamingStrategy

NamingStrategy lives in Marshaler. It derives CE type from Go type names.

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string  // Go type → CE type
}

// KebabNaming: OrderCreated → "order.created"
var KebabNaming NamingStrategy

// SnakeNaming: OrderCreated → "order_created"
var SnakeNaming NamingStrategy
```

## Usage

```go
marshaler := message.NewJSONMarshaler(message.JSONMarshalerConfig{
    Naming: message.KebabNaming,
})

// Register type (CE type derived via NamingStrategy)
marshaler.Register(reflect.TypeOf(OrderCreated{}))  // → "order.created"

// TypeName returns CE type for Go type
ceType := marshaler.TypeName(reflect.TypeOf(OrderCreated{}))  // "order.created"

// Marshal converts Go value to bytes
data, _ := marshaler.Marshal(OrderCreated{ID: "123"})
// data: {"id":"123"}

// Unmarshal uses CE type to find Go type, instantiate, and unmarshal
event, _ := marshaler.Unmarshal(data, "order.created")
order := event.(OrderCreated)

// Content type for DataContentType attribute
contentType := marshaler.ContentType()  // "application/json"
```

## Auto-Registration

Types are auto-registered when TypeName is called for unregistered types:

```go
// TypeName for unregistered type auto-registers it
ceType := marshaler.TypeName(reflect.TypeOf(OrderCreated{}))
// Now "order.created" ↔ OrderCreated is registered
```

This enables the Engine to auto-register handler types:

```go
engine.AddHandler(handler, HandlerConfig{Name: "process-order"})
// Engine calls: marshaler.TypeName(handler.EventType())
// → auto-registers the type if not already registered
```

## Files to Create

- `message/marshaler.go` - Marshaler interface
- `message/json_marshaler.go` - JSONMarshaler implementation
- `message/naming.go` - NamingStrategy interface and implementations

## Test Plan

1. Register type, Unmarshal returns typed value
2. Unregistered CE type returns error on Unmarshal
3. TypeName returns CE type for registered Go type
4. TypeName auto-registers unregistered types
5. Round-trip: Marshal then Unmarshal preserves data
6. KebabNaming converts correctly (OrderCreated → "order.created")
7. SnakeNaming converts correctly (OrderCreated → "order_created")
8. ContentType returns "application/json"
9. Thread-safe concurrent registration and access
10. Bidirectional registry: CE type ↔ Go type both work

## Acceptance Criteria

- [ ] Marshaler interface defined with Register, TypeName, Marshal, Unmarshal, ContentType
- [ ] JSONMarshaler implements interface
- [ ] NamingStrategy interface with KebabNaming, SnakeNaming
- [ ] Bidirectional type registry (CE type ↔ Go type)
- [ ] Auto-registration on TypeName for unregistered types
- [ ] Thread-safe
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)

# Plan 0002: Marshaler Implementation

**Status:** Proposed
**Related ADRs:** [0021](../adr/0021-codec-marshaling-pattern.md), [0022](../adr/0022-message-package-redesign.md)
**Required By:** [Plan 0001](0001-message-engine.md) (Message Engine)

## Overview

Implement a lightweight Marshaler with type registry for CloudEvents type ↔ Go type mapping.

## Interface

```go
type Marshaler interface {
    // Register adds a type to the registry, deriving CE type name via NamingStrategy
    Register(v any)

    // Marshal converts a Go value to []byte, returns data and CE type
    Marshal(v any) (data []byte, ceType string, err error)

    // Unmarshal converts []byte to a Go value based on CloudEvents type
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
    Naming NamingStrategy  // default: KebabNaming
}

func NewJSONMarshaler(cfg JSONMarshalerConfig) *JSONMarshaler

func (m *JSONMarshaler) Register(v any)
func (m *JSONMarshaler) Marshal(v any) ([]byte, string, error)
func (m *JSONMarshaler) Unmarshal(data []byte, ceType string) (any, error)
func (m *JSONMarshaler) ContentType() string  // "application/json"
```

## NamingStrategy

NamingStrategy is shared between Marshaler and Engine for consistency.

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string    // Go type → CE type
    OutputName(t reflect.Type) string  // Go type → output name
}

// KebabNaming: OrderCreated → "order.created", output: "orders"
var KebabNaming NamingStrategy

// SnakeNaming: OrderCreated → "order_created", output: "orders"
var SnakeNaming NamingStrategy
```

## Usage

```go
marshaler := message.NewJSONMarshaler(message.JSONMarshalerConfig{
    // Naming: message.KebabNaming (default)
})

// Register type (CE type derived from NamingStrategy)
marshaler.Register(OrderCreated{})  // "order.created"

// Marshal returns data + CE type
data, ceType, _ := marshaler.Marshal(OrderCreated{ID: "123"})
// data: {"id":"123"}, ceType: "order.created"

// Unmarshal uses CE type to find Go type
event, _ := marshaler.Unmarshal(data, "order.created")
order := event.(OrderCreated)

// Content type for DataContentType attribute
contentType := marshaler.ContentType()  // "application/json"
```

## Auto-Registration

Types can be auto-registered when first marshaled:

```go
// First marshal of unregistered type auto-registers it
data, ceType, _ := marshaler.Marshal(OrderCreated{ID: "123"})
// Now "order.created" is registered
```

## Files to Create

- `message/marshaler.go` - Marshaler interface
- `message/json_marshaler.go` - JSONMarshaler implementation
- `message/naming.go` - NamingStrategy interface and implementations

## Test Plan

1. Register type, Unmarshal returns typed value
2. Unregistered CE type returns error
3. Marshal returns data and CE type
4. Auto-registration on first Marshal
5. Round-trip: Marshal then Unmarshal preserves data
6. KebabNaming converts correctly (OrderCreated → "order.created")
7. SnakeNaming converts correctly (OrderCreated → "order_created")
8. OutputName extracts domain (OrderCreated → "orders")
9. ContentType returns "application/json"
10. Thread-safe concurrent registration and access

## Acceptance Criteria

- [ ] Marshaler interface defined
- [ ] JSONMarshaler implements interface
- [ ] NamingStrategy interface with KebabNaming, SnakeNaming
- [ ] Auto-registration on Marshal
- [ ] Thread-safe
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)

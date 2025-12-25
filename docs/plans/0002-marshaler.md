# Plan 0002: Marshaler Implementation

**Status:** Proposed
**Related ADRs:** [0021](../adr/0021-codec-marshaling-pattern.md), [0022](../adr/0022-message-package-redesign.md)
**Required By:** [Plan 0001](0001-message-engine.md) (Message Engine)

## Overview

Implement a Marshaler with bidirectional type registry for CloudEvents type <-> Go type mapping.

## Interface

```go
// Marshaler handles serialization and type mapping
type Marshaler interface {
    // Register maps a CloudEvents type to a Go type prototype
    Register(ceType string, prototype any)

    // Name returns the CloudEvents type for a Go value
    Name(v any) string

    // TypeFor returns the Go type for a CloudEvents type
    TypeFor(ceType string) (reflect.Type, bool)

    // Marshal converts a Go value to []byte
    Marshal(v any) ([]byte, error)

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
    ceToGo map[string]reflect.Type  // "order.created" -> OrderCreated
    goToCE map[reflect.Type]string  // OrderCreated -> "order.created"
    naming NamingStrategy
}

func NewJSONMarshaler() *JSONMarshaler

func (m *JSONMarshaler) Register(ceType string, prototype any)
func (m *JSONMarshaler) Name(v any) string
func (m *JSONMarshaler) TypeFor(ceType string) (reflect.Type, bool)
func (m *JSONMarshaler) Marshal(v any) ([]byte, error)
func (m *JSONMarshaler) Unmarshal(data []byte, ceType string) (any, error)
func (m *JSONMarshaler) ContentType() string
func (m *JSONMarshaler) SetNaming(strategy NamingStrategy)
```

## Naming Strategies

```go
type NamingStrategy func(t reflect.Type) string

// NamingKebab: OrderCreated -> "order.created"
var NamingKebab NamingStrategy

// NamingSimple: OrderCreated -> "OrderCreated"
var NamingSimple NamingStrategy
```

## Usage

```go
marshaler := message.NewJSONMarshaler()

// Explicit registration
marshaler.Register("order.created", OrderCreated{})

// Or rely on auto-naming (NamingKebab is default)
// OrderCreated{} -> "order.created" automatically

// Get CE type from Go value
name := marshaler.Name(OrderCreated{})  // "order.created"

// Get Go type from CE type
goType, ok := marshaler.TypeFor("order.created")

// Marshal/Unmarshal
data, _ := marshaler.Marshal(OrderCreated{ID: "123"})
event, _ := marshaler.Unmarshal(data, "order.created")
order := event.(OrderCreated)
```

## Files to Create

- `message/marshaler.go` - Marshaler interface and JSONMarshaler

## Test Plan

1. Register type, unmarshal returns typed value
2. Unregistered type returns raw `[]byte`
3. Name() returns registered CE type
4. Name() falls back to naming strategy for unregistered types
5. Round-trip: Marshal then Unmarshal preserves data
6. NamingKebab converts correctly (OrderCreated -> order.created)
7. Thread-safe concurrent registration

## Acceptance Criteria

- [ ] Marshaler interface defined
- [ ] JSONMarshaler implements interface
- [ ] Naming strategies work
- [ ] Thread-safe
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)

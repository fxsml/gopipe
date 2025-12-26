# Plan 0002: Codec, TypeRegistry, and NamingStrategy

**Status:** Proposed
**Related ADRs:** [0021](../adr/0021-codec-marshaling-pattern.md), [0022](../adr/0022-message-package-redesign.md)
**Required By:** [Plan 0001](0001-message-engine.md) (Message Engine)

## Overview

Implement three separate components with clear responsibilities:

| Component | Responsibility |
|-----------|----------------|
| **Codec** | Pure serialization (Marshal/Unmarshal/ContentType) |
| **TypeRegistry** | Maps CE type ↔ Go type for unmarshaling |
| **NamingStrategy** | Derives CE type from Go type names |

## Codec Interface

Pure serialization with no type awareness:

```go
type Codec interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
    ContentType() string  // e.g., "application/json"
}
```

### JSON Implementation

```go
type JSONCodec struct{}

func NewJSONCodec() *JSONCodec

func (c *JSONCodec) Marshal(v any) ([]byte, error)      // json.Marshal
func (c *JSONCodec) Unmarshal(data []byte, v any) error // json.Unmarshal
func (c *JSONCodec) ContentType() string                 // "application/json"
```

## TypeRegistry Interface

Maps CE types to Go types for instantiation during unmarshaling:

```go
type TypeRegistry interface {
    Register(ceType string, goType reflect.Type)
    Lookup(ceType string) (reflect.Type, bool)
}
```

### Implementation

```go
type typeRegistry struct {
    mu    sync.RWMutex
    types map[string]reflect.Type  // "order.created" → OrderCreated
}

func NewTypeRegistry() TypeRegistry

func (r *typeRegistry) Register(ceType string, goType reflect.Type)
func (r *typeRegistry) Lookup(ceType string) (reflect.Type, bool)
```

**Usage by Engine:**
```go
// Engine registers handler types automatically
engine.AddHandler(handler, HandlerConfig{Name: "process-order"})
// Engine calls: registry.Register(handler.EventType(), handler.GoType())
```

## NamingStrategy Interface

Standalone utility for deriving CE type from Go type:

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string  // Go type → CE type
}

var KebabNaming NamingStrategy   // OrderCreated → "order.created"
var SnakeNaming NamingStrategy   // OrderCreated → "order_created"
```

### Implementations

```go
type kebabNaming struct{}

func (k kebabNaming) TypeName(t reflect.Type) string {
    // Split PascalCase: OrderCreated → ["Order", "Created"]
    // Join with dot, lowercase: "order.created"
}

type snakeNaming struct{}

func (s snakeNaming) TypeName(t reflect.Type) string {
    // Split PascalCase: OrderCreated → ["Order", "Created"]
    // Join with underscore, lowercase: "order_created"
}
```

**Used by handler constructors:**
```go
handler := message.NewHandler(processOrder, message.KebabNaming)
// handler.EventType() returns "order.created"
```

## Usage

### Engine Setup

```go
engine := message.NewEngine(message.EngineConfig{
    Codec:    message.NewJSONCodec(),
    Registry: message.NewTypeRegistry(),
})
```

### Handler Construction

```go
// NamingStrategy used at construction time
handler := message.NewHandler(
    func(ctx context.Context, msg *TypedMessage[OrderCreated]) ([]*Message, error) {
        // ...
    },
    message.KebabNaming,  // derives EventType from OrderCreated
)
// handler.EventType() returns "order.created"
// handler.GoType() returns reflect.Type of OrderCreated
```

### Unmarshaling Flow

```go
// 1. Engine receives []byte + CE type from input
data := []byte(`{"order_id": "123"}`)
ceType := "order.created"

// 2. Lookup Go type in registry
goType, ok := registry.Lookup(ceType)  // OrderCreated

// 3. Instantiate and unmarshal
instance := reflect.New(goType).Interface()
codec.Unmarshal(data, instance)
```

## Files to Create

| File | Component | Notes |
|------|-----------|-------|
| `message/codec.go` | Codec | Interface definition |
| `message/json_codec.go` | JSONCodec | JSON implementation |
| `message/registry.go` | TypeRegistry | Interface and implementation |
| `message/naming.go` | NamingStrategy | Interface, KebabNaming, SnakeNaming |

## Test Plan

### Codec Tests
1. JSONCodec.Marshal converts struct to JSON bytes
2. JSONCodec.Unmarshal parses JSON into struct
3. JSONCodec.ContentType returns "application/json"
4. Round-trip: Marshal then Unmarshal preserves data

### TypeRegistry Tests
5. Register and Lookup returns correct type
6. Lookup for unregistered type returns false
7. Thread-safe concurrent registration and lookup
8. Multiple registrations for same CE type (last wins or error?)

### NamingStrategy Tests
9. KebabNaming: OrderCreated → "order.created"
10. KebabNaming: CreateOrderCommand → "create.order.command"
11. SnakeNaming: OrderCreated → "order_created"
12. SnakeNaming: CreateOrderCommand → "create_order_command"
13. Handles edge cases: ID, URL, HTTPRequest

## Acceptance Criteria

### MVP (Required)
- [ ] Codec interface with Marshal, Unmarshal, ContentType
- [ ] JSONCodec implements Codec
- [ ] TypeRegistry interface with Register, Lookup
- [ ] TypeRegistry implementation (thread-safe)
- [ ] NamingStrategy interface with TypeName
- [ ] KebabNaming implementation
- [ ] SnakeNaming implementation
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)

### Design Validation
- [ ] Codec has no type registry - pure serialization
- [ ] TypeRegistry has no NamingStrategy - explicit registration
- [ ] NamingStrategy is standalone utility used by handler constructors

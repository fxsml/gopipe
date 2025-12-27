# Plan 0002: Codec and NamingStrategy

**Status:** Proposed
**Related ADRs:** [0021](../adr/0021-codec-marshaling-pattern.md), [0022](../adr/0022-message-package-redesign.md)
**Required By:** [Plan 0001](0001-message-engine.md) (Message Engine)

## Overview

Implement two separate components with clear responsibilities:

| Component | Responsibility |
|-----------|----------------|
| **Codec** | Pure serialization (Marshal/Unmarshal/ContentType) |
| **NamingStrategy** | Derives CE type from Go type names |

Note: TypeRegistry is no longer a public API. Handler.NewInput() provides instance creation for unmarshaling.

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
    Codec: message.NewJSONCodec(),
})
```

### Handler Construction

```go
// NamingStrategy used at construction time
handler := message.NewHandler(
    func(ctx context.Context, msg *Message[OrderCreated]) ([]*RawMessage, error) {
        // ...
    },
    message.KebabNaming,  // derives EventType from OrderCreated
)
// handler.EventType() returns "order.created"
// handler.NewInput() returns *OrderCreated for unmarshaling
```

### Unmarshaling Flow

```go
// 1. Engine receives []byte + CE type from input
data := []byte(`{"order_id": "123"}`)
ceType := "order.created"

// 2. Get handler for CE type, create instance via handler.NewInput()
handler := engine.handlers[ceType]
instance := handler.NewInput()  // returns *OrderCreated

// 3. Unmarshal into instance
codec.Unmarshal(data, instance)
```

## Files to Create

| File | Component | Notes |
|------|-----------|-------|
| `message/codec.go` | Codec | Interface definition |
| `message/json_codec.go` | JSONCodec | JSON implementation |
| `message/naming.go` | NamingStrategy | Interface, KebabNaming, SnakeNaming |

## Test Plan

### Codec Tests
1. JSONCodec.Marshal converts struct to JSON bytes
2. JSONCodec.Unmarshal parses JSON into struct
3. JSONCodec.ContentType returns "application/json"
4. Round-trip: Marshal then Unmarshal preserves data

### NamingStrategy Tests
5. KebabNaming: OrderCreated → "order.created"
6. KebabNaming: CreateOrderCommand → "create.order.command"
7. SnakeNaming: OrderCreated → "order_created"
8. SnakeNaming: CreateOrderCommand → "create_order_command"
9. Handles edge cases: ID, URL, HTTPRequest

## Acceptance Criteria

### MVP (Required)
- [ ] Codec interface with Marshal, Unmarshal, ContentType
- [ ] JSONCodec implements Codec
- [ ] NamingStrategy interface with TypeName
- [ ] KebabNaming implementation
- [ ] SnakeNaming implementation
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)

### Design Validation
- [ ] Codec has no type registry - pure serialization
- [ ] NamingStrategy is standalone utility used by handler constructors
- [ ] Handler.NewInput() provides instance creation (no public TypeRegistry)

# ADR 0020: Non-Generic Message Type

**Date:** 2025-12-13
**Status:** Proposed
**Supersedes:** ADR 0004 (Dual Message Types)

## Context

gopipe currently uses generics for type-safe message handling:

```go
// Generic typed message
type TypedMessage[T any] struct {
    Data       T
    Attributes map[string]any
    a          *Acking
}

// Type alias for pub/sub
type Message = TypedMessage[[]byte]
```

While this provides compile-time type safety, it creates several problems:

### 1. Composable Pipeline Complexity

`NewProcessPipe` and other pipe constructors require explicit generic parameters:

```go
// Current - verbose and hard to compose
pipe1 := gopipe.NewProcessPipe[OrderCommand, OrderEvent](handleOrder)
pipe2 := gopipe.NewProcessPipe[OrderEvent, ShippingCommand](handleShipping)
// Can't easily chain without type gymnastics
```

### 2. Type Erasure at Boundaries

At system boundaries (HTTP, Kafka, etc.), data is always `[]byte`. The generic parameter adds no value:

```go
// Receiver returns Message (TypedMessage[[]byte])
msgs, _ := receiver.Receive(ctx)

// Handler needs to unmarshal manually
for msg := range msgs {
    var order Order
    json.Unmarshal(msg.Data, &order)  // Type safety lost anyway
}
```

### 3. Internal Routing Friction

Building internal message flows requires manual type conversion:

```go
// Can't route TypedMessage[OrderEvent] to TypedMessage[ShippingCommand] handler
// without explicit conversion
```

### 4. CQRS Handler Complexity

Handlers must deal with both generic types and byte serialization:

```go
// Current CQRS handler - marshaler handles conversion
type CommandHandler[Cmd, Evt any] struct {
    // Complex interaction between generics and marshaler
}
```

## Decision

Remove generics from Message, using `Data any` instead:

### 1. Unified Message Type

```go
// Message represents a CloudEvents-compliant message
type Message struct {
    Data       any            // Go type internally, serialized at boundaries
    Attributes Attributes     // CloudEvents attributes (mandatory)
    a          *Acking        // Acknowledgment coordination
}
```

### 2. Type Safety via ContentType

Instead of compile-time generics, use runtime type safety based on `datacontenttype`:

```go
// DataContentType determines serialization and can imply Go type
const (
    ContentTypeJSON        = "application/json"
    ContentTypeJSONOrder   = "application/json; type=Order"
    ContentTypeProtobuf    = "application/protobuf"
)

// Type assertion at handler boundary
func (h *OrderHandler) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
    order, ok := msg.Data.(Order)
    if !ok {
        return nil, fmt.Errorf("expected Order, got %T", msg.Data)
    }
    // Type-safe handling
}
```

### 3. Type Registry for Deserialization

Register Go types for automatic deserialization:

```go
// TypeRegistry maps event types to Go types
type TypeRegistry struct {
    types map[string]reflect.Type  // type attribute -> Go type
}

// Register a type for automatic deserialization
func (r *TypeRegistry) Register(eventType string, prototype any) {
    r.types[eventType] = reflect.TypeOf(prototype)
}

// Example registration
registry := NewTypeRegistry()
registry.Register("order.created", Order{})
registry.Register("order.shipped", ShippingEvent{})
```

### 4. Simplified Pipes

Without generics, pipes become simpler:

```go
// Before (generic)
func NewProcessPipe[In, Out any](
    handle func(context.Context, In) ([]Out, error),
) Pipe[In, Out]

// After (non-generic)
func NewProcessPipe(
    handle func(context.Context, *Message) ([]*Message, error),
) Pipe
```

### 5. Type-Safe Handlers via Wrapper

Provide type-safe wrappers for handlers that need specific types:

```go
// TypedHandler wraps a typed function as a generic Handler
func TypedHandler[In, Out any](
    handle func(context.Context, In) ([]Out, error),
    opts ...TypedHandlerOption,
) Handler {
    return &typedHandler[In, Out]{
        handle:     handle,
        inType:     reflect.TypeOf((*In)(nil)).Elem(),
        outType:    reflect.TypeOf((*Out)(nil)).Elem(),
        marshaler:  DefaultMarshaler,
    }
}

// Usage - clean and composable
router.AddHandler(TypedHandler(handleOrder))
router.AddHandler(TypedHandler(handleShipping))
```

### 6. Message Construction

```go
// Create message with Go type
func New(data any, attrs Attributes) (*Message, error) {
    if err := ValidateCloudEvents(attrs); err != nil {
        return nil, err
    }
    return &Message{Data: data, Attributes: attrs}, nil
}

// Example
order := Order{ID: "123", Amount: 100}
msg, _ := message.New(order, message.Attributes{
    message.AttrID:              uuid.NewString(),
    message.AttrSource:          "/orders",
    message.AttrSpecVersion:     "1.0",
    message.AttrType:            "order.created",
    message.AttrDataContentType: "application/json",
})
```

### 7. Data Access Helpers

```go
// DataAs performs type assertion with error
func (m *Message) DataAs(target any) error {
    rv := reflect.ValueOf(target)
    if rv.Kind() != reflect.Ptr {
        return errors.New("target must be a pointer")
    }

    dataVal := reflect.ValueOf(m.Data)
    if !dataVal.Type().AssignableTo(rv.Elem().Type()) {
        return fmt.Errorf("cannot assign %T to %T", m.Data, target)
    }

    rv.Elem().Set(dataVal)
    return nil
}

// Usage
var order Order
if err := msg.DataAs(&order); err != nil {
    return err
}
```

## Rationale

1. **Simplicity**: Single message type, no generic complexity
2. **Composability**: Pipes and handlers work with unified `*Message`
3. **Internal Type Safety**: Go types preserved within the system
4. **Boundary Serialization**: `[]byte` conversion only at system edges
5. **CloudEvents Alignment**: `type` and `datacontenttype` drive behavior
6. **Runtime Flexibility**: Can route any message type through any handler

## Consequences

### Positive

- Dramatically simpler pipe composition
- Internal routing without type conversion
- Type registry enables automatic deserialization
- Handlers can work with any message type
- Cleaner API surface

### Negative

- Loss of compile-time type safety (mitigated by TypedHandler wrapper)
- Runtime type assertions required
- Breaking change: removes `TypedMessage[T]`
- Existing code using generics needs migration

### Migration Guide

```go
// Old code - TypedMessage generic
type OrderPipe = gopipe.Pipe[TypedMessage[Order], TypedMessage[ShippingCommand]]
pipe := gopipe.NewProcessPipe[TypedMessage[Order], TypedMessage[ShippingCommand]](
    func(ctx context.Context, msg TypedMessage[Order]) ([]TypedMessage[ShippingCommand], error) {
        // ...
    },
)

// New code - unified Message with typed wrapper
pipe := gopipe.NewProcessPipe(
    message.TypedHandler(func(ctx context.Context, order Order) ([]ShippingCommand, error) {
        // Type-safe without explicit generics everywhere
    }),
)

// Or explicit type assertion
pipe := gopipe.NewProcessPipe(
    func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        var order Order
        if err := msg.DataAs(&order); err != nil {
            return nil, err
        }
        // Process order...
    },
)
```

## Links

- [ADR 0004: Dual Message Types](0004-dual-message-types.md) (superseded)
- [ADR 0019: CloudEvents Mandatory](0019-cloudevents-mandatory.md)
- [ADR 0021: ContentType Serialization](0021-contenttype-serialization.md)
- [Feature 10: Non-Generic Message](../features/10-non-generic-message.md)
- [CloudEvents Standardization Plan](../plans/cloudevents-standardization.md)

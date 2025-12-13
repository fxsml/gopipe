# Feature: Non-Generic Message Type

**Package:** `message`
**Status:** Proposed
**Related ADRs:**
- [ADR 0020](../adr/0020-non-generic-message.md) - Non-Generic Message Type
- [ADR 0004](../adr/0004-dual-message-types.md) - Dual Message Types (superseded)

## Summary

Remove generics from Message, changing `Data` from `[]byte` to `any`. This simplifies pipeline composition, enables internal type-safe routing, and eliminates verbose generic parameters.

## Motivation

- Simplify pipe composition (no more `Pipe[In, Out]` everywhere)
- Enable internal routing with Go types
- Let `type` and `datacontenttype` drive behavior instead of compile-time generics
- Support flexible internal data flow

## Implementation

### Unified Message Type

```go
// Message represents a CloudEvents-compliant message
type Message struct {
    Data       any        // Go type internally, []byte at boundaries
    Attributes Attributes // CloudEvents attributes (mandatory)
    a          *Acking    // Acknowledgment coordination
}
```

### Type Assertion Helpers

```go
// DataAs performs type assertion to target pointer
func (m *Message) DataAs(target any) error {
    rv := reflect.ValueOf(target)
    if rv.Kind() != reflect.Ptr {
        return errors.New("target must be a pointer")
    }

    dataVal := reflect.ValueOf(m.Data)
    targetType := rv.Elem().Type()

    if !dataVal.Type().AssignableTo(targetType) {
        return fmt.Errorf("cannot assign %T to %T", m.Data, target)
    }

    rv.Elem().Set(dataVal)
    return nil
}

// MustDataAs panics on failure (for tests)
func (m *Message) MustDataAs(target any) {
    if err := m.DataAs(target); err != nil {
        panic(err)
    }
}

// IsDataType checks if Data is of given type
func (m *Message) IsDataType(prototype any) bool {
    return reflect.TypeOf(m.Data) == reflect.TypeOf(prototype)
}
```

### Type Registry

```go
// TypeRegistry maps event types to Go types
type TypeRegistry struct {
    mu    sync.RWMutex
    types map[string]reflect.Type
}

func NewTypeRegistry() *TypeRegistry {
    return &TypeRegistry{types: make(map[string]reflect.Type)}
}

// Register type for event type
func (r *TypeRegistry) Register(eventType string, prototype any) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.types[eventType] = reflect.TypeOf(prototype)
}

// RegisterAll registers multiple types
func (r *TypeRegistry) RegisterAll(types map[string]any) {
    for et, proto := range types {
        r.Register(et, proto)
    }
}

// Lookup returns Go type for event type
func (r *TypeRegistry) Lookup(eventType string) (reflect.Type, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    t, ok := r.types[eventType]
    return t, ok
}

// NewInstance creates new instance of registered type
func (r *TypeRegistry) NewInstance(eventType string) (any, bool) {
    t, ok := r.Lookup(eventType)
    if !ok {
        return nil, false
    }
    return reflect.New(t).Interface(), true
}
```

### Typed Handler Wrapper

```go
// TypedHandler wraps a typed function as a generic Handler
func TypedHandler[In, Out any](
    handle func(context.Context, In) ([]Out, error),
    opts ...TypedHandlerOption,
) Handler {
    return &typedHandler[In, Out]{
        handle:  handle,
        inType:  reflect.TypeOf((*In)(nil)).Elem(),
        outType: reflect.TypeOf((*Out)(nil)).Elem(),
        config:  applyTypedHandlerOptions(opts...),
    }
}

type typedHandler[In, Out any] struct {
    handle  func(context.Context, In) ([]Out, error)
    inType  reflect.Type
    outType reflect.Type
    config  typedHandlerConfig
}

func (h *typedHandler[In, Out]) Handle(ctx context.Context, msg *Message) ([]*Message, error) {
    // Type assert input
    in, ok := msg.Data.(In)
    if !ok {
        return nil, fmt.Errorf("expected %v, got %T", h.inType, msg.Data)
    }

    // Call typed handler
    outs, err := h.handle(ctx, in)
    if err != nil {
        return nil, err
    }

    // Convert outputs to messages
    msgs := make([]*Message, len(outs))
    for i, out := range outs {
        msgs[i] = h.config.messageBuilder.Build(out)
    }
    return msgs, nil
}

func (h *typedHandler[In, Out]) Match(attrs Attributes) bool {
    // Match by type attribute
    t, _ := attrs[AttrType].(string)
    return h.config.matchType == "" || t == h.config.matchType
}
```

### Simplified Pipes

```go
// Before: Generic pipe
// type Pipe[In, Out any] interface { ... }

// After: Non-generic pipe
type Pipe interface {
    Start(ctx context.Context, in <-chan *Message) <-chan *Message
}

// NewProcessPipe creates a message processing pipe
func NewProcessPipe(
    handle func(context.Context, *Message) ([]*Message, error),
    opts ...PipeOption,
) Pipe
```

## Usage Example

```go
// Type registry setup
registry := message.NewTypeRegistry()
registry.RegisterAll(map[string]any{
    "order.created":    Order{},
    "shipping.request": ShippingCommand{},
})

// Handler with typed wrapper
handleOrder := message.TypedHandler(
    func(ctx context.Context, order Order) ([]ShippingCommand, error) {
        return []ShippingCommand{{OrderID: order.ID}}, nil
    },
    message.WithOutputTopic("shipping"),
    message.WithOutputType("shipping.request"),
)

// Handler with explicit type assertion
handleRaw := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
    var order Order
    if err := msg.DataAs(&order); err != nil {
        return nil, err
    }
    // Process order...
    return nil, nil
}

// Create message with Go type
order := Order{ID: "123", Amount: 100}
msg, _ := message.New(order, message.Attributes{
    message.AttrID:              uuid.NewString(),
    message.AttrSource:          "/orders",
    message.AttrSpecVersion:     "1.0",
    message.AttrType:            "order.created",
    message.AttrDataContentType: "application/json",
})
```

## Files Changed

- `message/message.go` - Change Data to `any`, update constructors
- `message/registry.go` - New type registry
- `message/typed_handler.go` - Typed handler wrapper
- `message/helpers.go` - DataAs and other helpers
- `pipe.go` - Simplify Pipe interface (remove generics)
- `processor.go` - Update processor for non-generic messages
- All handlers and pipes - Migration to new interface

## Migration Guide

```go
// Old: Generic TypedMessage
type OrderPipe = gopipe.Pipe[message.TypedMessage[Order], message.TypedMessage[ShippingCommand]]

// New: Use TypedHandler wrapper
pipe := gopipe.NewProcessPipe(
    message.TypedHandler(func(ctx context.Context, order Order) ([]ShippingCommand, error) {
        return []ShippingCommand{{OrderID: order.ID}}, nil
    }),
)

// Or explicit assertion
pipe := gopipe.NewProcessPipe(
    func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        var order Order
        if err := msg.DataAs(&order); err != nil {
            return nil, err
        }
        // ...
    },
)
```

## Related Features

- [09-cloudevents-mandatory](09-cloudevents-mandatory.md) - Prerequisite
- [11-contenttype-serialization](11-contenttype-serialization.md) - Depends on this

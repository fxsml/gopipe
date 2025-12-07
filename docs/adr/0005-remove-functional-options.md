# ADR 0005: Remove Functional Options from Message Construction

**Date:** 2025-12-07
**Status:** Accepted

## Context

The current message API uses functional options with generic type parameters:

```go
msg := message.New([]byte("data"),
    message.WithID[[]byte]("msg-1"),
    message.WithAcking[[]byte](ack, nack),
    message.WithSubject[[]byte]("orders.created"),
    message.WithDeadline[[]byte](deadline),
    message.WithProperty[[]byte]("key", "value"),
)
```

### Problems with Current Approach

1. **Generic Noise**: Every option requires a type parameter `[T]`, creating visual clutter
2. **Redundant Type Information**: The type parameter is redundant since it's inferred from the message
3. **Ceremony for Simple Cases**: Common use case ([]byte payload with properties) requires excessive boilerplate
4. **Inconsistent with Go Conventions**: Most Go messaging libraries use simple constructors

### Example: Watermill vs gopipe

**Watermill** (simple):
```go
msg := message.NewMessage("msg-1", []byte("data"))
msg.Metadata.Set("subject", "orders.created")
```

**Current gopipe** (verbose):
```go
msg := message.New([]byte("data"),
    message.WithID[[]byte]("msg-1"),
    message.WithSubject[[]byte]("orders.created"),
)
```

## Decision

**Remove functional options entirely** and use a simple 3-parameter constructor:

```go
func New[T any](payload T, props Properties, a *acking) *TypedMessage[T]
```

Introduce `Properties` as a type alias for better readability:

```go
type Properties map[string]any
```

### New API

```go
// Create properties
props := message.Properties{
    message.PropID:      "msg-1",
    message.PropSubject: "orders.created",
    "custom-key":        "custom-value",
}

// Create acking
acking := message.NewAcking(ack, nack)

// Create message
msg := message.New([]byte("data"), props, acking)
```

### Helper Functions

Provide convenience helpers for common cases:

```go
// No acking needed
msg := message.New(data, props, nil)

// No properties needed
msg := message.New(data, nil, acking)

// Minimal message
msg := message.New(data, nil, nil)
```

Provide acking constructor:

```go
func NewAcking(ack func(), nack func(error)) *acking
```

## Rationale

### Benefits

1. **Simpler API**: No type parameters on options, cleaner code
2. **More Explicit**: Properties are visible at construction time
3. **Better for Message Alias**: Non-generic `Message` usage is much cleaner
4. **Flexible**: Can create properties map separately and reuse
5. **Go-like**: Follows standard Go patterns (struct initialization)

### Comparison

**Before** (functional options):
```go
message.New([]byte("data"),
    message.WithID[[]byte]("msg-1"),
    message.WithSubject[[]byte]("orders"),
    message.WithAcking[[]byte](ack, nack),
)
```

**After** (simple constructor):
```go
message.New([]byte("data"), message.Properties{
    message.PropID:      "msg-1",
    message.PropSubject: "orders",
}, message.NewAcking(ack, nack))
```

For the common non-generic `Message` type:
```go
// Much cleaner without [[]byte] everywhere!
props := message.Properties{
    message.PropID:      "msg-1",
    message.PropSubject: "orders.created",
}
msg := message.New(data, props, message.NewAcking(ack, nack))
```

## Breaking Changes

### Removed APIs

All functional options removed:
- ❌ `WithDeadline[T]()`
- ❌ `WithAcking[T]()`
- ❌ `WithProperty[T]()`
- ❌ `WithProperties[T]()`
- ❌ `WithID[T]()`
- ❌ `WithCorrelationID[T]()`
- ❌ `WithCreatedAt[T]()`
- ❌ `WithSubject[T]()`
- ❌ `WithContentType[T]()`

### New APIs

**Constructor**:
```go
func New[T any](payload T, props Properties, a *acking) *TypedMessage[T]
```

**Type Alias**:
```go
type Properties map[string]any
```

**Acking Constructor**:
```go
func NewAcking(ack func(), nack func(error)) *acking
```

**Property Access** (unchanged):
```go
func IDProps(m Properties) (string, bool)
func CorrelationIDProps(m Properties) (string, bool)
func CreatedAtProps(m Properties) (time.Time, bool)
func SubjectProps(m Properties) (string, bool)
func ContentTypeProps(m Properties) (string, bool)
```

## Migration Guide

### Simple Message Creation

**Before**:
```go
msg := message.New(data,
    message.WithID[[]byte]("msg-1"),
)
```

**After**:
```go
msg := message.New(data, message.Properties{
    message.PropID: "msg-1",
}, nil)
```

### With Acknowledgment

**Before**:
```go
msg := message.New(data,
    message.WithAcking[[]byte](ack, nack),
)
```

**After**:
```go
msg := message.New(data, nil, message.NewAcking(ack, nack))
```

### Full Example

**Before**:
```go
msg := message.New([]byte("data"),
    message.WithID[[]byte]("msg-1"),
    message.WithSubject[[]byte]("orders.created"),
    message.WithAcking[[]byte](ack, nack),
    message.WithDeadline[[]byte](deadline),
    message.WithProperty[[]byte]("tenant", "acme"),
)
```

**After**:
```go
props := message.Properties{
    message.PropID:       "msg-1",
    message.PropSubject:  "orders.created",
    message.PropDeadline: deadline,
    "tenant":             "acme",
}
msg := message.New([]byte("data"), props, message.NewAcking(ack, nack))
```

### Reusable Properties

The new approach makes it easier to reuse property sets:

```go
baseProps := message.Properties{
    message.PropSubject: "orders.created",
    "tenant":            "acme",
}

// Create multiple messages with shared properties
msg1 := message.New(data1, baseProps, ack1)
msg2 := message.New(data2, baseProps, ack2)
```

## Consequences

### Positive

1. **Cleaner pub/sub code**: No `[[]byte]` type parameters scattered everywhere
2. **More explicit**: Properties visible in one place
3. **Better composability**: Properties can be built incrementally
4. **Simpler implementation**: Less code to maintain

### Negative

1. **Breaking change**: All existing code must be updated
2. **Less discoverable**: IDE autocomplete won't suggest property setters
3. **More verbose for single property**: Need to create map for one property

### Mitigations

- Provide clear migration examples
- Update all examples and tests
- Document common patterns in README

## Examples

### Minimal Message (No Properties, No Acking)

```go
msg := message.New([]byte("data"), nil, nil)
```

### Message with Properties Only

```go
msg := message.New([]byte("data"), message.Properties{
    message.PropID:      "msg-1",
    message.PropSubject: "orders",
}, nil)
```

### Message with Acking Only

```go
msg := message.New([]byte("data"), nil, message.NewAcking(
    func() { log.Println("acked") },
    func(err error) { log.Println("nacked:", err) },
))
```

### Complete Message

```go
props := message.Properties{
    message.PropID:          "msg-1",
    message.PropCorrelationID: "order-123",
    message.PropSubject:     "orders.created",
    message.PropCreatedAt:   time.Now(),
    message.PropDeadline:    time.Now().Add(30 * time.Second),
    "tenant":                "acme-corp",
    "priority":              "high",
}

acking := message.NewAcking(
    func() { broker.Ack(msgID) },
    func(err error) { broker.Nack(msgID, err) },
)

msg := message.New(orderData, props, acking)
```

## References

- [ADR 0001: Public Message Fields](./0001-public-message-fields.md)
- [ADR 0002: Remove Properties Thread-Safety](./0002-remove-properties-thread-safety.md)
- [ADR 0004: Dual Message Types](./0004-dual-message-types.md)
- [Watermill Message API](https://watermill.io/docs/messages-router/)

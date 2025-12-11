# Feature: Message Core Refactoring

**Package:** `message`
**Status:** ✅ Implemented
**Related ADRs:**
- [ADR 0001](../adr/0001-public-message-fields.md) - Public Message Fields
- [ADR 0002](../adr/0002-remove-properties-thread-safety.md) - Remove Properties Thread Safety
- [ADR 0003](../adr/0003-remove-noisy-properties.md) - Remove Noisy Properties
- [ADR 0004](../adr/0004-dual-message-types.md) - Dual Message Types
- [ADR 0005](../adr/0005-remove-functional-options.md) - Remove Functional Options
- [ADR 0017](../adr/0017-message-acknowledgment.md) - Message Acknowledgment

## Summary

Refactors the core `Message` type to be simpler, more idiomatic Go, and aligned with CloudEvents specification. Removes complexity while improving usability.

## Key Changes

### 1. Public Fields (Breaking)
```go
// Before
msg.Data()
msg.Attributes()

// After
msg.Data
msg.Attributes
```

### 2. Simplified Attributes
```go
type Attributes map[string]any
type Message struct {
    Data       []byte
    Attributes Attributes
}
```

**Removed noisy properties:** Replaced complex property getters with direct map access.

### 3. Dual Message Types
```go
// Generic typed message
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
}

// Non-generic raw message
type Message struct {
    Data       []byte
    Attributes Attributes
}
```

### 4. Constructor Simplification
```go
// Before (functional options)
msg := message.New(data, message.WithID("123"), message.WithRetry(3))

// After (direct construction)
msg := &message.Message{
    Data: data,
    Attributes: message.Attributes{
        message.AttrID: "123",
    },
}
```

### 5. CloudEvents Alignment
Attributes now follow CloudEvents v1.0.2 naming:
- `subject` - Message subject/routing key
- `type` - Event/message type
- `source` - Event source identifier
- `id` - Unique message identifier
- `time` - Message timestamp
- `datacontenttype` - Content type of data

### 6. Message Acknowledgment
```go
// Automatic acknowledgment
msg := message.New(data)
// Manual acknowledgment
msg := message.NewWithAcking(data)
defer msg.Ack()
```

## Files Changed

- `message/message.go` - Core Message and TypedMessage types
- `message/message_test.go` - Updated tests
- `message/attributes.go` - Attributes helpers and constants

## Migration Guide

```go
// Old code
msg := message.New(data,
    message.WithID(id),
    message.WithSubject(subject),
)
value := msg.Data()
attrs := msg.Attributes()

// New code
msg := &message.Message{
    Data: data,
    Attributes: message.Attributes{
        message.AttrID: id,
        message.AttrSubject: subject,
    },
}
value := msg.Data
attrs := msg.Attributes
```

## Breaking Changes

- Public `Data` and `Attributes` fields (no accessor methods)
- No functional options
- Thread-safe property access removed (direct map access)
- CloudEvents-aligned attribute names

## Related Features

- [03-message-pubsub](03-message-pubsub.md) - Uses simplified Message type
- [04-message-router](04-message-router.md) - Routes based on Attributes

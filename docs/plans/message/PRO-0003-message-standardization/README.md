# PRO-0003: Message Standardization

**Status:** Proposed
**Priority:** High
**Depends On:** PRO-0002-migration
**Related ADRs:** 0019, 0020, 0021
**Related Features:** 09, 10, 11

## Overview

This layer makes CloudEvents a mandatory specification for all gopipe messages, removes generics from the Message type, and implements automatic serialization at system boundaries.

## Goals

1. Enforce CloudEvents required attributes on all messages
2. Keep `TypedMessage[T]` for non-messaging pipelines (no validation)
3. Define `Message = TypedMessage[any]` for CloudEvents messaging
4. Implement automatic serialization/deserialization at boundaries
5. Provide TypeRegistry for type-safe deserialization

## Prerequisites

Layer 0 must be complete:
- [ ] ProcessorConfig implemented
- [ ] Middleware package consolidated
- [ ] Subscriber interface defined

## Sub-Tasks

### Task 1.1: CloudEvents Validation

**Goal:** All messages MUST have valid CloudEvents required attributes

**Required Attributes:**
- `id` - Unique identifier (string)
- `source` - URI reference (string)
- `specversion` - Always "1.0" (string)
- `type` - Event type identifier (string)

**Implementation:**
```go
// message/validation.go
func ValidateCloudEvents(attrs Attributes) error {
    required := []string{AttrID, AttrSource, AttrSpecVersion, AttrType}
    for _, attr := range required {
        if v, ok := attrs[attr].(string); !ok || v == "" {
            return fmt.Errorf("missing or empty CloudEvents attribute: %s", attr)
        }
    }
    if sv := attrs.SpecVersion(); sv != "1.0" {
        return fmt.Errorf("invalid specversion: %s", sv)
    }
    return nil
}
```

**Files to Create/Modify:**
- `message/validation.go` (new) - ValidateCloudEvents function
- `message/message.go` (modify) - New() returns error
- `message/builder.go` (new) - Builder with auto-generated defaults

**⚠️ Breaking Change:**
```go
// Current
msg := message.New(data, attrs)  // Always succeeds

// New
msg, err := message.New(data, attrs)  // Returns error if invalid
if err != nil { return err }

// Or use MustNew for tests
msg := message.MustNew(data, attrs)  // Panics on error
```

---

### Task 1.2: TypedMessage and Message Separation

**Goal:** Keep TypedMessage for pipelines, use Message for CloudEvents messaging

**Current:**
```go
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
    a          *Acking
}
type Message = TypedMessage[[]byte]  // DEPRECATED
```

**Target:**
```go
// TypedMessage stays - useful for non-messaging pipelines
// New constructor without error return (current behavior preserved)
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
    a          *Acking
}

// Deprecated: NewTyped - use message.NewTyped instead
func New[T any](data T, attrs Attributes) *TypedMessage[T]  // DEPRECATED

// NewTyped creates a typed message (no validation, for pipelines)
func NewTyped[T any](data T, attrs Attributes) *TypedMessage[T]

// Message is TypedMessage[any] for CloudEvents messaging
type Message = TypedMessage[any]

// New creates a Message with CloudEvents validation (returns error)
func New(data any, attrs Attributes) (*Message, error)

// MustNew panics on validation error (for tests)
func MustNew(data any, attrs Attributes) *Message
```

**Type Access:**
```go
// Direct type assertion
order, ok := msg.Data.(Order)

// Or helper method
var order Order
if err := msg.DataAs(&order); err != nil {
    return err
}
```

**Files to Modify:**
- `message/message.go` - Add NewTyped, change Message alias, deprecate old New
- `message/helpers.go` (new) - DataAs, MustDataAs helpers

**Deprecations:**
```go
// Mark as deprecated in code
// Deprecated: Use NewTyped for pipelines or New for CloudEvents messages.
func New[T any](data T, attrs Attributes) *TypedMessage[T]

// Deprecated: Message is now TypedMessage[any]. Use *Message for CloudEvents.
type Message = TypedMessage[[]byte]
```

**Migration:**
```go
// Pipeline (no CloudEvents) - use NewTyped
msg := message.NewTyped(order, nil)  // No validation

// CloudEvents messaging - use New (validates)
msg, err := message.New(order, message.Attributes{
    message.AttrType:   "order.created",
    message.AttrSource: "/orders",
    // ...
})
```

---

### Task 1.3: Type Registry

**Goal:** Map event types to Go types for automatic deserialization

**Implementation:**
```go
// message/registry.go
type TypeRegistry struct {
    mu    sync.RWMutex
    types map[string]reflect.Type
}

func (r *TypeRegistry) Register(eventType string, prototype any) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.types[eventType] = reflect.TypeOf(prototype)
}

func (r *TypeRegistry) Lookup(eventType string) (reflect.Type, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    t, ok := r.types[eventType]
    return t, ok
}

// Usage
registry := message.NewTypeRegistry()
registry.Register("order.created", Order{})
registry.Register("order.shipped", ShippingEvent{})
```

**Files to Create:**
- `message/registry.go` - TypeRegistry
- `message/registry_test.go` - Tests

---

### Task 1.4: ContentType Serialization

**Goal:** Automatic serialization/deserialization based on DataContentType

**Serializer Interface:**
```go
type Serializer interface {
    Serialize(data any, contentType string) ([]byte, error)
    Deserialize(data []byte, contentType, eventType string) (any, error)
}

type ContentTypeHandler interface {
    ContentType() string
    Serialize(v any) ([]byte, error)
    Deserialize(data []byte, target reflect.Type) (any, error)
}
```

**Built-in Handlers:**
| Content Type | Handler |
|--------------|---------|
| `application/json` | JSON (default) |
| `application/protobuf` | Protocol Buffers |
| `text/plain` | String |
| `application/octet-stream` | Raw bytes |

**Sender Integration:**
```go
// message/sender.go
type SerializingSender struct {
    raw        RawSender
    serializer Serializer
}

func (s *SerializingSender) Send(ctx context.Context, msgs []*Message) error {
    for _, msg := range msgs {
        // Skip if already []byte
        if _, ok := msg.Data.([]byte); ok {
            continue
        }
        ct := msg.DataContentType()
        if ct == "" {
            ct = "application/json"
        }
        data, err := s.serializer.Serialize(msg.Data, ct)
        if err != nil {
            return err
        }
        msg.Data = data
    }
    return s.raw.Send(ctx, msgs)
}
```

**Receiver Integration:**
```go
// message/receiver.go
type DeserializingReceiver struct {
    raw        RawReceiver
    serializer Serializer
}

func (r *DeserializingReceiver) Receive(ctx context.Context) (<-chan *Message, error) {
    rawCh, _ := r.raw.Receive(ctx)
    out := make(chan *Message)
    go func() {
        defer close(out)
        for msg := range rawCh {
            data := msg.Data.([]byte)
            v, err := r.serializer.Deserialize(data, msg.DataContentType(), msg.Type())
            if err != nil {
                msg.Nack(err)
                continue
            }
            msg.Data = v
            out <- msg
        }
    }()
    return out, nil
}
```

**Files to Create:**
- `message/serializer.go` - Serializer interface
- `message/serializer_json.go` - JSON handler
- `message/serializer_proto.go` - Protobuf handler
- `message/sender_serializing.go` - SerializingSender
- `message/receiver_deserializing.go` - DeserializingReceiver

---

### Task 1.5: Message Builder

**Goal:** Convenient message construction with auto-generated defaults

**Implementation:**
```go
// message/builder.go
type Builder struct {
    source      string
    idGenerator func() string
}

func NewBuilder(source string) *Builder {
    return &Builder{
        source:      source,
        idGenerator: uuid.NewString,
    }
}

func (b *Builder) Build(data any, msgType string, opts ...BuildOption) (*Message, error) {
    attrs := Attributes{
        AttrID:          b.idGenerator(),
        AttrSource:      b.source,
        AttrSpecVersion: "1.0",
        AttrType:        msgType,
    }
    for _, opt := range opts {
        opt(attrs)
    }
    return New(data, attrs)
}

// Build options
func WithDestination(dest string) BuildOption {
    return func(attrs Attributes) { attrs[AttrDestination] = dest }
}

func WithTopic(topic string) BuildOption {
    return func(attrs Attributes) { attrs[AttrTopic] = topic }
}
```

---

## Data Flow

```
External System                    gopipe Internal                    External System
     │                                   │                                  │
     ▼                                   │                                  │
┌─────────┐                              │                                  │
│ []byte  │                              │                                  │
└────┬────┘                              │                                  │
     │ DeserializingReceiver             │                                  │
     │ (deserialize by ContentType)      │                                  │
     ▼                                   ▼                                  │
┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐                   │
│ Message │ -> │ Handler │ -> │ Message │ -> │ Handler │                   │
│ Data:   │    └─────────┘    │ Data:   │    └─────────┘                   │
│ GoType  │                   │ GoType  │                                  │
└─────────┘                   └─────────┘                                  │
                                   │                                       │
                                   │ SerializingSender                     │
                                   │ (serialize by ContentType)            │
                                   ▼                                       ▼
                              ┌─────────┐
                              │ []byte  │
                              └─────────┘
```

## Implementation Order

```
1. CloudEvents Validation ────────────────────┐
                                              │
2. Non-Generic Message ───────────────────────┤
                                              │
3. Type Registry ─────────────────────────────┼──► 4. ContentType Serialization
                                              │
                                              └──► 5. Message Builder
```

**Recommended PR Sequence:**
1. **PR 1:** CloudEvents Validation + Non-Generic Message
2. **PR 2:** Type Registry
3. **PR 3:** ContentType Serialization
4. **PR 4:** Message Builder

## Validation Checklist

Before marking Layer 1 complete:

- [ ] `New()` returns error when CE attributes missing
- [ ] `MustNew()` panics with clear error message
- [ ] `NewTyped[T]()` works without validation (for pipelines)
- [ ] `TypedMessage[T]` preserved for non-messaging use
- [ ] `Message = TypedMessage[any]` for CloudEvents
- [ ] Old `New[T]()` marked deprecated
- [ ] `DataAs()` helper works with type assertions
- [ ] TypeRegistry maps event types to Go types
- [ ] SerializingSender serializes Go types to bytes
- [ ] DeserializingReceiver deserializes bytes to Go types
- [ ] Builder auto-generates id, specversion
- [ ] All tests pass
- [ ] CHANGELOG updated

## Related Documentation

- [ADR 0019: CloudEvents Mandatory](../adr/0019-cloudevents-mandatory.md)
- [ADR 0020: Non-Generic Message](../adr/0020-non-generic-message.md)
- [ADR 0021: ContentType Serialization](../adr/0021-contenttype-serialization.md)
- [Feature 09: CloudEvents Mandatory](../features/09-cloudevents-mandatory.md)
- [Feature 10: Non-Generic Message](../features/10-non-generic-message.md)
- [Feature 11: ContentType Serialization](../features/11-contenttype-serialization.md)

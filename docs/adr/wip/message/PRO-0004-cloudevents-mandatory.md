# ADR 0019: CloudEvents Mandatory Specification

**Date:** 2025-12-13
**Status:** Proposed
**Supersedes:** None

## Context

gopipe currently treats CloudEvents attributes as optional. While ADR 0018 aligned terminology with CloudEvents spec v1.0.2, it did not enforce that messages MUST contain the required CloudEvents attributes.

This leads to:

1. **Inconsistent behavior**: Some messages have full CE attributes, others have partial or none
2. **Runtime errors**: Code expecting `type` or `source` fails when attributes are missing
3. **Serialization issues**: CloudEvents HTTP/JSON encoding fails with incomplete attributes
4. **No validation**: Invalid messages propagate through the system until they cause downstream failures

The CloudEvents specification defines **required attributes** that MUST be present:
- `id` - Unique identifier
- `source` - URI reference identifying the event source
- `specversion` - CloudEvents version (always "1.0")
- `type` - Event type identifier

## Decision

Make CloudEvents a **mandatory specification** for all gopipe messages:

### 1. Required Attributes Validation

All messages MUST have the four required CloudEvents attributes. Validation occurs at message creation:

```go
// RequiredAttributes defines the CloudEvents required attributes
var RequiredAttributes = []string{
    AttrID,          // "id"
    AttrSource,      // "source"
    AttrSpecVersion, // "specversion"
    AttrType,        // "type"
}

// ValidateCloudEvents checks that required CE attributes are present and valid
func ValidateCloudEvents(attrs Attributes) error {
    for _, attr := range RequiredAttributes {
        v, ok := attrs[attr]
        if !ok {
            return fmt.Errorf("missing required CloudEvents attribute: %s", attr)
        }
        if v == nil || v == "" {
            return fmt.Errorf("empty required CloudEvents attribute: %s", attr)
        }
    }

    // Validate specversion
    if sv, _ := attrs[AttrSpecVersion].(string); sv != "1.0" {
        return fmt.Errorf("invalid specversion: %s (expected 1.0)", sv)
    }

    return nil
}
```

### 2. Message Construction with Validation

Update `New()` to validate and return error:

```go
// New creates a validated Message with required CloudEvents attributes
func New(data []byte, attrs Attributes) (*Message, error) {
    if err := ValidateCloudEvents(attrs); err != nil {
        return nil, err
    }
    return &Message{Data: data, Attributes: attrs}, nil
}

// MustNew panics if validation fails (for tests/initialization)
func MustNew(data []byte, attrs Attributes) *Message {
    msg, err := New(data, attrs)
    if err != nil {
        panic(err)
    }
    return msg
}
```

### 3. Builder Pattern for Convenience

Provide a builder that auto-generates defaults:

```go
// Builder constructs messages with CloudEvents defaults
type Builder struct {
    source      string
    defaultType string
    idGenerator func() string
}

// NewBuilder creates a builder with required source
func NewBuilder(source string) *Builder {
    return &Builder{
        source:      source,
        idGenerator: uuid.NewString,
    }
}

// Build creates a message with auto-generated id and specversion
func (b *Builder) Build(data []byte, msgType string, extraAttrs ...Attributes) *Message {
    attrs := Attributes{
        AttrID:          b.idGenerator(),
        AttrSource:      b.source,
        AttrSpecVersion: "1.0",
        AttrType:        msgType,
    }
    // Merge extra attributes
    for _, extra := range extraAttrs {
        for k, v := range extra {
            attrs[k] = v
        }
    }
    return MustNew(data, attrs)
}
```

### 4. Automatic Type from Handler Registration

When using CQRS handlers, auto-derive `type` from registered command/event type:

```go
// CommandHandler automatically sets type from command struct name
type CommandHandler[Cmd, Evt any] struct {
    cmdType string  // Derived from reflect.TypeOf(Cmd).Name()
}

func NewCommandHandler[Cmd, Evt any](
    handle func(context.Context, Cmd) ([]Evt, error),
    marshaler CommandMarshaler,
) *CommandHandler[Cmd, Evt] {
    return &CommandHandler[Cmd, Evt]{
        cmdType: reflect.TypeOf((*Cmd)(nil)).Elem().Name(),
        // ...
    }
}
```

### 5. gopipe Extensions

These attributes are **gopipe-specific extensions**, not part of CloudEvents core:

| Extension | Purpose | Behavior |
|-----------|---------|----------|
| `topic` | Pub/sub routing | Used for routing, NOT serialized to external systems |
| `correlationid` | Distributed tracing | Serialized as CE extension |
| `deadline` | Processing timeout | Internal only |

The `topic` attribute aligns with CloudEvents guidance that routing is protocol-level:
> "Routing information is not just redundant, it detracts."

### 6. Backward Compatibility

Provide migration utilities:

```go
// Migrate adds required CE attributes if missing
func Migrate(msg *Message, source, msgType string) *Message {
    if msg.Attributes == nil {
        msg.Attributes = make(Attributes)
    }

    if _, ok := msg.Attributes[AttrID]; !ok {
        msg.Attributes[AttrID] = uuid.NewString()
    }
    if _, ok := msg.Attributes[AttrSource]; !ok {
        msg.Attributes[AttrSource] = source
    }
    if _, ok := msg.Attributes[AttrSpecVersion]; !ok {
        msg.Attributes[AttrSpecVersion] = "1.0"
    }
    if _, ok := msg.Attributes[AttrType]; !ok {
        msg.Attributes[AttrType] = msgType
    }

    return msg
}
```

## Rationale

1. **Fail Fast**: Catch invalid messages at creation, not at serialization
2. **Consistent Behavior**: All messages have the same structure
3. **Automatic Type**: Using `type` attribute enables automatic routing and serialization
4. **Standards Compliance**: Full CloudEvents v1.0.2 compliance
5. **Topic Separation**: Keeps routing concern separate from event description

## Consequences

### Positive

- All messages are valid CloudEvents
- Enables automatic routing based on `type`
- Serialization always succeeds
- Clear error messages at point of failure
- Enables future features (schema registry, event catalog)

### Negative

- Breaking change: existing code using `New()` must handle error
- Requires migration for existing messages without CE attributes
- Slightly more verbose message creation

### Migration Guide

```go
// Old code
msg := message.New(data, message.Attributes{
    message.AttrType: "order.created",
})

// New code - Option 1: Handle error
msg, err := message.New(data, message.Attributes{
    message.AttrID:          uuid.NewString(),
    message.AttrSource:      "/orders",
    message.AttrSpecVersion: "1.0",
    message.AttrType:        "order.created",
})
if err != nil {
    return err
}

// New code - Option 2: Use Builder
builder := message.NewBuilder("/orders")
msg := builder.Build(data, "order.created")

// New code - Option 3: MustNew for tests
msg := message.MustNew(data, message.Attributes{
    message.AttrID:          "test-id",
    message.AttrSource:      "/test",
    message.AttrSpecVersion: "1.0",
    message.AttrType:        "test.event",
})
```

## Links

- [CloudEvents Specification v1.0.2](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md)
- [ADR 0018: CloudEvents Terminology](0018-cloudevents-terminology.md)
- [Feature 09: CloudEvents Mandatory](../features/09-cloudevents-mandatory.md)
- [CloudEvents Standardization Plan](../plans/cloudevents-standardization.md)

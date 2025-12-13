# Feature: CloudEvents Mandatory Specification

**Package:** `message`
**Status:** Proposed
**Related ADRs:**
- [ADR 0019](../adr/0019-cloudevents-mandatory.md) - CloudEvents Mandatory

## Summary

Enforce CloudEvents v1.0.2 required attributes on all gopipe messages. This provides validation at message creation, automatic type-based routing, and guaranteed serialization success.

## Motivation

- Catch invalid messages early (fail fast)
- Enable automatic routing based on `type` attribute
- Guarantee CloudEvents serialization always succeeds
- Standardize message structure across the system

## Implementation

### Required Attributes

```go
// RequiredAttributes defines CloudEvents required attributes
var RequiredAttributes = []string{
    AttrID,          // Unique identifier
    AttrSource,      // Event source URI
    AttrSpecVersion, // Always "1.0"
    AttrType,        // Event type
}
```

### Validation Function

```go
// ValidateCloudEvents validates required CE attributes
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

    if sv, _ := attrs[AttrSpecVersion].(string); sv != "1.0" {
        return fmt.Errorf("invalid specversion: %s (expected 1.0)", sv)
    }

    return nil
}
```

### Message Construction

```go
// New creates a validated Message
func New(data []byte, attrs Attributes) (*Message, error) {
    if err := ValidateCloudEvents(attrs); err != nil {
        return nil, err
    }
    return &Message{Data: data, Attributes: attrs}, nil
}

// MustNew panics on validation failure (for tests)
func MustNew(data []byte, attrs Attributes) *Message {
    msg, err := New(data, attrs)
    if err != nil {
        panic(err)
    }
    return msg
}
```

### Builder Pattern

```go
// Builder constructs messages with auto-generated defaults
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

func (b *Builder) Build(data []byte, msgType string, extra ...Attributes) *Message {
    attrs := Attributes{
        AttrID:          b.idGenerator(),
        AttrSource:      b.source,
        AttrSpecVersion: "1.0",
        AttrType:        msgType,
    }
    for _, e := range extra {
        for k, v := range e {
            attrs[k] = v
        }
    }
    return MustNew(data, attrs)
}
```

### Migration Utility

```go
// Migrate adds missing required CE attributes
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

## Usage Example

```go
// Option 1: Explicit attributes
msg, err := message.New(data, message.Attributes{
    message.AttrID:          uuid.NewString(),
    message.AttrSource:      "/orders",
    message.AttrSpecVersion: "1.0",
    message.AttrType:        "order.created",
})
if err != nil {
    return err
}

// Option 2: Builder (recommended)
builder := message.NewBuilder("/orders")
msg := builder.Build(data, "order.created")

// Option 3: Builder with extra attributes
msg := builder.Build(data, "order.created", message.Attributes{
    message.AttrSubject: "orders/123",
    message.AttrTopic:   "orders",
})

// Option 4: MustNew for tests
msg := message.MustNew(data, message.Attributes{
    message.AttrID:          "test-id",
    message.AttrSource:      "/test",
    message.AttrSpecVersion: "1.0",
    message.AttrType:        "test.event",
})
```

## Files Changed

- `message/message.go` - Update `New()` to validate and return error
- `message/validation.go` - New validation functions
- `message/builder.go` - New builder pattern
- `message/migrate.go` - Migration utilities
- `message/message_test.go` - Validation tests
- `message/builder_test.go` - Builder tests

## Testing Strategy

```go
func TestNew_ValidatesRequiredAttributes(t *testing.T) {
    tests := []struct {
        name    string
        attrs   Attributes
        wantErr string
    }{
        {"missing id", Attributes{AttrSource: "/", AttrSpecVersion: "1.0", AttrType: "t"}, "missing required.*id"},
        {"missing source", Attributes{AttrID: "1", AttrSpecVersion: "1.0", AttrType: "t"}, "missing required.*source"},
        {"missing specversion", Attributes{AttrID: "1", AttrSource: "/", AttrType: "t"}, "missing required.*specversion"},
        {"missing type", Attributes{AttrID: "1", AttrSource: "/", AttrSpecVersion: "1.0"}, "missing required.*type"},
        {"invalid specversion", Attributes{AttrID: "1", AttrSource: "/", AttrSpecVersion: "2.0", AttrType: "t"}, "invalid specversion"},
        {"valid", Attributes{AttrID: "1", AttrSource: "/", AttrSpecVersion: "1.0", AttrType: "t"}, ""},
    }
    // ...
}
```

## Related Features

- [10-non-generic-message](10-non-generic-message.md) - Depends on this feature
- [06-message-cloudevents](06-message-cloudevents.md) - Extends CloudEvents support

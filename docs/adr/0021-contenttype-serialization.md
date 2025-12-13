# ADR 0021: ContentType-Based Serialization

**Date:** 2025-12-13
**Status:** Proposed
**Depends on:** ADR 0019, ADR 0020

## Context

With ADR 0020's non-generic Message (Data as `any`), we need a clear strategy for serialization at system boundaries:

### Current State

Serialization is handled inconsistently:
- CQRS marshalers handle serialization per handler
- HTTP broker serializes to CloudEvents format
- IO broker uses JSONL envelope
- No automatic deserialization based on content type

### Problems

1. **Manual Serialization**: Handlers must manually serialize/deserialize
2. **ContentType Ignored**: The `datacontenttype` attribute is informational only
3. **Inconsistent Formats**: Each broker implements its own serialization
4. **Type Registry Disconnected**: No automatic mapping from `type` to Go type

## Decision

Implement ContentType-based automatic serialization at system boundaries:

### 1. Serialization Contract

**Internal**: Messages carry Go types in `Data any`
**External**: Messages carry `[]byte` in `Data` when crossing system boundaries

```go
// Internal message (within gopipe)
msg := &Message{
    Data: Order{ID: "123", Amount: 100},  // Go type
    Attributes: Attributes{
        AttrDataContentType: "application/json",
        AttrType:            "order.created",
        // ...
    },
}

// External message (from/to broker)
msg := &Message{
    Data: []byte(`{"id":"123","amount":100}`),  // Serialized
    Attributes: Attributes{
        AttrDataContentType: "application/json",
        AttrType:            "order.created",
        // ...
    },
}
```

### 2. Serializer Interface

```go
// Serializer handles encoding/decoding based on content type
type Serializer interface {
    // Serialize converts Go type to []byte
    Serialize(data any, contentType string) ([]byte, error)

    // Deserialize converts []byte to registered Go type
    // Uses type attribute to look up Go type in registry
    Deserialize(data []byte, contentType string, eventType string) (any, error)
}

// DefaultSerializer with JSON and Protobuf support
type DefaultSerializer struct {
    registry *TypeRegistry
}
```

### 3. Content Type Handlers

```go
// ContentTypeHandler handles a specific content type
type ContentTypeHandler interface {
    ContentType() string
    Serialize(v any) ([]byte, error)
    Deserialize(data []byte, target reflect.Type) (any, error)
}

// Built-in handlers
var (
    JSONHandler     ContentTypeHandler = &jsonHandler{}
    ProtobufHandler ContentTypeHandler = &protobufHandler{}
    TextHandler     ContentTypeHandler = &textHandler{}
)

// Register custom handlers
serializer.RegisterContentType("application/xml", &xmlHandler{})
```

### 4. Sender Serialization

Senders automatically serialize based on `datacontenttype`:

```go
// Sender interface with automatic serialization
type Sender interface {
    Send(ctx context.Context, msgs []*Message) error
}

// SerializingSender wraps a raw sender with serialization
type SerializingSender struct {
    raw        RawSender           // Sends []byte
    serializer Serializer
}

func (s *SerializingSender) Send(ctx context.Context, msgs []*Message) error {
    for _, msg := range msgs {
        // Skip if already []byte
        if _, ok := msg.Data.([]byte); ok {
            continue
        }

        // Get content type (default to JSON)
        ct := msg.Attributes.DataContentType()
        if ct == "" {
            ct = "application/json"
        }

        // Serialize
        data, err := s.serializer.Serialize(msg.Data, ct)
        if err != nil {
            return fmt.Errorf("serialize %s: %w", msg.Attributes.Type(), err)
        }
        msg.Data = data
    }

    return s.raw.Send(ctx, msgs)
}
```

### 5. Receiver Deserialization

Receivers automatically deserialize based on `type` and `datacontenttype`:

```go
// DeserializingReceiver wraps a raw receiver with deserialization
type DeserializingReceiver struct {
    raw        RawReceiver
    serializer Serializer
}

func (r *DeserializingReceiver) Receive(ctx context.Context) (<-chan *Message, error) {
    rawCh, err := r.raw.Receive(ctx)
    if err != nil {
        return nil, err
    }

    out := make(chan *Message)
    go func() {
        defer close(out)
        for msg := range rawCh {
            // Get bytes
            data, ok := msg.Data.([]byte)
            if !ok {
                // Already deserialized
                out <- msg
                continue
            }

            // Get content type and event type
            ct := msg.Attributes.DataContentType()
            et := msg.Attributes.Type()

            // Deserialize to registered Go type
            v, err := r.serializer.Deserialize(data, ct, et)
            if err != nil {
                // Log error, optionally nack
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

### 6. Type Registry Integration

```go
// TypeRegistry maps event types to Go types
type TypeRegistry struct {
    mu    sync.RWMutex
    types map[string]reflect.Type
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

// Lookup returns the Go type for an event type
func (r *TypeRegistry) Lookup(eventType string) (reflect.Type, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    t, ok := r.types[eventType]
    return t, ok
}
```

### 7. Configuration

```go
// SerializerConfig configures the default serializer
type SerializerConfig struct {
    DefaultContentType string        // Default: "application/json"
    Registry           *TypeRegistry // Type registry for deserialization
    StrictMode         bool          // Error on unknown types
}

// NewSerializer creates a serializer with config
func NewSerializer(config SerializerConfig) *DefaultSerializer {
    if config.DefaultContentType == "" {
        config.DefaultContentType = "application/json"
    }
    return &DefaultSerializer{
        config:   config,
        registry: config.Registry,
    }
}
```

### 8. Raw Data Handling

For cases where deserialization should be skipped:

```go
// RawMessage wraps []byte to prevent deserialization
type RawMessage struct {
    Data []byte
}

// Serializer recognizes RawMessage and skips processing
func (s *DefaultSerializer) Serialize(data any, ct string) ([]byte, error) {
    if raw, ok := data.(RawMessage); ok {
        return raw.Data, nil
    }
    // Normal serialization...
}
```

### 9. Supported Content Types

| Content Type | Serialization | Notes |
|--------------|---------------|-------|
| `application/json` | JSON | Default |
| `application/json; charset=utf-8` | JSON | UTF-8 variant |
| `application/cloudevents+json` | CloudEvents JSON | Structured mode |
| `application/protobuf` | Protocol Buffers | Requires proto registration |
| `text/plain` | String | For string data |
| `application/octet-stream` | Raw bytes | No transformation |

## Rationale

1. **Separation of Concerns**: Handlers work with Go types, serialization at boundaries
2. **ContentType as Contract**: Standard HTTP content negotiation pattern
3. **Type Registry**: Automatic type resolution from event type
4. **Flexibility**: Custom content type handlers for specialized formats
5. **Backward Compatibility**: Raw bytes pass through unchanged

## Consequences

### Positive

- Handlers never deal with serialization
- ContentType drives serialization format
- Type registry enables automatic deserialization
- Clean separation between internal and external representations
- Supports multiple serialization formats

### Negative

- Requires type registration for automatic deserialization
- Unknown types default to `[]byte` (unless strict mode)
- Runtime overhead for reflection-based deserialization
- ContentType must be set correctly

### Example Usage

```go
// Setup
registry := message.NewTypeRegistry()
registry.RegisterAll(map[string]any{
    "order.created":    Order{},
    "order.shipped":    ShippingEvent{},
    "payment.received": PaymentEvent{},
})

serializer := message.NewSerializer(message.SerializerConfig{
    Registry: registry,
})

// Sender with automatic serialization
sender := message.NewSerializingSender(httpSender, serializer)

// Receiver with automatic deserialization
receiver := message.NewDeserializingReceiver(httpReceiver, serializer)

// Handler works with Go types
handler := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
    // msg.Data is already deserialized Order
    order := msg.Data.(Order)

    // Create response with Go type
    return []*message.Message{
        message.MustNew(ShippingCommand{OrderID: order.ID}, message.Attributes{
            message.AttrType:            "shipping.requested",
            message.AttrDataContentType: "application/json",
            // ...
        }),
    }, nil
}
```

## Links

- [ADR 0019: CloudEvents Mandatory](0019-cloudevents-mandatory.md)
- [ADR 0020: Non-Generic Message](0020-non-generic-message.md)
- [ADR 0022: Internal Message Routing](0022-internal-message-routing.md)
- [Feature 11: ContentType Serialization](../features/11-contenttype-serialization.md)
- [CloudEvents Standardization Plan](../plans/cloudevents-standardization.md)

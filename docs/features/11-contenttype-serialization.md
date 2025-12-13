# Feature: ContentType-Based Serialization

**Package:** `message`
**Status:** Proposed
**Related ADRs:**
- [ADR 0021](../adr/0021-contenttype-serialization.md) - ContentType-Based Serialization

## Summary

Automatic serialization/deserialization at system boundaries based on the `datacontenttype` attribute. Internal handlers work with Go types; serialization happens only when messages cross system boundaries.

## Motivation

- Handlers shouldn't deal with serialization
- ContentType provides standard format negotiation
- Type registry enables automatic deserialization
- Clean separation between internal (Go types) and external (`[]byte`)

## Implementation

### Serializer Interface

```go
// Serializer handles encoding/decoding based on content type
type Serializer interface {
    Serialize(data any, contentType string) ([]byte, error)
    Deserialize(data []byte, contentType string, eventType string) (any, error)
}

// DefaultSerializer with JSON and type registry
type DefaultSerializer struct {
    registry *TypeRegistry
    handlers map[string]ContentTypeHandler
    config   SerializerConfig
}

type SerializerConfig struct {
    DefaultContentType string
    Registry           *TypeRegistry
    StrictMode         bool // Error on unknown types
}
```

### Content Type Handlers

```go
// ContentTypeHandler handles specific content types
type ContentTypeHandler interface {
    ContentType() string
    Serialize(v any) ([]byte, error)
    Deserialize(data []byte, target reflect.Type) (any, error)
}

// JSONHandler for application/json
type jsonHandler struct{}

func (h *jsonHandler) ContentType() string { return "application/json" }

func (h *jsonHandler) Serialize(v any) ([]byte, error) {
    return json.Marshal(v)
}

func (h *jsonHandler) Deserialize(data []byte, target reflect.Type) (any, error) {
    instance := reflect.New(target).Interface()
    if err := json.Unmarshal(data, instance); err != nil {
        return nil, err
    }
    return reflect.ValueOf(instance).Elem().Interface(), nil
}
```

### Default Serializer Implementation

```go
func NewSerializer(config SerializerConfig) *DefaultSerializer {
    if config.DefaultContentType == "" {
        config.DefaultContentType = "application/json"
    }
    s := &DefaultSerializer{
        registry: config.Registry,
        config:   config,
        handlers: make(map[string]ContentTypeHandler),
    }
    // Register built-in handlers
    s.RegisterHandler(&jsonHandler{})
    s.RegisterHandler(&textHandler{})
    s.RegisterHandler(&rawHandler{})
    return s
}

func (s *DefaultSerializer) Serialize(data any, contentType string) ([]byte, error) {
    // Already bytes - return as-is
    if b, ok := data.([]byte); ok {
        return b, nil
    }
    // RawMessage wrapper - return inner bytes
    if raw, ok := data.(RawMessage); ok {
        return raw.Data, nil
    }

    // Get handler for content type
    handler := s.handlerFor(contentType)
    if handler == nil {
        return nil, fmt.Errorf("unsupported content type: %s", contentType)
    }

    return handler.Serialize(data)
}

func (s *DefaultSerializer) Deserialize(data []byte, contentType, eventType string) (any, error) {
    // Look up Go type from registry
    targetType, ok := s.registry.Lookup(eventType)
    if !ok {
        if s.config.StrictMode {
            return nil, fmt.Errorf("unknown event type: %s", eventType)
        }
        // Return raw bytes for unknown types
        return data, nil
    }

    // Get handler for content type
    handler := s.handlerFor(contentType)
    if handler == nil {
        return nil, fmt.Errorf("unsupported content type: %s", contentType)
    }

    return handler.Deserialize(data, targetType)
}
```

### Serializing Sender

```go
// SerializingSender wraps sender with automatic serialization
type SerializingSender struct {
    raw        RawSender
    serializer Serializer
}

func NewSerializingSender(raw RawSender, serializer Serializer) *SerializingSender {
    return &SerializingSender{raw: raw, serializer: serializer}
}

func (s *SerializingSender) Send(ctx context.Context, msgs []*Message) error {
    serialized := make([]*Message, len(msgs))
    for i, msg := range msgs {
        // Skip if already bytes
        if _, ok := msg.Data.([]byte); ok {
            serialized[i] = msg
            continue
        }

        // Get content type
        ct := msg.Attributes.DataContentType()
        if ct == "" {
            ct = "application/json"
        }

        // Serialize
        data, err := s.serializer.Serialize(msg.Data, ct)
        if err != nil {
            return fmt.Errorf("serialize %s: %w", msg.Attributes.Type(), err)
        }

        // Copy message with serialized data
        serialized[i] = msg.Copy()
        serialized[i].Data = data
    }

    return s.raw.Send(ctx, serialized)
}
```

### Deserializing Receiver

```go
// DeserializingReceiver wraps receiver with automatic deserialization
type DeserializingReceiver struct {
    raw        RawReceiver
    serializer Serializer
}

func NewDeserializingReceiver(raw RawReceiver, serializer Serializer) *DeserializingReceiver {
    return &DeserializingReceiver{raw: raw, serializer: serializer}
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

            // Deserialize
            v, err := r.serializer.Deserialize(data, ct, et)
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

### Supported Content Types

| Content Type | Handler | Notes |
|--------------|---------|-------|
| `application/json` | JSON | Default |
| `application/json; charset=utf-8` | JSON | UTF-8 variant |
| `text/plain` | Text | String conversion |
| `application/octet-stream` | Raw | No transformation |
| `application/protobuf` | Protobuf | Optional, requires registration |

## Usage Example

```go
// Setup type registry
registry := message.NewTypeRegistry()
registry.RegisterAll(map[string]any{
    "order.created":    Order{},
    "order.shipped":    ShippingEvent{},
    "payment.received": PaymentEvent{},
})

// Create serializer
serializer := message.NewSerializer(message.SerializerConfig{
    Registry:           registry,
    DefaultContentType: "application/json",
})

// Wrap HTTP sender with serialization
httpSender := broker.NewHTTPSender(client, config)
sender := message.NewSerializingSender(httpSender, serializer)

// Wrap HTTP receiver with deserialization
httpReceiver := broker.NewHTTPReceiver()
receiver := message.NewDeserializingReceiver(httpReceiver, serializer)

// Handler works with Go types
handler := func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
    // msg.Data is already Order (deserialized)
    order := msg.Data.(Order)

    // Return Go type - will be serialized on send
    return []*message.Message{
        message.MustNew(ShippingCommand{OrderID: order.ID}, message.Attributes{
            message.AttrType:            "shipping.requested",
            message.AttrDataContentType: "application/json",
            // ...
        }),
    }, nil
}
```

## Files Changed

- `message/serializer.go` - Serializer interface and default implementation
- `message/content_type.go` - Content type handlers (JSON, text, raw)
- `message/sender.go` - SerializingSender wrapper
- `message/receiver.go` - DeserializingReceiver wrapper
- `message/serializer_test.go` - Serialization tests
- `message/broker/http.go` - Update to use serializer

## Testing Strategy

```go
func TestSerializer_Roundtrip(t *testing.T) {
    registry := NewTypeRegistry()
    registry.Register("order.created", Order{})

    s := NewSerializer(SerializerConfig{Registry: registry})

    // Serialize
    order := Order{ID: "123", Amount: 100}
    data, err := s.Serialize(order, "application/json")
    require.NoError(t, err)

    // Deserialize
    v, err := s.Deserialize(data, "application/json", "order.created")
    require.NoError(t, err)

    got := v.(Order)
    assert.Equal(t, order, got)
}
```

## Related Features

- [10-non-generic-message](10-non-generic-message.md) - Prerequisite
- [12-internal-message-routing](12-internal-message-routing.md) - Uses this for boundary serialization

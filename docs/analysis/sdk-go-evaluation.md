# CloudEvents SDK-Go Integration Evaluation

## Executive Summary

**Recommendation: Hybrid Approach** - Use sdk-go at the marshalling/protocol boundary while keeping our internal typed `Message` for handler processing.

## SDK-Go Overview

The [CloudEvents SDK for Go](https://github.com/cloudevents/sdk-go) is the official CNCF SDK providing:

- **Event type**: Canonical CloudEvents representation
- **Protocol bindings**: HTTP, Kafka (Sarama/Confluent), NATS, AMQP, MQTT, PubSub
- **Validation**: Spec-compliant attribute validation
- **Serialization**: JSON/binary encoding with automatic base64 handling

### SDK Event Structure

```go
type Event struct {
    Context     EventContext    // CE attributes (id, source, type, etc.)
    DataEncoded []byte          // Raw payload bytes
    DataBase64  bool            // Base64 encoding flag
    FieldErrors map[string]error
}

// Key methods
event.SetID(id)
event.SetSource(source)
event.SetType(ceType)
event.SetData(contentType, payload)  // Encodes payload to DataEncoded
event.DataAs(&target)                 // Decodes DataEncoded to target
event.Validate()                      // CloudEvents compliance check
```

## Integration Options Analysis

### Option 1: Replace Message with sdk-go Event

```go
// Handlers receive cloudevents.Event directly
type Handler interface {
    Handle(ctx context.Context, evt cloudevents.Event) ([]cloudevents.Event, error)
}
```

| Pros | Cons |
|------|------|
| ✅ Full CE compliance | ❌ No compile-time type safety |
| ✅ No custom Message type | ❌ Data always as []byte internally |
| ✅ Protocol bindings included | ❌ DataAs() requires reflection |
| ✅ Less code to maintain | ❌ Handlers must decode every time |

**Verdict**: Loses type safety - our core value proposition.

### Option 2: Wrap sdk-go Event

```go
// Our Message wraps sdk-go Event
type Message struct {
    Event   cloudevents.Event  // SDK event
    Payload any                // Decoded typed payload
}
```

| Pros | Cons |
|------|------|
| ✅ Type safety preserved | ❌ Dual storage (Event + Payload) |
| ✅ Full CE compliance via Event | ❌ Must sync Payload with Event.Data |
| ✅ Protocol bindings available | ❌ Confusion about source of truth |

**Verdict**: Awkward dual state management.

### Option 3: SDK at Boundary Only (Recommended)

```go
// Internal Message - typed payload for handlers
type Message struct {
    // CloudEvents attributes (mirrored)
    ID              string
    Source          string
    Type            string
    // ... other CE attributes

    // Typed payload for handlers
    Payload         any
}

// Marshalling layer converts Message ↔ cloudevents.Event
type SDKMarshaller struct {
    registry *TypeRegistry
}

func (m *SDKMarshaller) ToEvent(msg *Message) cloudevents.Event
func (m *SDKMarshaller) FromEvent(evt cloudevents.Event) (*Message, error)
```

| Pros | Cons |
|------|------|
| ✅ Type safety in handlers | ⚠️ Conversion at boundaries |
| ✅ CE compliance via SDK | ⚠️ Attribute sync required |
| ✅ Protocol bindings optional | |
| ✅ Internal loops skip conversion | |
| ✅ Full control over internal type | |

**Verdict**: Best of both worlds.

### Option 4: No SDK

| Pros | Cons |
|------|------|
| ✅ No external dependency | ❌ Must implement CE spec |
| ✅ Full control | ❌ Risk of spec violations |
| | ❌ No protocol bindings |
| | ❌ More maintenance |

**Verdict**: Reinventing the wheel.

## Recommended Architecture

```
┌────────────────────────────────────────────────────────────────────────────┐
│                              goengine                                       │
│                                                                             │
│  PROTOCOL BOUNDARY (sdk-go)                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │                     cloudevents.Event                                │  │
│  │                                                                      │  │
│  │  Subscriber ←── Protocol Binding ←── External Broker                │  │
│  │  Publisher  ──► Protocol Binding ──► External Broker                │  │
│  │                                                                      │  │
│  │  SDK handles: Wire format, CE validation, Protocol specifics        │  │
│  └────────────────────────────────────┬────────────────────────────────┘  │
│                                       │                                    │
│  MARSHALLING LAYER (conversion)       │                                    │
│  ┌────────────────────────────────────▼────────────────────────────────┐  │
│  │                     SDKMarshaller                                    │  │
│  │                                                                      │  │
│  │  FromEvent(evt) → Message     Uses TypeRegistry to decode payload   │  │
│  │  ToEvent(msg)   → Event       Uses Codec to encode payload          │  │
│  │                                                                      │  │
│  └────────────────────────────────────┬────────────────────────────────┘  │
│                                       │                                    │
│  INTERNAL PROCESSING (typed)          │                                    │
│  ┌────────────────────────────────────▼────────────────────────────────┐  │
│  │                       Message                                        │  │
│  │                                                                      │  │
│  │  Typed Payload (any)   →   Handlers process with type assertions   │  │
│  │  CE Attributes         →   Routing, correlation, tracing           │  │
│  │                                                                      │  │
│  │  Internal loops (gopipe://) stay as Message - no SDK conversion    │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────────┘
```

## Do We Still Need Our Message Struct?

**Yes, for these reasons:**

1. **Type Safety**: `Payload any` allows handlers to use type assertions/switches
2. **Internal Loops**: `gopipe://` routing skips serialization entirely
3. **Handler Ergonomics**: Handlers work with typed data, not `[]byte`
4. **Performance**: No decode/encode for internal message passing

## Where to Use SDK-Go

| Component | Use SDK? | Rationale |
|-----------|----------|-----------|
| **ByteMessage** | Yes | Use `cloudevents.Event` for protocol boundary |
| **Marshaller** | Yes | `SDKMarshaller` converts Event ↔ Message |
| **Unmarshaller** | Yes | Uses `event.DataAs()` + TypeRegistry |
| **Adapters** | Optional | Can use SDK protocol bindings OR direct libs |
| **Internal Message** | No | Keep typed `Message` for handlers |
| **Internal Loops** | No | Zero-copy, no SDK involvement |

## Protocol Bindings Decision

SDK-go provides protocol bindings for:
- HTTP, Kafka (Sarama/Confluent), NATS, STAN, AMQP, MQTT, PubSub

**Options:**

### A) Use SDK Protocol Bindings
```go
// NATS via SDK
import nats_ce "github.com/cloudevents/sdk-go/protocol/nats/v2"

protocol, _ := nats_ce.NewConsumer(conn, "subject", nats_ce.NatsOptions())
client, _ := cloudevents.NewClient(protocol)
client.StartReceiver(ctx, handler)
```

**Pros**: CE compliance guaranteed, less code
**Cons**: SDK patterns may not fit engine architecture

### B) Use SDK for Serialization, Direct Libs for Transport
```go
// NATS via nats.go, SDK for serialization
import "github.com/nats-io/nats.go"
import cloudevents "github.com/cloudevents/sdk-go/v2"

// Receive raw bytes
sub, _ := nc.Subscribe("subject", func(msg *nats.Msg) {
    // Use SDK to parse
    event := cloudevents.NewEvent()
    json.Unmarshal(msg.Data, &event)
    // Convert to internal Message
    internal := marshaller.FromEvent(event)
})
```

**Pros**: Flexible, fits engine patterns
**Cons**: More code, but more control

**Recommendation**: Option B - Use SDK for serialization, direct broker libs for transport. This gives us control over connection management, consumer groups, and acknowledgment while ensuring CE compliance.

## Implementation Summary

### Message Types

```go
// goengine/message.go

// Message is the internal typed representation
type Message struct {
    // CloudEvents required attributes
    ID          string
    Source      string
    SpecVersion string
    Type        string

    // CloudEvents optional attributes
    Subject         string
    Time            time.Time
    DataContentType string
    DataSchema      string

    // Extensions
    Extensions map[string]any

    // Typed payload (decoded)
    Payload any
}
```

### SDKMarshaller

```go
// goengine/marshaller.go

type SDKMarshaller struct {
    registry *TypeRegistry
    codecs   map[string]Codec
}

// FromEvent converts sdk-go Event to internal Message
func (m *SDKMarshaller) FromEvent(evt cloudevents.Event) (*Message, error) {
    msg := &Message{
        ID:              evt.ID(),
        Source:         evt.Source(),
        SpecVersion:    evt.SpecVersion(),
        Type:           evt.Type(),
        Subject:        evt.Subject(),
        Time:           evt.Time(),
        DataContentType: evt.DataContentType(),
        DataSchema:     evt.DataSchema(),
        Extensions:     evt.Extensions(),
    }

    // Use TypeRegistry to find Go type
    entry, ok := m.registry.Lookup(evt.Type())
    if !ok {
        // Unknown type - keep as raw bytes
        msg.Payload = evt.Data()
        return msg, nil
    }

    // Decode to typed payload using SDK
    target := entry.Factory()
    if err := evt.DataAs(target); err != nil {
        return nil, fmt.Errorf("decode payload: %w", err)
    }
    msg.Payload = target

    return msg, nil
}

// ToEvent converts internal Message to sdk-go Event
func (m *SDKMarshaller) ToEvent(msg *Message) (cloudevents.Event, error) {
    evt := cloudevents.NewEvent()
    evt.SetID(msg.ID)
    evt.SetSource(msg.Source)
    evt.SetType(msg.Type)

    if !msg.Time.IsZero() {
        evt.SetTime(msg.Time)
    }
    if msg.Subject != "" {
        evt.SetSubject(msg.Subject)
    }
    if msg.DataSchema != "" {
        evt.SetDataSchema(msg.DataSchema)
    }

    // Set extensions
    for k, v := range msg.Extensions {
        evt.SetExtension(k, v)
    }

    // Encode payload
    contentType := msg.DataContentType
    if contentType == "" {
        contentType = cloudevents.ApplicationJSON
    }
    if err := evt.SetData(contentType, msg.Payload); err != nil {
        return evt, fmt.Errorf("encode payload: %w", err)
    }

    return evt, nil
}
```

## Benefits of This Approach

1. **CloudEvents Compliance**: SDK handles spec validation
2. **Type Safety**: Internal Message keeps typed payloads
3. **Protocol Flexibility**: Can use SDK bindings OR direct libs
4. **Performance**: Internal loops bypass SDK entirely
5. **Maintainability**: SDK maintained by CNCF community
6. **Future-Proof**: SDK updates bring new protocol bindings

## Sources

- [CloudEvents SDK-Go GitHub](https://github.com/cloudevents/sdk-go)
- [SDK-Go Documentation](https://cloudevents.github.io/sdk-go/)
- [SDK-Go Event Package](https://pkg.go.dev/github.com/cloudevents/sdk-go/v2/event)
- [Protocol Bindings](https://cloudevents.github.io/sdk-go/protocol_implementations.html)

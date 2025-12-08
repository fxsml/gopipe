# ADR 0011: CloudEvents Compatibility

**Date:** 2025-12-08
**Status:** Proposed
**Related:**
- [ADR 0010: Pub/Sub Package Structure](./0010-pubsub-package-structure.md)
- [Critical Analysis: Message Design](../critical-analysis-message-design.md)
- [CloudEvents Specification v1.0.2](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md)

## Context

[CloudEvents](https://cloudevents.io/) is a CNCF specification for describing event data in a common way. It's widely adopted in cloud-native ecosystems (Kubernetes, Knative, serverless platforms).

### CloudEvents Specification

**Required attributes:**
- `id` - Unique event identifier
- `source` - Event origin (URI-reference)
- `specversion` - Specification version ("1.0")
- `type` - Event type (e.g., "com.example.order.created")

**Optional attributes:**
- `datacontenttype` - MIME type of data
- `dataschema` - URI of data schema
- `subject` - Event subject (for filtering)
- `time` - RFC 3339 timestamp
- `data` - Event payload

### gopipe Message vs CloudEvents

```go
// gopipe Message
type Message struct {
    Payload    []byte
    Properties map[string]any
}

// CloudEvents JSON
{
    "id": "A234-1234-1234",
    "source": "https://example.com/orders",
    "specversion": "1.0",
    "type": "com.example.order.created",
    "datacontenttype": "application/json",
    "time": "2025-12-08T10:00:00Z",
    "data": {...}
}
```

### User Needs

1. **Interoperability**: Integrate with CloudEvents-based systems
2. **Standards compliance**: Emit/consume CloudEvents-compliant messages
3. **Flexibility**: Not all use cases need CloudEvents
4. **Simplicity**: Don't complicate core message API

## Decision

Create a **CloudEvents compatibility layer** as a separate, optional package: `pubsub/cloudevents`.

### Design Principles

1. **Optional, not mandatory** - Users opt-in to CloudEvents
2. **No breaking changes** - Works with existing `message.Message`
3. **Thin wrapper** - Minimal overhead
4. **Type-safe** - Helpers ensure correct CloudEvents structure
5. **Validation** - Can validate CloudEvents compliance
6. **Interoperable** - Easy conversion to/from standard formats

## Architecture

### Package Structure

```
pubsub/
├── cloudevents/
│   ├── cloudevents.go       ← Event wrapper, constructors, validation
│   ├── attributes.go        ← Attribute constants and helpers
│   ├── codec.go             ← JSON/HTTP encoding/decoding
│   ├── cloudevents_test.go
│   └── README.md
```

### Core Types (pubsub/cloudevents/cloudevents.go)

```go
package cloudevents

import (
    "time"

    "github.com/fxsml/gopipe/message"
)

// Event wraps a gopipe Message with CloudEvents semantics.
// The underlying message Properties map contains CloudEvents attributes.
type Event struct {
    *message.Message
}

// NewEvent creates a CloudEvents-compliant message.
// Required attributes: id, source, type
// The specversion is automatically set to "1.0"
func NewEvent(
    id string,
    source string,
    evtType string,
    data []byte,
    opts ...Option,
) *Event {
    props := message.Properties{
        AttrID:          id,
        AttrSource:      source,
        AttrSpecVersion: "1.0",
        AttrType:        evtType,
    }
    for _, opt := range opts {
        opt(props)
    }
    return &Event{Message: message.New(data, props)}
}

// FromMessage wraps a gopipe Message as a CloudEvent.
// Use this when receiving messages that may be CloudEvents-compliant.
func FromMessage(msg *message.Message) *Event {
    return &Event{Message: msg}
}

// ToMessage returns the underlying gopipe Message.
func (e *Event) ToMessage() *message.Message {
    return e.Message
}

// Validate checks if the event conforms to CloudEvents v1.0 specification.
func (e *Event) Validate() error {
    if e.ID() == "" {
        return fmt.Errorf("missing required attribute: id")
    }
    if e.Source() == "" {
        return fmt.Errorf("missing required attribute: source")
    }
    if e.SpecVersion() != "1.0" {
        return fmt.Errorf("invalid specversion: expected '1.0', got '%s'", e.SpecVersion())
    }
    if e.Type() == "" {
        return fmt.Errorf("missing required attribute: type")
    }
    return nil
}

// Option configures optional CloudEvents attributes.
type Option func(props message.Properties)

// WithContentType sets the datacontenttype attribute.
func WithContentType(ct string) Option {
    return func(props message.Properties) {
        props[AttrDataContentType] = ct
    }
}

// WithSubject sets the subject attribute.
func WithSubject(subject string) Option {
    return func(props message.Properties) {
        props[AttrSubject] = subject
    }
}

// WithTime sets the time attribute (RFC 3339 format).
func WithTime(t time.Time) Option {
    return func(props message.Properties) {
        props[AttrTime] = t.Format(time.RFC3339)
    }
}

// WithDataSchema sets the dataschema attribute.
func WithDataSchema(schema string) Option {
    return func(props message.Properties) {
        props[AttrDataSchema] = schema
    }
}

// WithExtension sets a custom extension attribute.
// Extension names must be lowercase alphanumeric (max 20 chars).
func WithExtension(name string, value any) Option {
    return func(props message.Properties) {
        props[name] = value
    }
}
```

### Attribute Getters (pubsub/cloudevents/attributes.go)

```go
package cloudevents

// CloudEvents attribute names (v1.0 specification)
const (
    // Required attributes
    AttrID          = "id"
    AttrSource      = "source"
    AttrSpecVersion = "specversion"
    AttrType        = "type"

    // Optional attributes
    AttrDataContentType = "datacontenttype"
    AttrDataSchema      = "dataschema"
    AttrSubject         = "subject"
    AttrTime            = "time"
)

// ID returns the event ID (required).
func (e *Event) ID() string {
    v, _ := e.Properties[AttrID].(string)
    return v
}

// Source returns the event source (required).
func (e *Event) Source() string {
    v, _ := e.Properties[AttrSource].(string)
    return v
}

// SpecVersion returns the CloudEvents version (required).
func (e *Event) SpecVersion() string {
    v, _ := e.Properties[AttrSpecVersion].(string)
    return v
}

// Type returns the event type (required).
func (e *Event) Type() string {
    v, _ := e.Properties[AttrType].(string)
    return v
}

// DataContentType returns the data content type (optional).
func (e *Event) DataContentType() string {
    v, _ := e.Properties[AttrDataContentType].(string)
    return v
}

// DataSchema returns the data schema URI (optional).
func (e *Event) DataSchema() string {
    v, _ := e.Properties[AttrDataSchema].(string)
    return v
}

// Subject returns the event subject (optional).
func (e *Event) Subject() string {
    v, _ := e.Properties[AttrSubject].(string)
    return v
}

// Time returns the event timestamp (optional).
// Returns zero time if not set or invalid.
func (e *Event) Time() time.Time {
    v, _ := e.Properties[AttrTime].(string)
    t, _ := time.Parse(time.RFC3339, v)
    return t
}

// Extension returns a custom extension attribute.
func (e *Event) Extension(name string) (any, bool) {
    v, ok := e.Properties[name]
    return v, ok
}

// Data returns the event payload.
func (e *Event) Data() []byte {
    return e.Payload
}
```

### Codec (pubsub/cloudevents/codec.go)

```go
package cloudevents

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"

    "github.com/fxsml/gopipe/message"
)

// MarshalJSON encodes a CloudEvent to JSON (structured content mode).
func (e *Event) MarshalJSON() ([]byte, error) {
    m := map[string]any{
        AttrID:          e.ID(),
        AttrSource:      e.Source(),
        AttrSpecVersion: e.SpecVersion(),
        AttrType:        e.Type(),
    }

    if ct := e.DataContentType(); ct != "" {
        m[AttrDataContentType] = ct
    }
    if schema := e.DataSchema(); schema != "" {
        m[AttrDataSchema] = schema
    }
    if subject := e.Subject(); subject != "" {
        m[AttrSubject] = subject
    }
    if t := e.Time(); !t.IsZero() {
        m[AttrTime] = t.Format(time.RFC3339)
    }

    // Include extension attributes
    for k, v := range e.Properties {
        if isExtension(k) {
            m[k] = v
        }
    }

    // Parse data based on content type
    if e.DataContentType() == "application/json" && len(e.Data()) > 0 {
        var data any
        if err := json.Unmarshal(e.Data(), &data); err != nil {
            return nil, fmt.Errorf("unmarshal data: %w", err)
        }
        m["data"] = data
    } else {
        m["data"] = e.Data()
    }

    return json.Marshal(m)
}

// UnmarshalJSON decodes a CloudEvent from JSON (structured content mode).
func (e *Event) UnmarshalJSON(data []byte) error {
    var m map[string]any
    if err := json.Unmarshal(data, &m); err != nil {
        return err
    }

    props := make(message.Properties)

    // Required attributes
    id, _ := m[AttrID].(string)
    source, _ := m[AttrSource].(string)
    specversion, _ := m[AttrSpecVersion].(string)
    evtType, _ := m[AttrType].(string)

    props[AttrID] = id
    props[AttrSource] = source
    props[AttrSpecVersion] = specversion
    props[AttrType] = evtType

    // Optional attributes
    if ct, ok := m[AttrDataContentType].(string); ok {
        props[AttrDataContentType] = ct
    }
    if schema, ok := m[AttrDataSchema].(string); ok {
        props[AttrDataSchema] = schema
    }
    if subject, ok := m[AttrSubject].(string); ok {
        props[AttrSubject] = subject
    }
    if timeStr, ok := m[AttrTime].(string); ok {
        props[AttrTime] = timeStr
    }

    // Extension attributes
    for k, v := range m {
        if isExtension(k) {
            props[k] = v
        }
    }

    // Data
    var payload []byte
    if dataVal, ok := m["data"]; ok {
        payload, _ = json.Marshal(dataVal)
    }

    e.Message = message.New(payload, props)
    return nil
}

// WriteHTTP writes the CloudEvent to an HTTP response (binary content mode).
// Attributes are set as HTTP headers, data as response body.
func (e *Event) WriteHTTP(w http.ResponseWriter) error {
    w.Header().Set("Ce-Id", e.ID())
    w.Header().Set("Ce-Source", e.Source())
    w.Header().Set("Ce-Specversion", e.SpecVersion())
    w.Header().Set("Ce-Type", e.Type())

    if ct := e.DataContentType(); ct != "" {
        w.Header().Set("Content-Type", ct)
    }
    if subject := e.Subject(); subject != "" {
        w.Header().Set("Ce-Subject", subject)
    }
    if t := e.Time(); !t.IsZero() {
        w.Header().Set("Ce-Time", t.Format(time.RFC3339))
    }
    if schema := e.DataSchema(); schema != "" {
        w.Header().Set("Ce-Dataschema", schema)
    }

    // Extension attributes
    for k, v := range e.Properties {
        if isExtension(k) {
            w.Header().Set("Ce-"+k, fmt.Sprint(v))
        }
    }

    _, err := w.Write(e.Data())
    return err
}

// ReadHTTP reads a CloudEvent from an HTTP request (binary content mode).
func ReadHTTP(r *http.Request) (*Event, error) {
    props := make(message.Properties)

    // Required attributes
    props[AttrID] = r.Header.Get("Ce-Id")
    props[AttrSource] = r.Header.Get("Ce-Source")
    props[AttrSpecVersion] = r.Header.Get("Ce-Specversion")
    props[AttrType] = r.Header.Get("Ce-Type")

    // Optional attributes
    if ct := r.Header.Get("Content-Type"); ct != "" {
        props[AttrDataContentType] = ct
    }
    if subject := r.Header.Get("Ce-Subject"); subject != "" {
        props[AttrSubject] = subject
    }
    if timeStr := r.Header.Get("Ce-Time"); timeStr != "" {
        props[AttrTime] = timeStr
    }
    if schema := r.Header.Get("Ce-Dataschema"); schema != "" {
        props[AttrDataSchema] = schema
    }

    // Extension attributes (any other Ce-* headers)
    for k := range r.Header {
        if len(k) > 3 && k[:3] == "Ce-" {
            name := strings.ToLower(k[3:])
            if isExtension(name) {
                props[name] = r.Header.Get(k)
            }
        }
    }

    // Read body
    payload, err := io.ReadAll(r.Body)
    if err != nil {
        return nil, fmt.Errorf("read body: %w", err)
    }

    return &Event{Message: message.New(payload, props)}, nil
}

// isExtension checks if attribute name is an extension.
func isExtension(name string) bool {
    return name != AttrID &&
        name != AttrSource &&
        name != AttrSpecVersion &&
        name != AttrType &&
        name != AttrDataContentType &&
        name != AttrDataSchema &&
        name != AttrSubject &&
        name != AttrTime
}
```

## Usage Examples

### Creating CloudEvents

```go
import (
    "github.com/fxsml/gopipe/pubsub/cloudevents"
    "time"
)

// Create CloudEvent
event := cloudevents.NewEvent(
    "order-123",
    "https://example.com/orders",
    "com.example.order.created",
    payload,
    cloudevents.WithContentType("application/json"),
    cloudevents.WithSubject("order-123"),
    cloudevents.WithTime(time.Now()),
)

// Validate
if err := event.Validate(); err != nil {
    log.Fatal("invalid CloudEvent:", err)
}

// Use with pub/sub
sender.Send(ctx, "orders", []*message.Message{event.ToMessage()})
```

### Receiving CloudEvents

```go
import "github.com/fxsml/gopipe/pubsub/cloudevents"

// Receive message
msgs, _ := receiver.Receive(ctx, "orders")

// Wrap as CloudEvent
event := cloudevents.FromMessage(msgs[0])

// Access CloudEvents attributes
fmt.Printf("Event: %s\n", event.Type())
fmt.Printf("Source: %s\n", event.Source())
fmt.Printf("Time: %s\n", event.Time())

// Validate
if err := event.Validate(); err != nil {
    log.Println("Not a valid CloudEvent:", err)
}
```

### HTTP Integration

```go
import "github.com/fxsml/gopipe/pubsub/cloudevents"

// HTTP handler (receive CloudEvents)
func handleWebhook(w http.ResponseWriter, r *http.Request) {
    event, err := cloudevents.ReadHTTP(r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    if err := event.Validate(); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Process event
    processEvent(event)

    w.WriteHeader(http.StatusOK)
}

// Send CloudEvent via HTTP
func sendEvent(url string, event *cloudevents.Event) error {
    var buf bytes.Buffer
    if err := event.WriteHTTP(&httptest.ResponseRecorder{Body: &buf}); err != nil {
        return err
    }

    req, _ := http.NewRequest("POST", url, &buf)
    event.WriteHTTP(req) // Sets headers

    resp, err := http.DefaultClient.Do(req)
    // ...
}
```

### JSON Encoding/Decoding

```go
import "github.com/fxsml/gopipe/pubsub/cloudevents"

// Encode to JSON
event := cloudevents.NewEvent("id-1", "source", "type", data)
jsonData, _ := json.Marshal(event)

// Output:
// {
//   "id": "id-1",
//   "source": "source",
//   "specversion": "1.0",
//   "type": "type",
//   "data": {...}
// }

// Decode from JSON
var event cloudevents.Event
json.Unmarshal(jsonData, &event)
```

## Benefits

### 1. Optional, Non-Intrusive

```go
// Don't need CloudEvents? Don't use it!
msg := message.New(payload, props)
sender.Send(ctx, "topic", []*message.Message{msg})

// Need CloudEvents? Opt-in!
event := cloudevents.NewEvent(id, source, evtType, payload)
sender.Send(ctx, "topic", []*message.Message{event.ToMessage()})
```

### 2. Standards Compliant

- ✅ Implements CloudEvents v1.0 specification
- ✅ Required attributes enforced
- ✅ Validation available
- ✅ JSON and HTTP binary modes supported

### 3. Type-Safe

```go
// Type-safe attribute access
id := event.ID()           // string
source := event.Source()   // string
time := event.Time()       // time.Time
```

### 4. Interoperable

```go
// Receive from CloudEvents-based system
msg := <-receiver.Subscribe(ctx, "topic")
event := cloudevents.FromMessage(msg)

// Validate it's a proper CloudEvent
if err := event.Validate(); err != nil {
    // Not a CloudEvent, handle as regular message
    processRegularMessage(msg)
} else {
    // Valid CloudEvent
    processCloudEvent(event)
}
```

### 5. Easy Migration

```go
// Existing code unchanged
msg := message.New(payload, message.Properties{
    "id":     "123",
    "source": "orders",
    "type":   "created",
})

// Make it CloudEvents-compliant by adding specversion
msg.Properties["specversion"] = "1.0"
event := cloudevents.FromMessage(msg)
event.Validate()  // Now valid!
```

## Comparison with Alternatives

### Alternative 1: Make Message CloudEvents-Native (❌ Rejected)

```go
// Would require breaking changes
type Message struct {
    ID          string    // Required
    Source      string    // Required
    SpecVersion string    // Required: "1.0"
    Type        string    // Required
    Data        []byte
    Extensions  map[string]any
}
```

**Rejected because:**
- ❌ Breaking change for all users
- ❌ Forces CloudEvents on everyone
- ❌ Less flexible
- ❌ Not idiomatic Go

### Alternative 2: CloudEvents as Convention (⚠️ Partial)

```go
// Just use Properties with CloudEvents keys
msg := message.New(payload, message.Properties{
    "id":          "123",
    "source":      "orders",
    "specversion": "1.0",
    "type":        "created",
})
```

**Rejected because:**
- ⚠️ No type safety
- ⚠️ No validation
- ⚠️ No convenience helpers
- ⚠️ Easy to create invalid CloudEvents

**But:** This is actually what `pubsub/cloudevents` builds on!

### Alternative 3: Third-Party CloudEvents SDK (❌ Rejected)

Use existing CloudEvents Go SDK.

**Rejected because:**
- ❌ External dependency
- ❌ Different message types (incompatible with gopipe)
- ❌ Conversion overhead
- ❌ Less control over API

## Migration and Adoption

### Phase 1: Implement Core Package

1. Create `pubsub/cloudevents/` package
2. Implement `Event`, constructors, validation
3. Implement attribute getters
4. Add tests

### Phase 2: Add Codecs

1. Implement JSON codec (`MarshalJSON`, `UnmarshalJSON`)
2. Implement HTTP codec (`WriteHTTP`, `ReadHTTP`)
3. Add codec tests

### Phase 3: Documentation and Examples

1. Add package README
2. Create examples
3. Add godoc documentation

### Adoption Pattern

Users can adopt incrementally:

```go
// Step 1: Use regular messages (existing code)
msg := message.New(payload, props)

// Step 2: Add CloudEvents attributes
msg.Properties["id"] = uuid.New().String()
msg.Properties["source"] = "orders-service"
msg.Properties["specversion"] = "1.0"
msg.Properties["type"] = "com.example.order.created"

// Step 3: Use CloudEvents package
event := cloudevents.FromMessage(msg)
event.Validate()

// Step 4: Create CloudEvents directly
event := cloudevents.NewEvent(id, source, evtType, payload)
```

## Breaking Changes

**None** - This is a new, optional package that doesn't modify existing APIs.

## References

- [CloudEvents Specification v1.0.2](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md)
- [CloudEvents Primer](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/primer.md)
- [CloudEvents JSON Format](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/formats/json-format.md)
- [CloudEvents HTTP Protocol Binding](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/bindings/http-protocol-binding.md)
- [ADR 0010: Pub/Sub Package Structure](./0010-pubsub-package-structure.md)
- [Critical Analysis: Message Design](../critical-analysis-message-design.md)

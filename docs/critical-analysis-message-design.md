# Critical Analysis: gopipe Message Design

**Date:** 2025-12-08
**Status:** Analysis & Recommendations

This document provides a critical analysis of gopipe's current message design and broker architecture, examining three key areas:

1. Handler design: Generic vs non-generic, marshalling placement
2. Pub/sub package structure and broker implementations
3. CloudEvents compatibility

## Executive Summary

### Current State (Already Implemented)

✅ **Non-generic Message** (`Message = TypedMessage[[]byte]`)
✅ **Broker interfaces** in `message/pubsub.go` (`Sender`, `Receiver`, `Broker`)
✅ **Broker implementations** in `message/broker/` (in-memory, IO, HTTP)
✅ **TypedMessage[T]** for type-safe pipelines
✅ **Marshalling in handlers** (`NewJSONHandler`)

### Key Recommendations

1. **Keep dual approach**: Non-generic `Message` for pub/sub + `TypedMessage[T]` for pipelines ✅
2. **Move broker to dedicated `pubsub` package**: Better organization, clearer API surface
3. **Add CloudEvents compatibility layer**: Optional, non-intrusive, in separate package

---

## Part 1: Handler Design - Generic vs Non-Generic

### Current Implementation

```go
// Non-generic handler (pub/sub pattern)
type Handler interface {
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
    Match(prop Attributes) bool
}

// Generic handler helper (type-safe)
func NewJSONHandler[In, Out any](
    handle func(ctx context.Context, data In) ([]Out, error),
    match func(prop Attributes) bool,
    props func(prop Attributes) Attributes,
) Handler
```

**Key insight:** The code already uses both approaches:
- `Handler` interface is non-generic (works with `Message`)
- `NewJSONHandler` provides generic convenience wrapper

### Critical Analysis

#### Option A: Non-Generic Handler (Current ✅)

**Pros:**
- ✅ **Interface simplicity**: No generics in interface definition
- ✅ **Broker compatibility**: Works naturally with `Sender`/`Receiver` interfaces
- ✅ **Dynamic routing**: Can inspect and route messages without type knowledge
- ✅ **Flexibility**: Handlers can process multiple message types
- ✅ **Standard Go idiom**: Similar to `http.Handler`, `io.Reader/Writer`
- ✅ **No type parameters in signatures**: Easier to store in slices, pass around

**Cons:**
- ⚠️ **Manual marshalling**: User code must unmarshal datas
- ⚠️ **No compile-time type safety**: Unmarsh al failures at runtime
- ⚠️ **Boilerplate**: Repeated unmarshal/marshal code

**Mitigations (Already Implemented):**
```go
// ✅ NewJSONHandler provides type safety
handler := message.NewJSONHandler[CreateOrder, OrderCreated](
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // Type-safe! No manual unmarshalling
        return []OrderCreated{{...}}, nil
    },
    matchFunc,
    propsFunc,
)
```

#### Option B: Generic Handler (Alternative)

```go
// Hypothetical generic handler
type Handler[In, Out any] interface {
    Handle(ctx context.Context, data In) ([]Out, error)
    Match(prop Attributes) bool
}
```

**Pros:**
- ✅ **Type safety**: Compile-time guarantees
- ✅ **No unmarshalling in handler**: Framework handles it

**Cons:**
- ❌ **Interface generics complexity**: `[]Handler[?, ?]` impossible
- ❌ **Broker incompatibility**: Can't connect to `Sender`/`Receiver` easily
- ❌ **Dynamic routing impossible**: Can't inspect data without unmarshalling
- ❌ **Storage limitations**: Can't store different `Handler[A,B]` and `Handler[C,D]` in same slice
- ❌ **Breaking change**: Would require redesigning Router
- ❌ **Type parameter explosion**: Every consumer needs type params

**Example of the problem:**
```go
// ❌ Can't do this with generic Handler interface
router := message.NewRouter(
    config,
    handleCreateOrder,    // Handler[CreateOrder, OrderCreated]
    handleChargePayment,  // Handler[ChargePayment, PaymentCharged]
    // ^ These have different type parameters!
)
```

### Marshalling Placement Analysis

#### Current Approach: Marshalling in Handlers ✅

**Location:** `NewJSONHandler` unmarshals/marshals inside handler wrapper

```go
func NewJSONHandler[In, Out any](...) Handler {
    h := func(ctx context.Context, msg *Message) ([]*Message, error) {
        var data In
        json.Unmarshal(msg.Data, &data)  // ← In handler

        out, err := handle(ctx, data)

        json.Marshal(out)  // ← In handler
        return msgs, nil
    }
    return &handler{handle: h, match: match}
}
```

**Pros:**
- ✅ **Per-handler marshaller**: Different handlers can use different formats (JSON, Protobuf, Avro)
- ✅ **Content negotiation**: Can inspect `Content-Type` property
- ✅ **Flexibility**: Handlers can access raw bytes if needed
- ✅ **Composability**: Can chain raw handlers and typed handlers
- ✅ **No overhead for raw handlers**: Brokers/proxies don't pay marshalling cost

**Cons:**
- ⚠️ **Repeated logic**: Each handler helper repeats unmarshal/marshal pattern
- ⚠️ **Broker ignorance**: Sender/Receiver don't know about content types

#### Alternative: Marshalling in Sender/Receiver

```go
// Hypothetical
type TypedSender[T any] interface {
    Send(ctx context.Context, topic string, datas []T) error
}

type TypedReceiver[T any] interface {
    Receive(ctx context.Context, topic string) ([]T, error)
}
```

**Pros:**
- ✅ **Type safety at boundaries**: Compile-time guarantees
- ✅ **Less boilerplate**: No manual marshalling

**Cons:**
- ❌ **Single format per broker**: Can't mix JSON/Protobuf
- ❌ **Type parameter explosion**: `Publisher[T]`, `Subscriber[T]`, etc.
- ❌ **Router incompatibility**: Can't route different types
- ❌ **Breaking change**: Incompatible with current `message.Message`
- ❌ **Less flexible**: Hard to inspect/log raw bytes
- ❌ **Not idiomatic**: Most message brokers use `[]byte` (Kafka, NATS, RabbitMQ)

### Recommendation: Keep Current Design ✅

**Verdict:** The current design is **idiomatic, flexible, and correct**.

**Rationale:**
1. Non-generic `Handler` interface matches Go idioms (`http.Handler`, `io.Reader/Writer`)
2. Marshalling in handlers allows per-handler format negotiation
3. `NewJSONHandler` provides type safety where needed
4. Brokers remain simple and format-agnostic
5. Compatible with all message broker implementations (Kafka, NATS, etc.)

**No changes needed** - this is already the right design!

---

## Part 2: Pub/Sub Package Structure

### Current Structure

```
message/
├── message.go           ← TypedMessage[T], Message
├── attributes.go        ← Attributes helpers
├── router.go            ← Router, Handler, NewHandler, NewJSONHandler
├── pubsub.go            ← Sender, Receiver, Broker, Publisher, Subscriber
└── broker/
    ├── broker.go        ← NewBroker (in-memory)
    ├── http.go          ← HTTP broker
    └── io.go            ← IO broker
```

### Critical Analysis

#### Current Issues

1. **Naming confusion**: `NewBroker` in `message/broker` is too generic
   - Should be `NewInMemoryBroker` or `NewMemoryBroker`

2. **Package organization**: `pubsub.go` in `message/` feels misplaced
   - Interfaces like `Sender`, `Receiver`, `Broker` are pub/sub-specific
   - They're not core message concepts (like `Message`, `Attributes`)

3. **Import paths**: Users must import both `message` and `message/broker`
   ```go
   import (
       "github.com/fxsml/gopipe/message"
       "github.com/fxsml/gopipe/message/broker"
   )
   broker := broker.NewBroker(...) // Confusing!
   ```

4. **ADR-25 inconsistency**: ADR mentions `Sender[T]` and `Receiver[T]` (generic) but implementation is non-generic
   - ADR needs updating to reflect current design

### Proposed Structure: Dedicated `pubsub` Package

```
pubsub/
├── pubsub.go            ← Sender, Receiver, Broker, Publisher, Subscriber interfaces
├── memory/
│   └── broker.go        ← NewBroker() - in-memory implementation
├── http/
│   └── broker.go        ← NewSender(), NewReceiver(), NewBroker()
├── io/
│   └── broker.go        ← NewSender(), NewReceiver(), NewBroker()
└── channel/             ← New! Channel-based broker (zero-copy)
    └── broker.go        ← NewBroker() using gopipe channels

message/
├── message.go           ← TypedMessage[T], Message (unchanged)
├── attributes.go        ← Attributes helpers (unchanged)
└── router.go            ← Router, Handler (unchanged)
```

**Import examples:**
```go
// Clear and explicit
import (
    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/pubsub"
    "github.com/fxsml/gopipe/pubsub/memory"
    "github.com/fxsml/gopipe/pubsub/http"
)

broker := memory.NewBroker(memory.Config{...})
httpSender := http.NewSender(url, http.Config{...})
```

### Implementation Options Analysis

#### Option 1: Subpackages Provide Only Broker

```go
// pubsub/memory/broker.go
func NewBroker(config Config) pubsub.Broker

// pubsub/http/broker.go
func NewBroker(config Config) pubsub.Broker
func NewSender(url string, config Config) pubsub.Sender
func NewReceiver(config Config) pubsub.Receiver
```

**Pros:**
- ✅ Consistent API across implementations
- ✅ Users can choose Broker or Sender/Receiver

**Cons:**
- ⚠️ HTTP can't implement full Broker easily (sender and receiver are separate)

#### Option 2: Subpackages Provide Sender/Receiver OR Broker

```go
// pubsub/memory/broker.go
func NewBroker(config Config) pubsub.Broker  // Implements both

// pubsub/http/sender.go
func NewSender(url string, config Config) pubsub.Sender

// pubsub/http/receiver.go
func NewReceiver(config Config) pubsub.Receiver
```

**Pros:**
- ✅ Each implementation provides what makes sense
- ✅ HTTP doesn't pretend to be a full broker

**Cons:**
- ⚠️ Less consistent API

#### Option 3: Channel-Based Broker (New!)

```go
// pubsub/channel/broker.go
type Broker struct {
    topics map[string]chan *message.Message
}

func NewBroker() pubsub.Broker {
    // Zero-copy, native Go channels
    // Leverages gopipe's channel utilities
}
```

**Why this is interesting:**
- ✅ **Zero-copy**: Messages stay in Go memory
- ✅ **Native concurrency**: Go channels handle backpressure
- ✅ **gopipe integration**: Can use `channel.FanOut`, `channel.Merge`, etc.
- ✅ **Simplest possible**: No marshalling, no network, just channels
- ✅ **Different from memory**: Memory broker stores messages, channel broker streams them

**Use cases:**
- In-process pub/sub
- Testing
- Microservices in same process (monolith migration)

### Recommendation: Create `pubsub` Package

**Changes needed:**

1. **Move interfaces** from `message/pubsub.go` → `pubsub/pubsub.go`
2. **Move implementations** from `message/broker/` → `pubsub/memory/`, `pubsub/http/`, `pubsub/io/`
3. **Rename**: `NewBroker` → `NewBroker` (but in `pubsub/memory/`)
4. **Add**: `pubsub/channel/` implementation
5. **Update**: ADR-25 to reflect non-generic design

**Migration path:**
```go
// Before
import "github.com/fxsml/gopipe/message/broker"
b := broker.NewBroker(cfg)

// After
import "github.com/fxsml/gopipe/pubsub/memory"
b := memory.NewBroker(cfg)
```

---

## Part 3: CloudEvents Compatibility

### CloudEvents Specification

**Required attributes:**
- `id` - Unique event identifier
- `source` - Event origin (URI-reference)
- `specversion` - "1.0"
- `type` - Event type (e.g., "com.example.order.created")

**Optional attributes:**
- `datacontenttype` - MIME type (e.g., "application/json")
- `dataschema` - Schema URI
- `subject` - Event subject (for filtering)
- `time` - RFC 3339 timestamp
- `data` - Event data

### Current gopipe Message vs CloudEvents

```go
// Current gopipe Message
type Message struct {
    Data    []byte                 // ← data
    Attributes map[string]any         // ← attributes?
}

// CloudEvents
{
    "id": "...",                       // Required
    "source": "...",                   // Required
    "specversion": "1.0",              // Required
    "type": "...",                     // Required
    "datacontenttype": "...",          // Optional
    "time": "...",                     // Optional
    "data": {...}                      // Data
}
```

### Analysis: Should Message Be CloudEvents Compatible?

#### Option A: Make Message CloudEvents-Native (❌ Not Recommended)

```go
type Message struct {
    ID          string    // Required
    Source      string    // Required
    SpecVersion string    // Required: "1.0"
    Type        string    // Required
    ContentType string    // Optional
    Time        time.Time // Optional
    Data        []byte    // Data
    Extensions  map[string]any
}
```

**Pros:**
- ✅ CloudEvents compatibility out of the box

**Cons:**
- ❌ **Breaking change**: Incompatible with current `Message`
- ❌ **Opinionated**: Forces CloudEvents on all users
- ❌ **Less flexible**: Not all use cases need CloudEvents
- ❌ **Overhead**: Required fields even for simple messages
- ❌ **Not idiomatic**: gopipe is about flexibility, not standards enforcement

**Verdict:** ❌ **Reject** - Too opinionated, breaks existing code

#### Option B: CloudEvents as Attributes Convention (✅ Recommended)

```go
// Use existing Attributes with CloudEvents keys
msg := message.New(data, message.Attributes{
    "id":          "uuid-123",
    "source":      "orders-service",
    "specversion": "1.0",
    "type":        "com.example.order.created",
    "time":        "2025-12-08T10:00:00Z",
})
```

**Pros:**
- ✅ **No breaking changes**: Uses existing `Attributes`
- ✅ **Optional**: Users opt-in to CloudEvents
- ✅ **Flexible**: Can mix CloudEvents and custom attributes
- ✅ **Simple**: No new types

**Cons:**
- ⚠️ **No type safety**: Attributes are `map[string]any`
- ⚠️ **No validation**: Can create invalid CloudEvents
- ⚠️ **No discovery**: Hard to know which attributes are CloudEvents

#### Option C: CloudEvents Compatibility Layer (✅ Recommended)

Create a separate package for CloudEvents interop:

```go
// pubsub/cloudevents/cloudevents.go
package cloudevents

import "github.com/fxsml/gopipe/message"

// Required CloudEvents attributes
const (
    AttrID          = "id"
    AttrSource      = "source"
    AttrSpecVersion = "specversion"
    AttrType        = "type"
)

// Optional CloudEvents attributes
const (
    AttrDataContentType = "datacontenttype"
    AttrDataSchema      = "dataschema"
    AttrSubject         = "subject"
    AttrTime            = "time"
)

// Event wraps a gopipe Message with CloudEvents semantics
type Event struct {
    *message.Message
}

// NewEvent creates a CloudEvents-compatible message
func NewEvent(
    id string,
    source string,
    evtType string,
    data []byte,
    opts ...Option,
) *Event {
    props := message.Attributes{
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

// Option configures optional CloudEvents attributes
type Option func(props message.Attributes)

func WithContentType(ct string) Option {
    return func(props message.Attributes) {
        props[AttrDataContentType] = ct
    }
}

func WithSubject(subject string) Option {
    return func(props message.Attributes) {
        props[AttrSubject] = subject
    }
}

func WithTime(t time.Time) Option {
    return func(props message.Attributes) {
        props[AttrTime] = t.Format(time.RFC3339)
    }
}

// Validate checks if message conforms to CloudEvents spec
func (e *Event) Validate() error {
    if e.ID() == "" {
        return fmt.Errorf("missing required attribute: id")
    }
    if e.Source() == "" {
        return fmt.Errorf("missing required attribute: source")
    }
    if e.Type() == "" {
        return fmt.Errorf("missing required attribute: type")
    }
    if e.SpecVersion() != "1.0" {
        return fmt.Errorf("invalid specversion: %s", e.SpecVersion())
    }
    return nil
}

// Getters for CloudEvents attributes
func (e *Event) ID() string {
    v, _ := e.Attributes[AttrID].(string)
    return v
}

func (e *Event) Source() string {
    v, _ := e.Attributes[AttrSource].(string)
    return v
}

func (e *Event) SpecVersion() string {
    v, _ := e.Attributes[AttrSpecVersion].(string)
    return v
}

func (e *Event) Type() string {
    v, _ := e.Attributes[AttrType].(string)
    return v
}

func (e *Event) ContentType() string {
    v, _ := e.Attributes[AttrDataContentType].(string)
    return v
}

func (e *Event) Subject() string {
    v, _ := e.Attributes[AttrSubject].(string)
    return v
}

func (e *Event) Time() time.Time {
    v, _ := e.Attributes[AttrTime].(string)
    t, _ := time.Parse(time.RFC3339, v)
    return t
}

// FromMessage wraps a gopipe Message as a CloudEvent
func FromMessage(msg *message.Message) *Event {
    return &Event{Message: msg}
}

// ToMessage returns the underlying gopipe Message
func (e *Event) ToMessage() *message.Message {
    return e.Message
}
```

**Usage:**
```go
import "github.com/fxsml/gopipe/pubsub/cloudevents"

// Create CloudEvent
event := cloudevents.NewEvent(
    "order-123",
    "orders-service",
    "com.example.order.created",
    data,
    cloudevents.WithContentType("application/json"),
    cloudevents.WithSubject("order-123"),
    cloudevents.WithTime(time.Now()),
)

// Validate
if err := event.Validate(); err != nil {
    log.Fatal(err)
}

// Use as normal message
sender.Send(ctx, "orders", []*message.Message{event.ToMessage()})

// On receiver side
msg := <-messages
event := cloudevents.FromMessage(msg)
fmt.Println(event.Type())  // "com.example.order.created"
```

**Pros:**
- ✅ **Optional**: Users opt-in
- ✅ **No breaking changes**: Works with existing `Message`
- ✅ **Type-safe**: Helpers ensure correct attributes
- ✅ **Validation**: Can validate CloudEvents compliance
- ✅ **Interoperable**: Easy to convert to/from CloudEvents
- ✅ **Documented**: Clear CloudEvents semantics
- ✅ **Small surface area**: Separate package, not forced on everyone

**Cons:**
- ⚠️ **Extra package**: More code to maintain
- ⚠️ **Wrapper overhead**: Minor (just property access)

### Recommendation: CloudEvents Compatibility Layer

**Verdict:** Create `pubsub/cloudevents` package with:
1. Constants for CloudEvents attribute names
2. `Event` wrapper with typed getters/setters
3. `NewEvent` constructor with options
4. `Validate()` method
5. `FromMessage` / `ToMessage` converters

**Benefits:**
- Users who need CloudEvents get first-class support
- Users who don't need CloudEvents aren't affected
- gopipe remains flexible and unopinionated
- Easy interop with CloudEvents ecosystem

---

## Summary of Recommendations

### 1. Handler Design: Keep Current ✅

**No changes needed** - current design is idiomatic and correct:
- Non-generic `Handler` interface
- Marshalling in handlers (per-handler format support)
- `NewJSONHandler` for type-safe convenience

### 2. Pub/Sub Package Structure: Refactor 🔄

**Create dedicated `pubsub` package:**
```
pubsub/
├── pubsub.go        ← Interfaces (Sender, Receiver, Broker, etc.)
├── memory/          ← In-memory broker
├── http/            ← HTTP sender/receiver
├── io/              ← IO sender/receiver
├── channel/         ← NEW: Channel-based broker
└── cloudevents/     ← NEW: CloudEvents compatibility layer
```

**Changes:**
1. Move `message/pubsub.go` → `pubsub/pubsub.go`
2. Move `message/broker/*` → `pubsub/memory/`, `pubsub/http/`, `pubsub/io/`
3. Add `pubsub/channel/` (channel-based broker)
4. Add `pubsub/cloudevents/` (CloudEvents layer)
5. Update ADR-25 to reflect non-generic design

### 3. CloudEvents Compatibility: Add Layer 🆕

**Create `pubsub/cloudevents` package:**
- `Event` wrapper with CloudEvents semantics
- Typed getters for CloudEvents attributes
- Validation
- Conversion to/from `message.Message`

**Key principles:**
- Optional, not mandatory
- No breaking changes
- Works with existing `Message`
- Clean separation of concerns

---

## Migration Path

### Phase 1: Create `pubsub` Package

1. Create `pubsub/pubsub.go` with interfaces
2. Create `pubsub/memory/`, `pubsub/http/`, `pubsub/io/`
3. Keep `message/pubsub.go` as deprecated aliases (for compatibility)

### Phase 2: Add New Implementations

1. Implement `pubsub/channel/` broker
2. Implement `pubsub/cloudevents/` layer

### Phase 3: Update Examples and Docs

1. Update examples to use `pubsub` package
2. Update ADR-25
3. Create migration guide

### Phase 4: Deprecate Old Paths

1. Mark `message/pubsub.go` as deprecated
2. Mark `message/broker/` as deprecated
3. Keep for 2 major versions, then remove

---

## Conclusion

gopipe's current message design is **fundamentally sound**. The main improvements are organizational:

1. **Handler design** is already correct - no changes needed
2. **Package structure** needs refactoring for clarity - create `pubsub` package
3. **CloudEvents** should be optional layer - new `pubsub/cloudevents` package

These changes maintain backward compatibility while improving organization and adding new capabilities.

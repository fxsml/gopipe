# Message Refactoring Proposal: Generic to Non-Generic

## Problem Statement

The current `Message[T any]` type creates friction for pub/sub messaging patterns where:
1. All brokers use `[]byte` as the data format (Kafka, RabbitMQ, NATS, etc.)
2. Generic type parameters require repetitive boilerplate in every function signature
3. Type conversions create cognitive overhead for simple messaging use cases

**Current state** (verbose):
```go
message.New([]byte("hello"),
    message.WithID[[]byte]("msg-1"),
    message.WithAcking[[]byte](ack, nack),
)

func process(msg *message.Message[[]byte]) error {
    // ...
}
```

**Watermill** (simple):
```go
message.NewMessage("msg-1", []byte("hello"))

func process(msg *message.Message) error {
    // ...
}
```

## Proposal: Non-Generic Message

Follow Watermill's approach with gopipe enhancements.

### New Message Structure

```go
package message

// Message is a non-generic message with []byte data
type Message struct {
    uuid       string
    data    []byte
    attributes *Attributes
    a          *acking
}

// Constructor
func New(data []byte, opts ...Option) *Message {
    m := &Message{
        data:    data,
        attributes: NewAttributes(nil),
    }
    for _, opt := range opts {
        opt(m)
    }
    return m
}

// Simplified options (no type parameters)
func WithID(id string) Option
func WithAcking(ack func(), nack func(error)) Option
func WithAttribute(key string, value any) Option
```

### Benefits

1. **Simplicity**: No type parameters, cleaner signatures
2. **Compatibility**: Aligns with standard messaging libraries
3. **Broker integration**: Natural fit for pub/sub adapters
4. **Less boilerplate**: Shorter function signatures
5. **Easier migration**: Users coming from Watermill

### Drawbacks

1. **Type safety loss**: No compile-time data type checking
2. **Marshal overhead**: Always requires encoding/decoding
3. **Breaking change**: All existing code must be updated
4. **Generic pipeline loss**: Cannot use `Message[Order]`, `Message[int]`, etc.

## Alternative Approaches

### Option 1: Dual Types (Recommended)

Provide both generic and non-generic variants.

```go
// Non-generic for pub/sub
type Message struct {
    uuid       string
    data    []byte
    attributes *Attributes
    a          *acking
}

// Generic for typed pipelines
type TypedMessage[T any] struct {
    uuid       string
    data    T
    attributes *Attributes
    a          *acking
}

// Convert between them
func (m *Message) Typed() *TypedMessage[[]byte]
func Untyped[T any](m *TypedMessage[T], marshal func(T) ([]byte, error)) (*Message, error)
```

**Pros**:
- Best of both worlds
- No breaking changes if `TypedMessage` = current `Message`
- Pub/sub uses `Message`, pipelines use `TypedMessage[T]`

**Cons**:
- Two types to maintain
- Conversion overhead
- API complexity

### Option 2: Data Interface

Use `interface{}` data with type assertion helpers.

```go
type Message struct {
    uuid       string
    data    interface{}  // or any
    attributes *Attributes
    a          *acking
}

// Type assertion helpers
func (m *Message) Bytes() ([]byte, bool)
func (m *Message) String() (string, bool)
func (m *Message) As(v interface{}) error  // json.Unmarshal style
```

**Pros**:
- Single type
- Flexible data
- No generics

**Cons**:
- Runtime type errors
- No compile-time safety
- Less Go-idiomatic

### Option 3: Stay Generic, Add Convenience Alias

Keep `Message[T]`, add type alias for common case.

```go
// Current generic type
type Message[T any] struct { /* ... */ }

// Alias for messaging (just documentation)
type ByteMessage = Message[[]byte]

// Simplified constructors
func NewByteMessage(data []byte, opts ...ByteOption) *ByteMessage
```

**Pros**:
- Minimal changes
- Backward compatible
- Keeps type safety

**Cons**:
- Still verbose (`Message[[]byte]`)
- Alias doesn't remove type parameters from functions
- Doesn't solve the core problem

## Impact Analysis

### Current Usage in gopipe

```bash
$ grep -r "Message\[" . --include="*.go" | wc -l
68
```

**Files affected**:
- `message/message.go` (core implementation)
- `message/message_test.go` (all tests)
- `examples/message/main.go` (example)
- Any user code

### Migration Path

#### Breaking Change (v2.0.0)

**Before**:
```go
msg := message.New(42,
    message.WithID[int]("msg-1"),
    message.WithAcking[int](ack, nack),
)

func process(ctx context.Context, msg *message.Message[int]) error {
    val := msg.Data() // int
    return nil
}
```

**After** (non-generic):
```go
data, _ := json.Marshal(42)
msg := message.New(data,
    message.WithID("msg-1"),
    message.WithAcking(ack, nack),
)

func process(ctx context.Context, msg *message.Message) error {
    var val int
    json.Unmarshal(msg.Data(), &val)
    return nil
}
```

#### Gradual Migration (Dual Types)

**Step 1**: Rename current `Message[T]` → `TypedMessage[T]`
**Step 2**: Introduce new non-generic `Message`
**Step 3**: Deprecate `TypedMessage[T]` over time (or keep both)

## Recommendation

**Choose Option 1: Dual Types**

### Implementation Plan

```go
// message/message.go

// Message is a non-generic message for pub/sub use cases.
type Message struct {
    uuid       string
    data    []byte
    attributes *Attributes
    a          *acking
}

// TypedMessage is a generic message for type-safe pipelines.
// This is the current Message[T] renamed.
type TypedMessage[T any] struct {
    uuid       string
    data    T
    attributes *Attributes
    a          *acking
}

// New creates a non-generic message.
func New(data []byte, opts ...Option) *Message

// NewTyped creates a generic typed message.
func NewTyped[T any](data T, opts ...TypedOption[T]) *TypedMessage[T]

// Conversion helpers
func (m *Message) AsTyped() *TypedMessage[[]byte]
func ToMessage[T any](m *TypedMessage[T], marshal func(T) ([]byte, error)) (*Message, error)
```

### Example: Pub/Sub with Non-Generic

```go
// Publisher
func (p *KafkaPublisher) Publish(ctx context.Context, topic string, msg *message.Message) error {
    return p.producer.Produce(&kafka.Message{
        TopicPartition: kafka.TopicPartition{Topic: &topic},
        Value:          msg.Data(),
    })
}

// Subscriber
func (s *KafkaSubscriber) Subscribe(ctx context.Context, topic string) <-chan *message.Message {
    out := make(chan *message.Message)
    go func() {
        for {
            msg := s.consumer.ReadMessage(-1)
            out <- message.New(msg.Value,
                message.WithID(string(msg.Key)),
            )
        }
    }()
    return out
}

// Usage (clean!)
sub := kafka.Subscribe(ctx, "orders")
for msg := range sub {
    var order Order
    json.Unmarshal(msg.Data(), &order)
    // process...
    msg.Ack()
}
```

### Example: Pipeline with Generic

```go
// Type-safe pipeline (existing pattern)
pipe := gopipe.NewTransformPipe(
    func(ctx context.Context, msg *message.TypedMessage[Order]) (*message.TypedMessage[OrderConfirmed], error) {
        confirmed := OrderConfirmed{ID: msg.Data().ID}
        return message.Copy(msg, confirmed), nil
    },
)
```

## Version Strategy

### v2.0.0 Breaking Changes

1. **Rename** `Message[T]` → `TypedMessage[T]`
2. **Add** non-generic `Message` for pub/sub
3. **Restore** `Publisher`, `Subscriber`, `Router` using non-generic `Message`
4. **Update** examples to show both patterns

### Migration Guide

```markdown
# Migrating from v1 to v2

## For typed pipelines (no marshal/unmarshal)

Replace `message.Message[T]` with `message.TypedMessage[T]`:

-import "github.com/fxsml/gopipe/message"
+import "github.com/fxsml/gopipe/message"

-msg := message.New(42, message.WithID[int]("x"))
+msg := message.NewTyped(42, message.WithID[int]("x"))

-func process(msg *message.Message[int]) error
+func process(msg *message.TypedMessage[int]) error

## For pub/sub messaging (with marshal/unmarshal)

Use non-generic `message.Message`:

-msg := message.New([]byte("data"), message.WithID[[]byte]("x"))
+msg := message.New([]byte("data"), message.WithID("x"))

-func process(msg *message.Message[[]byte]) error
+func process(msg *message.Message) error
```

## Open Questions

1. **Should TypedMessage[T] be kept long-term or deprecated?**
   - **Keep**: Valuable for internal pipelines with type safety
   - **Deprecate**: Simpler API surface, push users to marshal/unmarshal

2. **Should Message and TypedMessage share the same Options?**
   - Current: `WithID[T]()` → separate for each
   - Alternative: `WithID()` → works for both (but how?)

3. **UUID field naming: uuid vs ID?**
   - Watermill uses `UUID` (string field, not actual UUID type)
   - gopipe uses `ID` property (more accurate naming)
   - Proposal: Keep `ID` in attributes, add `UUID()` accessor for compatibility

## Conclusion

**Recommended path forward**:

1. ✅ **Introduce non-generic `Message`** for pub/sub patterns
2. ✅ **Keep `TypedMessage[T]`** for type-safe pipelines
3. ✅ **Restore pub/sub primitives** using non-generic `Message`
4. ⚠️ **Breaking change in v2.0.0** with clear migration guide
5. 📝 **Document both patterns** with clear use case guidance

This balances:
- **Simplicity** for messaging use cases (like Watermill)
- **Type safety** for pipeline use cases (gopipe's strength)
- **Flexibility** for users to choose the right tool

The dual-type approach acknowledges that gopipe serves two distinct use cases:
1. **Messaging**: Broker integration, pub/sub, event-driven (use `Message`)
2. **Pipelining**: Data transformation, type-safe processing (use `TypedMessage[T]`)

Both are valid, both should be supported.

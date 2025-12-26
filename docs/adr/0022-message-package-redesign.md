# ADR 0022: Message Package Redesign

**Date:** 2025-12-25
**Status:** Proposed
**Incorporates:** ADR 0019, ADR 0020, ADR 0021

## Context

The message package grew too complex too early. Current state includes:

- `Sender`/`Receiver` interfaces (poll-based, doesn't fit push-based brokers)
- `Subscriber`/`Publisher` structs wrapping Sender/Receiver
- `Router` with attribute-based matching
- `Handler` with `Match(Attributes)` predicate
- Custom `cqrs/` subpackage with its own marshaler
- `multiplex/` subpackage for topic routing
- `broker/` subpackage with channel/http/io implementations
- `cloudevents/` subpackage with conversion utilities

**Problems:**

1. **Too much, too early** - Built infrastructure before understanding real needs
2. **Custom CloudEvents handling** - Reinventing what the official SDK provides
3. **Poll-based model** - `Receiver.Receive()` doesn't match modern brokers
4. **Complex routing** - Attribute matching is flexible but adds complexity
5. **No clear path** - Multiple overlapping patterns confuse users

## Decision

**Clean redesign of the message package:**

1. **Keep only essentials:**
   - `Message` (alias for `TypedMessage[[]byte]`)
   - `TypedMessage[T]` with `Data`, `Attributes`, `Ack()`, `Nack()`
   - `Attributes` map and accessor methods with consts for CloudEvents
   - `Acking` for acknowledgment coordination

2. **Remove everything else:**
   - `Sender`, `Receiver` interfaces
   - `Subscriber`, `Publisher` structs
   - `Router`, `Handler`, `Pipe`, `Generator` types
   - `Middleware` type
   - `broker/`, `cqrs/`, `multiplex/`, `cloudevents/` subpackages

3. **Engine in `message/` package:**
   - Orchestrates flow, doesn't own I/O lifecycle
   - `AddInput(ch, InputConfig)` - channel with optional Matcher for filtering
   - `AddOutput(OutputConfig)` - returns channel, routes by Matcher
   - `AddLoopback(LoopbackConfig)` - internal re-processing (bypasses marshal)
   - `AddHandler(h, HandlerConfig)` - config-based registration
   - Two handler types: explicit (`NewHandler`) and convention (`NewCommandHandler`)
   - Pattern-based output routing via Matcher (SQL LIKE syntax)
   - Handler creates complete messages, engine only sets DataContentType

4. **Codec, TypeRegistry, NamingStrategy (separate concerns):**
   - `Codec`: Pure serialization - `Marshal(v) []byte`, `Unmarshal(data, v)`, `ContentType()`
   - `TypeRegistry`: Maps CE type ↔ Go type - `Register(ceType, goType)`, `Lookup(ceType)`
   - `NamingStrategy`: Standalone utility - `TypeName(goType) string`
   - Handler is self-describing: `GoType()`, `EventType()`, `Handle()`
   - Handler constructors take NamingStrategy to derive EventType at construction

5. **CloudEvents bridge in `message/cloudevents/`:**
   - Adapter package imports both `message` and `cloudevents/sdk-go/v2`
   - `FromEvent(cloudevents.Event) *message.Message` - convert SDK event to message
   - `ToEvent(*message.Message) cloudevents.Event` - convert message to SDK event
   - Subscriber/Publisher adapters wrapping SDK clients
   - Core `message/` remains dependency-free

## Consequences

**Benefits:**

- **Simple core** - Message package does one thing well, no external deps
- **Native CloudEvents** - Official SDK for protocol/serialization via bridge
- **Clean slate** - No legacy patterns to maintain
- **Idiomatic Go** - Core is standalone, adapters add integrations
- **Separation of concerns** - Engine orchestrates, handlers create messages, marshalers serialize

**Breaking Changes (pre-v1):**

All removed types are breaking changes. Since we're pre-v1 with single user, this is acceptable.

## Usage

```go
import (
    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/message/match"
    ce "github.com/fxsml/gopipe/message/cloudevents"
)

// Create engine with Codec and TypeRegistry
engine := message.NewEngine(message.EngineConfig{
    Codec:    message.NewJSONCodec(),
    Registry: message.NewTypeRegistry(),
})

// Add handler (convention-based) - NamingStrategy in config
handler := message.NewCommandHandler(
    func(ctx context.Context, msg *TypedMessage[CreateOrder]) ([]OrderCreated, error) {
        return []OrderCreated{{OrderID: "123"}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders-service",
        Naming: message.KebabNaming,  // CreateOrder → "create.order"
    },
)
// handler.EventType() returns "create.order"
// handler.GoType() returns reflect.Type of CreateOrder

engine.AddHandler(handler, message.HandlerConfig{Name: "create-order"})

// Input: external subscription with optional Matcher for filtering
client, _ := cloudevents.NewClientHTTP()
subscriber := ce.NewSubscriber(client)
ch, _ := subscriber.Subscribe(ctx, "orders")
engine.AddInput(ch, message.InputConfig{Name: "order-events"})

// Output: pattern-based routing via Matcher
ordersOut := engine.AddOutput(message.OutputConfig{Matcher: match.Types("order.%")})
defaultOut := engine.AddOutput(message.OutputConfig{})  // nil = catch-all

// External publishing (Publish runs in goroutine internally)
publisher := ce.NewPublisher(client)
publisher.Publish(ctx, ordersOut)

// Start engine
done, _ := engine.Start(ctx)
<-done
```

## Implementation

See:
- [Plan 0001](../plans/0001-message-engine.md) - Engine implementation
- [Plan 0002](../plans/0002-marshaler.md) - Codec, TypeRegistry, NamingStrategy
- [Design State](../plans/0001-message-engine.state.md) - Current design decisions

## Links

- Incorporates: ADR 0019 (Remove Sender/Receiver)
- Incorporates: ADR 0020 (Message Engine Architecture)
- Incorporates: ADR 0021 (Codec/Marshaling Pattern)
- Related: ADR 0018 (Interface Naming Conventions)

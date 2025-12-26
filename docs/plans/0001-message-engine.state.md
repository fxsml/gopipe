# Plan 0001: Message Engine - Design State

**Status:** In Progress
**Last Updated:** 2025-12-26

## Current Design Decisions

### Separation of Concerns

| Component | Responsibility |
|-----------|----------------|
| **Engine** | Orchestrates flow, routing, lifecycle |
| **Codec** | Pure serialization (Marshal/Unmarshal/ContentType) |
| **TypeRegistry** | Maps CE type ↔ Go type for unmarshaling |
| **Handler** | Self-describing (GoType, EventType), business logic |
| **NamingStrategy** | Standalone utility, used by handler constructors |
| **Input/Output** | Named channels (external lifecycle) |

### Engine - Orchestration Only

Engine accepts channels, doesn't own I/O lifecycle.

```go
// Input: channel is primary, config has optional name for tracing
engine.AddInput(ch, message.InputConfig{Name: "order-events"})
engine.AddInput(ch, message.InputConfig{})  // no name

// Output: match pattern is primary, returns channel directly
ordersOut := engine.AddOutput(message.OutputConfig{Match: "Order*"})
defaultOut := engine.AddOutput(message.OutputConfig{Match: "*"})  // catch-all

// External code manages subscription lifecycle
subscriber := ce.NewSubscriber(client)
ch, _ := subscriber.Subscribe(ctx, "orders")
engine.AddInput(ch, message.InputConfig{})

// External code manages publishing lifecycle (Publish runs in goroutine internally)
publisher := ce.NewPublisher(client)
publisher.Publish(ctx, ordersOut)
```

### InputConfig / OutputConfig

Both use Matcher interface. Nil Matcher = match all (returns true).

```go
type InputConfig struct {
    Name    string   // optional, for tracing/metrics
    Matcher Matcher  // optional, nil = match all
}

type OutputConfig struct {
    Name    string   // optional, for logging/metrics
    Matcher Matcher  // optional, nil = match all (catch-all)
}

type LoopbackConfig struct {
    Name    string   // optional, for tracing/metrics
    Matcher Matcher  // required, matches output to loop back
}

type HandlerConfig struct {
    Name string  // required, handler name for logging/metrics
}
// Note: CE type comes from handler.EventType(), not config
```

### Matcher Interface and Match Package

Interface in `message/`, implementations in `message/match/`:

```go
// message/matcher.go
type Matcher interface {
    Match(msg *Message) bool
}

// message/match/match.go
import "github.com/fxsml/gopipe/message/match"

func All(matchers ...message.Matcher) message.Matcher   // AND
func Any(matchers ...message.Matcher) message.Matcher   // OR
func Sources(patterns ...string) message.Matcher        // CE source filter
func Types(patterns ...string) message.Matcher          // CE type filter
```

**Default behavior (nil Matcher = match all):**

```go
// Nil Matcher returns true for all messages - no expression evaluation
engine.AddInput(ch, message.InputConfig{})  // accepts all
engine.AddOutput(message.OutputConfig{})    // catch-all output
```

**Usage with matchers:**

```go
// Input filtering
engine.AddInput(ch, message.InputConfig{
    Name: "order-events",
    Matcher: match.All(
        match.Sources("https://my-domain.com/orders/%"),
        match.Types("order.%"),
    ),
})

// Output routing
ordersOut := engine.AddOutput(message.OutputConfig{
    Matcher: match.Types("order.%"),
})
defaultOut := engine.AddOutput(message.OutputConfig{})  // catch-all
```

**Why separate package?**
- Clean namespace: `match.All` vs `message.MatchAll`
- Future CESQL can isolate CE-SDK dependency
- Interface stays in core `message/`

### SQL LIKE Pattern Syntax

Used by match.Types() and match.Sources():
- `%` matches any sequence of characters
- `_` matches a single character

**Patterns:**
- `"order.%"` - prefix match (order.created, order.shipped)
- `"%.created"` - suffix match (order.created, user.created)
- `"order.created"` - exact match

**Output routing priority:**
1. First output with matching Matcher wins
2. Nil Matcher (catch-all) should be registered last

### Codec and TypeRegistry - Separate Concerns

Codec handles pure serialization. TypeRegistry maps CE types to Go types.

```go
// Codec - pure serialization, no type awareness
type Codec interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
    ContentType() string  // e.g., "application/json"
}

// TypeRegistry - maps CE type ↔ Go type
type TypeRegistry interface {
    Register(ceType string, goType reflect.Type)
    Lookup(ceType string) (reflect.Type, bool)
}

// NamingStrategy - standalone utility
type NamingStrategy interface {
    TypeName(t reflect.Type) string  // Go type → CE type
}

var KebabNaming NamingStrategy   // OrderCreated → "order.created"
var SnakeNaming NamingStrategy   // OrderCreated → "order_created"
```

**Registration flow (via handler):**
```go
handler := message.NewHandler(processOrder, message.KebabNaming)
// handler.EventType() returns "order.created"
// handler.GoType() returns reflect.Type of OrderCreated

engine.AddHandler(handler, HandlerConfig{Name: "process-order"})
// 1. Engine reads handler.EventType() → "order.created"
// 2. Engine reads handler.GoType() → OrderCreated
// 3. Engine registers in TypeRegistry: "order.created" → OrderCreated
// 4. Engine registers route: typeRoutes["order.created"] = "process-order"
```

### Handler - Self-Describing with NamingStrategy

Handler is self-describing - knows its CE type and Go type at construction time.

```go
type Handler interface {
    GoType() reflect.Type    // Go type - for TypeRegistry
    EventType() string       // CE type - for routing
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}

// Generic constructor - NamingStrategy derives EventType from T
func NewHandler[T any](
    fn func(ctx context.Context, msg *TypedMessage[T]) ([]*Message, error),
    naming NamingStrategy,
) Handler

// CQRS constructor - NamingStrategy derives EventType from C, output types from E
func NewCommandHandler[C, E any](
    fn func(ctx context.Context, msg *TypedMessage[C]) ([]E, error),
    cfg CommandHandlerConfig,
) Handler

// Registration - Name only, CE type comes from handler.EventType()
engine.AddHandler(handler, message.HandlerConfig{Name: "process-order"})
```

### CommandHandlerConfig

```go
type CommandHandlerConfig struct {
    Source string          // required, CE source attribute
    Naming NamingStrategy  // derives CE types for input (C) and output (E)
}

// Usage
handler := message.NewCommandHandler(
    func(ctx context.Context, msg *TypedMessage[CreateOrder]) ([]OrderCreated, error) {
        return []OrderCreated{{OrderID: "123"}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders-service",
        Naming: message.KebabNaming,
    },
)
// handler.EventType() returns "create.order" (from CreateOrder)
// Output messages get Type "order.created" (from OrderCreated)
```

**CommandHandler auto-generates for output messages:**
- `ID`: UUID
- `SpecVersion`: "1.0"
- `Time`: now()
- `Type`: via config.Naming (e.g., `OrderCreated` → `"order.created"`)
- `Source`: from config.Source

### NewHandler - Explicit Messages

`NewHandler` requires handler to create complete messages with all attributes:

```go
handler := message.NewHandler(
    func(ctx context.Context, msg *TypedMessage[OrderCreated]) ([]*Message, error) {
        order := msg.Data  // typed access
        return []*Message{
            message.New(OrderShipped{OrderID: order.ID}, message.Attributes{
                ID:          uuid.New().String(),
                SpecVersion: "1.0",
                Type:        "order.shipped",
                Source:      "/orders-service",
                Time:        time.Now(),
            }),
        }, nil
    },
    message.KebabNaming,  // derives handler.EventType() from OrderCreated
)
// handler.EventType() returns "order.created"
// handler.GoType() returns reflect.Type of OrderCreated
```

Use optional validation middleware to ensure CE compliance:
```go
engine.Use(message.ValidateCE())  // validates required CE attributes
```

**Summary:**
- `NewHandler`: Takes NamingStrategy, handler creates complete output messages
- `NewCommandHandler`: Takes config with Source + Naming, auto-generates output message attributes

### DataContentType

Engine sets `DataContentType` at marshal boundary (from Codec.ContentType()):

```
AddInput → Unmarshal → Handler (typed) → Marshal (sets DataContentType) → Output
       (Codec+Registry)                    (Codec)
                ↑                                        │
                └─────── loopback (no marshal) ──────────┘
```

- Unmarshal happens directly after AddInput (uses Codec + TypeRegistry)
- Marshal happens directly before Output (uses Codec)
- Loopback bypasses marshal/unmarshal (already typed messages)
- Handler deals with typed data, not bytes

### Middleware

Correlation ID middleware for first draft (optional):

```go
engine.Use(message.WithCorrelationID())  // propagates or generates correlation ID
```

| Attribute | Owner |
|-----------|-------|
| `DataContentType` | Engine (from Codec.ContentType()) |
| `Type` | Handler (via NamingStrategy at construction) |
| `Source` | Handler or CommandHandlerConfig |
| `Subject` | Handler (explicit) |
| `ID`, `Time`, `SpecVersion` | Handler or CommandHandler |
| `CorrelationID` | Middleware (optional) |

### Routing

**Ingress (CE type → handler):**
```go
handler := message.NewHandler(processOrder, message.KebabNaming)
// handler.EventType() returns "order.created"

engine.AddHandler(handler, message.HandlerConfig{Name: "process-orders"})
// Engine routes by handler.EventType()
```

**Egress (handler output → output channel):**
```go
// Pattern matching in OutputConfig - first matching output wins
ordersOut := engine.AddOutput(message.OutputConfig{Matcher: match.Types("order.%")})
defaultOut := engine.AddOutput(message.OutputConfig{})  // catch-all (nil = match all)
```

**Loopback:**
```go
// Dedicated function for internal re-processing
engine.AddLoopback(message.LoopbackConfig{
    Name:    "validation-loop",
    Matcher: match.Types("validate.%"),
})
```

### Error Handling for Routing

```go
var (
    ErrInputRejected    = errors.New("message rejected by input matcher")
    ErrNoMatchingOutput = errors.New("no output matches message type")
    ErrNoHandler        = errors.New("no handler for CE type")
)
```

**Routing errors:**
- Input rejected by Matcher → `ErrInputRejected` to ErrorHandler
- No handler for CE type → `ErrNoHandler` to ErrorHandler
- No output matches → try next output, finally `ErrNoMatchingOutput` to ErrorHandler

### NamingStrategy - Standalone Utility

NamingStrategy is a standalone utility used by handler constructors:

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string  // Go type → CE type
}

var KebabNaming NamingStrategy   // OrderCreated → "order.created"
var SnakeNaming NamingStrategy   // OrderCreated → "order_created"
```

**Used by:**
- `NewHandler(fn, naming)` - derives EventType from T
- `NewCommandHandler(fn, cfg)` - derives EventType from C, output types from E

**Not used by:**
- Engine (reads handler.EventType() directly)
- Codec (pure serialization)
- TypeRegistry (explicit registration)

### Attributes

- No per-message routing attribute by default (pattern-based output matching)
- Input name (optional) set on message for tracing
- Tracing/observability is middleware concern, not engine

## Rejected Ideas

### Handler owns its name
```go
type Handler interface {
    Name() string  // ❌
}
```
**Why rejected:** Name is wiring concern, not handler logic.

### Variadic config for handlers
```go
func NewCommandHandler[C, E any](fn ..., cfg ...Config) Handler  // ❌
```
**Why rejected:** Not idiomatic Go. Config is required parameter.

### Per-message routing attribute
```go
message.New(event, Attributes{Output: "shipments"})  // ❌
```
**Why rejected:** Routing by pattern matching on CE type. No per-message routing needed.

### Named outputs with RouteOutput
```go
engine.AddOutput("shipments", ch)  // ❌ name as primary
engine.RouteOutput("handler", "shipments")  // ❌ explicit routing
out := engine.Output("shipments")  // ❌ getter by name
```
**Why rejected:** Match pattern is primary. AddOutput returns channel directly. Names are optional metadata.

### Engine owns I/O lifecycle
```go
engine.AddSubscriber("orders", subscriber)  // ❌ Engine subscribes internally
```
**Why rejected:** Doesn't handle leader election, dynamic scaling. External concern.

### Marshaler with TypeName and Registry
```go
type Marshaler interface {
    Register(goType reflect.Type)        // ❌
    TypeName(goType reflect.Type) string // ❌
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, ceType string) (any, error)
}
```
**Why rejected:** Marshaler was doing too much. Split into:
- Codec (pure serialization)
- TypeRegistry (CE type ↔ Go type mapping)
- NamingStrategy (utility for handler constructors)

### Single ambiguous Route() method
```go
engine.Route("order.created", "process-orders")  // ❌
```
**Why rejected:** Two methods are more explicit.

### Explicit RouteType for ingress
```go
engine.RouteType("order.created", "process-orders")  // ❌
```
**Why rejected:** Redundant. Handler is self-describing - engine reads `handler.EventType()` directly.

### Manual loopback via AddOutput/AddInput
```go
loopback := engine.AddOutput(OutputConfig{Matcher: ...})
engine.AddInput(loopback, InputConfig{Name: "loopback"})  // ❌
```
**Why rejected:** Dedicated `AddLoopback()` is cleaner and avoids marshal/unmarshal overhead.

## Idiomaticity Review

| Aspect | Pattern | Idiomatic? |
|--------|---------|------------|
| Handler interface | `GoType()`, `EventType()`, `Handle()` - self-describing | ✅ Interface tells you what it is |
| Handler constructors | `NewHandler[T](fn, naming)`, `NewCommandHandler[C,E](fn, cfg)` | ✅ Go generic pattern |
| Config pattern | Required config for AddInput/AddOutput/CommandHandler | ✅ Explicit over magic |
| Interface segregation | Codec, TypeRegistry, NamingStrategy, Handler separate | ✅ Single responsibility |
| Engine lifecycle | Accepts channels, doesn't own I/O | ✅ Composition over inheritance |
| Input/Output | Config-based, names optional | ✅ Primary data first |
| Output routing | Pattern matching on CE type | ✅ Declarative |
| Optional features | Middleware (`Use()`) | ✅ Standard pattern |

## Open Questions

1. **Loopback detection**: How to detect/prevent infinite loops when output feeds back to input?
2. **No match**: What happens to messages matching no output pattern? Error? Drop? (catch-all recommended)
3. **Middleware order**: Pre-handler vs post-handler middleware? Or single chain?
4. **Handler error handling**: Per-handler error handler or engine-level only?
5. **CESQL support**: Full CESQL or subset? Dependency on cloudevents/sdk-go? (see Plan 0003)

## Ready to Implement

Core components are well-defined:

| Component | Status | Notes |
|-----------|--------|-------|
| NamingStrategy | ✅ Ready | Standalone utility, KebabNaming, SnakeNaming |
| Codec | ✅ Ready | Pure serialization interface |
| JSONCodec | ✅ Ready | JSON implementation |
| TypeRegistry | ✅ Ready | CE type ↔ Go type mapping |
| Matcher | ✅ Ready | Interface in message/, implementations in match/ |
| match.All/Any | ✅ Ready | Combinators for AND/OR |
| match.Sources | ✅ Ready | CE source pattern matching |
| match.Types | ✅ Ready | CE type pattern matching |
| Handler | ✅ Ready | Self-describing with GoType(), EventType() |
| NewHandler | ✅ Ready | Takes NamingStrategy, explicit output messages |
| NewCommandHandler | ✅ Ready | Takes config with Source + Naming |
| HandlerConfig | ✅ Ready | Name only (CE type from handler.EventType()) |
| InputConfig | ✅ Ready | Optional name + Matcher for filtering |
| OutputConfig | ✅ Ready | Matcher for routing (SQL LIKE syntax) |
| LoopbackConfig | ✅ Ready | Name + Matcher for internal re-processing |
| Engine | ✅ Ready | AddInput, AddOutput, AddLoopback, AddHandler, Start() |
| Errors | ✅ Ready | ErrInputRejected, ErrNoMatchingOutput, ErrNoHandler |
| Middleware | ✅ Ready | ValidateCE, WithCorrelationID (optional) |

## Implementation Order

1. `message/naming.go` - NamingStrategy interface, KebabNaming, SnakeNaming
2. `message/codec.go` - Codec interface
3. `message/json_codec.go` - JSONCodec implementation
4. `message/registry.go` - TypeRegistry interface and implementation
5. `message/matcher.go` - Matcher interface
6. `message/match/like.go` - SQL LIKE pattern matching
7. `message/match/types.go` - Types matcher
8. `message/match/sources.go` - Sources matcher
9. `message/match/match.go` - All, Any combinators
10. `message/config.go` - InputConfig, OutputConfig, LoopbackConfig, HandlerConfig
11. `message/handler.go` - Handler interface, NewHandler, NewCommandHandler
12. `message/errors.go` - ErrInputRejected, ErrNoMatchingOutput, ErrNoHandler
13. `message/engine.go` - Engine with AddInput, AddOutput, AddLoopback, AddHandler
14. `message/middleware.go` - ValidateCE, WithCorrelationID (optional)
15. `message/cloudevents/` - CE adapter package (optional)

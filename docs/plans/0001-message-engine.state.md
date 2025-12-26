# Plan 0001: Message Engine - Design State

**Status:** In Progress
**Last Updated:** 2025-12-26

## Current Design Decisions

### Separation of Concerns

| Component | Responsibility |
|-----------|----------------|
| **Engine** | Orchestrates flow, routing, lifecycle |
| **Marshaler** | Serialization + type registry + NamingStrategy |
| **Handler** | Business logic, creates output messages |
| **NamingStrategy** | Derives CE type from Go type (in Marshaler) |
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
    Name string  // required, handler name for registration
    Type string  // optional, CE type to handle (derived via marshaler if not set)
}
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

### Marshaler - Type Registry with NamingStrategy

Marshaler handles serialization, type registry, and CE type derivation via NamingStrategy.

```go
type Marshaler interface {
    Register(goType reflect.Type)              // register Go type, CE type derived via NamingStrategy
    TypeName(goType reflect.Type) string       // Go type → CE type (lookup or derive + auto-register)
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, ceType string) (any, error)
    ContentType() string
}
```

**Bidirectional registry:**
```
CE Type → Go Type    │    Go Type → CE Type
─────────────────    │    ─────────────────
"order.created" → OrderCreated
"order.shipped" → OrderShipped
```

- **Unmarshal**: CE type string → Go type → instantiate
- **TypeName**: Go type → CE type string (auto-registers if not found)

**Auto-registration flow:**
```go
engine.AddHandler(handler, HandlerConfig{Name: "process-order"})
// 1. Engine calls marshaler.TypeName(handler.EventType())
// 2. Marshaler derives: OrderCreated → "order.created" via NamingStrategy
// 3. Marshaler registers bidirectional mapping
// 4. Engine registers: typeRoutes["order.created"] = "process-order"
```

### Handler - Constructors, Config-based Registration

Handlers created via constructors, registered with config on engine.

```go
type Handler interface {
    EventType() reflect.Type
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}

// Generic constructor - typed access to message data
func NewHandler[T any](fn func(ctx context.Context, msg *TypedMessage[T]) ([]*Message, error)) Handler

// CQRS constructor - always requires config
func NewCommandHandler[C, E any](
    fn func(ctx context.Context, msg *TypedMessage[C]) ([]E, error),
    cfg CommandHandlerConfig,
) Handler

// Registration with config (consistent with AddInput, AddOutput, AddLoopback)
engine.AddHandler(handler, message.HandlerConfig{
    Name: "process-order",
    // Type: "order.created" (optional, derived via marshaler if not set)
})
```

### CommandHandlerConfig

```go
type CommandHandlerConfig struct {
    // Source for CE source attribute (required, no default)
    Source string
}

// Usage
handler := message.NewCommandHandler(
    func(ctx context.Context, msg *TypedMessage[CreateOrder]) ([]OrderCreated, error) {
        return []OrderCreated{{OrderID: "123"}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders-service",  // required
    },
)
```

**CommandHandler auto-generates:**
- `ID`: UUID
- `SpecVersion`: "1.0"
- `Time`: now()
- `Type`: via marshaler's NamingStrategy (e.g., `OrderCreated` → `"order.created"`)
- `Source`: from config

### NewHandler - Explicit Messages

`NewHandler` requires handler to create complete messages with all attributes:

```go
handler := message.NewHandler(func(ctx context.Context, msg *TypedMessage[OrderCreated]) ([]*Message, error) {
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
})
```

Use optional validation middleware to ensure CE compliance:
```go
engine.Use(message.ValidateCE())  // validates required CE attributes
```

**Summary:**
- `NewHandler`: Explicit - handler creates complete messages with all attributes
- `NewCommandHandler`: Convention-based - uses NamingStrategy to auto-generate attributes

### DataContentType

Engine sets `DataContentType` at marshal boundary (from marshaler):

```
AddInput → Unmarshal → Handler (typed) → Marshal (sets DataContentType) → Output
                ↑                                        │
                └─────── loopback (no marshal) ──────────┘
```

- Unmarshal happens directly after AddInput
- Marshal happens directly before Output
- Loopback bypasses marshal/unmarshal (already typed messages)
- Handler deals with typed data, not bytes

### Middleware

Correlation ID middleware for first draft:

```go
engine.Use(message.WithCorrelationID())  // propagates or generates correlation ID
```

| Attribute | Owner |
|-----------|-------|
| `DataContentType` | Engine (from marshaler) |
| `Type` | Handler or NamingStrategy |
| `Source` | Handler or Config |
| `Subject` | Handler (explicit) |
| `ID`, `Time`, `SpecVersion` | Handler or CommandHandler |
| `CorrelationID` | Middleware |

### Routing

**Ingress (CE type → handler):**
```go
engine.AddHandler(handler, message.HandlerConfig{
    Name: "process-orders",
    // Type: "order.created" (optional, derived via marshaler if not set)
})
// If Type not set: marshaler.TypeName(handler.EventType()) → "order.created"
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

### NamingStrategy - Marshaler Only

NamingStrategy lives in Marshaler only. Engine uses marshaler to derive CE type:

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string  // Go type → CE type
}

var KebabNaming NamingStrategy   // OrderCreated → "order.created"
var SnakeNaming NamingStrategy   // OrderCreated → "order_created"
```

**CE type derivation:**
```go
// If HandlerConfig.Type is empty:
ceType := marshaler.TypeName(handler.EventType())  // "order.created"
```

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

### Heavy Marshaler with naming
```go
type Marshaler interface {
    TypeName(v any) string     // ❌ separate concern
    IsCommand(v any) bool      // ❌ separate concern
}
```
**Why rejected:** Marshaler should be lightweight. Naming is separate strategy.

### Single ambiguous Route() method
```go
engine.Route("order.created", "process-orders")  // ❌
```
**Why rejected:** Two methods are more explicit.

### Explicit RouteType for ingress
```go
engine.RouteType("order.created", "process-orders")  // ❌
```
**Why rejected:** Redundant. Engine auto-derives routing from `handler.EventType()` via NamingStrategy. Type registry in Marshaler already knows the mapping.

### Manual loopback via AddOutput/AddInput
```go
loopback := engine.AddOutput(OutputConfig{Matcher: ...})
engine.AddInput(loopback, InputConfig{Name: "loopback"})  // ❌
```
**Why rejected:** Dedicated `AddLoopback()` is cleaner and avoids marshal/unmarshal overhead.

## Idiomaticity Review

| Aspect | Pattern | Idiomatic? |
|--------|---------|------------|
| Handler constructors | `NewHandler[T]()`, `NewCommandHandler[C,E](fn, cfg)` | ✅ Go generic pattern |
| Config pattern | Required config for AddInput/AddOutput/CommandHandler | ✅ Explicit over magic |
| Interface segregation | Marshaler, NamingStrategy, Handler separate | ✅ Single responsibility |
| Engine lifecycle | Accepts channels, doesn't own I/O | ✅ Composition over inheritance |
| Input/Output | Config-based, names optional | ✅ Primary data first |
| Output routing | Pattern matching on CE type | ✅ Declarative |
| Optional features | Middleware (`Use()`) | ✅ Standard pattern |

## Open Questions

1. **Loopback detection**: How to detect/prevent infinite loops when output feeds back to input?
2. **No match**: What happens to messages matching no output pattern? Error? Drop? (catch-all `"*"` recommended)
3. **Type auto-registration**: Should `NewCommandHandler` auto-register input/output types with marshaler?
4. **Middleware order**: Pre-handler vs post-handler middleware? Or single chain?
5. **Handler error handling**: Per-handler error handler or engine-level only?
6. **CESQL support**: Full CESQL or subset? Dependency on cloudevents/sdk-go?

## Ready to Implement

Core components are well-defined:

| Component | Status | Notes |
|-----------|--------|-------|
| NamingStrategy | ✅ Ready | Interface + KebabNaming, SnakeNaming (in Marshaler) |
| Marshaler | ✅ Ready | Serialization + type registry + NamingStrategy |
| Matcher | ✅ Ready | Interface in message/, implementations in match/ |
| match.All/Any | ✅ Ready | Combinators for AND/OR |
| match.Sources | ✅ Ready | CE source pattern matching |
| match.Types | ✅ Ready | CE type pattern matching |
| Handler | ✅ Ready | NewHandler (explicit), NewCommandHandler (convention) |
| HandlerConfig | ✅ Ready | Name (required) + Type (optional, derived via marshaler) |
| InputConfig | ✅ Ready | Optional name + Matcher for filtering |
| OutputConfig | ✅ Ready | Matcher for routing (SQL LIKE syntax) |
| LoopbackConfig | ✅ Ready | Name + Matcher for internal re-processing |
| Engine | ✅ Ready | AddInput, AddOutput, AddLoopback, AddHandler, Start() |
| Errors | ✅ Ready | ErrInputRejected, ErrNoMatchingOutput, ErrNoHandler |
| Middleware | ✅ Ready | ValidateCE, WithCorrelationID |

## Implementation Order

1. `message/naming.go` - NamingStrategy interface and implementations
2. `message/marshaler.go` - Marshaler interface
3. `message/json_marshaler.go` - JSONMarshaler
4. `message/matcher.go` - Matcher interface
5. `message/match/like.go` - SQL LIKE pattern matching
6. `message/match/match.go` - All, Any combinators
7. `message/match/sources.go` - Sources matcher
8. `message/match/types.go` - Types matcher
9. `message/config.go` - InputConfig, OutputConfig, LoopbackConfig, HandlerConfig
10. `message/handler.go` - Handler interface, NewHandler, NewCommandHandler
11. `message/middleware.go` - ValidateCE, WithCorrelationID
12. `message/errors.go` - ErrInputRejected, ErrNoMatchingOutput, ErrNoHandler, etc.
13. `message/engine.go` - Engine with AddInput, AddOutput, AddLoopback, AddHandler (auto-routing)
14. `message/cloudevents/` - CE adapter package

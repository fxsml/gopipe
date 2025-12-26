# Plan 0001: Message Engine - Design State

**Status:** In Progress
**Last Updated:** 2025-12-26

## Current Design Decisions

### Separation of Concerns

| Component | Responsibility |
|-----------|----------------|
| **Engine** | Orchestrates flow, routing, lifecycle |
| **Marshaler** | Serialization + type registry |
| **Handler** | Business logic, creates output messages |
| **NamingStrategy** | Convention for deriving CE types/names |
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

```go
type InputConfig struct {
    Name    string   // optional, for tracing/metrics
    Matcher Matcher  // optional, defense in depth filtering
}

type OutputConfig struct {
    Name  string  // optional, for logging/metrics
    Match string  // required: "%", "order.%", uses SQL LIKE syntax
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

**Usage:**

```go
engine.AddInput(ch, message.InputConfig{
    Name: "order-events",
    Matcher: match.All(
        match.Sources("https://my-domain.com/orders/%"),
        match.Types("order.%"),
    ),
})
```

**Why separate package?**
- Clean namespace: `match.All` vs `message.MatchAll`
- Future CESQL can isolate CE-SDK dependency
- Interface stays in core `message/`

### SQL LIKE Pattern Syntax

Uses SQL LIKE for consistency with CESQL:
- `%` matches any sequence of characters
- `_` matches a single character

**Match patterns:**
- `"%"` - catch-all (default output)
- `"order.%"` - prefix match (order.created, order.shipped)
- `"%.created"` - suffix match (order.created, user.created)
- `"order.created"` - exact match

**Matching priority:**
1. Exact match
2. Prefix/suffix match
3. Catch-all `"%"`

### Marshaler - Lightweight with Registry

Marshaler handles serialization and type registry (needed for unmarshal).

```go
type Marshaler interface {
    Register(v any)                                // register type, derive CE type name
    Marshal(v any) ([]byte, string, error)         // returns data, CE type, error
    Unmarshal(data []byte, ceType string) (any, error)
    ContentType() string
}
```

Registry is required because `Unmarshal` receives CE type string, needs to know which Go type to instantiate.

### Handler - Constructors, Not Engine Methods

Handlers created via constructors, registered by name on engine.

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

// Registration
engine.AddHandler("process-order", handler)
```

### CommandHandlerConfig

```go
type CommandHandlerConfig struct {
    // NamingStrategy derives CE type and output name from Go types
    // Default: KebabNaming
    Naming NamingStrategy

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
        // Naming: message.KebabNaming (default)
    },
)
```

**CommandHandler auto-generates:**
- `ID`: UUID
- `SpecVersion`: "1.0"
- `Time`: now()
- `Type`: via `Naming.TypeName()` (e.g., `OrderCreated` → `"order.created"`)
- `Source`: from config
- Output routing: via `Naming.OutputName()` (e.g., `OrderCreated` → `"orders"`)

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
engine.RouteType("order.created", "process-orders")   // explicit
// Or convention: Handler's EventType() derives CE type via NamingStrategy
```

**Egress (handler output → output channel):**
```go
// Pattern matching in OutputConfig - no explicit routing needed
ordersOut := engine.AddOutput(message.OutputConfig{Match: "Order*"})
// Handler returns OrderCreated → matches "Order*" → goes to ordersOut
```

**Loopback:**
```go
// Output that feeds back to engine input
loopback := engine.AddOutput(message.OutputConfig{Match: "Validate*"})
engine.AddInput(loopback, message.InputConfig{Name: "loopback"})
```

### Routing Strategies

```go
type RoutingStrategy int

const (
    ConventionRouting RoutingStrategy = iota  // default: auto-route by naming
    ExplicitRouting                           // manual RouteType calls required
)
```

**Convention rules:**
- Ingress: Handler's `EventType()` derives CE type via NamingStrategy
- Egress: Output's `Match` pattern matches message CE type

### NamingStrategy - Separate Concern

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string    // Go type → CE type
    OutputName(t reflect.Type) string  // Go type → output name
}

// CQRS convention
var CQRSNaming NamingStrategy  // CreateOrder→"create.order", OrderCreated→"orders"
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
| NamingStrategy | ✅ Ready | Interface + KebabNaming, SnakeNaming |
| Marshaler | ✅ Ready | Lightweight, uses NamingStrategy |
| Matcher | ✅ Ready | Interface in message/, implementations in match/ |
| match.All/Any | ✅ Ready | Combinators for AND/OR |
| match.Sources | ✅ Ready | CE source pattern matching |
| match.Types | ✅ Ready | CE type pattern matching |
| Handler | ✅ Ready | NewHandler (explicit), NewCommandHandler (convention) |
| InputConfig | ✅ Ready | Optional name + Matcher for filtering |
| OutputConfig | ✅ Ready | Match pattern (SQL LIKE syntax) |
| Engine | ✅ Ready | AddInput, AddOutput, RouteType, Start() |
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
9. `message/config.go` - InputConfig, OutputConfig
10. `message/handler.go` - Handler interface, NewHandler, NewCommandHandler
11. `message/middleware.go` - ValidateCE, WithCorrelationID
12. `message/errors.go` - Error definitions
13. `message/engine.go` - Engine implementation
14. `message/cloudevents/` - CE adapter package

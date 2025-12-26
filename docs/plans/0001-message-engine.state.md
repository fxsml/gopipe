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

// External code manages publishing lifecycle
publisher := ce.NewPublisher(client)
go publisher.Publish(ctx, ordersOut)
```

### InputConfig / OutputConfig

```go
type InputConfig struct {
    Name string  // optional, for tracing/metrics
}

type OutputConfig struct {
    Name  string  // optional, for logging/metrics
    Match string  // required: "*", "Order*", "order.*", or CESQL
}
```

**Match patterns:**
- `"*"` - catch-all (default output)
- `"Order*"` - Go type prefix match
- `"order.*"` - CE type prefix match
- CESQL: `"type LIKE 'order.%' AND data.priority = 'high'"`

**Matching priority:**
1. Exact match
2. Prefix match
3. CESQL expression
4. Catch-all `"*"`

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
Input → Unmarshal (engine) → Handler (typed) → Marshal (engine, sets DataContentType) → Output
```

Handler deals with typed data, not bytes. Engine knows the marshaler and sets content type.

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
| Handler | ✅ Ready | NewHandler (explicit), NewCommandHandler (convention) |
| InputConfig | ✅ Ready | Optional name for tracing |
| OutputConfig | ✅ Ready | Match pattern (wildcards, CESQL) |
| Engine | ✅ Ready | AddInput, AddOutput, RouteType, Start() |
| Middleware | ✅ Ready | ValidateCE, WithCorrelationID |

## Implementation Order

1. `message/naming.go` - NamingStrategy interface and implementations
2. `message/marshaler.go` - Marshaler interface
3. `message/json_marshaler.go` - JSONMarshaler
4. `message/config.go` - InputConfig, OutputConfig
5. `message/match.go` - Pattern matching for OutputConfig.Match
6. `message/handler.go` - Handler interface, NewHandler, NewCommandHandler
7. `message/middleware.go` - ValidateCE, WithCorrelationID
8. `message/errors.go` - Error definitions
9. `message/engine.go` - Engine implementation
10. `message/cloudevents/` - CE adapter package

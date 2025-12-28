# Plan 0001: Message Engine - Design State

**Status:** In Progress
**Last Updated:** 2025-12-26

## Current Design Decisions

### Separation of Concerns

| Component | Responsibility |
|-----------|----------------|
| **Engine** | Orchestrates flow, routing, lifecycle |
| **Marshaler** | Pure serialization (Marshal/Unmarshal/DataContentType) |
| **Handler** | Self-describing (EventType, NewInput), business logic |
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

### Marshaler - Pure Serialization

Marshaler handles pure serialization. No TypeRegistry needed - Handler.NewInput() provides instance creation.

```go
// Marshaler - pure serialization, no type awareness
type Marshaler interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
    DataContentType() string  // CE attribute, e.g., "application/json"
}

// NamingStrategy - standalone utility
type NamingStrategy interface {
    TypeName(t reflect.Type) string  // Go type → CE type
}

var KebabNaming NamingStrategy   // OrderCreated → "order.created"
var SnakeNaming NamingStrategy   // OrderCreated → "order_created"
```

**Handler registration flow:**
```go
handler := message.NewHandler(processOrder, message.KebabNaming)
// handler.EventType() returns "order.created"
// handler.NewInput() returns *OrderCreated for unmarshaling

engine.AddHandler(handler, HandlerConfig{Name: "process-order"})
// 1. Engine reads handler.EventType() → "order.created"
// 2. Engine registers route: typeRoutes["order.created"] = handler
// 3. When message arrives, engine calls handler.NewInput() to get instance for unmarshaling
```

### Handler - Self-Describing with NamingStrategy

Handler is self-describing - knows its CE type and can create instances for unmarshaling.

```go
type Handler interface {
    EventType() string       // CE type - for routing
    NewInput() any           // creates new instance for unmarshaling (e.g., *OrderCreated)
    Handle(ctx context.Context, msg *Message) ([]*RawMessage, error)
}

// Generic constructor - NamingStrategy derives EventType from T
func NewHandler[T any](
    fn func(ctx context.Context, msg *Message[T]) ([]*RawMessage, error),
    naming NamingStrategy,
) Handler

// CQRS constructor - receives command directly, attributes via context
// NamingStrategy derives EventType from C, output types from E
func NewCommandHandler[C, E any](
    fn func(ctx context.Context, cmd C) ([]E, error),
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

// Usage - receives command directly, not Message[T]
handler := message.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // Access message attributes via context if needed:
        // attrs := message.AttributesFromContext(ctx)
        return []OrderCreated{{OrderID: "123"}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders-service",
        Naming: message.KebabNaming,
    },
)
// handler.EventType() returns "create.order" (from CreateOrder)
// handler.NewInput() returns *CreateOrder for unmarshaling
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
    func(ctx context.Context, msg *Message[OrderCreated]) ([]*RawMessage, error) {
        order := msg.Data  // typed access
        return []*RawMessage{
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
// handler.NewInput() returns *OrderCreated for unmarshaling
```

Use optional validation middleware to ensure CE compliance:
```go
engine.Use(message.ValidateCE())  // validates required CE attributes
```

**Summary:**
- `NewHandler`: Takes NamingStrategy, handler creates complete output messages, receives Message[T]
- `NewCommandHandler`: Takes config with Source + Naming, auto-generates output message attributes, receives cmd directly

### Two-Merger Architecture

Engine uses two mergers to support both raw (broker) and typed (internal) message flows:

```
RawInput₁ ─┐
RawInput₂ ─┼→ RawMerger[*RawMessage] → Unmarshal ─┐
RawInput₃ ─┘                                      │
                                                  ↓
TypedInput ─────────────────────────→ TypedMerger[*Message] → Handler → Distributor
                                           ↑                                │
                                           └─────── Loopback (typed) ───────┤
                                                                            ↓
                                                                     ┌──────┴──────┐
                                                              TypedOutput     Marshal → RawOutput
```

**API:**
```go
// Typed API (primary, for internal use)
AddInput(ch <-chan *Message, cfg InputConfig) error      // → TypedMerger
AddOutput(cfg OutputConfig) <-chan *Message              // ← Distributor (no marshal)

// Raw API (for broker integration)
AddRawInput(ch <-chan *RawMessage, cfg RawInputConfig) error   // → RawMerger → Unmarshal
AddRawOutput(cfg RawOutputConfig) <-chan *RawMessage           // ← Distributor → Marshal
```

**Benefits:**
- Single unmarshal pipe (shared by all raw inputs)
- Single marshal pipe (shared by all raw outputs)
- Loopback is clean - just another typed input to TypedMerger
- TypedInput bypasses unmarshaling entirely (for internal use, testing)
- TypedOutput bypasses marshaling entirely (for internal routing)
- Clear separation: typed = internal, raw = external boundary

**Config structure:**
```go
// InputConfig for typed inputs
type InputConfig struct {
    Name    string
    Matcher Matcher
}

// RawInputConfig for raw inputs
type RawInputConfig struct {
    Name      string
    Matcher   Matcher
}

// OutputConfig for typed outputs
type OutputConfig struct {
    Name    string
    Matcher Matcher
}

// RawOutputConfig for raw outputs
type RawOutputConfig struct {
    Name      string
    Matcher   Matcher
}
```

### DataContentType

Engine sets `DataContentType` at marshal boundary (from Marshaler.DataContentType()):

```
AddInput → Unmarshal → Handler (typed) → Marshal (sets DataContentType) → Output
     (Marshaler+Registry)                (Marshaler)
                ↑                                        │
                └─────── loopback (no marshal) ──────────┘
```

- Unmarshal happens directly after AddInput (uses Marshaler + TypeRegistry)
- Marshal happens directly before Output (uses Marshaler)
- Loopback bypasses marshal/unmarshal (already typed messages)
- Handler deals with typed data, not bytes

### Middleware

Middleware wraps handler execution with pre/post-handler logic. Similar to `pipe` module but with concrete `*Message` type.

```go
type Middleware func(next HandlerFunc) HandlerFunc

type HandlerFunc func(ctx context.Context, msg *Message) ([]*RawMessage, error)

// Usage
engine.Use(message.WithCorrelationID())  // propagates or generates correlation ID
engine.Use(message.ValidateCE())         // validates required CE attributes
```

**Execution order** (middleware applied in registration order):
```
Use(A) → Use(B) → handler
Request:  A.pre → B.pre → handler
Response: handler → B.post → A.post
```

| Attribute | Owner |
|-----------|-------|
| `DataContentType` | Engine (from Marshaler.DataContentType()) |
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
// handler.NewInput() returns *OrderCreated for unmarshaling

engine.AddHandler(handler, message.HandlerConfig{Name: "process-orders"})
// Engine routes by handler.EventType(), uses handler.NewInput() for unmarshaling
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
- Engine (reads handler.EventType() and handler.NewInput() directly)
- Codec (pure serialization)

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

### Combined Marshaler with TypeName and Registry
```go
type Marshaler interface {
    Register(goType reflect.Type)        // ❌
    TypeName(goType reflect.Type) string // ❌
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, ceType string) (any, error)
}
```
**Why rejected:** Combined interface was doing too much. Split into:
- Marshaler (pure serialization)
- NamingStrategy (utility for handler constructors)
- Handler.NewInput() (provides instances for unmarshaling)

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

### Single RawMerger Simplification
```go
// Proposed simplification:
Input₁ → Filter₁ ─┐
Input₂ → Filter₂ ─┼→ Merger[*RawMessage] → Unmarshal → Handler → Distributor → Marshal → Output
Input₃ → Filter₃ ─┘
```
**Why rejected:** Breaks loopback. Loopback feeds `*Message` (typed) back, but the single merger expects `*RawMessage`. Would require wasteful marshal/unmarshal of loopback messages.

**Solution:** Two-merger architecture (see Current Design Decisions).

## Idiomaticity Review

| Aspect | Pattern | Idiomatic? |
|--------|---------|------------|
| Handler interface | `EventType()`, `NewInput()`, `Handle()` - self-describing | ✅ Interface tells you what it is |
| Handler constructors | `NewHandler[T](fn, naming)`, `NewCommandHandler[C,E](fn, cfg)` | ✅ Go generic pattern |
| Config pattern | Required config for AddInput/AddOutput/CommandHandler | ✅ Explicit over magic |
| Interface segregation | Codec, NamingStrategy, Handler separate | ✅ Single responsibility |
| Engine lifecycle | Accepts channels, doesn't own I/O | ✅ Composition over inheritance |
| Input/Output | Config-based, names optional | ✅ Primary data first |
| Output routing | Pattern matching on CE type | ✅ Declarative |
| Optional features | Middleware (`Use()`) | ✅ Standard pattern |

## Design Decisions (Resolved)

1. **Loopback detection**: Handler responsibility. Handlers must not output messages that loop back to themselves indefinitely. Optional middleware can add hop count/TTL if needed.
2. **No match**: `ErrNoMatchingOutput` sent to engine's ErrorHandler.
3. **Middleware order**: Pre-handler and post-handler, similar to `pipe` module but with concrete `*Message` type. Applied to all handlers.
4. **Handler error handling**: Engine-level only. Handlers return errors, engine's ErrorHandler processes them.
5. **CESQL support**: Full CESQL support planned (future - see Plan 0003).

## Ready to Implement

Core components are well-defined:

| Component | Status | Notes |
|-----------|--------|-------|
| NamingStrategy | ✅ Ready | Standalone utility, KebabNaming, SnakeNaming |
| Marshaler | ✅ Ready | Pure serialization interface |
| JSONMarshaler | ✅ Ready | JSON implementation |
| Matcher | ✅ Ready | Interface in message/, implementations in match/ |
| match.All/Any | ✅ Ready | Combinators for AND/OR |
| match.Sources | ✅ Ready | CE source pattern matching |
| match.Types | ✅ Ready | CE type pattern matching |
| Handler | ✅ Ready | Self-describing with EventType(), NewInput() |
| NewHandler | ✅ Ready | Takes NamingStrategy, explicit output messages |
| NewCommandHandler | ✅ Ready | Takes config, receives cmd directly, attrs via context |
| HandlerConfig | ✅ Ready | Name only (CE type from handler.EventType()) |
| InputConfig | ✅ Ready | Optional name + Matcher for filtering |
| OutputConfig | ✅ Ready | Matcher for routing (SQL LIKE syntax) |
| LoopbackConfig | ✅ Ready | Name + Matcher for internal re-processing |
| Engine | ✅ Ready | AddInput, AddOutput, AddLoopback, AddHandler, Start() |
| Errors | ✅ Ready | ErrInputRejected, ErrNoMatchingOutput, ErrNoHandler |
| Middleware | ✅ Ready | ValidateCE, WithCorrelationID (optional) |

## Implementation Order

1. `message/naming.go` - NamingStrategy interface, KebabNaming, SnakeNaming
2. `message/marshaler.go` - Marshaler interface
3. `message/json_marshaler.go` - JSONMarshaler implementation
4. `message/matcher.go` - Matcher interface
5. `message/match/like.go` - SQL LIKE pattern matching
6. `message/match/types.go` - Types matcher
7. `message/match/sources.go` - Sources matcher
8. `message/match/match.go` - All, Any combinators
9. `message/config.go` - InputConfig, OutputConfig, LoopbackConfig, HandlerConfig
10. `message/handler.go` - Handler interface, NewHandler, NewCommandHandler
11. `message/context.go` - AttributesFromContext
12. `message/errors.go` - ErrInputRejected, ErrNoMatchingOutput, ErrNoHandler
13. `message/engine.go` - Engine with AddInput, AddOutput, AddLoopback, AddHandler
14. `message/middleware.go` - ValidateCE, WithCorrelationID (optional)
15. `message/cloudevents/` - CE adapter package (optional)

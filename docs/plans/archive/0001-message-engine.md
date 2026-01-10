# Plan 0001: Message Engine Implementation

**Status:** Proposed
**Related ADRs:** [0019](../adr/0019-remove-sender-receiver.md), [0020](../adr/0020-message-engine-architecture.md), [0021](../adr/0021-codec-marshaling-pattern.md), [0022](../adr/0022-message-package-redesign.md)
**Depends On:** [Plan 0002](0002-marshaler.md) (Codec & NamingStrategy)
**Design State:** [0001-message-engine.state.md](0001-message-engine.state.md)

## Overview

Implement a Message Engine that orchestrates message flow with marshaling at boundaries and pattern-based output routing.

## Design Principles

1. **Reuse `pipe` and `channel` packages** - Build on existing foundation
2. **Engine implements `Start()` signature** - Orchestration type per ADR 0018
3. **Engine doesn't own I/O lifecycle** - Accepts channels, external code manages subscription/publishing
4. **Handler creates complete messages** - Engine only sets DataContentType from Marshaler
5. **Handler is self-describing** - `EventType()` for routing, `NewInput()` for unmarshaling
6. **Two handler types** - Explicit (`NewHandler`) and convention-based (`NewCommandHandler`)
7. **Pattern-based output routing** - Match patterns on CE type, not named routing
8. **Clean separation** - Marshaler (serialization), NamingStrategy (utility), Handler (self-contained)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         message.Engine                           │
│                                                                  │
│  AddInput(ch, cfg) ──> unmarshal ──┐                             │
│     (Marshaler + handler.NewInput) ├──> Merger ──> route ──> handlers
│             AddLoopback ───────────┘   (by handler.EventType())  │
│                  ↑                                           ↓   │
│                  │                              marshal (Marshaler)
│                  │                                           │   │
│                  │       ┌─── Matcher: "order.%" ────────────┤   │
│  AddOutput(cfg) ─┼───────┼─── Matcher: "payment.%" ──────────┤   │
│    returns       │       └─── Matcher: nil (catch-all) ──────┘   │
│                  │                                               │
│           (loopback bypasses marshal - already typed)            │
└─────────────────────────────────────────────────────────────────┘
```

## Core Types

### Handler Interface

Handler is self-describing - knows its CE type and can create instances for unmarshaling:

```go
type Handler interface {
    EventType() string       // CE type - for routing
    NewInput() any           // creates new instance for unmarshaling (e.g., *OrderCreated)
    Handle(ctx context.Context, msg *Message[any]) ([]*Message[any], error)
}
```

### Handler Constructors

```go
// Generic constructor - explicit message creation
// NamingStrategy derives EventType() from T
func NewHandler[T any](
    fn func(ctx context.Context, msg *Message[T]) ([]*Message[any], error),
    naming NamingStrategy,
) Handler

// CQRS constructor - convention-based with config
// Handler receives command directly, attributes available via context
// NamingStrategy derives EventType() from C, and output CE types from E
func NewCommandHandler[C, E any](
    fn func(ctx context.Context, cmd C) ([]E, error),
    cfg CommandHandlerConfig,
) Handler
```

### CommandHandlerConfig

```go
type CommandHandlerConfig struct {
    Source string          // required, CE source attribute
    Naming NamingStrategy  // derives CE types for input (C) and output (E)
}
```

### Accessing Message Attributes in CommandHandler

CommandHandler receives the command directly. Message attributes are available via context:

```go
handler := message.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // Access attributes if needed
        attrs := message.AttributesFromContext(ctx)
        correlationID, _ := attrs.Get("correlationid")

        return []OrderCreated{{OrderID: "123"}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders-service",
        Naming: message.KebabNaming,
    },
)
```

### InputConfig / OutputConfig

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

### Matcher Interface

```go
// In message/matcher.go
type Matcher interface {
    Match(msg *Message[any]) bool
}
```

### Match Package

Implementations in `message/match/`:

```go
import "github.com/fxsml/gopipe/message/match"

// Combinators
func All(matchers ...message.Matcher) message.Matcher   // AND
func Any(matchers ...message.Matcher) message.Matcher   // OR

// Attribute matchers (use SQL LIKE syntax: % = any, _ = single char)
func Sources(patterns ...string) message.Matcher  // match CE source
func Types(patterns ...string) message.Matcher    // match CE type
```

**Default behavior (nil Matcher = match all):**

```go
// Catch-all output - nil Matcher returns true for all messages
defaultOut := engine.AddOutput(message.OutputConfig{Name: "default"})

// Catch-all input - accepts all messages
engine.AddInput(ch, message.InputConfig{})
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
paymentsOut := engine.AddOutput(message.OutputConfig{
    Matcher: match.Types("payment.%"),
})
defaultOut := engine.AddOutput(message.OutputConfig{})  // catch-all
```

### Marshaler and NamingStrategy

```go
// Marshaler - pure serialization
type Marshaler interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
    DataContentType() string  // CE attribute, e.g., "application/json"
}

// NamingStrategy - standalone utility for deriving CE types
type NamingStrategy interface {
    TypeName(t reflect.Type) string  // Go type → CE type
}

var KebabNaming NamingStrategy   // OrderCreated → "order.created"
var SnakeNaming NamingStrategy   // OrderCreated → "order_created"
```

Note: No TypeRegistry needed - Handler.NewInput() provides instance creation.

### Engine

```go
type Engine struct {
    marshaler    Marshaler
    handlers     map[string]Handler  // CE type → handler
    inputs       []inputEntry
    outputs      []outputEntry
    loopbacks    []loopbackEntry

    mu      sync.Mutex
    started bool
}

type EngineConfig struct {
    Marshaler    Marshaler
    ErrorHandler func(msg *Message[any], err error)
}

func NewEngine(cfg EngineConfig) *Engine

func (e *Engine) AddHandler(h Handler, cfg HandlerConfig) error  // uses handler.EventType() and handler.NewInput()
func (e *Engine) AddInput(ch <-chan *RawMessage, cfg InputConfig) error
func (e *Engine) AddOutput(cfg OutputConfig) <-chan *RawMessage  // returns RawMessage for broker
func (e *Engine) AddLoopback(cfg LoopbackConfig) error

func (e *Engine) Use(middleware Middleware)

func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error)
```

## Usage

### Explicit Handler (NewHandler)

```go
// Create engine with Marshaler
engine := message.NewEngine(message.EngineConfig{
    Marshaler: message.NewJSONMarshaler(),
})

// Create handler with NamingStrategy (derives EventType at construction)
handler := message.NewHandler(
    func(ctx context.Context, msg *Message[OrderCreated]) ([]*Message[any], error) {
        order := msg.Data
        return []*Message[any]{
            message.New[any](OrderShipped{OrderID: order.ID}, message.Attributes{
                ID:          uuid.New().String(),
                SpecVersion: "1.0",
                Type:        "order.shipped",
                Source:      "/orders-service",
                Time:        time.Now(),
            }),
        }, nil
    },
    message.KebabNaming,  // OrderCreated → "order.created"
)

// handler.EventType() returns "order.created"
// handler.NewInput() returns *OrderCreated

engine.AddHandler(handler, message.HandlerConfig{Name: "process-order"})

// Input: external subscription management
subscriber := ce.NewSubscriber(client)
ch, _ := subscriber.Subscribe(ctx, "orders")
engine.AddInput(ch, message.InputConfig{Name: "order-events"})

// Output: pattern-based routing, returns channel directly
ordersOut := engine.AddOutput(message.OutputConfig{Matcher: match.Types("order.%")})
defaultOut := engine.AddOutput(message.OutputConfig{})  // catch-all

// External publishing (Publish runs in goroutine internally)
publisher := ce.NewPublisher(client)
publisher.Publish(ctx, ordersOut)

done, _ := engine.Start(ctx)
<-done
```

### Convention Handler (NewCommandHandler)

```go
handler := message.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // cmd is the command directly, not wrapped in Message[T]
        // Access attributes via context if needed:
        // attrs := message.AttributesFromContext(ctx)
        return []OrderCreated{{OrderID: uuid.New().String()}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders-service",
        Naming: message.KebabNaming,  // CreateOrder → "create.order", OrderCreated → "order.created"
    },
)

// handler.EventType() returns "create.order" (from CreateOrder)
// Output messages get Type "order.created" (from OrderCreated)

engine.AddHandler(handler, message.HandlerConfig{Name: "create-order"})
```

## Output Routing

Outputs use Matcher for routing. Messages go to the first matching output.

**Pattern syntax (SQL LIKE):**
- `%` matches any sequence of characters
- `_` matches a single character

**Matching priority:**
1. First output with matching Matcher
2. Catch-all (nil Matcher) should be last

**Example:**
```go
// Register outputs with matchers
ordersOut := engine.AddOutput(message.OutputConfig{
    Matcher: match.Types("order.%"),
})
paymentsOut := engine.AddOutput(message.OutputConfig{
    Matcher: match.Types("payment.%"),
})
defaultOut := engine.AddOutput(message.OutputConfig{})  // nil = catch-all

// Handler returns order.created → matches "order.%" → goes to ordersOut
// Handler returns payment.received → matches "payment.%" → goes to paymentsOut
// Handler returns unknown.event → no matcher matches, goes to defaultOut (nil)
```

**Complex routing:**
```go
// Priority routing with multiple conditions
priorityOut := engine.AddOutput(message.OutputConfig{
    Name: "priority",
    Matcher: match.All(
        match.Types("order.%"),
        match.Sources("https://premium.%"),
    ),
})
```

## Attribute Ownership

| Attribute | Owner |
|-----------|-------|
| `DataContentType` | Engine (from Marshaler.DataContentType() at marshal boundary) |
| `Type` | Handler (via NamingStrategy at construction) |
| `Source` | Handler or CommandHandlerConfig |
| `Subject` | Handler (explicit) |
| `ID`, `Time`, `SpecVersion` | Handler or CommandHandler |
| `CorrelationID` | Middleware |

## Middleware

Middleware wraps handler execution with pre/post-handler logic:

```go
type Middleware func(next HandlerFunc) HandlerFunc

type HandlerFunc func(ctx context.Context, msg *Message[any]) ([]*Message[any], error)

// Usage
engine.Use(message.ValidateCE())         // validate required CE attributes
engine.Use(message.WithCorrelationID())  // propagate or generate correlation ID
```

**Execution order** (middleware applied in registration order):
```
Use(A) → Use(B) → handler
Request:  A.pre → B.pre → handler
Response: handler → B.post → A.post
```

## Error Handling

```go
var (
    ErrAlreadyStarted    = errors.New("engine already started")
    ErrInputRejected     = errors.New("message rejected by input matcher")
    ErrNoMatchingOutput  = errors.New("no output matches message type")
    ErrNoHandler         = errors.New("no handler for CE type")
    ErrHandlerNotFound   = errors.New("handler not found")
)

// ErrorHandler signature
type ErrorHandler func(msg *Message[any], err error)

// Default: log via slog.Error
```

**Error flow:**
- Input rejected by Matcher → `ErrInputRejected` to ErrorHandler
- No handler for CE type → `ErrNoHandler` to ErrorHandler
- No output matches → try next output, finally `ErrNoMatchingOutput` to ErrorHandler

## Loopback

Dedicated function for internal re-processing (bypasses marshal/unmarshal):

```go
engine.AddLoopback(message.LoopbackConfig{
    Name:    "validation-loop",
    Matcher: match.Types("validate.%"),
})
```

## MVP Implementation Order

### Phase 1: Core Types (depends on Plan 0002)
1. `message/naming.go` - NamingStrategy interface, KebabNaming, SnakeNaming
2. `message/marshaler.go` - Marshaler interface
3. `message/json_marshaler.go` - JSONMarshaler implementation

### Phase 2: Matcher System
4. `message/matcher.go` - Matcher interface
5. `message/match/like.go` - SQL LIKE pattern matching
6. `message/match/types.go` - Types matcher
7. `message/match/sources.go` - Sources matcher
8. `message/match/match.go` - All, Any combinators

### Phase 3: Handler & Engine
9. `message/config.go` - InputConfig, OutputConfig, LoopbackConfig, HandlerConfig
10. `message/handler.go` - Handler interface, NewHandler, NewCommandHandler
11. `message/context.go` - AttributesFromContext
12. `message/errors.go` - ErrInputRejected, ErrNoMatchingOutput, ErrNoHandler
13. `message/engine.go` - Engine with AddInput, AddOutput, AddLoopback, AddHandler

### Phase 4: Optional Enhancements
14. `message/middleware.go` - ValidateCE, WithCorrelationID (optional - can defer)

## Files Summary

| File | Priority | Notes |
|------|----------|-------|
| `message/naming.go` | MVP | NamingStrategy interface and implementations |
| `message/marshaler.go` | MVP | Marshaler interface |
| `message/json_marshaler.go` | MVP | JSONMarshaler implementation |
| `message/matcher.go` | MVP | Matcher interface |
| `message/match/like.go` | MVP | SQL LIKE pattern matching |
| `message/match/types.go` | MVP | Types matcher |
| `message/match/sources.go` | MVP | Sources matcher |
| `message/match/match.go` | MVP | All, Any combinators |
| `message/config.go` | MVP | Config structs |
| `message/handler.go` | MVP | Handler interface and constructors |
| `message/context.go` | MVP | AttributesFromContext |
| `message/errors.go` | MVP | Error types |
| `message/engine.go` | MVP | Engine orchestration |
| `message/middleware.go` | Optional | ValidateCE, WithCorrelationID |

## Test Plan

1. NewHandler receives Message[T] with typed data
2. NewHandler.EventType() returns derived CE type
3. NewHandler.NewInput() returns new instance for unmarshaling
4. NewCommandHandler receives command directly (not Message[T])
5. NewCommandHandler auto-generates CE attributes for output
6. AttributesFromContext returns message attributes in CommandHandler
7. AddHandler routes by handler.EventType()
8. AddOutput with Matcher routes correctly
9. Nil Matcher catches all (default)
10. match.Types("order.%") matches order.created, order.shipped
11. match.Types("%.created") matches order.created, user.created
12. AddLoopback routes matching output back to handlers
13. Engine sets DataContentType from Marshaler.DataContentType()
14. ValidateCE middleware rejects invalid messages (optional)
15. WithCorrelationID propagates correlation ID (optional)
16. ErrorHandler receives ErrInputRejected, ErrNoHandler, ErrNoMatchingOutput
17. Start() returns ErrAlreadyStarted on second call
18. match.All combines matchers with AND
19. match.Any combines matchers with OR
20. match.Sources filters by source pattern
21. InputConfig.Matcher filters incoming messages
22. OutputConfig.Matcher routes to correct output
23. Loopback bypasses marshal/unmarshal
24. KebabNaming: OrderCreated → "order.created"
25. SnakeNaming: OrderCreated → "order_created"

## Acceptance Criteria

### MVP (Required)
- [ ] Handler interface with EventType(), NewInput(), Handle()
- [ ] NewHandler takes NamingStrategy, derives EventType at construction
- [ ] NewCommandHandler takes config with Source + Naming, receives cmd directly
- [ ] AttributesFromContext for accessing message attributes in handlers
- [ ] Marshaler interface with Marshal, Unmarshal, DataContentType
- [ ] NamingStrategy with KebabNaming, SnakeNaming
- [ ] Engine.Start() orchestrates flow
- [ ] AddInput(ch, cfg) with optional Matcher for filtering
- [ ] AddOutput(cfg) returns channel, routes by Matcher
- [ ] AddLoopback(cfg) creates internal loop
- [ ] AddHandler routes by handler.EventType()
- [ ] Nil Matcher = match all (catch-all)
- [ ] SQL LIKE pattern matching works (%, _)
- [ ] match.All, match.Any, match.Sources, match.Types work
- [ ] Error handling: ErrInputRejected, ErrNoHandler, ErrNoMatchingOutput
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)

### Optional (Can Defer)
- [ ] Middleware support (ValidateCE, WithCorrelationID)

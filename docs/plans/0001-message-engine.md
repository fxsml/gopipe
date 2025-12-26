# Plan 0001: Message Engine Implementation

**Status:** Proposed
**Related ADRs:** [0019](../adr/0019-remove-sender-receiver.md), [0020](../adr/0020-message-engine-architecture.md), [0021](../adr/0021-codec-marshaling-pattern.md), [0022](../adr/0022-message-package-redesign.md)
**Depends On:** [Plan 0002](0002-marshaler.md) (Codec & TypeRegistry)
**Design State:** [0001-message-engine.state.md](0001-message-engine.state.md)

## Overview

Implement a Message Engine that orchestrates message flow with marshaling at boundaries and pattern-based output routing.

## Design Principles

1. **Reuse `pipe` and `channel` packages** - Build on existing foundation
2. **Engine implements `Start()` signature** - Orchestration type per ADR 0018
3. **Engine doesn't own I/O lifecycle** - Accepts channels, external code manages subscription/publishing
4. **Handler creates complete messages** - Engine only sets DataContentType from Codec
5. **Handler is self-describing** - `EventType()` returns CE type, `GoType()` returns reflect.Type
6. **Two handler types** - Explicit (`NewHandler`) and convention-based (`NewCommandHandler`)
7. **Pattern-based output routing** - Match patterns on CE type, not named routing
8. **Clean separation** - Codec (serialization), TypeRegistry (CE↔Go mapping), NamingStrategy (utility)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         message.Engine                           │
│                                                                  │
│  AddInput(ch, cfg) ──> unmarshal ──┐                             │
│                  (Codec+Registry)  ├──> Merger ──> route ──> handlers
│             AddLoopback ───────────┘   (by handler.EventType())  │
│                  ↑                                           ↓   │
│                  │                                  marshal (Codec)
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

Handler is self-describing - knows its CE type and Go type:

```go
type Handler interface {
    GoType() reflect.Type    // Go type - for TypeRegistry
    EventType() string       // CE type - for routing
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}
```

### Handler Constructors

Both constructors take NamingStrategy to derive CE type at construction time:

```go
// Generic constructor - explicit message creation
// NamingStrategy derives EventType() from T
func NewHandler[T any](
    fn func(ctx context.Context, msg *TypedMessage[T]) ([]*Message, error),
    naming NamingStrategy,
) Handler

// CQRS constructor - convention-based with config
// NamingStrategy derives EventType() from C, and output CE types from E
func NewCommandHandler[C, E any](
    fn func(ctx context.Context, msg *TypedMessage[C]) ([]E, error),
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
    Match(msg *Message) bool
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

### Codec and TypeRegistry

Separate concerns for serialization and type mapping:

```go
// Codec - pure serialization, no type registry
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

// NamingStrategy - standalone utility for deriving CE types
type NamingStrategy interface {
    TypeName(t reflect.Type) string  // Go type → CE type
}

var KebabNaming NamingStrategy   // OrderCreated → "order.created"
var SnakeNaming NamingStrategy   // OrderCreated → "order_created"
```

### Engine

```go
type Engine struct {
    codec        Codec
    registry     TypeRegistry
    handlers     map[string]Handler
    typeRoutes   map[string]string  // CE type → handler name
    inputs       []inputEntry
    outputs      []outputEntry
    loopbacks    []loopbackEntry

    mu      sync.Mutex
    started bool
}

type EngineConfig struct {
    Codec        Codec
    Registry     TypeRegistry
    ErrorHandler func(msg *Message, err error)
}

func NewEngine(cfg EngineConfig) *Engine

func (e *Engine) AddHandler(h Handler, cfg HandlerConfig) error  // uses handler.EventType() and handler.GoType()
func (e *Engine) AddInput(ch <-chan *Message, cfg InputConfig) error
func (e *Engine) AddOutput(cfg OutputConfig) <-chan *Message
func (e *Engine) AddLoopback(cfg LoopbackConfig) error

func (e *Engine) Use(middleware Middleware)

func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error)
```

## Usage

### Explicit Handler (NewHandler)

```go
// Create engine with Codec and TypeRegistry
engine := message.NewEngine(message.EngineConfig{
    Codec:    message.NewJSONCodec(),
    Registry: message.NewTypeRegistry(),
})

// Create handler with NamingStrategy (derives EventType at construction)
handler := message.NewHandler(
    func(ctx context.Context, msg *TypedMessage[OrderCreated]) ([]*Message, error) {
        order := msg.Data
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
    message.KebabNaming,  // OrderCreated → "order.created"
)

// handler.EventType() returns "order.created"
// handler.GoType() returns reflect.Type of OrderCreated

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
    func(ctx context.Context, msg *TypedMessage[CreateOrder]) ([]OrderCreated, error) {
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
| `DataContentType` | Engine (from Codec.ContentType() at marshal boundary) |
| `Type` | Handler (via NamingStrategy at construction) |
| `Source` | Handler or CommandHandlerConfig |
| `Subject` | Handler (explicit) |
| `ID`, `Time`, `SpecVersion` | Handler or CommandHandler |
| `CorrelationID` | Middleware |

## Middleware

```go
engine.Use(message.ValidateCE())         // validate required CE attributes
engine.Use(message.WithCorrelationID())  // propagate or generate correlation ID
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
type ErrorHandler func(msg *Message, err error)

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
2. `message/codec.go` - Codec interface
3. `message/json_codec.go` - JSONCodec implementation
4. `message/registry.go` - TypeRegistry interface and implementation

### Phase 2: Matcher System
5. `message/matcher.go` - Matcher interface
6. `message/match/like.go` - SQL LIKE pattern matching
7. `message/match/types.go` - Types matcher
8. `message/match/sources.go` - Sources matcher
9. `message/match/match.go` - All, Any combinators

### Phase 3: Handler & Engine
10. `message/config.go` - InputConfig, OutputConfig, LoopbackConfig, HandlerConfig
11. `message/handler.go` - Handler interface, NewHandler, NewCommandHandler
12. `message/errors.go` - ErrInputRejected, ErrNoMatchingOutput, ErrNoHandler
13. `message/engine.go` - Engine with AddInput, AddOutput, AddLoopback, AddHandler

### Phase 4: Optional Enhancements
14. `message/middleware.go` - ValidateCE, WithCorrelationID (optional - can defer)

## Files Summary

| File | Priority | Notes |
|------|----------|-------|
| `message/naming.go` | MVP | NamingStrategy interface and implementations |
| `message/codec.go` | MVP | Codec interface |
| `message/json_codec.go` | MVP | JSONCodec implementation |
| `message/registry.go` | MVP | TypeRegistry interface and implementation |
| `message/matcher.go` | MVP | Matcher interface |
| `message/match/like.go` | MVP | SQL LIKE pattern matching |
| `message/match/types.go` | MVP | Types matcher |
| `message/match/sources.go` | MVP | Sources matcher |
| `message/match/match.go` | MVP | All, Any combinators |
| `message/config.go` | MVP | Config structs |
| `message/handler.go` | MVP | Handler interface and constructors |
| `message/errors.go` | MVP | Error types |
| `message/engine.go` | MVP | Engine orchestration |
| `message/middleware.go` | Optional | ValidateCE, WithCorrelationID |

## Test Plan

1. NewHandler receives TypedMessage with typed data
2. NewHandler.EventType() returns derived CE type
3. NewHandler.GoType() returns reflect.Type
4. NewCommandHandler auto-generates CE attributes for output
5. AddHandler routes by handler.EventType()
6. AddHandler registers handler.GoType() in TypeRegistry
7. AddOutput with Matcher routes correctly
8. Nil Matcher catches all (default)
9. match.Types("order.%") matches order.created, order.shipped
10. match.Types("%.created") matches order.created, user.created
11. AddLoopback routes matching output back to handlers
12. Engine sets DataContentType from Codec.ContentType()
13. ValidateCE middleware rejects invalid messages (optional)
14. WithCorrelationID propagates correlation ID (optional)
15. ErrorHandler receives ErrInputRejected, ErrNoHandler, ErrNoMatchingOutput
16. Start() returns ErrAlreadyStarted on second call
17. match.All combines matchers with AND
18. match.Any combines matchers with OR
19. match.Sources filters by source pattern
20. InputConfig.Matcher filters incoming messages
21. OutputConfig.Matcher routes to correct output
22. Loopback bypasses marshal/unmarshal
23. KebabNaming: OrderCreated → "order.created"
24. SnakeNaming: OrderCreated → "order_created"

## Acceptance Criteria

### MVP (Required)
- [ ] Handler interface with GoType() and EventType()
- [ ] NewHandler takes NamingStrategy, derives EventType at construction
- [ ] NewCommandHandler takes config with Source + Naming
- [ ] Codec interface with Marshal, Unmarshal, ContentType
- [ ] TypeRegistry interface with Register, Lookup
- [ ] NamingStrategy with KebabNaming, SnakeNaming
- [ ] Engine.Start() orchestrates flow
- [ ] AddInput(ch, cfg) with optional Matcher for filtering
- [ ] AddOutput(cfg) returns channel, routes by Matcher
- [ ] AddLoopback(cfg) creates internal loop
- [ ] AddHandler routes by handler.EventType(), registers handler.GoType()
- [ ] Nil Matcher = match all (catch-all)
- [ ] SQL LIKE pattern matching works (%, _)
- [ ] match.All, match.Any, match.Sources, match.Types work
- [ ] Error handling: ErrInputRejected, ErrNoHandler, ErrNoMatchingOutput
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)

### Optional (Can Defer)
- [ ] Middleware support (ValidateCE, WithCorrelationID)

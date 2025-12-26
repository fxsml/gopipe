# Plan 0001: Message Engine Implementation

**Status:** Proposed
**Related ADRs:** [0019](../adr/0019-remove-sender-receiver.md), [0020](../adr/0020-message-engine-architecture.md), [0021](../adr/0021-codec-marshaling-pattern.md), [0022](../adr/0022-message-package-redesign.md)
**Depends On:** [Plan 0002](0002-marshaler.md) (Marshaler)
**Design State:** [0001-message-engine.state.md](0001-message-engine.state.md)

## Overview

Implement a Message Engine that orchestrates message flow with marshaling at boundaries and pattern-based output routing.

## Design Principles

1. **Reuse `pipe` and `channel` packages** - Build on existing foundation
2. **Engine implements `Start()` signature** - Orchestration type per ADR 0018
3. **Engine doesn't own I/O lifecycle** - Accepts channels, external code manages subscription/publishing
4. **Handler creates complete messages** - Engine only sets DataContentType from marshaler
5. **Two handler types** - Explicit (`NewHandler`) and convention-based (`NewCommandHandler`)
6. **Pattern-based output routing** - Match patterns on CE type, not named routing

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         message.Engine                           │
│                                                                  │
│  AddInput(ch, cfg) ──> unmarshal ──┐                             │
│                                    ├──> Merger ──> route ──> handlers
│             AddLoopback ───────────┘  (auto via NamingStrategy)  │
│                  ↑                                           ↓   │
│                  │                                       marshal │
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

```go
type Handler interface {
    EventType() reflect.Type
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}
```

### Handler Constructors

```go
// Generic constructor - explicit message creation
func NewHandler[T any](fn func(ctx context.Context, msg *TypedMessage[T]) ([]*Message, error)) Handler

// CQRS constructor - convention-based with config
func NewCommandHandler[C, E any](
    fn func(ctx context.Context, msg *TypedMessage[C]) ([]E, error),
    cfg CommandHandlerConfig,
) Handler
```

### CommandHandlerConfig

```go
type CommandHandlerConfig struct {
    Naming NamingStrategy  // default: KebabNaming
    Source string          // required, no default
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
    Name string  // required, handler name for registration
    Type string  // optional, CE type to handle (derived via marshaler if not set)
}
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

### Engine

```go
type Engine struct {
    marshaler    Marshaler
    handlers     map[string]Handler
    typeRoutes   map[string]string  // CE type → handler name
    inputs       []inputEntry
    outputs      []outputEntry
    loopbacks    []loopbackEntry

    mu      sync.Mutex
    started bool
}

type EngineConfig struct {
    Marshaler    Marshaler
    ErrorHandler func(msg *Message, err error)
}

func NewEngine(cfg EngineConfig) *Engine

func (e *Engine) AddHandler(h Handler, cfg HandlerConfig) error  // derives CE type via marshaler if cfg.Type empty
func (e *Engine) AddInput(ch <-chan *Message, cfg InputConfig) error
func (e *Engine) AddOutput(cfg OutputConfig) <-chan *Message
func (e *Engine) AddLoopback(cfg LoopbackConfig) error

func (e *Engine) Use(middleware Middleware)

func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error)
```

### NamingStrategy

NamingStrategy lives in Marshaler. Used to derive CE type from Go types:

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string  // Go type → CE type
}

var KebabNaming NamingStrategy   // OrderCreated → "order.created"
var SnakeNaming NamingStrategy   // OrderCreated → "order_created"
```

**Usage:**
- **Engine**: If `HandlerConfig.Type` is empty, derives via `marshaler.TypeName(handler.EventType())`
- **CommandHandler**: Derives output CE type via marshaler

## Usage

### Explicit Handler (NewHandler)

```go
engine := message.NewEngine(message.EngineConfig{
    Marshaler: message.NewJSONMarshaler(message.JSONMarshalerConfig{
        // Naming: message.KebabNaming (default)
    }),
})

handler := message.NewHandler(func(ctx context.Context, msg *TypedMessage[OrderCreated]) ([]*Message, error) {
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
})

engine.AddHandler(handler, message.HandlerConfig{
    Name: "process-order",
    // Type: "order.created" (optional, derived via marshaler)
})

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
    },
)

engine.AddHandler(handler, message.HandlerConfig{
    Name: "create-order",
    // Type: "create.order" (optional, derived via marshaler)
})
// Output CE type: OrderCreated → "order.created" (via marshaler's NamingStrategy)
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
| `DataContentType` | Engine (from marshaler at marshal boundary) |
| `Type` | Handler or NamingStrategy |
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

## Files to Create/Modify

- `message/config.go` - InputConfig, OutputConfig, LoopbackConfig, HandlerConfig
- `message/matcher.go` - Matcher interface
- `message/match/match.go` - All, Any combinators
- `message/match/like.go` - SQL LIKE pattern matching
- `message/match/sources.go` - Sources matcher
- `message/match/types.go` - Types matcher
- `message/engine.go` - Engine with AddInput, AddOutput, AddLoopback, AddHandler (auto-routing)
- `message/handler.go` - Handler interface, NewHandler, NewCommandHandler
- `message/naming.go` - NamingStrategy interface, KebabNaming, SnakeNaming
- `message/middleware.go` - ValidateCE, WithCorrelationID
- `message/errors.go` - ErrInputRejected, ErrNoMatchingOutput, ErrNoHandler, etc.

## Test Plan

1. NewHandler receives TypedMessage with typed data
2. NewCommandHandler auto-generates CE attributes
3. AddHandler auto-routes via NamingStrategy
4. AddOutput with Matcher routes correctly
5. Nil Matcher catches all (default)
6. match.Types("order.%") matches order.created, order.shipped
7. match.Types("%.created") matches order.created, user.created
8. AddLoopback routes matching output back to handlers
9. Engine sets DataContentType from marshaler
10. ValidateCE middleware rejects invalid messages
11. WithCorrelationID propagates correlation ID
12. ErrorHandler receives ErrInputRejected, ErrNoHandler, ErrNoMatchingOutput
13. Start() returns ErrAlreadyStarted on second call
14. match.All combines matchers with AND
15. match.Any combines matchers with OR
16. match.Sources filters by source pattern
17. match.Types filters by type pattern
18. InputConfig.Matcher filters incoming messages
19. OutputConfig.Matcher routes to correct output
20. Loopback bypasses marshal/unmarshal

## Acceptance Criteria

- [ ] Engine.Start() orchestrates flow
- [ ] AddInput(ch, cfg) with optional Matcher for filtering
- [ ] AddOutput(cfg) returns channel, routes by Matcher
- [ ] AddLoopback(cfg) creates internal loop
- [ ] AddHandler auto-routes CE type via NamingStrategy
- [ ] Nil Matcher = match all (catch-all)
- [ ] NewHandler and NewCommandHandler constructors work
- [ ] SQL LIKE pattern matching works (%, _)
- [ ] match.All, match.Any, match.Sources, match.Types work
- [ ] Error handling: ErrInputRejected, ErrNoHandler, ErrNoMatchingOutput
- [ ] Middleware support (ValidateCE, WithCorrelationID)
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)

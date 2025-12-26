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
│                   loopback ────────┘                         │   │
│                      ↑                                       ↓   │
│                      │                                   marshal │
│                      │                                       │   │
│                      │       ┌─── Match: "Order*" ───────────┤   │
│  AddOutput(cfg) ─────┼───────┼─── Match: "Payment*" ─────────┤   │
│    returns           │       └─── Match: "*" ────────────────┘   │
│                      │                                           │
│               (loopback bypasses marshal - already typed)        │
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
    Name string  // optional, for tracing/metrics
}

type OutputConfig struct {
    Name  string  // optional, for logging/metrics
    Match string  // required: "*", "Order*", "order.*", or CESQL
}
```

### Engine

```go
type Engine struct {
    marshaler Marshaler
    naming    NamingStrategy
    handlers  map[reflect.Type][]Handler
    inputs    []inputEntry
    outputs   []outputEntry

    mu      sync.Mutex
    started bool
}

type EngineConfig struct {
    Marshaler    Marshaler
    Naming       NamingStrategy    // default: KebabNaming
    Routing      RoutingStrategy   // default: ConventionRouting
    ErrorHandler func(msg *Message, err error)
}

func NewEngine(cfg EngineConfig) *Engine

func (e *Engine) AddHandler(name string, h Handler) error
func (e *Engine) AddInput(ch <-chan *Message, cfg InputConfig) error
func (e *Engine) AddOutput(cfg OutputConfig) <-chan *Message

// Explicit ingress routing (required if RoutingStrategy is ExplicitRouting)
func (e *Engine) RouteType(ceType string, handlerName string) error

func (e *Engine) Use(middleware Middleware)

func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error)
```

### Routing Strategies

```go
type RoutingStrategy int

const (
    ConventionRouting RoutingStrategy = iota  // auto-route ingress by NamingStrategy
    ExplicitRouting                           // manual RouteType calls required
)
```

### NamingStrategy

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string    // Go type → CE type
}

var KebabNaming NamingStrategy   // OrderCreated → "order.created"
var SnakeNaming NamingStrategy   // OrderCreated → "order_created"
```

## Usage

### Explicit Handler (NewHandler)

```go
engine := message.NewEngine(message.EngineConfig{
    Marshaler: message.NewJSONMarshaler(message.JSONMarshalerConfig{}),
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

engine.AddHandler("process-order", handler)

// Input: external subscription management
subscriber := ce.NewSubscriber(client)
ch, _ := subscriber.Subscribe(ctx, "orders")
engine.AddInput(ch, message.InputConfig{Name: "order-events"})

// Output: pattern-based routing, returns channel directly
ordersOut := engine.AddOutput(message.OutputConfig{Match: "Order*"})
defaultOut := engine.AddOutput(message.OutputConfig{Match: "*"})

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
        // Naming: message.KebabNaming (default)
    },
)

engine.AddHandler("create-order", handler)
```

## Output Pattern Matching

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

**Example:**
```go
// Register outputs with patterns
ordersOut := engine.AddOutput(message.OutputConfig{Match: "Order*"})
paymentsOut := engine.AddOutput(message.OutputConfig{Match: "Payment*"})
defaultOut := engine.AddOutput(message.OutputConfig{Match: "*"})

// Handler returns OrderCreated → matches "Order*" → goes to ordersOut
// Handler returns PaymentReceived → matches "Payment*" → goes to paymentsOut
// Handler returns UnknownEvent → matches "*" → goes to defaultOut
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
    ErrAlreadyStarted = errors.New("already started")
    ErrNoMatch        = errors.New("no output matches message type")
    ErrTypeNotFound   = errors.New("type not found")
    ErrNoHandler      = errors.New("no handler for type")
)

// ErrorHandler signature
type ErrorHandler func(msg *Message, err error)

// Default: log via slog.Error
```

## Loopback

Route output back to input for re-processing:

```go
// Create output for validation results
loopback := engine.AddOutput(message.OutputConfig{Match: "Validate*"})

// Feed it back to engine as input
engine.AddInput(loopback, message.InputConfig{Name: "loopback"})
```

## Files to Create/Modify

- `message/config.go` - InputConfig, OutputConfig
- `message/match.go` - Pattern matching for OutputConfig.Match
- `message/engine.go` - Engine implementation
- `message/handler.go` - Handler interface, NewHandler, NewCommandHandler
- `message/naming.go` - NamingStrategy interface, KebabNaming, SnakeNaming
- `message/middleware.go` - ValidateCE, WithCorrelationID
- `message/errors.go` - Error definitions

## Test Plan

1. NewHandler receives TypedMessage with typed data
2. NewCommandHandler auto-generates CE attributes
3. RouteType routes CE type to handler
4. AddOutput with Match pattern routes correctly
5. Wildcard match `"*"` catches all
6. Prefix match `"Order*"` matches OrderCreated, OrderShipped
7. Loopback: output channel fed to input
8. Engine sets DataContentType from marshaler
9. ValidateCE middleware rejects invalid messages
10. WithCorrelationID propagates correlation ID
11. ErrorHandler receives errors
12. Start() returns ErrAlreadyStarted on second call

## Acceptance Criteria

- [ ] Engine.Start() orchestrates flow
- [ ] AddInput(ch, cfg) for input channels
- [ ] AddOutput(cfg) returns channel, routes by pattern
- [ ] NewHandler and NewCommandHandler constructors work
- [ ] Pattern matching works (exact, prefix, wildcard, CESQL)
- [ ] Middleware support (ValidateCE, WithCorrelationID)
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)

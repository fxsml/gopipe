# Plan 0001: Message Engine Implementation

**Status:** Proposed
**Related ADRs:** [0019](../adr/0019-remove-sender-receiver.md), [0020](../adr/0020-message-engine-architecture.md), [0021](../adr/0021-codec-marshaling-pattern.md), [0022](../adr/0022-message-package-redesign.md)
**Depends On:** [Plan 0002](0002-marshaler.md) (Marshaler)
**Design State:** [0001-message-engine.state.md](0001-message-engine.state.md)

## Overview

Implement a Message Engine that orchestrates message flow with marshaling at boundaries and type-based routing.

## Design Principles

1. **Reuse `pipe` and `channel` packages** - Build on existing foundation
2. **Engine implements `Start()` signature** - Orchestration type per ADR 0018
3. **Engine doesn't own I/O lifecycle** - Accepts channels, external code manages subscription/publishing
4. **Handler creates complete messages** - Engine only sets DataContentType from marshaler
5. **Two handler types** - Explicit (`NewHandler`) and convention-based (`NewCommandHandler`)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         message.Engine                           │
│                                                                  │
│  AddInput() ──┐                                                  │
│               ├──> pipe.Merger ──> unmarshal ──> route ──> handlers
│               │         ↑                                   │    │
│               │         │                                   ↓    │
│               │         └─── loopback ─────────────── marshal    │
│               │                                         │        │
│               └─────────────────────────────────────────┼────────│
│                                                         ↓        │
│                                                    Output()      │
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

### Engine

```go
type Engine struct {
    marshaler Marshaler
    naming    NamingStrategy
    handlers  map[reflect.Type][]Handler
    inputs    map[string]<-chan *Message
    outputs   map[string]chan *Message

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
func (e *Engine) AddInput(name string, ch <-chan *Message) error
func (e *Engine) Output(name string) <-chan *Message

// Explicit routing (required if RoutingStrategy is ExplicitRouting)
func (e *Engine) RouteType(ceType string, handlerName string) error
func (e *Engine) RouteOutput(handlerName string, outputName string) error

func (e *Engine) Use(middleware Middleware)

func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error)
```

### Routing Strategies

```go
type RoutingStrategy int

const (
    ConventionRouting RoutingStrategy = iota  // auto-route by NamingStrategy
    ExplicitRouting                           // manual RouteType/RouteOutput required
)
```

### NamingStrategy

```go
type NamingStrategy interface {
    TypeName(t reflect.Type) string    // Go type → CE type
    OutputName(t reflect.Type) string  // Go type → output name
}

var KebabNaming NamingStrategy   // OrderCreated → "order.created", output: "orders"
var SnakeNaming NamingStrategy   // OrderCreated → "order_created", output: "orders"
```

## Usage

### Explicit Handler (NewHandler)

```go
engine := message.NewEngine(message.EngineConfig{
    Marshaler: message.NewJSONMarshaler(),
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

// External subscription management
subscriber := ce.NewSubscriber(client)
ch, _ := subscriber.Subscribe(ctx, "orders")
engine.AddInput("orders", ch)

// External publishing management
publisher := ce.NewPublisher(client)
go publisher.Publish(ctx, engine.Output("shipments"))

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
    ErrAlreadyStarted    = errors.New("already started")
    ErrOutputNotFound    = errors.New("output not found")
    ErrTypeNotFound      = errors.New("type not found")
    ErrNoHandler         = errors.New("no handler for type")
)

// ErrorHandler signature
type ErrorHandler func(msg *Message, err error)

// Default: log via slog.Error
```

## Loopback

Route handler output back to engine input:

```go
// With convention routing: handler output type determines routing
// OrderCreated → "orders" output
// If "orders" is also an input, it loops back

// With explicit routing:
engine.RouteOutput("validate-order", "process-order")  // handler → handler
```

## Files to Create/Modify

- `message/engine.go` - Engine implementation
- `message/handler.go` - Handler interface, NewHandler, NewCommandHandler
- `message/naming.go` - NamingStrategy interface, KebabNaming, SnakeNaming
- `message/middleware.go` - ValidateCE, WithCorrelationID
- `message/errors.go` - Error definitions

## Test Plan

1. NewHandler receives TypedMessage with typed data
2. NewCommandHandler auto-generates CE attributes
3. RouteType routes CE type to handler
4. RouteOutput routes handler output to output channel
5. ConventionRouting auto-routes by NamingStrategy
6. Loopback routes handler to handler
7. Engine sets DataContentType from marshaler
8. ValidateCE middleware rejects invalid messages
9. WithCorrelationID propagates correlation ID
10. ErrorHandler receives errors
11. Start() returns ErrAlreadyStarted on second call

## Acceptance Criteria

- [ ] Engine.Start() orchestrates flow
- [ ] AddInput/Output for channel management
- [ ] NewHandler and NewCommandHandler constructors work
- [ ] Convention and explicit routing strategies
- [ ] Middleware support (ValidateCE, WithCorrelationID)
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)

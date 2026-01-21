# Plan 0009: Message Package Implementation

**Status:** Complete
**Branch:** `feature/message-engine` → `claude/refactor-message-methods-*`

## Overview

CloudEvents-aligned message handling with type-based routing, built on the `channel` and `pipe` packages.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              Engine                                      │
│                                                                          │
│  RawInput₁ → Unmarshal ─┐                                               │
│  RawInput₂ → Unmarshal ─┼─→ Merger → Router → Distributor               │
│  TypedInput ────────────┘                            │                  │
│                                           ┌──────────┴──────────┐       │
│                                     TypedOutput            Marshal      │
│                                                               ↓         │
│                                                           RawOutput     │
└─────────────────────────────────────────────────────────────────────────┘
```

**Key insight:** Single merger handles both typed inputs and unmarshaled raw inputs.
Each raw input has its own unmarshal pipe that feeds into the shared merger.

Loopback is not built into Engine—use `plugin.Loopback` which connects a
TypedOutput back to TypedInput via the existing Add* APIs.

## Core Types

### Engine

```go
type EngineConfig struct {
    Marshaler       Marshaler
    BufferSize      int           // default 100
    ShutdownTimeout time.Duration // graceful shutdown
    Logger          Logger
    ErrorHandler    ErrorHandler
}

func NewEngine(cfg EngineConfig) *Engine

func (e *Engine) AddHandler(name string, matcher Matcher, h Handler) error
func (e *Engine) AddInput(name string, matcher Matcher, ch <-chan *Message) (<-chan struct{}, error)
func (e *Engine) AddRawInput(name string, matcher Matcher, ch <-chan *RawMessage) (<-chan struct{}, error)
func (e *Engine) AddOutput(name string, matcher Matcher) (<-chan *Message, error)
func (e *Engine) AddRawOutput(name string, matcher Matcher) (<-chan *RawMessage, error)
func (e *Engine) AddPlugin(plugins ...Plugin) error
func (e *Engine) Use(m ...Middleware) error
func (e *Engine) Start(ctx context.Context) (<-chan struct{}, error)
```

### Handler

```go
type Handler interface {
    EventType() string
    NewInput() any
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}

func NewHandler[T any](
    fn func(context.Context, *Message) ([]*Message, error),
    naming EventTypeNaming,
) Handler

func NewCommandHandler[C, E any](
    fn func(context.Context, C) ([]E, error),
    cfg CommandHandlerConfig,
) Handler

type CommandHandlerConfig struct {
    Source string
    Naming EventTypeNaming
}
```

### Router

```go
type RouterConfig struct {
    BufferSize   int
    ErrorHandler ErrorHandler
    Logger       Logger
}

func NewRouter(cfg RouterConfig) *Router

func (r *Router) AddHandler(name string, matcher Matcher, h Handler) error
func (r *Router) Use(m ...Middleware) error
func (r *Router) Pipe(ctx context.Context, in <-chan *Message) (<-chan *Message, error)
```

### Message Types

```go
type TypedMessage[T any] struct {
    Data       T
    Attributes Attributes
}

type Message = TypedMessage[any]
type RawMessage = TypedMessage[[]byte]
type Attributes map[string]any

func New(data any, attrs Attributes, acking *Acking) *Message
func NewTyped[T any](data T, attrs Attributes, acking *Acking) *TypedMessage[T]
func NewRaw(data []byte, attrs Attributes, acking *Acking) *RawMessage
func Copy[In, Out any](msg *TypedMessage[In], data Out) *TypedMessage[Out]

// Acking coordinates acknowledgment across messages
func NewAcking(ack func(), nack func(error)) *Acking
func NewSharedAcking(ack func(), nack func(error), expectedCount int) *Acking
```

### Matcher

```go
type Matcher interface {
    Match(attrs Attributes) bool
}

// match/ subpackage
func Types(patterns ...string) Matcher    // SQL LIKE on CE type
func Sources(patterns ...string) Matcher  // SQL LIKE on CE source
func All(matchers ...Matcher) Matcher     // AND
func Any(matchers ...Matcher) Matcher     // OR
```

### Plugin

```go
type Plugin func(*Engine) error

// plugin/ subpackage
func Loopback(name string, matcher Matcher) Plugin
func ProcessLoopback[In, Out any](name string, matcher Matcher, fn ProcessFunc[In, Out]) Plugin
func BatchLoopback[In, Out any](name string, matcher Matcher, fn BatchFunc[In, Out], cfg BatchConfig) Plugin
```

### Middleware

```go
type Middleware func(Handler) Handler

// middleware/ subpackage
func CorrelationID() Middleware
```

## Usage Examples

### Basic Engine Setup

```go
engine := message.NewEngine(message.EngineConfig{
    Marshaler: message.NewJSONMarshaler(),
})

handler := message.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        return []OrderCreated{{ID: cmd.ID, Status: "created"}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders",
        Naming: message.KebabNaming,
    },
)
engine.AddHandler("process-order", nil, handler)

engine.AddRawInput("input", nil, inputCh)
output, _ := engine.AddRawOutput("output", nil)

done, _ := engine.Start(ctx)
```

### With Loopback

```go
engine.AddPlugin(plugin.Loopback("events-loop", match.Types("event.%")))
```

### With Middleware

```go
engine.Use(middleware.CorrelationID())
```

### Output Routing

```go
// First matching output wins
ordersOut, _ := engine.AddRawOutput("orders", match.Types("order.%"))
paymentsOut, _ := engine.AddRawOutput("payments", match.Types("payment.%"))
defaultOut, _ := engine.AddRawOutput("default", nil)  // catch-all
```

## Implementation Files

| File | Purpose |
|------|---------|
| `engine.go` | Engine orchestrator, Add* methods, Start |
| `router.go` | Handler routing, middleware chain |
| `handler.go` | Handler interface, NewHandler, NewCommandHandler |
| `message.go` | Message types, Copy, Acking |
| `marshaler.go` | Marshaler interface, JSONMarshaler |
| `naming.go` | EventTypeNaming, KebabNaming, SnakeNaming |
| `registry.go` | InputRegistry, FactoryMap |
| `matcher.go` | Matcher interface |
| `pipes.go` | Marshal/Unmarshal pipes |
| `errors.go` | Error types |
| `match/*.go` | Matcher implementations |
| `middleware/*.go` | CorrelationID |
| `plugin/*.go` | Loopback, ProcessLoopback, BatchLoopback |

## Test Coverage

- Engine flow tests (basic, loopback, multiple inputs/outputs)
- Router tests (middleware, handler matching)
- Handler tests (NewHandler, NewCommandHandler)
- Message tests (Copy, Acking, thread safety)
- Matcher tests (Types, Sources, All, Any)
- Plugin tests (Loopback variants)
- Middleware tests (CorrelationID)

Run: `make test`

## Design Notes

For design rationale, rejected alternatives, and common mistakes, see [AGENTS.md](../../AGENTS.md).

For historical evolution, see `archive/` directory.

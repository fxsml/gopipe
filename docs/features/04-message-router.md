# Feature: Message Router and Handlers

**Package:** `message`
**Status:** ✅ Implemented
**Related ADRs:**
- [ADR 0013](../adr/0013-processor-abstraction.md) - Processor Abstraction
- [ADR 0014](../adr/0014-composable-pipe.md) - Composable Pipe
- [ADR 0015](../adr/0015-middleware-pattern.md) - Middleware Pattern

## Summary

Implements a routing system for dispatching messages to handlers based on attributes (subject, type, etc.), with support for middleware, matchers, and composable pipes.

## Core Types

### Handler Interface
```go
type Handler interface {
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
    Match(attrs Attributes) bool
}
```

### Router
```go
type Router struct { ... }

func NewRouter(config RouterConfig, handlers ...Handler) *Router

func (r *Router) Start(ctx context.Context, in <-chan *Message) <-chan *Message
```

### Matchers
```go
type Matcher func(Attributes) bool

// Predefined matchers
func MatchAll() Matcher
func MatchSubject(subject string) Matcher
func MatchType(typ string) Matcher

// Composable matchers
func And(matchers ...Matcher) Matcher
func Or(matchers ...Matcher) Matcher
func Not(m Matcher) Matcher
```

## Router Configuration

```go
type RouterConfig struct {
    // Concurrency for parallel handler execution (default: 1)
    Concurrency int

    // Middleware applied to all messages
    Middleware []func(Handler) Handler

    // Generator creates output messages for unhandled inputs
    Generator Generator
}
```

## Middleware Support

Middleware wraps handlers to add cross-cutting concerns:

```go
// Middleware signature
type Middleware func(Handler) Handler

// Example: logging middleware
func LoggingMiddleware(h Handler) Handler {
    return HandlerFunc(func(ctx context.Context, msg *Message) ([]*Message, error) {
        log.Printf("Handling message: %s", msg.Attributes[AttrID])
        return h.Handle(ctx, msg)
    })
}

// Usage
router := message.NewRouter(message.RouterConfig{
    Middleware: []func(Handler) Handler{
        LoggingMiddleware,
        middleware.Correlation(),
    },
}, handlers...)
```

## Generator

Generator creates messages for unhandled inputs (e.g., error responses):

```go
type Generator interface {
    Generate(ctx context.Context, msg *Message) ([]*Message, error)
}

// Example: error generator
errorGen := message.GeneratorFunc(func(ctx context.Context, msg *Message) ([]*Message, error) {
    return []*Message{{
        Data: []byte("unhandled message"),
        Attributes: Attributes{AttrType: "error"},
    }}, nil
})

router := message.NewRouter(message.RouterConfig{
    Generator: errorGen,
}, handlers...)
```

## Files Changed

- `message/router.go` - Router implementation
- `message/router_test.go` - Comprehensive router tests
- `message/handler.go` - Handler interface and helpers
- `message/matcher.go` - Matcher functions
- `message/generator.go` - Generator interface
- `message/middleware.go` - Middleware type
- `message/pipe.go` - Pipe type alias for gopipe compatibility

## Usage Example

```go
// Create handlers
orderHandler := message.HandlerFunc(
    func(ctx context.Context, msg *Message) ([]*Message, error) {
        // Process order
        return nil, nil
    },
)

// Configure router
router := message.NewRouter(
    message.RouterConfig{
        Concurrency: 10,
        Middleware: []func(Handler) Handler{
            middleware.Correlation(),
        },
    },
    message.NewHandler(orderHandler, message.MatchSubject("orders")),
)

// Start routing
out := router.Start(ctx, inputMsgs)
for msg := range out {
    // Handle output messages
}
```

## Integration with gopipe

Router implements gopipe's Pipe interface:

```go
type Pipe[In, Out any] func(ctx context.Context, in <-chan In) <-chan Out

// Router.Start matches this signature
var _ message.Pipe = (*message.Router)(nil).Start
```

## Related Features

- [05-message-cqrs](05-message-cqrs.md) - CQRS handlers use Router
- [08-middleware-package](08-middleware-package.md) - Middleware implementations

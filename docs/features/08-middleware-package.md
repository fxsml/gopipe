# Feature: Middleware Package

**Package:** `middleware`
**Status:** ✅ Implemented
**Related ADRs:**
- [ADR 0015](../adr/0015-middleware-pattern.md) - Middleware Pattern

## Summary

Provides reusable middleware components for message routers, including correlation ID propagation and message transformation. Middleware wraps handlers to add cross-cutting concerns.

## Middleware Signature

```go
type Middleware func(message.Handler) message.Handler
```

Middleware wraps a handler to add behavior before/after message handling.

## Available Middleware

### 1. Correlation Middleware

Propagates correlation IDs across message flows:

```go
func Correlation() func(message.Handler) message.Handler
```

**Behavior:**
- Extracts `correlation_id` from incoming message attributes
- Stores in context using `context.WithValue`
- Adds `correlation_id` to all output message attributes
- Generates new correlation ID if none present

**Usage:**
```go
router := message.NewRouter(message.RouterConfig{
    Middleware: []func(message.Handler) message.Handler{
        middleware.Correlation(),
    },
}, handlers...)
```

**Example Flow:**
```go
// Input message
input := &message.Message{
    Attributes: message.Attributes{
        "correlation_id": "req-123",
    },
}

// After middleware processing, all output messages have:
output.Attributes["correlation_id"] == "req-123"

// Access in handler
correlationID := ctx.Value(middleware.CorrelationIDKey).(string)
```

### 2. Message Middleware

Wraps handlers as message processors (input message → output messages):

```go
func NewMessageMiddleware(
    match message.Matcher,
    process func(context.Context, *message.Message) ([]*message.Message, error),
) message.Handler
```

**Behavior:**
- Matches messages using provided matcher
- Processes matched messages with provided function
- Returns output messages directly (doesn't delegate to next handler)
- Returns input message unchanged if not matched

**Usage:**
```go
// Add logging to all messages of type "audit"
auditLogger := middleware.NewMessageMiddleware(
    message.MatchType("audit"),
    func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        log.Printf("Audit: %s", string(msg.Data))
        return []*message.Message{msg}, nil
    },
)

router := message.NewRouter(
    message.RouterConfig{},
    auditLogger,
    // ... other handlers
)
```

## Creating Custom Middleware

### Simple Wrapper
```go
func LoggingMiddleware() func(message.Handler) message.Handler {
    return func(next message.Handler) message.Handler {
        return message.HandlerFunc(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            log.Printf("Processing: %s", msg.Attributes[message.AttrID])
            msgs, err := next.Handle(ctx, msg)
            log.Printf("Produced %d messages", len(msgs))
            return msgs, err
        })
    }
}
```

### Stateful Middleware
```go
func MetricsMiddleware(metrics *Metrics) func(message.Handler) message.Handler {
    return func(next message.Handler) message.Handler {
        return message.HandlerFunc(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            start := time.Now()
            msgs, err := next.Handle(ctx, msg)
            metrics.RecordDuration(time.Since(start))
            if err != nil {
                metrics.RecordError()
            }
            return msgs, err
        })
    }
}
```

### Attribute Enrichment
```go
func EnrichWithTimestamp() func(message.Handler) message.Handler {
    return func(next message.Handler) message.Handler {
        return message.HandlerFunc(func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            msgs, err := next.Handle(ctx, msg)
            for _, msg := range msgs {
                if msg.Attributes == nil {
                    msg.Attributes = make(message.Attributes)
                }
                msg.Attributes["processed_at"] = time.Now().Format(time.RFC3339)
            }
            return msgs, err
        })
    }
}
```

## Middleware Chain Execution Order

Middleware executes in the order specified, wrapping handlers from outside to inside:

```go
router := message.NewRouter(message.RouterConfig{
    Middleware: []func(message.Handler) message.Handler{
        LoggingMiddleware(),     // Outer (runs first)
        MetricsMiddleware(m),    // Middle
        Correlation(),           // Inner (runs last, closest to handler)
    },
}, handlers...)
```

**Execution flow:**
1. LoggingMiddleware (before)
2. MetricsMiddleware (before)
3. Correlation (before)
4. Handler
5. Correlation (after)
6. MetricsMiddleware (after)
7. LoggingMiddleware (after)

## Files Changed

- `middleware/correlation.go` - Correlation ID middleware
- `middleware/message.go` - Message processing middleware
- `middleware/message_test.go` - Middleware tests

## Complete Example

```go
// Setup middleware stack
router := message.NewRouter(
    message.RouterConfig{
        Concurrency: 10,
        Middleware: []func(message.Handler) message.Handler{
            // Log all messages
            LoggingMiddleware(),

            // Track metrics
            MetricsMiddleware(metrics),

            // Propagate correlation IDs
            middleware.Correlation(),

            // Add processing timestamp
            EnrichWithTimestamp(),
        },
    },
    // Handlers
    orderHandler,
    paymentHandler,
)

// Start processing
output := router.Start(ctx, input)
```

## Related Features

- [04-message-router](04-message-router.md) - Router supports middleware
- [05-message-cqrs](05-message-cqrs.md) - CQRS handlers work with middleware

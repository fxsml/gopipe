# Router Middleware

The `message.Router` supports middleware using the gopipe `MiddlewareFunc` pattern, enabling cross-cutting concerns at the router level without modifying individual handlers.

## Overview

Middleware wraps message processing to add behavior such as:
- Logging
- Metrics
- Tracing
- Authentication/Authorization
- Message transformation
- Error handling

## Types

```go
// gopipe.MiddlewareFunc wraps a Processor to add behavior
type MiddlewareFunc[In, Out any] func(Processor[In, Out]) Processor[In, Out]

// For message routing, use:
type MiddlewareFunc = gopipe.MiddlewareFunc[*message.Message, *message.Message]
```

## Usage

### Adding Middleware via RouterConfig

```go
router := message.NewRouter(
    message.RouterConfig{
        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
            LoggingMiddleware(),
            MetricsMiddleware(),
        },
    },
    handlers...,
)
```

Middleware executes in the order added: LoggingMiddleware → MetricsMiddleware → handler

## Creating Middleware

### Option 1: Using middleware.NewMessageMiddleware Helper

The `middleware.NewMessageMiddleware` helper simplifies middleware creation:

```go
import "github.com/fxsml/gopipe/middleware"

func LoggingMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
    return middleware.NewMessageMiddleware(
        func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
            log.Printf("Processing message")
            result, err := next()
            log.Printf("Completed: error=%v", err != nil)
            return result, err
        },
    )
}
```

### Option 2: Direct Processor Wrapping

For advanced use cases, wrap the Processor directly:

```go
func LoggingMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
    return func(proc gopipe.Processor[*message.Message, *message.Message]) gopipe.Processor[*message.Message, *message.Message] {
        return gopipe.NewProcessor(
            func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
                log.Printf("Processing message")
                result, err := proc.Process(ctx, msg)
                log.Printf("Completed: error=%v", err != nil)
                return result, err
            },
            func(msg *message.Message, err error) {
                // Handle cancellation
                proc.Cancel(msg, err)
            },
        )
    }
}
```

## Example Middleware

### 1. Logging Middleware

```go
func LoggingMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
    return middleware.NewMessageMiddleware(
        func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
            subject, _ := msg.Properties.Subject()
            log.Printf("[INFO] Processing: subject=%s", subject)

            start := time.Now()
            result, err := next()
            duration := time.Since(start)

            if err != nil {
                log.Printf("[ERROR] Failed: subject=%s, error=%v, duration=%v", subject, err, duration)
            } else {
                log.Printf("[INFO] Success: subject=%s, duration=%v", subject, duration)
            }

            return result, err
        },
    )
}
```

### 2. Metrics Middleware

```go
func MetricsMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
    return middleware.NewMessageMiddleware(
        func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
            subject, _ := msg.Properties.Subject()

            start := time.Now()
            result, err := next()
            duration := time.Since(start)

            // Record metrics
            messageProcessingDuration.WithLabelValues(subject).Observe(duration.Seconds())

            if err != nil {
                messageProcessingErrors.WithLabelValues(subject).Inc()
            } else {
                messageProcessingSuccess.WithLabelValues(subject).Inc()
            }

            return result, err
        },
    )
}
```

### 3. Validation Middleware

```go
func ValidationMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
    return middleware.NewMessageMiddleware(
        func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
            // Validate required properties
            if _, ok := msg.Properties.Subject(); !ok {
                err := fmt.Errorf("validation error: missing subject")
                msg.Nack(err)
                return nil, err
            }

            if len(msg.Payload) == 0 {
                err := fmt.Errorf("validation error: empty payload")
                msg.Nack(err)
                return nil, err
            }

            return next()
        },
    )
}
```

### 4. Authentication Middleware

```go
func AuthMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
    return middleware.NewMessageMiddleware(
        func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
            // Check authentication token
            token, ok := msg.Properties["auth-token"].(string)
            if !ok || !isValidToken(token) {
                err := fmt.Errorf("unauthorized: invalid or missing auth token")
                msg.Nack(err)
                return nil, err
            }

            return next()
        },
    )
}
```

### 5. Correlation ID Middleware

```go
func CorrelationIDMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
    return middleware.NewMessageMiddleware(
        func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
            // Ensure correlation ID exists
            if _, ok := msg.Properties.CorrelationID(); !ok {
                msg.Properties[message.PropCorrelationID] = uuid.New().String()
            }

            return next()
        },
    )
}
```

### 6. Tracing Middleware

```go
func TracingMiddleware(tracer opentracing.Tracer) gopipe.MiddlewareFunc[*message.Message, *message.Message] {
    return middleware.NewMessageMiddleware(
        func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
            subject, _ := msg.Properties.Subject()

            span, ctx := opentracing.StartSpanFromContext(ctx, "process-message")
            defer span.Finish()

            span.SetTag("subject", subject)
            if corrID, ok := msg.Properties.CorrelationID(); ok {
                span.SetTag("correlation-id", corrID)
            }

            result, err := next()

            if err != nil {
                span.SetTag("error", true)
                span.LogKV("error", err.Error())
            }

            return result, err
        },
    )
}
```

### 7. Short-Circuit Middleware

```go
func ShortCircuitMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
    return middleware.NewMessageMiddleware(
        func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
            // Skip processing for certain message types
            if msg.Properties["bypass"] == "yes" {
                msg.Ack()
                return nil, nil
            }

            return next()
        },
    )
}
```

## Pre-built Middleware

The `middleware` package provides pre-built middleware for common message operations:

```go
import "github.com/fxsml/gopipe/middleware"

router := message.NewRouter(
    message.RouterConfig{
        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
            middleware.MessageCorrelation(),         // Propagate correlation IDs
            middleware.MessageType("event"),          // Set type on output messages
            middleware.MessageSubject("OrderEvents"), // Set subject on output messages
            middleware.MessageTypeName[OrderCreated](), // Set subject from type name
        },
    },
    handlers...,
)
```

### Available Middleware Functions

- **`MessageCorrelation()`**: Propagates correlation ID from input to output messages. Use this for message-level correlation tracking. For type-level property transformation, use PropertyProviders in the marshaler.
- **`MessageType(msgType string)`**: Sets type property on all output messages
- **`MessageSubject(subject string)`**: Sets subject property on all output messages
- **`MessageTypeName[T]()`**: Sets subject based on T's reflected type name (combine with MessageType for both)

### Correlation ID Propagation

For proper correlation ID handling, use both generation and propagation middleware:

```go
router := message.NewRouter(
    message.RouterConfig{
        Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
            CorrelationIDMiddleware(),         // Generate ID if missing
            middleware.MessageCorrelation(),   // Propagate ID to output messages
            LoggingMiddleware(),               // Log with correlation context
        },
    },
    handlers...,
)
```

Note: Correlation ID propagation is handled at the message level by middleware, not by PropertyProviders in the marshaler. PropertyProviders work at the type level for extracting properties from the message payload.

## Complete Example

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/fxsml/gopipe"
    "github.com/fxsml/gopipe/cqrs"
    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/middleware"
)

// Logging middleware
func LoggingMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
    return middleware.NewMessageMiddleware(
        func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
            subject, _ := msg.Properties.Subject()
            log.Printf("[INFO] Processing: %s", subject)

            start := time.Now()
            result, err := next()

            log.Printf("[INFO] Completed: %s (duration=%v, error=%v)",
                subject, time.Since(start), err != nil)

            return result, err
        },
    )
}

// Metrics middleware
func MetricsMiddleware() gopipe.MiddlewareFunc[*message.Message, *message.Message] {
    return middleware.NewMessageMiddleware(
        func(ctx context.Context, msg *message.Message, next func() ([]*message.Message, error)) ([]*message.Message, error) {
            start := time.Now()
            result, err := next()

            log.Printf("[METRICS] duration=%v success=%v", time.Since(start), err == nil)

            return result, err
        },
    )
}

func main() {
    // Create marshaler with property providers
    marshaler := cqrs.NewJSONCommandMarshaler(
        cqrs.WithType("event"),
        cqrs.WithSubjectFromTypeName(),
    )

    // Create handlers
    createOrderHandler := cqrs.NewCommandHandler(
        func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
            // Business logic
            return []OrderCreated{{ID: cmd.ID}}, nil
        },
        marshaler,
        cqrs.Match(
            cqrs.MatchSubject("CreateOrder"),
            cqrs.MatchType("command"),
        ),
    )

    // Create router with middleware
    router := message.NewRouter(
        message.RouterConfig{
            Concurrency: 10,
            Recover:     true,
            Middleware: []gopipe.MiddlewareFunc[*message.Message, *message.Message]{
                LoggingMiddleware(),
                MetricsMiddleware(),
            },
        },
        createOrderHandler,
    )

    // Start processing
    ctx := context.Background()
    incoming := make(chan *message.Message)
    outgoing := router.Start(ctx, incoming)

    // Process messages
    for msg := range outgoing {
        log.Printf("Output message: %v", msg)
    }
}
```

## Key Benefits

1. **Unified Pattern**: Router middleware uses the same `gopipe.MiddlewareFunc` pattern as all gopipe middleware
2. **Composable**: Can use any gopipe middleware with the router
3. **Powerful**: Access to both Process and Cancel interception
4. **CQRS Middleware**: Pre-built middleware for common property transformations
5. **Type-Safe**: Generic types ensure compile-time safety

## Order of Execution

Given middleware added in order: `[A, B, C]`

```
Request Flow:
Middleware A (before) →
  Middleware B (before) →
    Middleware C (before) →
      Handler →
    Middleware C (after) →
  Middleware B (after) →
Middleware A (after)
```

## Best Practices

1. **Order Matters**: Place authentication/validation before logging
2. **Error Handling**: Always handle errors appropriately in middleware
3. **Context Propagation**: Pass context through the chain
4. **Performance**: Keep middleware lightweight for high-throughput scenarios
5. **Use Helpers**: Use `middleware.NewMessageMiddleware` for simpler cases
6. **Direct Wrapping**: Use direct Processor wrapping when you need Cancel interception

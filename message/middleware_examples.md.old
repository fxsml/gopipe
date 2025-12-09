# Router Middleware

The `message.Router` supports middleware similar to Watermill's approach, allowing you to add cross-cutting concerns at the router level without modifying individual handlers.

## Overview

Middleware wraps handler functions to add additional behavior such as:
- Logging
- Metrics
- Tracing
- Authentication/Authorization
- Message transformation
- Error handling

## Types

```go
// HandlerFunc is the core handler function signature
type HandlerFunc func(ctx context.Context, msg *Message) ([]*Message, error)

// HandlerMiddleware wraps a HandlerFunc to add additional behavior
type HandlerMiddleware func(next HandlerFunc) HandlerFunc
```

## Usage

### Adding Middleware

```go
router := message.NewRouter(message.RouterConfig{}, handlers...)
router.AddMiddleware(loggingMiddleware, metricsMiddleware, authMiddleware)
```

Middleware is executed in the order added:
- `loggingMiddleware` → `metricsMiddleware` → `authMiddleware` → `handler`

## Example Middleware

### 1. Logging Middleware

```go
func LoggingMiddleware(next message.HandlerFunc) message.HandlerFunc {
    return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        subject, _ := msg.Properties.Subject()
        log.Printf("Processing message: subject=%s", subject)

        start := time.Now()
        result, err := next(ctx, msg)
        duration := time.Since(start)

        if err != nil {
            log.Printf("Failed to process message: subject=%s, error=%v, duration=%v",
                subject, err, duration)
        } else {
            log.Printf("Successfully processed message: subject=%s, duration=%v",
                subject, duration)
        }

        return result, err
    }
}
```

### 2. Metrics Middleware

```go
func MetricsMiddleware(next message.HandlerFunc) message.HandlerFunc {
    return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        subject, _ := msg.Properties.Subject()

        start := time.Now()
        result, err := next(ctx, msg)
        duration := time.Since(start)

        // Record metrics
        messageProcessingDuration.WithLabelValues(subject).Observe(duration.Seconds())

        if err != nil {
            messageProcessingErrors.WithLabelValues(subject).Inc()
        } else {
            messageProcessingSuccess.WithLabelValues(subject).Inc()
        }

        return result, err
    }
}
```

### 3. Correlation ID Middleware

```go
func CorrelationIDMiddleware(next message.HandlerFunc) message.HandlerFunc {
    return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        // Ensure correlation ID exists
        if _, ok := msg.Properties.CorrelationID(); !ok {
            // Generate new correlation ID if not present
            msg.Properties[message.PropCorrelationID] = uuid.New().String()
        }

        return next(ctx, msg)
    }
}
```

### 4. Authentication Middleware

```go
func AuthMiddleware(next message.HandlerFunc) message.HandlerFunc {
    return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        // Check authentication token
        token, ok := msg.Properties["auth-token"].(string)
        if !ok || !isValidToken(token) {
            err := fmt.Errorf("unauthorized: invalid or missing auth token")
            msg.Nack(err)
            return nil, err
        }

        // Add user info to context
        ctx = context.WithValue(ctx, "user", getUserFromToken(token))

        return next(ctx, msg)
    }
}
```

### 5. Retry Middleware

```go
func RetryMiddleware(maxRetries int, backoff time.Duration) message.HandlerMiddleware {
    return func(next message.HandlerFunc) message.HandlerFunc {
        return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            var result []*message.Message
            var err error

            for attempt := 0; attempt <= maxRetries; attempt++ {
                result, err = next(ctx, msg)
                if err == nil {
                    return result, nil
                }

                if attempt < maxRetries {
                    time.Sleep(backoff * time.Duration(attempt+1))
                }
            }

            return result, fmt.Errorf("max retries exceeded: %w", err)
        }
    }
}
```

### 6. Message Validation Middleware

```go
func ValidationMiddleware(next message.HandlerFunc) message.HandlerFunc {
    return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        // Validate required properties
        if _, ok := msg.Properties.Subject(); !ok {
            err := fmt.Errorf("invalid message: missing subject")
            msg.Nack(err)
            return nil, err
        }

        if len(msg.Payload) == 0 {
            err := fmt.Errorf("invalid message: empty payload")
            msg.Nack(err)
            return nil, err
        }

        return next(ctx, msg)
    }
}
```

### 7. Tracing Middleware

```go
func TracingMiddleware(tracer opentracing.Tracer) message.HandlerMiddleware {
    return func(next message.HandlerFunc) message.HandlerFunc {
        return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
            subject, _ := msg.Properties.Subject()

            span, ctx := opentracing.StartSpanFromContext(ctx, "process-message")
            defer span.Finish()

            span.SetTag("subject", subject)
            if corrID, ok := msg.Properties.CorrelationID(); ok {
                span.SetTag("correlation-id", corrID)
            }

            result, err := next(ctx, msg)

            if err != nil {
                span.SetTag("error", true)
                span.LogKV("error", err.Error())
            }

            return result, err
        }
    }
}
```

### 8. Short-Circuit Middleware

```go
func ShortCircuitMiddleware(next message.HandlerFunc) message.HandlerFunc {
    return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        // Skip processing for certain message types
        if msg.Properties["bypass"] == "yes" {
            msg.Ack()
            return nil, nil
        }

        return next(ctx, msg)
    }
}
```

## Complete Example

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/fxsml/gopipe/cqrs"
    "github.com/fxsml/gopipe/message"
)

// Logging middleware
func LoggingMiddleware(next message.HandlerFunc) message.HandlerFunc {
    return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        subject, _ := msg.Properties.Subject()
        log.Printf("[INFO] Processing: %s", subject)

        start := time.Now()
        result, err := next(ctx, msg)

        log.Printf("[INFO] Completed: %s (duration=%v, error=%v)",
            subject, time.Since(start), err != nil)

        return result, err
    }
}

// Metrics middleware
func MetricsMiddleware(next message.HandlerFunc) message.HandlerFunc {
    return func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        start := time.Now()
        result, err := next(ctx, msg)

        // Record metrics here
        log.Printf("[METRICS] duration=%v success=%v", time.Since(start), err == nil)

        return result, err
    }
}

func main() {
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
        cqrs.WithTypeAndName[OrderCreated]("event"),
    )

    // Create router
    router := message.NewRouter(
        message.RouterConfig{
            Concurrency: 10,
            Recover:     true,
        },
        createOrderHandler,
    )

    // Add middleware (order matters!)
    router.AddMiddleware(
        LoggingMiddleware,
        MetricsMiddleware,
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

1. **Separation of Concerns**: Cross-cutting concerns are isolated from business logic
2. **Reusability**: Middleware can be reused across different routers
3. **Composability**: Multiple middleware can be chained together
4. **Router-Level**: No need to modify individual handlers
5. **Watermill Compatibility**: Similar pattern to Watermill's middleware approach

## Order of Execution

Given middleware added in order: `A, B, C`

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
5. **Idempotency**: Middleware should be safe to apply multiple times

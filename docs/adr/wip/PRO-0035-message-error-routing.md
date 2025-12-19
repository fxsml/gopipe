# ADR 0035: Message-Specific Error Routing

**Date:** 2025-12-17
**Status:** Proposed

## Context

With the simplified Processor interface (PRO-0034), error handling moves out of the processor. For messaging systems, errors need to be routed to appropriate destinations (dead-letter queues, error topics).

## Decision

Provide error routing middleware specific to messaging systems:

```go
// middleware/message/routing.go
package message

// ErrorRoutingConfig configures error message routing
type ErrorRoutingConfig struct {
    Destination    string // e.g., "gopipe://errors", "kafka://dlq"
    IncludeInput   bool   // Include original message in error
    IncludeStack   bool   // Include stack trace
}

// WithErrorRouting routes errors to a destination
func WithErrorRouting(config ErrorRoutingConfig) middleware.Middleware[*message.Message, *message.Message]
```

**Usage:**

```go
loop.Route("orders", handler,
    ProcessorConfig{Concurrency: 4},
    WithErrorRouting(ErrorRoutingConfig{
        Destination:  "gopipe://errors",
        IncludeInput: true,
        IncludeStack: true,
    }),
)
```

The middleware:
1. Catches errors from the processor
2. Creates an error message with context
3. Routes to the configured destination
4. Optionally includes original input and stack trace

## Consequences

**Positive:**
- Clean error routing for messaging systems
- Configurable destination (DLQ, error topic, etc.)
- Debugging context (original input, stack trace)
- Composable with other middleware

**Negative:**
- Message-specific, not general-purpose
- Requires destination infrastructure
- Additional message overhead

## Links

- Extracted from: [PRO-0026](PRO-0026-pipe-processor-simplification.md)
- Related: [PRO-0034](PRO-0034-simplified-processor-interface.md) - Simplified Processor Interface
- Related: [PRO-0037](PRO-0037-middleware-package-consolidation.md) - Middleware Package Consolidation

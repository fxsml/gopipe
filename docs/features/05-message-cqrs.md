# Feature: CQRS Command and Event Handlers

**Package:** `message/cqrs`
**Status:** ✅ Implemented (Core), 🔄 Proposed (Saga/Outbox)
**Related ADRs:**
- [ADR 0006](../adr/0006-cqrs-implementation.md) - CQRS Implementation (Implemented)
- [ADR 0007](../adr/0007-saga-coordinator-pattern.md) - Saga Coordinator (Proposed)
- [ADR 0008](../adr/0008-compensating-saga-pattern.md) - Compensating Saga (Proposed)
- [ADR 0009](../adr/0009-transactional-outbox-pattern.md) - Transactional Outbox (Proposed)

## Summary

Provides type-safe CQRS handlers for commands and events with pluggable marshaling and attribute providers. Separates read/write operations and enables event sourcing patterns.

## Core Components

### Marshaler Interface
```go
type Marshaler interface {
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
    Name(v any) string  // Returns type name
}

// Predefined marshalers
func JSONMarshaler(nameFunc func(any) string) Marshaler
```

### Command Handler
Commands trigger state changes and emit events:

```go
func NewCommandHandler[Cmd, Evt any](
    handle func(ctx context.Context, cmd Cmd) ([]Evt, error),
    match message.Matcher,
    marshaler Marshaler,
) message.Handler
```

**Example:**
```go
type CreateOrder struct {
    OrderID string
    Items   []string
}

type OrderCreated struct {
    OrderID string
}

handler := cqrs.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // Process command
        return []OrderCreated{{OrderID: cmd.OrderID}}, nil
    },
    message.MatchType("CreateOrder"),
    cqrs.JSONMarshaler(cqrs.TypeName),
)
```

### Event Handler
Events trigger side effects (projections, notifications):

```go
func NewEventHandler[Evt any](
    handle func(ctx context.Context, evt Evt) error,
    match message.Matcher,
    marshaler Marshaler,
) message.Handler
```

**Example:**
```go
handler := cqrs.NewEventHandler(
    func(ctx context.Context, evt OrderCreated) error {
        // Update read model, send notification, etc.
        return nil
    },
    message.MatchType("OrderCreated"),
    cqrs.JSONMarshaler(cqrs.TypeName),
)
```

## Attribute Providers

Enrich messages with metadata:

```go
type AttributeProvider interface {
    Provide(v any) message.Attributes
}

// Combine multiple providers
func CombineProviders(providers ...AttributeProvider) AttributeProvider

// Predefined providers
func TypeProvider(nameFunc func(any) string) AttributeProvider
func SubjectProvider(subjectFunc func(any) string) AttributeProvider
```

**Example:**
```go
provider := cqrs.CombineProviders(
    cqrs.TypeProvider(cqrs.TypeName),
    cqrs.SubjectProvider(func(v any) string {
        if order, ok := v.(CreateOrder); ok {
            return "orders." + order.OrderID
        }
        return ""
    }),
)
```

## Advanced Patterns (Proposed)

### Saga Coordinator (Proposed - ADR 0007)
Orchestrates multi-step workflows:

```go
type SagaCoordinator interface {
    OnEvent(ctx context.Context, msg *message.Message) ([]*message.Message, error)
}
```

See [examples/cqrs-saga-coordinator](../../examples/cqrs-saga-coordinator) for implementation example.

### Compensating Saga (Proposed - ADR 0008)
Handles rollback for failed workflows.

### Transactional Outbox (Proposed - ADR 0009)
Ensures reliable event publishing.

## Files Changed

- `message/cqrs/handler.go` - Command and event handlers
- `message/cqrs/handler_test.go` - Handler tests
- `message/cqrs/marshaler.go` - Marshaler interface and JSON impl
- `message/cqrs/attributes.go` - Attribute providers
- `message/cqrs/matcher.go` - CQRS-specific matchers
- `message/cqrs/type.go` - Type name utilities
- `message/cqrs/generator.go` - Message generator
- `message/cqrs/pipe.go` - Pipe type alias
- `message/cqrs/doc.go` - Package documentation

## Complete Example

```go
// Define types
type CreateOrder struct { OrderID string }
type OrderCreated struct { OrderID string }
type OrderFailed struct { OrderID, Reason string }

// Marshaler
marshaler := cqrs.JSONMarshaler(cqrs.TypeName)

// Command handler
cmdHandler := cqrs.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]any, error) {
        if validate(cmd) {
            return []any{OrderCreated{OrderID: cmd.OrderID}}, nil
        }
        return []any{OrderFailed{OrderID: cmd.OrderID, Reason: "invalid"}}, nil
    },
    message.MatchType("CreateOrder"),
    marshaler,
)

// Event handler
evtHandler := cqrs.NewEventHandler(
    func(ctx context.Context, evt OrderCreated) error {
        log.Printf("Order created: %s", evt.OrderID)
        return nil
    },
    message.MatchType("OrderCreated"),
    marshaler,
)

// Setup router
router := message.NewRouter(
    message.RouterConfig{Concurrency: 10},
    cmdHandler,
    evtHandler,
)
```

## Related Features

- [04-message-router](04-message-router.md) - Router dispatches to CQRS handlers
- [06-message-cloudevents](06-message-cloudevents.md) - Event serialization format

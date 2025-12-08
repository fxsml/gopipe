# ADR 0006: CQRS Implementation

**Date:** 2025-12-08
**Status:** Implemented
**Related:**
- [ADR 0007: Saga Coordinator Pattern](./0007-saga-coordinator-pattern.md)
- [ADR 0008: Compensating Saga Pattern](./0008-compensating-saga-pattern.md) (proposed)
- [ADR 0009: Transactional Outbox Pattern](./0009-transactional-outbox-pattern.md) (proposed)

## Context

Command Query Responsibility Segregation (CQRS) is a widely-used pattern in event-driven architectures that separates read and write operations. Watermill, a popular Go messaging library, provides a CQRS component that simplifies building event-driven applications.

gopipe currently has a strong foundation for CQRS with:
- Type-safe message routing via `message.Router`
- Generic handlers via `message.NewHandler()`
- Property-based message matching
- Built-in JSON marshaling support

However, gopipe lacks explicit CQRS semantics and convenience APIs that make the pattern easy to adopt.

### Research: Watermill's CQRS Approach

Watermill's CQRS implementation ([docs](https://watermill.io/docs/cqrs/)) provides:

1. **CommandBus** - For sending commands (imperative requests)
2. **EventBus** - For publishing events (declarative facts)
3. **Marshaler Interface** - Pluggable serialization (JSON, Protobuf, etc.)
4. **Handler Registration** - Type-safe handler functions

**Key Distinctions:**
- **Commands**: Imperative ("CreateOrder"), single handler, request for action
- **Events**: Past tense ("OrderCreated"), multiple handlers, notification

## Decision

Implement a dedicated **`cqrs` package** that builds on gopipe's `message` package, providing CQRS-specific semantics while maintaining gopipe's philosophy of simplicity and type safety.

### Design Principles

1. **Type-Safe**: Use generics for commands and events
2. **Simple**: Leverage existing `message.Router` and `message.Handler`
3. **Flexible**: Support custom marshalers via interface
4. **Explicit**: Clear distinction between commands and events
5. **Minimal**: Don't over-engineer - provide just what's needed
6. **Composable**: Users wire handlers with `message.NewRouter()` directly

## Architecture

### Core Components

```go
package cqrs

// Marshaler serializes commands/events to messages
type Marshaler interface {
    // Marshal converts a command/event to message payload
    Marshal(v any) ([]byte, error)

    // Unmarshal converts message payload to command/event
    Unmarshal(data []byte, v any) error

    // Name returns the name for a command/event type
    Name(v any) string
}

// JSONMarshaler provides JSON serialization
type JSONMarshaler struct{}

func (m JSONMarshaler) Marshal(v any) ([]byte, error) {
    return json.Marshal(v)
}

func (m JSONMarshaler) Unmarshal(data []byte, v any) error {
    return json.Unmarshal(data, v)
}

func (m JSONMarshaler) Name(v any) string {
    t := reflect.TypeOf(v)
    if t.Kind() == reflect.Ptr {
        t = t.Elem()
    }
    return t.Name()
}
```

### Handler Functions

```go
// NewCommandHandler creates a handler that processes commands and returns events.
//
// Type Parameters:
//   - Cmd: The command struct type (e.g., CreateOrder)
//   - Evt: The event struct type (e.g., OrderCreated)
func NewCommandHandler[Cmd, Evt any](
    cmdName string,
    marshaler Marshaler,
    handle func(ctx context.Context, cmd Cmd) ([]Evt, error),
) message.Handler

// NewEventHandler creates a handler that processes events and performs side effects.
//
// Type Parameters:
//   - Evt: The event struct type (e.g., OrderCreated)
func NewEventHandler[Evt any](
    evtName string,
    marshaler Marshaler,
    handle func(ctx context.Context, evt Evt) error,
) message.Handler
```

### Utility Functions

```go
// CreateCommand creates a command message from a command struct.
func CreateCommand(
    marshaler Marshaler,
    cmd any,
    props message.Properties,
) *message.Message

// CreateCommands creates multiple command messages with the same correlation ID.
func CreateCommands(
    marshaler Marshaler,
    correlationID string,
    cmds ...any,
) []*message.Message
```

## Implementation Example

### 1. Define Commands and Events

```go
// Command (imperative)
type CreateOrder struct {
    ID         string `json:"id"`
    CustomerID string `json:"customer_id"`
    Amount     int    `json:"amount"`
}

// Event (past tense)
type OrderCreated struct {
    ID         string    `json:"id"`
    CustomerID string    `json:"customer_id"`
    Amount     int       `json:"amount"`
    CreatedAt  time.Time `json:"created_at"`
}
```

### 2. Command Handler (Command → Events)

```go
marshaler := cqrs.NewJSONMarshaler()

createOrderHandler := cqrs.NewCommandHandler(
    "CreateOrder",
    marshaler,
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // ✅ Pure function: Command → Events
        // ✅ Type-safe: cmd is CreateOrder struct
        // ✅ Testable: no dependencies on buses

        // Business logic
        if err := saveOrder(cmd); err != nil {
            return nil, err
        }

        // Return events
        return []OrderCreated{{
            ID:         cmd.ID,
            CustomerID: cmd.CustomerID,
            Amount:     cmd.Amount,
            CreatedAt:  time.Now(),
        }}, nil
    },
)
```

### 3. Event Handler (Event → Side Effects)

```go
emailHandler := cqrs.NewEventHandler(
    "OrderCreated",
    marshaler,
    func(ctx context.Context, evt OrderCreated) error {
        // ✅ Pure side effect: no commands returned
        // ✅ Type-safe: evt is OrderCreated struct
        // ✅ Easy to test

        log.Printf("📧 Sending confirmation email...")
        return emailService.Send(evt.CustomerID, "Order created!")
    },
)

analyticsHandler := cqrs.NewEventHandler(
    "OrderCreated",
    marshaler,
    func(ctx context.Context, evt OrderCreated) error {
        log.Printf("📊 Tracking analytics...")
        return analyticsService.Track("order_created", evt)
    },
)
```

### 4. Wire Together

```go
// Command router
commandRouter := message.NewRouter(
    message.RouterConfig{
        Concurrency: 10,
        Recover:     true,
    },
    createOrderHandler,
    // ... other command handlers
)

// Event router (side effects)
eventRouter := message.NewRouter(
    message.RouterConfig{
        Concurrency: 20,
        Recover:     true,
    },
    emailHandler,
    analyticsHandler,
    // ... other event handlers
)

// Commands → Events
commands := make(chan *message.Message, 10)
events := commandRouter.Start(ctx, commands)

// Events → Side effects
eventRouter.Start(ctx, events)

// Send command
commands <- cqrs.CreateCommand(
    marshaler,
    CreateOrder{ID: "order-1", CustomerID: "customer-1", Amount: 100},
    message.Properties{
        message.PropCorrelationID: "corr-123",
    },
)
```

## Benefits

### 1. Type Safety

```go
// ✅ Before cqrs package: Manual casting
func handler(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
    var cmd CreateOrder
    json.Unmarshal(msg.Payload, &cmd) // Manual work
    // ...
}

// ✅ With cqrs package: Type-safe
cqrs.NewCommandHandler(
    "CreateOrder",
    marshaler,
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // cmd is already typed!
    },
)
```

### 2. Clear CQRS Semantics

```go
// Clear intent: this is a command
createOrderHandler := cqrs.NewCommandHandler(...)

// Clear intent: this is an event
orderCreatedHandler := cqrs.NewEventHandler(...)
```

### 3. Testability

```go
// ✅ Test pure business logic
func TestCreateOrder(t *testing.T) {
    events, err := handleCreateOrder(ctx, CreateOrder{
        ID:     "order-1",
        Amount: 100,
    })

    assert.NoError(t, err)
    assert.Len(t, events, 1)
    assert.Equal(t, "order-1", events[0].ID)
}
```

### 4. Decoupling

```go
// Events can have multiple independent handlers
cqrs.NewEventHandler("OrderCreated", ..., sendEmail)
cqrs.NewEventHandler("OrderCreated", ..., trackAnalytics)
cqrs.NewEventHandler("OrderCreated", ..., updateInventory)
```

## Implementation Status

### ✅ Implemented Components

Package: `github.com/fxsml/gopipe/cqrs`

- ✅ `cqrs.Marshaler` interface (`cqrs/marshaler.go:7`)
- ✅ `cqrs.JSONMarshaler` implementation (`cqrs/marshaler.go:20`)
- ✅ `cqrs.NewCommandHandler[Cmd, Evt]()` (`cqrs/handler.go:42`)
- ✅ `cqrs.NewEventHandler[Evt]()` (`cqrs/handler.go:115`)
- ✅ `cqrs.CreateCommand()` utility (`cqrs/util.go:13`)
- ✅ `cqrs.CreateCommands()` utility (`cqrs/util.go:49`)
- ✅ Package documentation (`cqrs/doc.go`)
- ✅ Complete example (`examples/cqrs-package/`)

### Key Design Decisions

1. **No CommandProcessor/EventProcessor classes** - Users compose handlers with `message.NewRouter()` directly, maintaining gopipe's simplicity
2. **Type-safe handler functions** - Generics provide compile-time type safety
3. **Command handlers return events** - Pure functions (Command → Events)
4. **Event handlers perform side effects only** - No commands returned
5. **Independent acking per stage** - Correlation IDs for end-to-end tracing

## Advanced Patterns

For complex workflows and advanced use cases, see related ADRs:

### Multi-Step Workflows (Sagas)

**[ADR 0007: Saga Coordinator Pattern](./0007-saga-coordinator-pattern.md)** ✅ Implemented

Coordinate multi-step workflows where events trigger commands:

```
CreateOrder → OrderCreated → ChargePayment → PaymentCharged → ShipOrder
```

- ✅ `SagaCoordinator` interface
- ✅ Event → Commands workflow logic
- ✅ Feedback loops via `channel.Merge`

### Saga Failure Handling (Compensations)

**[ADR 0008: Compensating Saga Pattern](./0008-compensating-saga-pattern.md)** 📋 Proposed

Automatic rollback on saga failure:

```
ChargePayment → PaymentCharged
ReserveInventory → ❌ FAILED!
    ↓
Execute compensations:
    RefundPayment ✅
    CancelOrder ✅
```

- 📋 `cqrs/compensation` package
- 📋 `SagaStore` interface for state persistence
- 📋 LIFO compensation execution

### Exactly-Once Message Delivery

**[ADR 0009: Transactional Outbox Pattern](./0009-transactional-outbox-pattern.md)** 📋 Proposed

Atomic database writes + message publishing:

```
┌─── DB Transaction ───┐
│ INSERT INTO orders   │
│ INSERT INTO outbox   │ ← Atomic!
│ COMMIT               │
└──────────────────────┘
```

- 📋 `cqrs/outbox` package
- 📋 `OutboxStore` interface
- 📋 Background outbox processor

## Comparison: Watermill vs gopipe

### Watermill (Configuration-Heavy)

```go
commandBus, err := cqrs.NewCommandBusWithConfig(
    publisher,
    cqrs.CommandBusConfig{
        GeneratePublishTopic: func(...) (string, error) { ... },
        Marshaler:            cqrsMarshaler,
        Logger:               logger,
    })

err = commandProcessor.AddHandlers(
    cqrs.NewCommandHandler("BookRoomHandler",
        BookRoomHandler{eventBus}.Handle),
)
```

### gopipe (Simple)

```go
marshaler := cqrs.NewJSONMarshaler()

handler := cqrs.NewCommandHandler(
    "CreateOrder",
    marshaler,
    handleCreateOrder,
)

router := message.NewRouter(
    message.RouterConfig{Concurrency: 10},
    handler,
)
```

## Migration Path

Users currently using `message.Router` directly can migrate incrementally:

**Before (direct Router usage):**
```go
handler := message.NewHandler(
    func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        var cmd CreateOrder
        json.Unmarshal(msg.Payload, &cmd)
        // ... manual work
    },
    func(prop message.Properties) bool {
        subject, _ := prop.Subject()
        return subject == "CreateOrder"
    },
)
```

**After (CQRS):**
```go
handler := cqrs.NewCommandHandler(
    "CreateOrder",
    marshaler,
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // Type-safe, clean API
        return []OrderCreated{{...}}, nil
    },
)
```

## Breaking Changes

None - this is a new package that builds on existing `message` package APIs.

## Alternatives Considered

### Alternative 1: Helper Functions in message Package

Add `message.NewCommandHandler()` and `message.NewEventHandler()` as convenience functions.

**Rejected because:**
- Pollutes the `message` package with CQRS-specific concepts
- Harder to extend with CQRS-specific features later
- Less clear separation of concerns

### Alternative 2: event Package (not cqrs)

Create an `event` package focused only on events, not commands.

**Rejected because:**
- Commands and events are complementary concepts
- Users typically need both for CQRS
- CQRS is a well-known pattern name

### Alternative 3: Copy Watermill's API Exactly

Replicate Watermill's configuration-heavy approach.

**Rejected because:**
- Goes against gopipe's simplicity philosophy
- Too much ceremony for basic use cases
- gopipe already has Router configuration

## References

### CQRS
- [Watermill CQRS Component](https://watermill.io/docs/cqrs/)
- [How to use basic CQRS in Go](https://threedots.tech/post/basic-cqrs-in-go/)
- [Microsoft: CQRS Pattern](https://learn.microsoft.com/en-us/azure/architecture/patterns/cqrs)

### gopipe
- [ADR 0007: Saga Coordinator Pattern](./0007-saga-coordinator-pattern.md)
- [ADR 0008: Compensating Saga Pattern](./0008-compensating-saga-pattern.md)
- [ADR 0009: Transactional Outbox Pattern](./0009-transactional-outbox-pattern.md)
- [CQRS Overview](../cqrs-overview.md)
- [Example: cqrs-package](../../examples/cqrs-package/)

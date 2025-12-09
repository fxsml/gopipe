# CQRS Package Example

This example demonstrates the **cqrs package**, which provides a clean, type-safe API for implementing CQRS and Saga patterns in gopipe.

## Overview

The `cqrs` package simplifies event-driven architecture by providing:

- **NewCommandHandler**: Type-safe command → events handlers
- **NewEventHandler**: Type-safe event → side effects handlers
- **SagaCoordinator**: Interface for workflow orchestration
- **Marshaler**: Pluggable serialization (JSON, Protobuf)
- **Utility functions**: CreateCommand, CreateCommands

## Architecture

```
┌─────────────┐
│   Command   │
└──────┬──────┘
       ↓
┌──────────────┐
│   Command    │  ← cqrs.NewCommandHandler
│   Handler    │     (type-safe)
└──────┬───────┘
       ↓
┌──────────────┐
│    Event     │
└──────┬───────┘
       ├───────────────┬────────────────┐
       ↓               ↓                ↓
┌─────────────┐ ┌─────────────┐ ┌──────────────┐
│ Side Effects│ │    Saga     │ │ Side Effects │
│  (Email)    │ │ Coordinator │ │ (Analytics)  │
│             │ │             │ │              │
│  ← cqrs.    │ │  ← cqrs.    │ │  ← cqrs.     │
│    NewEvent │ │    Saga     │ │    NewEvent  │
│    Handler  │ │    Coord.   │ │    Handler   │
└─────────────┘ └──────┬──────┘ └──────────────┘
                       ↓
                ┌──────────────┐
                │   Commands   │
                │  (feedback)  │
                └──────────────┘
```

## Code Example

### 1. Command Handler (Type-Safe)

```go
import "github.com/fxsml/gopipe/cqrs"

marshaler := cqrs.NewJSONMarshaler()

createOrderHandler := cqrs.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // ✅ Type-safe! cmd is CreateOrder struct
        // ✅ Pure function: no dependencies on buses
        // ✅ Return events directly

        saveOrder(cmd)
        return []OrderCreated{{
            ID:         cmd.ID,
            CustomerID: cmd.CustomerID,
            Amount:     cmd.Amount,
            CreatedAt:  time.Now(),
        }}, nil
    },
    marshaler,
    cqrs.Match(cqrs.MatchSubject("CreateOrder"), cqrs.MatchType("command")),
    cqrs.WithTypeAndName[OrderCreated]("event"),
)
```

### 2. Event Handler (Side Effects)

```go
emailHandler := cqrs.NewEventHandler(
    func(ctx context.Context, evt OrderCreated) error {
        // ✅ Type-safe! evt is OrderCreated struct
        // ✅ Pure side effect: no commands returned
        // ✅ Easy to test

        return emailService.Send(evt.CustomerID, "Order created!")
    },
    marshaler,
    cqrs.Match(cqrs.MatchSubject("OrderCreated"), cqrs.MatchType("event")),
)
```

### 3. Saga Coordinator (Workflow Logic)

```go
type OrderSagaCoordinator struct {
    marshaler cqrs.Marshaler
}

func (s *OrderSagaCoordinator) OnEvent(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
    subject, _ := msg.Properties.Subject()
    corrID, _ := msg.Properties.CorrelationID()

    switch subject {
    case "OrderCreated":
        var evt OrderCreated
        s.marshaler.Unmarshal(msg.Payload, &evt)

        // ✅ Workflow logic: what happens next?
        // ✅ One event → multiple commands
        return cqrs.CreateCommands(s.marshaler, corrID,
            ChargePayment{OrderID: evt.ID, Amount: evt.Amount},
            ReserveInventory{OrderID: evt.ID, SKU: "SKU-123"},
        ), nil

    case "PaymentCharged":
        return cqrs.CreateCommands(s.marshaler, corrID,
            ShipOrder{OrderID: evt.OrderID},
        ), nil

    case "OrderShipped":
        return nil, nil // Terminal
    }

    return nil, nil
}
```

### 4. Wire Together

```go
// Command processor
commandRouter := message.NewRouter(
    message.RouterConfig{Concurrency: 10, Recover: true},
    createOrderHandler,
    chargePaymentHandler,
    reserveInventoryHandler,
    shipOrderHandler,
)

// Event processors
sideEffectsRouter := message.NewRouter(
    message.RouterConfig{Concurrency: 20, Recover: true},
    cqrs.NewEventHandler(handleEmail, marshaler, cqrs.Match(cqrs.MatchSubject("OrderCreated"), cqrs.MatchType("event"))),
    cqrs.NewEventHandler(handleAnalytics, marshaler, cqrs.Match(cqrs.MatchSubject("OrderCreated"), cqrs.MatchType("event"))),
)

sagaCoordinator := &OrderSagaCoordinator{marshaler: marshaler}
sagaHandler := message.NewHandler(
    sagaCoordinator.OnEvent,
    func(prop message.Properties) bool {
        msgType, _ := prop["type"].(string)
        return msgType == "event"
    },
)
sagaRouter := message.NewRouter(message.RouterConfig{}, sagaHandler)

// Feedback loop
initialCommands := make(chan *message.Message, 10)
sagaCommands := make(chan *message.Message, 100)
allCommands := channel.Merge(initialCommands, sagaCommands)

events := commandRouter.Start(ctx, allCommands)

// Fan-out events
eventChan1 := make(chan *message.Message, 100)
eventChan2 := make(chan *message.Message, 100)
go func() {
    for evt := range events {
        eventChan1 <- evt
        eventChan2 <- evt
    }
    close(eventChan1)
    close(eventChan2)
}()

sideEffectsRouter.Start(ctx, eventChan1)
sagaOut := sagaRouter.Start(ctx, eventChan2)

go func() {
    for cmd := range sagaOut {
        sagaCommands <- cmd
    }
}()
```

## Running the Example

```bash
cd examples/cqrs-package
go run main.go
```

**Expected Output:**

```
======================================================================
CQRS Package Example - Order Processing Saga
======================================================================

Flow:
  CreateOrder (command)
    → OrderCreated (event)
      → Email + Analytics (side effects)
      → ChargePayment + ReserveInventory (saga commands)
    → PaymentCharged + InventoryReserved (events)
      → ShipOrder (saga command)
    → OrderShipped (event)
      → Email (side effect)

🚀 Sending CreateOrder command...

📝 Command: CreateOrder
   💾 Saving order to database...
🔄 Saga: OrderCreated → triggering ChargePayment + ReserveInventory
📧 Side Effect: Sending order confirmation email to customer-456
📊 Side Effect: Tracking order_created event (amount: $350)
📝 Command: ChargePayment
   💳 Charging $350...
📝 Command: ReserveInventory
   📦 Reserving inventory for SKU-12345...
🔄 Saga: PaymentCharged → waiting for InventoryReserved...
📊 Side Effect: Tracking payment_charged event (amount: $350)
🔄 Saga: InventoryReserved → triggering ShipOrder
📝 Command: ShipOrder
   🚚 Shipping to 123 Main St...
✅ Saga: OrderShipped → saga complete!
📧 Side Effect: Sending shipping notification (tracking: TRACK-order-789)

======================================================================
Demo Complete!

Key Benefits of cqrs Package:
  ✅ Type-safe command and event handlers
  ✅ Clean separation: side effects vs workflow logic
  ✅ Pluggable marshalers (JSON, Protobuf, etc.)
  ✅ Built on gopipe's channel-based architecture
  ✅ Easy to test: pure functions
======================================================================
```

## Key Benefits

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
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // cmd is already typed!
    },
    marshaler,
    cqrs.Match(cqrs.MatchSubject("CreateOrder"), cqrs.MatchType("command")),
    cqrs.WithTypeAndName[OrderCreated]("event"),
)
```

### 2. Clean Separation

```go
// ✅ Side effects (event handlers)
func handleEmail(ctx, evt OrderCreated) error {
    return emailService.Send(...)  // No commands!
}

// ✅ Workflow logic (saga coordinator)
func (s *OrderSagaCoordinator) OnEvent(ctx, msg) ([]*Message, error) {
    return cqrs.CreateCommands(...)  // Workflow here!
}
```

### 3. Easy Testing

```go
func TestCreateOrderHandler(t *testing.T) {
    // ✅ Test pure business logic
    events, err := handleCreateOrder(ctx, CreateOrder{
        ID: "order-1",
        Amount: 100,
    })

    assert.NoError(t, err)
    assert.Len(t, events, 1)
    assert.Equal(t, "order-1", events[0].ID)
    assert.Equal(t, 100, events[0].Amount)
}
```

### 4. Pluggable

```go
// ✅ Use JSON marshaler
marshaler := cqrs.NewJSONMarshaler()

// ✅ Or create custom marshaler (Protobuf, MessagePack, etc.)
type ProtobufMarshaler struct{}

func (m ProtobufMarshaler) Marshal(v any) ([]byte, error) {
    return proto.Marshal(v.(proto.Message))
}
// ...
```

## Comparison with Other Examples

| Example | Complexity | Coupling | Type Safety | Recommended |
|---------|-----------|----------|-------------|-------------|
| **examples/cqrs** | Medium | ❌ High | ⚠️ Partial | No (initial design) |
| **examples/cqrs-v2** | Medium | ⚠️ Medium | ⚠️ Partial | No (revised but verbose) |
| **examples/cqrs-saga-coordinator** | Medium | ✅ Low | ⚠️ Partial | Yes (shows pattern) |
| **examples/cqrs-package** (this) | ✅ Low | ✅ Low | ✅ High | ✅ **Recommended** |

## When to Use

✅ **Use cqrs package when:**
- Building CQRS/event-driven applications
- Need type-safe handlers
- Want clean separation of concerns
- Multi-step workflows (sagas)
- Idiomatic gopipe patterns

❌ **Don't use when:**
- Very simple single-handler scenarios
- No need for type safety
- Prefer manual control over abstractions

## Advanced Patterns

For advanced patterns like compensations and outbox, see:

- [docs/cqrs-architecture-overview.md](../../docs/cqrs-architecture-overview.md)
- [docs/cqrs-advanced-patterns.md](../../docs/cqrs-advanced-patterns.md)

Future packages:
- `cqrs/compensation` - Automatic rollback on failure
- `cqrs/outbox` - Exactly-once semantics

## References

- [ADR 0006: CQRS Implementation](../../docs/adr/0006-cqrs-implementation.md)
- [CQRS Architecture Overview](../../docs/cqrs-architecture-overview.md)
- [Saga Pattern Comparison](../../docs/cqrs-saga-patterns.md)

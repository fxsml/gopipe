# ADR 0007: Saga Coordinator Pattern

**Date:** 2025-12-08
**Status:** Implemented
**Related:** [ADR 0006: CQRS Implementation](./0006-cqrs-implementation.md)

## Context

In event-driven architectures, complex workflows often require multiple steps coordinated across services. A common pattern is the **Saga Pattern**, which coordinates distributed transactions through a series of events and commands.

### The Coupling Problem

Initial CQRS implementations often tightly couple event handlers with command generation:

```go
// ❌ Problem: Event handler directly returns command types
handleOrderCreated := func(evt OrderCreated) ([]ChargePayment, error) {
    return []ChargePayment{{OrderID: evt.ID, Amount: evt.Amount}}, nil
}
```

**Issues:**
- Event handler is coupled to `ChargePayment` command type
- Hard to test in isolation
- Violates separation of concerns
- Side effects and workflow logic mixed

### The Solution: Saga Coordinator

Separate **workflow logic** from **event side effects**:

- **Event Handlers**: Perform side effects only (email, logging, analytics)
- **Saga Coordinators**: Define workflow logic (what commands to trigger next)

## Decision

Implement the **Saga Coordinator Pattern** as part of the `cqrs` package through the `SagaCoordinator` interface.

### Design

```go
// SagaCoordinator defines workflow logic for saga patterns.
type SagaCoordinator interface {
    // OnEvent handles an event and returns commands for the next saga steps.
    OnEvent(ctx context.Context, msg *message.Message) ([]*message.Message, error)
}
```

### Architecture

```
┌─────────────┐
│   Command   │
└──────┬──────┘
       ↓
┌──────────────────┐
│ Command Handler  │ ← Pure: Command → Events
└──────┬───────────┘
       ↓
┌──────────────┐
│    Event     │
└──────┬───────┘
       ├─────────────────┬─────────────────┐
       ↓                 ↓                 ↓
┌─────────────┐   ┌──────────────┐   ┌─────────────┐
│   Email     │   │    Saga      │   │  Analytics  │
│  Handler    │   │ Coordinator  │   │   Handler   │
│             │   │              │   │             │
│ (Side       │   │ (Workflow)   │   │ (Side       │
│  Effect)    │   │              │   │  Effect)    │
└─────────────┘   └──────┬───────┘   └─────────────┘
                         ↓
                  ┌──────────────┐
                  │   Commands   │ ← Feedback loop
                  └──────────────┘
```

## Implementation

### 1. Pure Event Handlers (Side Effects Only)

```go
// ✅ Event handler: side effects only, no commands
emailHandler := cqrs.NewEventHandler(
    "OrderCreated",
    marshaler,
    func(ctx context.Context, evt OrderCreated) error {
        log.Printf("📧 Sending confirmation email...")
        return emailService.Send(evt.CustomerID, "Order confirmed!")
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

### 2. Saga Coordinator (Workflow Logic)

```go
// ✅ Saga Coordinator: workflow logic, decoupled from event handlers
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

        log.Printf("🔄 Saga: OrderCreated → ChargePayment + ReserveInventory")

        // Workflow decision: what happens next?
        return cqrs.CreateCommands(s.marshaler, corrID,
            ChargePayment{OrderID: evt.ID, Amount: evt.Amount},
            ReserveInventory{OrderID: evt.ID, SKU: "SKU-123"},
        ), nil

    case "PaymentCharged":
        var evt PaymentCharged
        s.marshaler.Unmarshal(msg.Payload, &evt)

        log.Printf("🔄 Saga: PaymentCharged → waiting for inventory...")
        return nil, nil

    case "InventoryReserved":
        var evt InventoryReserved
        s.marshaler.Unmarshal(msg.Payload, &evt)

        log.Printf("🔄 Saga: InventoryReserved → ShipOrder")
        return cqrs.CreateCommands(s.marshaler, corrID,
            ShipOrder{OrderID: evt.OrderID, Address: "123 Main St"},
        ), nil

    case "OrderShipped":
        log.Printf("✅ Saga: Complete!")
        return nil, nil // Terminal event
    }

    return nil, nil
}
```

### 3. Wiring with Feedback Loop

```go
// Command processor
commandRouter := message.NewRouter(
    message.RouterConfig{Concurrency: 10, Recover: true},
    createOrderHandler,
    chargePaymentHandler,
    reserveInventoryHandler,
    shipOrderHandler,
)

// Side effects processor
sideEffectsRouter := message.NewRouter(
    message.RouterConfig{Concurrency: 20, Recover: true},
    emailHandler,
    analyticsHandler,
)

// Saga coordinator processor
sagaCoordinator := &OrderSagaCoordinator{marshaler: marshaler}
sagaHandler := message.NewHandler(
    sagaCoordinator.OnEvent,
    func(prop message.Properties) bool {
        msgType, _ := prop["type"].(string)
        return msgType == "event" // Reacts to ALL events
    },
)
sagaRouter := message.NewRouter(message.RouterConfig{}, sagaHandler)

// Feedback loop: merge initial + saga-triggered commands
initialCommands := make(chan *message.Message, 10)
sagaCommands := make(chan *message.Message, 100)
allCommands := channel.Merge(initialCommands, sagaCommands)

// Commands → Events
events := commandRouter.Start(ctx, allCommands)

// Fan-out events to side effects AND saga coordinator
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

// Feedback: route saga commands back to command processor
go func() {
    for cmd := range sagaOut {
        sagaCommands <- cmd
    }
}()
```

## Benefits

### 1. Separation of Concerns

```go
// ✅ Clear separation
Event → Side Effects (email, analytics, logging)
Event → Workflow Logic (saga coordinator) → Commands
```

### 2. Testability

```go
// ✅ Test side effects in isolation
func TestEmailHandler(t *testing.T) {
    err := handleEmail(ctx, OrderCreated{CustomerID: "customer-123"})
    assert.NoError(t, err)
    assert.True(t, emailService.WasCalled())
}

// ✅ Test workflow logic in isolation
func TestSagaCoordinator(t *testing.T) {
    coordinator := &OrderSagaCoordinator{marshaler: marshaler}
    msg := createEventMessage("OrderCreated", OrderCreated{ID: "order-1"})

    cmds, err := coordinator.OnEvent(ctx, msg)
    assert.NoError(t, err)
    assert.Len(t, cmds, 2) // ChargePayment + ReserveInventory

    assertCommand(t, cmds[0], "ChargePayment")
    assertCommand(t, cmds[1], "ReserveInventory")
}
```

### 3. Flexibility

```go
// ✅ Easy to change workflow without touching side effects
func (s *OrderSagaCoordinator) OnEvent(ctx, msg) ([]*Message, error) {
    switch subject {
    case "OrderCreated":
        // Change: Charge payment FIRST, then reserve inventory
        return cqrs.CreateCommands(s.marshaler, corrID,
            ChargePayment{...},      // Step 1
            // ReserveInventory moved to after payment
        ), nil

    case "PaymentCharged":
        // NEW: Now reserve inventory after payment succeeds
        return cqrs.CreateCommands(s.marshaler, corrID,
            ReserveInventory{...},   // Step 2
        ), nil
    }
}
```

### 4. Multistage Acking

```go
// ✅ One event → multiple commands (independent acking)
case "OrderCreated":
    return cqrs.CreateCommands(s.marshaler, corrID,
        ChargePayment{...},      // Acked independently
        ReserveInventory{...},   // Acked independently
        NotifyWarehouse{...},    // Acked independently
    ), nil
```

## Implementation Status

✅ **Implemented** in `cqrs` package:
- `SagaCoordinator` interface (`cqrs/coordinator.go:10`)
- `CreateCommands()` utility (`cqrs/util.go:49`)
- Complete example (`examples/cqrs-package/main.go`)

## Example Saga Flow

```
Initial Command:
  CreateOrder(id: "order-789", amount: 350)

Step 1: Command → Event
  CreateOrder → OrderCreated

Step 2: Event → Side Effects + Saga
  OrderCreated → Email (side effect)
  OrderCreated → Analytics (side effect)
  OrderCreated → Saga Coordinator → [ChargePayment, ReserveInventory]

Step 3: Commands → Events
  ChargePayment → PaymentCharged
  ReserveInventory → InventoryReserved

Step 4: Events → Saga
  PaymentCharged → Saga Coordinator → wait
  InventoryReserved → Saga Coordinator → ShipOrder

Step 5: Command → Event
  ShipOrder → OrderShipped

Step 6: Event → Side Effects + Saga
  OrderShipped → Email (side effect)
  OrderShipped → Saga Coordinator → nil (terminal)

✅ Saga Complete!
```

## Comparison with Alternatives

### Alternative 1: Direct Command Return (Initial Design)

```go
// ❌ Event handler returns command types directly
handleOrderCreated := func(evt OrderCreated) ([]ChargePayment, error) {
    return []ChargePayment{{...}}, nil
}
```

**Issues:**
- Tight coupling to command types
- Hard to test
- Can't have side effects without commands
- Violates single responsibility

### Alternative 2: Orchestrator Pattern

```go
// Central orchestrator with state machine
type OrderOrchestrator struct {
    state *SagaState
}

func (o *OrderOrchestrator) HandleEvent(evt Event) ([]Command, error) {
    switch o.state.Step {
    case 1: // Create order
        return []Command{ChargePayment{...}}, nil
    case 2: // Payment charged
        return []Command{ReserveInventory{...}}, nil
    }
}
```

**Tradeoffs:**
- ✅ Centralized control
- ✅ Easy to visualize workflow
- ❌ Requires persistent state management
- ❌ More complex to implement
- ❌ Single point of failure

**When to use:** Complex workflows with conditional branching, parallel steps, or need for saga state persistence. See [ADR 0008: Compensating Saga Pattern](./0008-compensating-saga-pattern.md) for stateful orchestration.

### Alternative 3: Process Manager Pattern

Full stateful process manager with event sourcing.

**Tradeoffs:**
- ✅ Complete audit trail
- ✅ Supports complex compensation
- ❌ Very complex
- ❌ Requires event store
- ❌ Overkill for most use cases

**When to use:** Mission-critical workflows requiring full audit trail and complex compensation logic.

## Decision Summary

**Use Saga Coordinator Pattern for:**
- ✅ Multi-step workflows (event choreography)
- ✅ Decoupled workflow logic from side effects
- ✅ Simple to medium complexity sagas
- ✅ When you don't need saga state persistence

**Consider Orchestrator/Process Manager for:**
- Complex workflows with branching
- Need for saga state persistence
- Compensation/rollback requirements
- See [ADR 0008](./0008-compensating-saga-pattern.md)

## References

- [ADR 0006: CQRS Implementation](./0006-cqrs-implementation.md)
- [ADR 0008: Compensating Saga Pattern](./0008-compensating-saga-pattern.md) (proposal)
- [Saga Pattern Comparison](../cqrs-saga-patterns.md)
- [CQRS Architecture Overview](../cqrs-architecture-overview.md)
- [Example: cqrs-package](../../examples/cqrs-package/)
- [Saga Pattern - Microservices.io](https://microservices.io/patterns/data/saga.html)
- [Event Choreography vs Orchestration](https://www.tenupsoft.com/blog/The-importance-of-cqrs-and-saga-in-microservices-architecture.html)

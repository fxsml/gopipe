# Saga Pattern Overview

This document provides a high-level overview of the Saga pattern and how to implement sagas in gopipe.

## What is a Saga?

A **Saga** is a pattern for managing distributed transactions by breaking them into a series of local transactions coordinated through events.

### The Problem

In microservices, traditional ACID transactions don't work across service boundaries:

```
❌ Can't do this across services:
BEGIN TRANSACTION
  INSERT INTO orders (service A)
  INSERT INTO inventory (service B)
  INSERT INTO payments (service C)
COMMIT
```

### The Solution: Sagas

Break the transaction into steps, each with its own local transaction:

```
✅ Saga approach:
Step 1: CreateOrder → OrderCreated
Step 2: ChargePayment → PaymentCharged
Step 3: ReserveInventory → InventoryReserved
Step 4: ShipOrder → OrderShipped
```

If a step fails, **compensate** (undo) previous steps.

## Saga Types

### 1. Event Choreography (Recommended)

**What:** Services react to events autonomously

```
Service A: CreateOrder → OrderCreated
    ↓
Service B: (listens to OrderCreated) → ChargePayment → PaymentCharged
    ↓
Service C: (listens to PaymentCharged) → ShipOrder → OrderShipped
```

**Pros:**
- ✅ Loose coupling
- ✅ Easy to add new steps
- ✅ No single point of failure

**Cons:**
- ⚠️ Harder to visualize overall flow
- ⚠️ Need to track saga state for compensation

**gopipe Implementation:** [ADR 0007: Saga Coordinator Pattern](./adr/0007-saga-coordinator-pattern.md)

### 2. Orchestration

**What:** Central coordinator controls the flow

```
Saga Orchestrator:
  1. CreateOrder
  2. Wait for OrderCreated
  3. ChargePayment
  4. Wait for PaymentCharged
  5. ShipOrder
  6. Wait for OrderShipped
  ✅ Done
```

**Pros:**
- ✅ Easy to visualize flow
- ✅ Centralized logic
- ✅ Simple compensation

**Cons:**
- ❌ Tight coupling
- ❌ Orchestrator is bottleneck
- ❌ Single point of failure

**When to use:** Complex workflows with branching logic, or when you need centralized control.

## gopipe Saga Implementation

gopipe uses the **Event Choreography** pattern through the `SagaCoordinator` interface.

### Basic Saga Coordinator

```go
type OrderSagaCoordinator struct {
    marshaler cqrs.Marshaler
}

func (s *OrderSagaCoordinator) OnEvent(
    ctx context.Context,
    msg *message.Message,
) ([]*message.Message, error) {
    subject, _ := msg.Attributes.Subject()
    corrID, _ := msg.Attributes.CorrelationID()

    switch subject {
    case "OrderCreated":
        var evt OrderCreated
        s.marshaler.Unmarshal(msg.Data, &evt)

        // Workflow: What commands to trigger?
        return createCommands(s.marshaler, corrID,
            ChargePayment{OrderID: evt.ID, Amount: evt.Amount},
            ReserveInventory{OrderID: evt.ID},
        ), nil

    case "PaymentCharged":
        // Wait for inventory...
        return nil, nil

    case "InventoryReserved":
        var evt InventoryReserved
        s.marshaler.Unmarshal(msg.Data, &evt)

        return createCommands(s.marshaler, corrID,
            ShipOrder{OrderID: evt.OrderID},
        ), nil

    case "OrderShipped":
        // Terminal - saga complete!
        return nil, nil
    }

    return nil, nil
}

// createCommands is a helper to create command messages
func createCommands(m cqrs.Marshaler, corrID string, cmds ...any) []*message.Message {
    // Implementation creates messages with proper attributes
    // See examples/cqrs-package for full implementation
    return nil
}
```

### Wiring with Feedback Loop

```go
// Create saga handler
sagaCoordinator := &OrderSagaCoordinator{marshaler: marshaler}
sagaHandler := message.NewHandler(
    sagaCoordinator.OnEvent,
    func(prop message.Attributes) bool {
        msgType, _ := prop["type"].(string)
        return msgType == "event"
    },
)

sagaRouter := message.NewRouter(message.RouterConfig{}, sagaHandler)

// Feedback loop: events → saga → commands → events
initialCommands := make(chan *message.Message, 10)
sagaCommands := make(chan *message.Message, 100)
allCommands := channel.Merge(initialCommands, sagaCommands)

events := commandRouter.Start(ctx, allCommands)
sagaOut := sagaRouter.Start(ctx, events)

go func() {
    for cmd := range sagaOut {
        sagaCommands <- cmd  // Feed back to command processor
    }
}()
```

## Saga Patterns in gopipe

### Pattern 1: Basic Saga (No Compensation)

**Status:** ✅ Implemented ([ADR 0007](./adr/0007-saga-coordinator-pattern.md))

**Use when:**
- Happy path is sufficient
- Failures are rare
- Manual compensation is acceptable

**Example Flow:**
```
CreateOrder → OrderCreated → ChargePayment → PaymentCharged → ShipOrder → OrderShipped
```

**Implementation:**
- `SagaCoordinator` interface
- Event → Commands workflow logic
- Correlation IDs for tracking

### Pattern 2: Compensating Saga

**Status:** 📋 Proposed ([ADR 0008](./adr/0008-compensating-saga-pattern.md))

**Use when:**
- Need automatic rollback on failure
- Critical workflows (payments, orders)
- Can't tolerate inconsistency

**Example Flow:**
```
Step 1: CreateOrder → OrderCreated [compensation: CancelOrder]
Step 2: ChargePayment → PaymentCharged [compensation: RefundPayment]
Step 3: ReserveInventory → ❌ FAILED!
    ↓
Compensate (LIFO):
    RefundPayment ✅
    CancelOrder ✅
```

**Implementation:**
- `CompensatingSagaCoordinator` interface
- `SagaStore` for state persistence
- LIFO compensation execution
- Package: `cqrs/compensation` (not yet implemented)

## Compensation Strategies

### Forward Recovery

**What:** Retry failed step until it succeeds

```
ReserveInventory → ❌ Failed
    ↓
Retry with backoff
    ↓
ReserveInventory → ✅ Succeeded
```

**Use when:**
- Failure is transient (network issues)
- Operation is idempotent
- Eventually will succeed

### Backward Recovery

**What:** Undo previous steps (compensation)

```
ChargePayment → PaymentCharged
ReserveInventory → ❌ Failed
    ↓
Compensate:
    RefundPayment ✅
    CancelOrder ✅
```

**Use when:**
- Failure is permanent
- Can't retry indefinitely
- Need to maintain consistency

## Saga State Management

### Stateless Saga (Basic)

**What:** No persistent state, relies on correlation IDs

**Pros:**
- ✅ Simple
- ✅ No database required
- ✅ Sufficient for most cases

**Cons:**
- ❌ Can't query saga status
- ❌ Can't recover from crashes
- ❌ Limited compensation support

**gopipe Implementation:** Basic `SagaCoordinator` ([ADR 0007](./adr/0007-saga-coordinator-pattern.md))

### Stateful Saga (Compensation)

**What:** Persistent state for tracking and compensation

**Pros:**
- ✅ Query saga status
- ✅ Automatic compensation
- ✅ Crash recovery
- ✅ Audit trail

**Cons:**
- ⚠️ More complex
- ⚠️ Requires database
- ⚠️ Higher overhead

**gopipe Implementation:** `SagaStore` interface ([ADR 0008](./adr/0008-compensating-saga-pattern.md), proposed)

## Example: Order Processing Saga

### Happy Path

```
1. CreateOrder
   ↓
2. OrderCreated
   ↓ (saga coordinator)
3. ChargePayment + ReserveInventory (parallel)
   ↓
4. PaymentCharged + InventoryReserved
   ↓ (saga coordinator)
5. ShipOrder
   ↓
6. OrderShipped
   ✅ Saga complete!
```

### Failure Path (with Compensation)

```
1. CreateOrder
   ↓
2. OrderCreated
   [Store compensation: CancelOrder]
   ↓
3. ChargePayment
   ↓
4. PaymentCharged
   [Store compensation: RefundPayment]
   ↓
5. ReserveInventory
   ↓
6. ❌ InventoryReservationFailed
   ↓
7. Execute compensations (LIFO):
   a. RefundPayment → PaymentRefunded ✅
   b. CancelOrder → OrderCancelled ✅
   ↓
8. Saga compensated gracefully
```

## Best Practices

### 1. Use Correlation IDs

✅ **Always track sagas with correlation IDs:**

```go
data, _ := marshaler.Marshal(CreateOrder{...})
msg := message.New(data, message.Attributes{
    message.AttrCorrelationID: "saga-order-12345",
    message.AttrSubject: "CreateOrder",
    message.AttrType: "CreateOrder",
})
```

### 2. Idempotent Operations

✅ **Make all operations idempotent:**

```go
func handleChargePayment(ctx, cmd ChargePayment) ([]PaymentCharged, error) {
    // Check if already charged
    if payment, _ := db.GetPayment(cmd.OrderID); payment != nil {
        return []PaymentCharged{{...}}, nil // Already done
    }

    // Charge payment
    // ...
}
```

### 3. Separate Workflow from Side Effects

✅ **Saga coordinator = workflow logic:**
```go
func (s *OrderSagaCoordinator) OnEvent(ctx, msg) ([]*Message, error) {
    // ✅ Workflow: what happens next?
    return createCommands(s.marshaler, corrID, NextCommand{...}), nil
}
```

✅ **Event handlers = side effects:**
```go
func handleOrderCreated(ctx, evt OrderCreated) error {
    // ✅ Side effects: email, logging, analytics
    return emailService.Send(...)
}
```

### 4. Terminal Events

✅ **Clearly mark terminal events:**

```go
switch subject {
case "OrderShipped":
    log.Printf("✅ Saga complete!")
    return nil, nil  // Terminal - no more commands
}
```

### 5. Error Handling

✅ **Handle errors appropriately:**

```go
case "PaymentFailed":
    // Option 1: Retry
    return createCommands(s.marshaler, corrID,
        ChargePayment{...},  // Retry
    ), nil

    // Option 2: Compensate
    compensations, _ := s.store.GetCompensations(ctx, sagaID)
    return compensations, nil
```

## When to Use Sagas

### Use Sagas When:

✅ Distributed transactions across services
✅ Long-running workflows
✅ Need eventual consistency
✅ Can't use 2PC (two-phase commit)
✅ Services are independent

### Don't Use Sagas When:

❌ Single database (use DB transactions)
❌ Need strong consistency (ACID)
❌ Operations can't be compensated
❌ Workflow is simple (one step)

## Saga vs Alternatives

### Saga vs Two-Phase Commit (2PC)

| Aspect | Saga | 2PC |
|--------|------|-----|
| **Consistency** | Eventual | Immediate (ACID) |
| **Availability** | High | Low (blocking) |
| **Complexity** | Medium | High |
| **Failure handling** | Compensation | Automatic rollback |
| **Use case** | Microservices | Monolith/distributed DB |

### Saga vs Database Transaction

| Aspect | Saga | DB Transaction |
|--------|------|----------------|
| **Scope** | Cross-service | Single database |
| **Duration** | Long-running | Short-lived |
| **Consistency** | Eventual | Immediate |
| **Complexity** | Higher | Lower |

## Common Pitfalls

### ❌ Pitfall 1: Coupling in Event Handlers

**Problem:**
```go
// ❌ Event handler returning commands
func handleOrderCreated(evt OrderCreated) ([]ChargePayment, error) {
    return []ChargePayment{{...}}, nil  // Tight coupling!
}
```

**Solution:**
```go
// ✅ Separate workflow (saga coordinator) from side effects (event handler)
// Saga coordinator handles workflow
func (s *Saga) OnEvent(...) ([]ChargePayment, error) { ... }

// Event handler handles side effects
func handleOrderCreated(evt OrderCreated) error {
    return sendEmail(...)  // Pure side effect
}
```

### ❌ Pitfall 2: Missing Correlation IDs

**Problem:**
```go
// ❌ Can't track saga end-to-end
data, _ := marshaler.Marshal(CreateOrder{...})
msg := message.New(data, nil)
```

**Solution:**
```go
// ✅ Always use correlation IDs
data, _ := marshaler.Marshal(CreateOrder{...})
msg := message.New(data, message.Attributes{
    message.AttrCorrelationID: uuid.New().String(),
    message.AttrSubject: "CreateOrder",
    message.AttrType: "CreateOrder",
})
```

### ❌ Pitfall 3: Non-Idempotent Operations

**Problem:**
```go
// ❌ Duplicate messages cause double-charging
func handleChargePayment(cmd ChargePayment) error {
    return stripeAPI.Charge(cmd.Amount)  // ❌ Not idempotent!
}
```

**Solution:**
```go
// ✅ Check for duplicates
func handleChargePayment(cmd ChargePayment) error {
    if exists, _ := db.PaymentExists(cmd.OrderID); exists {
        return nil  // Already processed
    }
    return stripeAPI.Charge(cmd.Amount, idempotencyKey)
}
```

### ❌ Pitfall 4: Lost Messages

**Problem:** Messages lost if broker crashes

**Solution:** Use [Transactional Outbox Pattern](./outbox-overview.md) for exactly-once delivery

## Related Documentation

### Architecture Decision Records

- [ADR 0006: CQRS Implementation](./adr/0006-cqrs-implementation.md) ✅ Implemented
- [ADR 0007: Saga Coordinator Pattern](./adr/0007-saga-coordinator-pattern.md) ✅ Implemented
- [ADR 0008: Compensating Saga Pattern](./adr/0008-compensating-saga-pattern.md) 📋 Proposed
- [ADR 0009: Transactional Outbox Pattern](./adr/0009-transactional-outbox-pattern.md) 📋 Proposed

### Pattern Guides

- [CQRS Overview](./cqrs-overview.md)
- [Outbox Pattern Overview](./outbox-overview.md)
- [Saga Pattern Comparison](./cqrs-saga-patterns.md)
- [CQRS Architecture Overview](./cqrs-architecture-overview.md)

### Examples

- [examples/cqrs-package](../examples/cqrs-package/) - Complete saga example
- [examples/cqrs-saga-coordinator](../examples/cqrs-saga-coordinator/) - Saga coordinator pattern

### External Resources

- [Saga Pattern - Microservices.io](https://microservices.io/patterns/data/saga.html)
- [Compensating Transaction Pattern - Microsoft](https://learn.microsoft.com/en-us/azure/architecture/patterns/compensating-transaction)
- [Saga Orchestration vs Choreography](https://temporal.io/blog/saga-pattern-made-easy)
- [CQRS and Saga in Microservices](https://medium.com/@ingila185/cqrs-and-saga-the-essential-patterns-for-high-performance-microservice-4f23a09889b4)

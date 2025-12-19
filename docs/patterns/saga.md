# Pattern: Saga

## Intent

Coordinate distributed transactions across multiple services using a sequence of local transactions, with compensating actions for rollback.

## Structure

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Saga Pattern                                 │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    Saga Coordinator                           │   │
│  └──────────────────────────────────────────────────────────────┘   │
│           │              │              │              │             │
│           ▼              ▼              ▼              ▼             │
│      ┌────────┐    ┌────────┐    ┌────────┐    ┌────────┐          │
│      │ Step 1 │───►│ Step 2 │───►│ Step 3 │───►│Complete│          │
│      │        │    │        │    │        │    │        │          │
│      └────────┘    └────────┘    └────────┘    └────────┘          │
│           │              │              │                           │
│           ▼              ▼              ▼                           │
│      ┌────────┐    ┌────────┐    ┌────────┐                        │
│      │Compen- │◄───│Compen- │◄───│Compen- │  ← On failure          │
│      │sate 1  │    │sate 2  │    │sate 3  │                        │
│      └────────┘    └────────┘    └────────┘                        │
└─────────────────────────────────────────────────────────────────────┘
```

## Saga Types

### Choreography

Each service publishes events that trigger the next service.

```
OrderService → OrderCreated → InventoryService
                                    │
                          InventoryReserved
                                    │
                                    ▼
                            PaymentService
                                    │
                            PaymentProcessed
                                    │
                                    ▼
                            ShippingService
```

**Pros:** Loose coupling, simple for small workflows
**Cons:** Hard to track, difficult to understand flow

### Orchestration (Recommended)

Central coordinator manages the saga flow.

```
┌─────────────────────────────┐
│      Saga Coordinator       │
│                             │
│  State: RESERVING_INVENTORY │
└─────────────────────────────┘
        │           ▲
        │ Command   │ Event
        ▼           │
   ┌─────────────────────┐
   │  InventoryService   │
   └─────────────────────┘
```

**Pros:** Clear flow, easy to monitor, centralized error handling
**Cons:** Coordinator can become bottleneck

## When to Use

**Good fit:**
- Multi-service business transactions
- Long-running workflows
- Eventual consistency acceptable
- Need compensating actions for rollback

**Poor fit:**
- Single-service operations
- Strong consistency required
- Simple workflows

## Implementation in goengine

### Saga Definition

```go
// Define saga steps
type OrderSaga struct {
    state    SagaState
    orderID  string
    steps    []SagaStep
}

type SagaStep struct {
    Name       string
    Execute    func(ctx context.Context) error
    Compensate func(ctx context.Context) error
}

func NewOrderSaga(orderID string) *OrderSaga {
    return &OrderSaga{
        orderID: orderID,
        steps: []SagaStep{
            {
                Name:       "reserve_inventory",
                Execute:    func(ctx) error { return reserveInventory(orderID) },
                Compensate: func(ctx) error { return releaseInventory(orderID) },
            },
            {
                Name:       "process_payment",
                Execute:    func(ctx) error { return processPayment(orderID) },
                Compensate: func(ctx) error { return refundPayment(orderID) },
            },
            {
                Name:       "ship_order",
                Execute:    func(ctx) error { return shipOrder(orderID) },
                Compensate: func(ctx) error { return cancelShipment(orderID) },
            },
        },
    }
}
```

### Saga Coordinator

```go
// Coordinator manages saga execution
type SagaCoordinator struct {
    store SagaStore
}

func (c *SagaCoordinator) Execute(ctx context.Context, saga *OrderSaga) error {
    for i, step := range saga.steps {
        saga.state = SagaState{Step: i, Status: "executing"}
        c.store.Save(saga)

        if err := step.Execute(ctx); err != nil {
            // Compensate all completed steps in reverse
            return c.compensate(ctx, saga, i-1)
        }

        saga.state = SagaState{Step: i, Status: "completed"}
        c.store.Save(saga)
    }
    return nil
}

func (c *SagaCoordinator) compensate(ctx context.Context, saga *OrderSaga, fromStep int) error {
    for i := fromStep; i >= 0; i-- {
        if err := saga.steps[i].Compensate(ctx); err != nil {
            // Log but continue compensating
            log.Error("compensation failed", "step", i, "error", err)
        }
    }
    return ErrSagaCompensated
}
```

### Event-Driven Saga

```go
// Handle saga events
sagaHandler := NewEventHandler(func(ctx context.Context, evt InventoryReserved) ([]*Message, error) {
    saga := loadSaga(evt.SagaID)

    // Advance to next step
    saga.MarkStepComplete("reserve_inventory")

    // Emit next command
    return []*Message{
        NewMessage(ProcessPayment{
            SagaID:  evt.SagaID,
            OrderID: evt.OrderID,
        }),
    }, nil
})
```

## Saga States

| State | Description |
|-------|-------------|
| `STARTED` | Saga initiated |
| `STEP_N_EXECUTING` | Executing step N |
| `STEP_N_COMPLETED` | Step N completed |
| `COMPENSATING` | Rolling back |
| `COMPENSATED` | Rollback complete |
| `COMPLETED` | All steps successful |
| `FAILED` | Unrecoverable failure |

## Related Patterns

- [CQRS](cqrs.md) - Often used together
- [Outbox](outbox.md) - Reliable step execution
- Event Sourcing - Track saga history

## Related ADRs

- [ADR 0007: Saga Coordinator Pattern](../goengine/adr/PRO-0007-saga-coordinator.md)
- [ADR 0008: Compensating Saga Pattern](../goengine/adr/PRO-0008-compensating-saga.md)

## Further Reading

- Chris Richardson: [Saga Pattern](https://microservices.io/patterns/data/saga.html)
- Caitie McCaffrey: [Applying the Saga Pattern](https://www.youtube.com/watch?v=xDuwrtwYHu8)

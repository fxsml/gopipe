# ADR 0008: Compensating Saga Pattern

**Date:** 2025-12-08
**Status:** Proposed
**Related:**
- [ADR 0006: CQRS Implementation](./0006-cqrs-implementation.md)
- [ADR 0007: Saga Coordinator Pattern](./0007-saga-coordinator-pattern.md)

## Context

While the basic [Saga Coordinator Pattern](./0007-saga-coordinator-pattern.md) handles happy-path workflows well, real-world distributed systems must handle failures gracefully.

### The Problem: Saga Failures

Consider an order processing saga:

```
CreateOrder → OrderCreated → ChargePayment → PaymentCharged
                          → ReserveInventory → ❌ FAILED!
```

**What happens now?**
- Payment was already charged
- Inventory reservation failed
- Order is in inconsistent state
- Need to **undo** previous steps

### Solution: Compensating Transactions

A **compensating saga** records compensation actions for each step, allowing automatic rollback on failure.

```
CreateOrder → OrderCreated
    [compensation: CancelOrder stored]
        ↓
ChargePayment → PaymentCharged
    [compensation: RefundPayment stored]
        ↓
ReserveInventory → ❌ FAILED!
        ↓
Execute compensations (LIFO):
    1. RefundPayment ✅
    2. CancelOrder ✅
        ↓
    Graceful failure with cleanup!
```

## Decision

Implement the **Compensating Saga Pattern** as an optional, pluggable package: `cqrs/compensation`.

### Design Principles

1. **Optional** - Not required for basic CQRS/saga usage
2. **Pluggable** - Can be added to existing sagas incrementally
3. **Storage-agnostic** - `SagaStore` interface supports any backend
4. **LIFO rollback** - Last-in, first-out compensation execution
5. **Idempotent** - Compensations can be safely retried

## Proposed Architecture

### 1. Core Interface: CompensatingSagaCoordinator

```go
package compensation

// CompensatingSagaCoordinator extends SagaCoordinator with compensation logic.
type CompensatingSagaCoordinator interface {
    // OnEventWithCompensation processes an event and returns the next saga step
    // including compensation commands.
    OnEventWithCompensation(ctx context.Context, msg *message.Message) (SagaStep, error)
}

// SagaStep represents a step in a saga with compensation.
type SagaStep struct {
    // Commands to execute for forward progress
    Commands []*message.Message

    // Compensations to execute if later steps fail
    // These are stored and executed in LIFO order on failure
    Compensations []*message.Message

    // IsTerminal indicates this is the final step (no more commands)
    IsTerminal bool

    // MarkComplete indicates this saga instance should be marked complete
    // after this step (for cleanup)
    MarkComplete bool
}
```

### 2. Saga Store Interface

```go
// SagaStore persists saga state and compensation commands.
type SagaStore interface {
    // StartSaga creates a new saga instance.
    StartSaga(ctx context.Context, sagaID string, sagaType string) error

    // RecordStep records a saga step with its compensations.
    RecordStep(ctx context.Context, sagaID string, step SagaStepRecord) error

    // GetCompensations retrieves all compensations for a saga in LIFO order.
    GetCompensations(ctx context.Context, sagaID string) ([]*message.Message, error)

    // MarkComplete marks a saga as successfully completed.
    MarkComplete(ctx context.Context, sagaID string) error

    // MarkFailed marks a saga as failed and records the error.
    MarkFailed(ctx context.Context, sagaID string, err error) error
}

// SagaStepRecord represents a recorded saga step.
type SagaStepRecord struct {
    StepNumber    int
    EventName     string
    EventPayload  []byte
    Compensations []*message.Message
    Timestamp     time.Time
}
```

### 3. In-Memory Implementation (for testing/simple cases)

```go
// InMemorySagaStore provides an in-memory saga store for testing.
type InMemorySagaStore struct {
    mu    sync.RWMutex
    sagas map[string]*SagaInstance
}

type SagaInstance struct {
    ID            string
    Type          string
    Steps         []SagaStepRecord
    Status        SagaStatus
    Error         error
    CreatedAt     time.Time
    CompletedAt   *time.Time
}

type SagaStatus string

const (
    SagaStatusActive    SagaStatus = "active"
    SagaStatusCompleted SagaStatus = "completed"
    SagaStatusFailed    SagaStatus = "failed"
)
```

### 4. PostgreSQL Implementation (production use)

```sql
CREATE TABLE saga_instances (
    id UUID PRIMARY KEY,
    type VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE saga_steps (
    id UUID PRIMARY KEY,
    saga_id UUID NOT NULL REFERENCES saga_instances(id),
    step_number INT NOT NULL,
    event_name VARCHAR(255) NOT NULL,
    event_payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(saga_id, step_number)
);

CREATE TABLE saga_compensations (
    id UUID PRIMARY KEY,
    saga_id UUID NOT NULL REFERENCES saga_instances(id),
    step_number INT NOT NULL,
    command_name VARCHAR(255) NOT NULL,
    command_payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_saga_instances_status ON saga_instances(status);
CREATE INDEX idx_saga_steps_saga_id ON saga_steps(saga_id);
CREATE INDEX idx_saga_compensations_saga_id ON saga_compensations(saga_id);
```

## Implementation Example

### 1. Compensating Saga Coordinator

```go
type OrderCompensatingSagaCoordinator struct {
    marshaler cqrs.Marshaler
    store     compensation.SagaStore
}

func (s *OrderCompensatingSagaCoordinator) OnEventWithCompensation(
    ctx context.Context,
    msg *message.Message,
) (compensation.SagaStep, error) {
    subject, _ := msg.Properties.Subject()
    corrID, _ := msg.Properties.CorrelationID()

    // Use correlation ID as saga ID for tracking
    sagaID := corrID

    switch subject {
    case "OrderCreated":
        var evt OrderCreated
        s.marshaler.Unmarshal(msg.Payload, &evt)

        // Start saga
        s.store.StartSaga(ctx, sagaID, "OrderSaga")

        // Forward: Charge payment and reserve inventory
        commands := cqrs.CreateCommands(s.marshaler, corrID,
            ChargePayment{OrderID: evt.ID, Amount: evt.Amount},
            ReserveInventory{OrderID: evt.ID, SKU: "SKU-123"},
        )

        // Compensation: Cancel order if later steps fail
        compensations := cqrs.CreateCommands(s.marshaler, corrID,
            CancelOrder{OrderID: evt.ID, Reason: "Saga failed"},
        )

        return compensation.SagaStep{
            Commands:      commands,
            Compensations: compensations,
            IsTerminal:    false,
        }, nil

    case "PaymentCharged":
        var evt PaymentCharged
        s.marshaler.Unmarshal(msg.Payload, &evt)

        // No forward commands (waiting for inventory)
        // But record compensation
        compensations := cqrs.CreateCommands(s.marshaler, corrID,
            RefundPayment{OrderID: evt.OrderID, Amount: evt.Amount},
        )

        return compensation.SagaStep{
            Commands:      nil,
            Compensations: compensations,
            IsTerminal:    false,
        }, nil

    case "InventoryReserved":
        var evt InventoryReserved
        s.marshaler.Unmarshal(msg.Payload, &evt)

        // Forward: Ship order
        commands := cqrs.CreateCommands(s.marshaler, corrID,
            ShipOrder{OrderID: evt.OrderID, Address: "123 Main St"},
        )

        // Compensation: Release inventory
        compensations := cqrs.CreateCommands(s.marshaler, corrID,
            ReleaseInventory{OrderID: evt.OrderID},
        )

        return compensation.SagaStep{
            Commands:      commands,
            Compensations: compensations,
            IsTerminal:    false,
        }, nil

    case "OrderShipped":
        // Terminal: Saga complete!
        return compensation.SagaStep{
            Commands:     nil,
            IsTerminal:   true,
            MarkComplete: true,
        }, nil

    case "PaymentFailed", "InventoryReservationFailed":
        // Trigger compensations
        log.Printf("❌ Saga failed: %s", subject)
        compensations, _ := s.store.GetCompensations(ctx, sagaID)

        // Mark saga as failed
        s.store.MarkFailed(ctx, sagaID, fmt.Errorf("step failed: %s", subject))

        return compensation.SagaStep{
            Commands:   compensations, // Execute compensations
            IsTerminal: true,
        }, nil
    }

    return compensation.SagaStep{}, nil
}
```

### 2. Compensation Processor

```go
// CompensationProcessor wraps a CompensatingSagaCoordinator and handles
// saga state management automatically.
type CompensationProcessor struct {
    coordinator CompensatingSagaCoordinator
    store       SagaStore
    marshaler   cqrs.Marshaler
}

func NewCompensationProcessor(
    coordinator CompensatingSagaCoordinator,
    store SagaStore,
    marshaler cqrs.Marshaler,
) *CompensationProcessor {
    return &CompensationProcessor{
        coordinator: coordinator,
        store:       store,
        marshaler:   marshaler,
    }
}

// OnEvent implements message.Handler process function.
func (p *CompensationProcessor) OnEvent(
    ctx context.Context,
    msg *message.Message,
) ([]*message.Message, error) {
    // Get saga step from coordinator
    step, err := p.coordinator.OnEventWithCompensation(ctx, msg)
    if err != nil {
        msg.Nack(err)
        return nil, err
    }

    // Extract saga ID from correlation ID
    corrID, _ := msg.Properties.CorrelationID()
    sagaID := corrID

    // Record step with compensations
    if !step.IsTerminal && len(step.Compensations) > 0 {
        subject, _ := msg.Properties.Subject()
        stepRecord := SagaStepRecord{
            EventName:     subject,
            EventPayload:  msg.Payload,
            Compensations: step.Compensations,
            Timestamp:     time.Now(),
        }

        if err := p.store.RecordStep(ctx, sagaID, stepRecord); err != nil {
            msg.Nack(err)
            return nil, err
        }
    }

    // Mark complete if requested
    if step.MarkComplete {
        if err := p.store.MarkComplete(ctx, sagaID); err != nil {
            log.Printf("Warning: Failed to mark saga complete: %v", err)
        }
    }

    msg.Ack()
    return step.Commands, nil
}
```

## Usage Example

```go
marshaler := cqrs.NewJSONMarshaler()

// Create saga store (in-memory for demo, PostgreSQL for production)
sagaStore := compensation.NewInMemorySagaStore()

// Create compensating saga coordinator
coordinator := &OrderCompensatingSagaCoordinator{
    marshaler: marshaler,
    store:     sagaStore,
}

// Wrap in compensation processor
compensationProcessor := compensation.NewCompensationProcessor(
    coordinator,
    sagaStore,
    marshaler,
)

// Create handler
sagaHandler := message.NewHandler(
    compensationProcessor.OnEvent,
    func(prop message.Properties) bool {
        msgType, _ := prop["type"].(string)
        return msgType == "event"
    },
)

// Wire into router
sagaRouter := message.NewRouter(message.RouterConfig{}, sagaHandler)

// Use normally - compensations handled automatically!
sagaOut := sagaRouter.Start(ctx, events)
```

## Benefits

### 1. Automatic Rollback

```
Step 1: CreateOrder → CancelOrder (stored)
Step 2: ChargePayment → RefundPayment (stored)
Step 3: ReserveInventory → ❌ FAILED!
        ↓
Automatic execution:
    1. RefundPayment ✅
    2. CancelOrder ✅
```

### 2. Saga State Visibility

```go
// Query saga status
instance, _ := sagaStore.GetSaga(ctx, sagaID)
fmt.Printf("Saga status: %s\n", instance.Status)
fmt.Printf("Steps completed: %d\n", len(instance.Steps))

if instance.Status == compensation.SagaStatusFailed {
    fmt.Printf("Error: %s\n", instance.Error)
}
```

### 3. Saga Recovery

```go
// Find all failed sagas and retry compensations
failedSagas, _ := sagaStore.FindByStatus(ctx, compensation.SagaStatusFailed)

for _, saga := range failedSagas {
    compensations, _ := sagaStore.GetCompensations(ctx, saga.ID)

    for _, comp := range compensations {
        // Retry compensation
        commandProcessor.Send(ctx, comp)
    }
}
```

## Implementation Phases

### Phase 1: Core Interfaces (Priority: Medium)

- [ ] `CompensatingSagaCoordinator` interface
- [ ] `SagaStore` interface
- [ ] `SagaStep` struct
- [ ] `CompensationProcessor` wrapper

### Phase 2: Storage Implementations (Priority: Medium)

- [ ] `InMemorySagaStore` (for testing)
- [ ] `PostgresSagaStore` (for production)
- [ ] Saga cleanup/archival utilities

### Phase 3: Advanced Features (Priority: Low)

- [ ] Saga visualization/monitoring
- [ ] Automatic retry policies
- [ ] Compensation timeout handling
- [ ] Saga versioning

## When to Use Compensating Sagas

### Use When:

✅ Multi-step distributed transactions
✅ Need to undo previous steps on failure
✅ Can't use database transactions (cross-service)
✅ Need audit trail of saga execution
✅ Critical business workflows (orders, payments)

### Don't Use When:

❌ Simple workflows (basic saga coordinator sufficient)
❌ All operations in single database (use DB transactions)
❌ Compensations are complex/impossible to implement
❌ Eventually consistent is acceptable

## Alternatives

### Alternative 1: Basic Saga Coordinator (Implemented)

See [ADR 0007: Saga Coordinator Pattern](./0007-saga-coordinator-pattern.md)

**Pros:**
- ✅ Simple
- ✅ No state management
- ✅ Sufficient for 90% of cases

**Cons:**
- ❌ No automatic rollback
- ❌ No saga state tracking

### Alternative 2: Two-Phase Commit (2PC)

Traditional distributed transaction protocol.

**Pros:**
- ✅ ACID guarantees
- ✅ Automatic rollback

**Cons:**
- ❌ Blocking protocol (poor availability)
- ❌ Single coordinator is bottleneck
- ❌ Not suitable for microservices

### Alternative 3: Transactional Outbox + Idempotency

Use transactional outbox ([ADR 0009](./0009-transactional-outbox-pattern.md)) with idempotent operations.

**Pros:**
- ✅ Exactly-once message delivery
- ✅ Simpler than compensations

**Cons:**
- ❌ Requires database per service
- ❌ Doesn't handle all failure scenarios
- ❌ Still need compensations for business logic failures

## Migration Path

Existing basic sagas can be incrementally migrated:

**Step 1:** Add `SagaStore` without changing logic
```go
// Start tracking saga state
sagaStore.StartSaga(ctx, sagaID, "OrderSaga")
```

**Step 2:** Add compensation commands
```go
// Record compensations but don't execute yet
sagaStore.RecordStep(ctx, sagaID, step)
```

**Step 3:** Add failure handling
```go
case "PaymentFailed":
    compensations, _ := sagaStore.GetCompensations(ctx, sagaID)
    return compensation.SagaStep{Commands: compensations}
```

## References

- [ADR 0006: CQRS Implementation](./0006-cqrs-implementation.md)
- [ADR 0007: Saga Coordinator Pattern](./0007-saga-coordinator-pattern.md)
- [ADR 0009: Transactional Outbox Pattern](./0009-transactional-outbox-pattern.md)
- [Advanced Patterns Documentation](../cqrs-advanced-patterns.md)
- [Saga Pattern - Microservices.io](https://microservices.io/patterns/data/saga.html)
- [Compensating Transaction Pattern - Microsoft](https://learn.microsoft.com/en-us/azure/architecture/patterns/compensating-transaction)

# Advanced Saga Patterns: Compensations and Outbox

## Design Philosophy

**gopipe principle:** Simple by default, advanced mechanisms pluggable

```
┌─────────────────────────────────────────────────────────────┐
│  Level 0: Basic CQRS (What we have now)                     │
│  ✅ Commands → Events → Side effects                        │
│  ✅ Independent acking                                       │
│  ✅ Correlation IDs                                          │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  Level 1: Saga Coordinator (Current implementation)         │
│  ✅ Event → Command mapping                                 │
│  ✅ Multi-step workflows                                     │
│  ✅ Decoupled from event handlers                           │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  Level 2: Compensating Saga (OPTIONAL, pluggable)          │
│  ⚙️  Compensation logic for rollback                        │
│  ⚙️  Saga state tracking                                    │
│  ⚙️  Automatic retry on failure                             │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  Level 3: Transactional Outbox (OPTIONAL, pluggable)       │
│  ⚙️  Atomic message publishing                              │
│  ⚙️  Exactly-once semantics                                 │
│  ⚙️  DB transaction + message publishing                    │
└─────────────────────────────────────────────────────────────┘
```

## Architecture: Compensating Sagas

### Problem: Saga Failure Mid-Flow

```
CreateOrder ✅ (acked, DB committed)
    ↓
OrderCreated ✅
    ↓
ChargePayment ❌ (payment failed!)

❓ What about CreateOrder? Already committed to DB!
❓ Need to UNDO (compensate) CreateOrder
```

### Solution: Compensation Actions

```
         Forward Flow              Compensation Flow
         (optimistic)              (on failure)

    ┌──────────────┐
    │ CreateOrder  │
    └──────┬───────┘
           ↓
    ┌──────────────┐              ┌──────────────┐
    │ saveOrder()  │◄─────────────┤ CancelOrder  │
    └──────┬───────┘  compensate  └──────────────┘
           ↓
    ┌──────────────┐
    │OrderCreated  │
    └──────┬───────┘
           ↓
    ┌──────────────┐              ┌──────────────┐
    │ChargePayment │              │ RefundPayment│
    └──────┬───────┘              └──────────────┘
           │                              ↑
           ↓                              │
    PaymentCharged ✅                     │
           │                              │
           ↓                              │
    ChargePayment ❌ ─────────────────────┘
    (triggers compensation!)
```

### Design: Pluggable Saga Coordinator

```go
// ============================================================================
// Minimal Interface (Level 1 - current implementation)
// ============================================================================

type SagaCoordinator interface {
    // OnEvent returns next commands to execute
    OnEvent(ctx context.Context, msg *Message) ([]*Message, error)
}

// ============================================================================
// Extended Interface (Level 2 - optional compensation support)
// ============================================================================

type CompensatingSagaCoordinator interface {
    SagaCoordinator

    // OnEvent returns commands + compensation info
    OnEventWithCompensation(ctx context.Context, msg *Message) (SagaStep, error)
}

type SagaStep struct {
    // Forward commands (what to do next)
    Commands []*Message

    // Compensation commands (how to undo if later steps fail)
    Compensations []*Message

    // Is this a terminal step?
    IsTerminal bool
}

// ============================================================================
// Saga State (Level 2 - track what happened)
// ============================================================================

type SagaState struct {
    SagaID        string
    CorrelationID string

    // Steps completed so far
    CompletedSteps []CompletedStep

    // Current status
    Status SagaStatus  // "running", "completed", "compensating", "failed"

    // Timestamps
    StartedAt   time.Time
    CompletedAt *time.Time
}

type CompletedStep struct {
    EventName      string
    CompletedAt    time.Time
    Compensations  []*Message  // Store for rollback
}

type SagaStatus string

const (
    SagaStatusRunning      SagaStatus = "running"
    SagaStatusCompleted    SagaStatus = "completed"
    SagaStatusCompensating SagaStatus = "compensating"
    SagaStatusFailed       SagaStatus = "failed"
)

// ============================================================================
// Saga Store (Level 2 - persistence)
// ============================================================================

type SagaStore interface {
    // Save saga state
    Save(ctx context.Context, state *SagaState) error

    // Load saga state
    Load(ctx context.Context, sagaID string) (*SagaState, error)

    // Delete completed saga
    Delete(ctx context.Context, sagaID string) error
}

// Implementations:
// - InMemorySagaStore (for testing, simple cases)
// - PostgresSagaStore (for production)
// - RedisSagaStore (for distributed scenarios)
```

## Example: Order Saga with Compensations

### Step 1: Define Compensation Logic

```go
type OrderCompensatingSaga struct {
    marshaler Marshaler
    store     SagaStore
}

func (s *OrderCompensatingSaga) OnEventWithCompensation(
    ctx context.Context,
    msg *Message,
) (SagaStep, error) {
    subject, _ := msg.Properties.Subject()
    corrID, _ := msg.Properties.CorrelationID()

    // Load saga state
    sagaID := deriveSagaID(corrID)
    state, err := s.store.Load(ctx, sagaID)
    if err != nil {
        state = &SagaState{
            SagaID:        sagaID,
            CorrelationID: corrID,
            Status:        SagaStatusRunning,
            StartedAt:     time.Now(),
        }
    }

    switch subject {
    case "OrderCreated":
        var evt OrderCreated
        json.Unmarshal(msg.Payload, &evt)

        // Forward: Charge payment
        forwardCmds := s.createCommands(
            ChargePayment{
                OrderID:    evt.ID,
                CustomerID: evt.CustomerID,
                Amount:     evt.Amount,
            },
        )

        // Compensation: Cancel order
        compensations := s.createCommands(
            CancelOrder{
                OrderID: evt.ID,
                Reason:  "Payment failed",
            },
        )

        // Save step
        state.CompletedSteps = append(state.CompletedSteps, CompletedStep{
            EventName:     "OrderCreated",
            CompletedAt:   time.Now(),
            Compensations: compensations,
        })
        s.store.Save(ctx, state)

        return SagaStep{
            Commands:      forwardCmds,
            Compensations: compensations,
            IsTerminal:    false,
        }, nil

    case "PaymentCharged":
        // Success path...
        forwardCmds := s.createCommands(ShipOrder{...})

        state.CompletedSteps = append(state.CompletedSteps, CompletedStep{
            EventName:   "PaymentCharged",
            CompletedAt: time.Now(),
            // No compensation needed (shipment will compensate if needed)
        })
        s.store.Save(ctx, state)

        return SagaStep{
            Commands:   forwardCmds,
            IsTerminal: false,
        }, nil

    case "PaymentFailed":
        // ❌ Failure! Trigger compensations
        var evt PaymentFailed
        json.Unmarshal(msg.Payload, &evt)

        log.Printf("⚠️  Payment failed for order %s, starting compensation...", evt.OrderID)

        state.Status = SagaStatusCompensating
        s.store.Save(ctx, state)

        // Execute compensations in REVERSE order
        compensations := s.collectCompensations(state)

        return SagaStep{
            Commands:   compensations,
            IsTerminal: false,  // Not terminal - wait for compensations to complete
        }, nil

    case "OrderCancelled":
        // Compensation completed
        state.Status = SagaStatusFailed
        state.CompletedAt = ptr(time.Now())
        s.store.Save(ctx, state)

        log.Printf("✅ Saga %s compensated successfully", sagaID)

        return SagaStep{
            IsTerminal: true,
        }, nil

    case "OrderShipped":
        // Success! Terminal event
        state.Status = SagaStatusCompleted
        state.CompletedAt = ptr(time.Now())
        s.store.Save(ctx, state)

        log.Printf("✅ Saga %s completed successfully", sagaID)

        return SagaStep{
            IsTerminal: true,
        }, nil
    }

    return SagaStep{}, nil
}

func (s *OrderCompensatingSaga) collectCompensations(state *SagaState) []*Message {
    var compensations []*Message

    // Execute in REVERSE order (LIFO)
    for i := len(state.CompletedSteps) - 1; i >= 0; i-- {
        step := state.CompletedSteps[i]
        compensations = append(compensations, step.Compensations...)
    }

    return compensations
}
```

### Step 2: Define Compensation Commands/Events

```go
// Compensation commands
type CancelOrder struct {
    OrderID string `json:"order_id"`
    Reason  string `json:"reason"`
}

type RefundPayment struct {
    OrderID string `json:"order_id"`
    Amount  int    `json:"amount"`
}

// Failure events
type PaymentFailed struct {
    OrderID string `json:"order_id"`
    Reason  string `json:"reason"`
}

type OrderCancelled struct {
    OrderID     string    `json:"order_id"`
    CancelledAt time.Time `json:"cancelled_at"`
}

// Compensation handlers
cancelOrderHandler := NewCommandHandler(
    "CancelOrder",
    marshaler,
    func(ctx context.Context, cmd CancelOrder) ([]OrderCancelled, error) {
        log.Printf("🔄 Compensating: Cancelling order %s", cmd.OrderID)

        // Undo order creation
        db.DeleteOrder(cmd.OrderID)

        return []OrderCancelled{{
            OrderID:     cmd.OrderID,
            CancelledAt: time.Now(),
        }}, nil
    },
)
```

### Flow Diagram: Compensation

```
Happy Path:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
CreateOrder
    ↓
OrderCreated (compensation: CancelOrder stored)
    ↓
ChargePayment
    ↓
PaymentCharged (compensation: RefundPayment stored)
    ↓
ShipOrder
    ↓
OrderShipped ✅
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Failure Path with Compensation:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
CreateOrder
    ↓
OrderCreated (compensation: CancelOrder stored)
    ↓
ChargePayment ❌
    ↓
PaymentFailed
    ↓
[Saga enters COMPENSATING state]
    ↓
Execute compensations in REVERSE:
    ↓
CancelOrder (undo OrderCreated)
    ↓
OrderCancelled
    ↓
[Saga enters FAILED state] ✅ (gracefully failed)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

## Architecture: Transactional Outbox Pattern

### Problem: Dual Write Problem

```
❌ Race condition:

    db.SaveOrder(order)        ✅ Committed
    publisher.Publish(event)   ❌ Network failure!

    Result: Order saved but no event published!
    Other services don't know order was created.
```

### Solution: Transactional Outbox

```
Atomic Transaction:
┌────────────────────────────────────┐
│  Database Transaction              │
│                                    │
│  1. INSERT INTO orders ...         │
│  2. INSERT INTO outbox (event)     │
│                                    │
│  COMMIT  ✅                        │
└────────────────────────────────────┘
           │
           ↓
┌────────────────────────────────────┐
│  Outbox Processor (async)          │
│                                    │
│  1. SELECT * FROM outbox           │
│  2. Publish to message broker      │
│  3. DELETE FROM outbox             │
│                                    │
└────────────────────────────────────┘
```

### Design: Pluggable Outbox

```go
// ============================================================================
// Outbox Interface (Level 3 - optional)
// ============================================================================

type OutboxStore interface {
    // Store message in outbox (within transaction)
    Store(ctx context.Context, tx Transaction, msg *Message) error

    // Get pending messages
    GetPending(ctx context.Context, limit int) ([]*OutboxMessage, error)

    // Mark as published
    MarkPublished(ctx context.Context, id string) error

    // Delete published messages
    DeletePublished(ctx context.Context, olderThan time.Time) error
}

type OutboxMessage struct {
    ID          string
    Payload     []byte
    Properties  map[string]any
    CreatedAt   time.Time
    PublishedAt *time.Time
}

type Transaction interface {
    // Commit transaction
    Commit() error

    // Rollback transaction
    Rollback() error

    // Execute query within transaction
    Exec(query string, args ...any) error
}

// ============================================================================
// Command Handler with Outbox
// ============================================================================

type CommandHandlerWithOutbox struct {
    handle  func(ctx context.Context, cmd any) ([]any, error)
    db      Database
    outbox  OutboxStore
}

func (h *CommandHandlerWithOutbox) Handle(
    ctx context.Context,
    msg *Message,
) ([]*Message, error) {
    var cmd CreateOrder
    json.Unmarshal(msg.Payload, &cmd)

    // Begin transaction
    tx, err := h.db.Begin(ctx)
    if err != nil {
        msg.Nack(err)
        return nil, err
    }
    defer tx.Rollback()  // Rollback if not committed

    // Execute business logic WITHIN transaction
    events, err := h.handle(ctx, cmd)
    if err != nil {
        msg.Nack(err)
        return nil, err
    }

    // Store events in outbox WITHIN same transaction
    var outMsgs []*Message
    for _, evt := range events {
        payload, _ := json.Marshal(evt)
        outMsg := New(payload, Properties{
            PropSubject: reflect.TypeOf(evt).Name(),
        })

        // ✅ Store in outbox (atomic with business logic!)
        if err := h.outbox.Store(ctx, tx, outMsg); err != nil {
            msg.Nack(err)
            return nil, err
        }

        outMsgs = append(outMsgs, outMsg)
    }

    // ✅ Commit: Both business logic AND outbox messages are atomic
    if err := tx.Commit(); err != nil {
        msg.Nack(err)
        return nil, err
    }

    msg.Ack()

    // Return messages (outbox processor will publish them)
    return outMsgs, nil
}

// ============================================================================
// Outbox Processor (Background worker)
// ============================================================================

type OutboxProcessor struct {
    outbox    OutboxStore
    publisher MessagePublisher
    interval  time.Duration
}

func (p *OutboxProcessor) Start(ctx context.Context) {
    ticker := time.NewTicker(p.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            p.processPending(ctx)
        case <-ctx.Done():
            return
        }
    }
}

func (p *OutboxProcessor) processPending(ctx context.Context) error {
    // Get pending messages
    messages, err := p.outbox.GetPending(ctx, 100)
    if err != nil {
        return err
    }

    for _, msg := range messages {
        // Publish to broker
        if err := p.publisher.Publish(ctx, msg); err != nil {
            log.Printf("Failed to publish outbox message %s: %v", msg.ID, err)
            continue
        }

        // Mark as published
        if err := p.outbox.MarkPublished(ctx, msg.ID); err != nil {
            log.Printf("Failed to mark message %s as published: %v", msg.ID, err)
        }
    }

    return nil
}
```

### Outbox Table Schema

```sql
CREATE TABLE message_outbox (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payload       BYTEA NOT NULL,
    properties    JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at  TIMESTAMPTZ,

    INDEX idx_outbox_pending (created_at) WHERE published_at IS NULL
);

-- Cleanup old messages
DELETE FROM message_outbox
WHERE published_at < NOW() - INTERVAL '7 days';
```

## Modular Architecture: Pluggable Layers

```
┌─────────────────────────────────────────────────────────────┐
│  Application Layer                                          │
│                                                             │
│  User chooses what to use:                                 │
│  - Basic saga (simple)                                     │
│  - Compensating saga (advanced)                            │
│  - Outbox pattern (reliable)                               │
│  - All of the above                                        │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  cqrs Package (Core)                                        │
│                                                             │
│  ✅ CommandProcessor, EventProcessor                       │
│  ✅ SagaCoordinator interface                              │
│  ✅ Marshaler interface                                     │
│  ✅ Basic implementations                                   │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  cqrs/compensation Package (Optional)                       │
│                                                             │
│  ⚙️  CompensatingSagaCoordinator                           │
│  ⚙️  SagaStore interface                                    │
│  ⚙️  InMemorySagaStore (testing)                           │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  cqrs/outbox Package (Optional)                             │
│                                                             │
│  ⚙️  OutboxStore interface                                  │
│  ⚙️  OutboxProcessor                                        │
│  ⚙️  PostgresOutboxStore                                    │
└─────────────────────────────────────────────────────────────┘
```

## Usage Examples

### Level 0: Basic (No compensations, rely on broker retries)

```go
// Simple saga, broker handles retries
coordinator := &OrderSagaCoordinator{marshaler}
sagaProcessor := message.NewRouter(config,
    message.NewHandler(coordinator.OnEvent, matchEvents))

// ✅ Simple
// ⚠️  No compensations
// ⚠️  Relies on broker retries (NATS, ServiceBus, etc.)
```

### Level 1: Compensating Saga (Manual retry/undo)

```go
// Saga with compensations
store := compensation.NewInMemorySagaStore()
coordinator := &OrderCompensatingSaga{
    marshaler: marshaler,
    store:     store,
}

sagaProcessor := compensation.NewCompensatingSagaProcessor(
    config,
    coordinator,
)

// ✅ Automatic compensations on failure
// ✅ Saga state tracking
// ⚠️  Requires saga state store
```

### Level 2: Transactional Outbox (Exactly-once)

```go
// Command handler with outbox
db := postgres.Connect(dsn)
outbox := outbox.NewPostgresOutboxStore(db)

commandHandler := outbox.NewCommandHandlerWithOutbox(
    handleCreateOrder,
    db,
    outbox,
)

// Start outbox processor
processor := outbox.NewOutboxProcessor(
    outbox,
    publisher,
    1 * time.Second,  // Poll interval
)
go processor.Start(ctx)

// ✅ Exactly-once semantics
// ✅ Atomic: DB + message publishing
// ⚠️  Requires database
// ⚠️  Additional complexity
```

### Level 3: Full Stack (Compensations + Outbox)

```go
// Use both for maximum reliability
db := postgres.Connect(dsn)
sagaStore := compensation.NewPostgresSagaStore(db)
outbox := outbox.NewPostgresOutboxStore(db)

coordinator := &OrderCompensatingSaga{
    marshaler: marshaler,
    store:     sagaStore,
}

commandHandler := outbox.NewCommandHandlerWithOutbox(
    handleCreateOrder,
    db,
    outbox,
)

sagaProcessor := compensation.NewCompensatingSagaProcessor(
    config,
    coordinator,
)

outboxProcessor := outbox.NewOutboxProcessor(outbox, publisher, 1*time.Second)
go outboxProcessor.Start(ctx)

// ✅ Exactly-once semantics
// ✅ Automatic compensations
// ✅ Full reliability
// ⚠️  Maximum complexity
```

## Comparison: Reliability Levels

| Level | Reliability | Complexity | When to Use |
|-------|------------|------------|-------------|
| **0: Basic** | Low | Very Low | Development, non-critical |
| **1: Compensating** | Medium | Medium | Production, can tolerate message loss |
| **2: Outbox** | High | High | Exactly-once required |
| **3: Full Stack** | Very High | Very High | Financial systems, critical data |

## Recommendations

### Default (90% of use cases): Basic + Broker Retries

```go
// ✅ Use broker's built-in reliability
// - NATS: at-least-once delivery
// - Azure Service Bus: retry policies
// - Kafka: consumer groups with retries

coordinator := &OrderSagaCoordinator{}  // Simple!
```

**When sufficient:**
- Idempotent operations
- Can tolerate duplicate processing
- Broker provides good enough reliability

### Advanced (10% of use cases): Add Compensations

```go
// ⚙️  Add when you need compensations
coordinator := &OrderCompensatingSaga{store: sagaStore}
```

**When needed:**
- Multi-step workflows that need undo
- Can't rely on broker retries alone
- Need visibility into saga state

### Rare (<1% of use cases): Add Outbox

```go
// ⚙️  Add when you need exactly-once
handler := outbox.NewCommandHandlerWithOutbox(handle, db, outbox)
```

**When needed:**
- Financial transactions
- Exactly-once semantics required
- Can't tolerate message loss

## Implementation Phases

### Phase 1: Core CQRS ✅ (Current)
- [x] CommandProcessor, EventProcessor
- [x] SagaCoordinator interface
- [x] Basic implementations
- [x] Examples

### Phase 2: Compensation Support (Future)
- [ ] `cqrs/compensation` package
- [ ] `CompensatingSagaCoordinator` interface
- [ ] `SagaStore` interface
- [ ] `InMemorySagaStore` implementation
- [ ] Example with compensations

### Phase 3: Outbox Pattern (Future)
- [ ] `cqrs/outbox` package
- [ ] `OutboxStore` interface
- [ ] `PostgresOutboxStore` implementation
- [ ] `OutboxProcessor` background worker
- [ ] Example with outbox

## Summary: Keep It Minimal

**gopipe philosophy:**
1. ✅ **Simple by default** - Basic saga coordinator (current)
2. ⚙️  **Advanced pluggable** - Compensation package (optional)
3. ⚙️  **Expert mode** - Outbox package (rarely needed)

**Users choose their level:**
- Start simple
- Add compensations if needed
- Add outbox if critical
- All layers are optional and composable

This keeps gopipe minimal while supporting advanced patterns for those who need them.

# CQRS Architecture Overview

## Design Philosophy: Minimal & Modular

```
Simple by Default ──────► Advanced Features Pluggable
       │                           │
       │                           │
       ▼                           ▼
    90% use                    10% use
    Basic saga                 Compensations
                                  │
                                  ▼
                               1% use
                               Outbox
```

## Layered Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                                                                │
│  Layer 0: message Package (Core)                              │
│  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  │
│                                                                │
│  • Router, Handler                                            │
│  • Message with acking                                        │
│  • Attributes (metadata)                                      │
│                                                                │
│  ✅ Everyone uses this                                        │
│                                                                │
└────────────────────────────────────────────────────────────────┘
                              ▲
                              │
                              │
┌────────────────────────────────────────────────────────────────┐
│                                                                │
│  Layer 1: cqrs Package (Recommended)                          │
│  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  │
│                                                                │
│  • CommandProcessor, EventProcessor                           │
│  • NewCommandHandler, NewEventHandler                         │
│  • Marshaler (JSON, Protobuf)                                 │
│  • SagaCoordinator (interface) - planned                      │
│                                                                │
│  ✅ Use this for CQRS patterns                                │
│  ⚠️  Saga coordination planned                                │
│                                                                │
└────────────────────────────────────────────────────────────────┘
                              ▲
                              │
                              │
┌────────────────────────────────────────────────────────────────┐
│                                                                │
│  Layer 2: cqrs/compensation Package (Optional)                │
│  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  │
│                                                                │
│  • CompensatingSagaCoordinator (interface)                    │
│  • SagaStore (persist saga state)                             │
│  • SagaState, SagaStep                                        │
│  • Automatic rollback on failure                              │
│                                                                │
│  ⚙️  Use when you need compensations                          │
│  ⚠️  Requires saga state store                                │
│                                                                │
└────────────────────────────────────────────────────────────────┘
                              ▲
                              │
                              │
┌────────────────────────────────────────────────────────────────┐
│                                                                │
│  Layer 3: cqrs/outbox Package (Rare)                          │
│  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  │
│                                                                │
│  • OutboxStore (atomic DB + messaging)                        │
│  • OutboxProcessor (background worker)                        │
│  • Transaction interface                                      │
│  • Exactly-once semantics                                     │
│                                                                │
│  ⚙️  Use for exactly-once guarantees                          │
│  ⚠️  Requires database                                        │
│  ⚠️  Additional operational complexity                        │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

## Feature Matrix

| Feature | Layer 0 (message) | Layer 1 (cqrs) | Layer 2 (compensation) | Layer 3 (outbox) |
|---------|------------------|----------------|----------------------|------------------|
| **Commands/Events** | ❌ | ✅ | ✅ | ✅ |
| **Saga Coordination** | ❌ | ✅ | ✅ | ✅ |
| **Compensations** | ❌ | ❌ | ✅ | ✅ |
| **Saga State** | ❌ | ❌ | ✅ | ✅ |
| **Exactly-Once** | ❌ | ❌ | ❌ | ✅ |
| **Database Required** | ❌ | ❌ | Optional | ✅ |
| **Complexity** | Low | Low | Medium | High |
| **Use Cases** | Pipelines | Most sagas | Critical workflows | Financial |

## Progressive Enhancement

### Start Simple (Layer 1)

```go
import "github.com/fxsml/gopipe/message/cqrs"

// Basic saga - broker handles retries
coordinator := &OrderSagaCoordinator{}
processor := cqrs.NewEventProcessor(config, coordinator)

// ✅ Simple, works for 90% of cases
```

### Add Compensations (Layer 2)

```go
import (
    "github.com/fxsml/gopipe/message/cqrs"
    "github.com/fxsml/gopipe/message/cqrs/compensation"
)

// Saga with automatic compensations
store := compensation.NewInMemorySagaStore()
coordinator := &OrderCompensatingSaga{store: store}
processor := compensation.NewCompensatingSagaProcessor(config, coordinator)

// ✅ Automatic rollback on failure
// ⚠️  Slightly more complex
```

### Add Outbox (Layer 3)

```go
import (
    "github.com/fxsml/gopipe/message/cqrs"
    "github.com/fxsml/gopipe/message/cqrs/outbox"
)

// Command handler with outbox
db := postgres.Connect(dsn)
outboxStore := outbox.NewPostgresOutboxStore(db)

handler := outbox.NewCommandHandlerWithOutbox(
    handleCreateOrder,
    db,
    outboxStore,
)

// Start background outbox processor
processor := outbox.NewOutboxProcessor(outboxStore, publisher, 1*time.Second)
go processor.Start(ctx)

// ✅ Exactly-once semantics
// ⚠️  Requires PostgreSQL
// ⚠️  Background processor needed
```

## Reliability Comparison

```
                        Reliability
                             ▲
                             │
        Outbox (Layer 3) ────┤─────────── 99.99%
                             │            (exactly-once)
                             │
  Compensation (Layer 2) ────┤─────────── 99.9%
                             │            (automatic rollback)
                             │
    Basic + Broker ──────────┤─────────── 99%
    (Layer 1)                │            (at-least-once)
                             │
                             └──────────────────────►
                                              Complexity
```

## Flow Examples

### Layer 1: Basic Saga (Simple)

```
CreateOrder
    ↓
OrderCreated
    ↓ (saga coordinator)
ChargePayment
    ↓
PaymentCharged ✅

If failure:
    ↓
PaymentFailed ❌
    ↓
Retry via broker (NATS, Service Bus)
```

### Layer 2: Compensating Saga

```
CreateOrder
    ↓
OrderCreated [compensation: CancelOrder stored]
    ↓ (saga coordinator)
ChargePayment
    ↓
PaymentCharged [compensation: RefundPayment stored] ✅

If failure:
    ↓
PaymentFailed ❌
    ↓
Saga enters COMPENSATING state
    ↓
Execute compensations (LIFO):
    ↓
CancelOrder (undo OrderCreated)
    ↓
OrderCancelled ✅ (graceful failure)
```

### Layer 3: Transactional Outbox

```
CreateOrder
    ↓
┌─────────────────────────────┐
│  Database Transaction       │
│                             │
│  1. INSERT INTO orders      │
│  2. INSERT INTO outbox      │
│     (OrderCreated event)    │
│                             │
│  COMMIT ✅ (atomic!)        │
└─────────────────────────────┘
    ↓
Outbox Processor (background)
    ↓
SELECT from outbox
    ↓
Publish OrderCreated to broker
    ↓
DELETE from outbox
    ↓
✅ Exactly-once delivery
```

## Package Structure (Future)

```
gopipe/
├── message/                    # Layer 0 (core)
│   ├── message.go
│   ├── router.go
│   └── attributes.go
│
├── cqrs/                       # Layer 1 (recommended)
│   ├── processor.go            # CommandProcessor, EventProcessor
│   ├── handler.go              # NewCommandHandler, NewEventHandler
│   ├── coordinator.go          # SagaCoordinator interface
│   ├── marshaler.go            # Marshaler interface, JSONMarshaler
│   └── examples/
│       └── order_saga.go
│
├── cqrs/compensation/          # Layer 2 (optional)
│   ├── coordinator.go          # CompensatingSagaCoordinator
│   ├── store.go                # SagaStore interface
│   ├── memory_store.go         # InMemorySagaStore
│   ├── postgres_store.go       # PostgresSagaStore
│   └── examples/
│       └── order_compensation.go
│
└── cqrs/outbox/                # Layer 3 (rare)
    ├── store.go                # OutboxStore interface
    ├── processor.go            # OutboxProcessor
    ├── postgres_store.go       # PostgresOutboxStore
    ├── handler.go              # CommandHandlerWithOutbox
    └── examples/
        └── order_outbox.go
```

## When to Use Each Layer

### Layer 1: cqrs (90% of projects)

**Use when:**
- ✅ Building CQRS/event-driven applications
- ✅ Multi-step workflows
- ✅ Can tolerate at-least-once delivery
- ✅ Broker provides retries (NATS, Service Bus)

**Don't need:**
- ❌ Manual compensations (broker retries are enough)
- ❌ Exactly-once guarantees

### Layer 2: cqrs/compensation (10% of projects)

**Use when:**
- ✅ Need to undo operations on failure
- ✅ Multi-step workflows with dependencies
- ✅ Want visibility into saga state
- ✅ Can't rely on broker retries alone

**Don't need:**
- ❌ Exactly-once guarantees (at-least-once is OK)

### Layer 3: cqrs/outbox (<1% of projects)

**Use when:**
- ✅ Financial transactions
- ✅ Exactly-once semantics required
- ✅ Can't tolerate duplicate processing
- ✅ Already using PostgreSQL

**Don't need:**
- ❌ Simple workflows
- ❌ Idempotent operations
- ❌ Can tolerate duplicates

## Migration Path

```
Start ──► Layer 1 (cqrs)
            │
            │ Need compensations?
            ├─ No ──► Stay on Layer 1 ✅
            │
            └─ Yes ──► Add Layer 2 (compensation)
                        │
                        │ Need exactly-once?
                        ├─ No ──► Stay on Layer 2 ✅
                        │
                        └─ Yes ──► Add Layer 3 (outbox)
                                    │
                                    └─► Production-grade ✅
```

## Implementation Status

| Layer | Status | Package | Priority |
|-------|--------|---------|----------|
| **0: message** | ✅ Implemented | `message` | Core |
| **1: cqrs** | ✅ **Implemented** | `cqrs` | High |
| **2: compensation** | 📋 Designed | `cqrs/compensation` | Medium |
| **3: outbox** | 📋 Designed | `cqrs/outbox` | Low |

**Layer 1 (cqrs) is now available:**
- `cqrs.NewCommandHandler[Cmd, Evt]()` - Type-safe command handlers
- `cqrs.NewEventHandler[Evt]()` - Type-safe event handlers
- `cqrs.JSONMarshaler` - JSON serialization (Protobuf coming soon)

**Planned:**
- `cqrs.SagaCoordinator` interface - Workflow orchestration (proposed)

See `examples/cqrs-package/` for complete usage example.

## Summary

**gopipe's approach:**

1. **Simple by default** - Layer 1 for most use cases
2. **Progressive enhancement** - Add complexity only when needed
3. **Modular** - Each layer is independent and optional
4. **Pluggable** - Interfaces allow custom implementations
5. **Testable** - Each layer can be tested independently

**Users get:**
- ✅ Simple start (Layer 1)
- ⚙️  Add compensations if needed (Layer 2)
- ⚙️  Add outbox for critical systems (Layer 3)
- ✅ All optional, all composable

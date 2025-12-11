# CQRS Overview

This document provides a high-level overview of CQRS (Command Query Responsibility Segregation) and how it's implemented in gopipe.

## What is CQRS?

**CQRS** separates read and write operations in an application:

- **Commands**: Write operations that change state (imperative: "CreateOrder", "CancelOrder")
- **Queries**: Read operations that return data (not covered by this package)
- **Events**: Notifications that something happened (past tense: "OrderCreated", "OrderCancelled")

### Core Concept

```
Traditional Approach:
┌──────────────┐
│  Single      │
│  Model       │ ← Create, Read, Update, Delete
└──────────────┘

CQRS Approach:
┌──────────────┐        ┌──────────────┐
│ Write Model  │        │  Read Model  │
│ (Commands)   │   →    │  (Queries)   │
└──────────────┘ Events └──────────────┘
```

## Benefits

### 1. Separation of Concerns

- **Write side**: Optimized for consistency and validation
- **Read side**: Optimized for queries and denormalization
- Different models for different purposes

### 2. Event-Driven Architecture

- Commands produce events
- Events can trigger other actions
- Loose coupling between services

### 3. Scalability

- Scale read and write sides independently
- Read replicas can use different databases (SQL vs NoSQL)
- Event sourcing becomes natural

### 4. Audit Trail

- All changes tracked as events
- Complete history of what happened
- Easy to replay events

## gopipe Implementation

gopipe's CQRS implementation is built on the `message` package and provides type-safe, composable APIs.

### Package Structure

```
message/
├── router.go             ← Router, RouterConfig, NewRouter
├── handler.go            ← Handler interface, NewHandler
├── pipe.go               ← Pipe interface, NewPipe
└── matcher.go            ← Matcher, Match, MatchSubject, etc.

message/cqrs/             ← CQRS handlers (ADR 0006) ✅ Implemented
├── handler.go            ← NewCommandHandler, NewEventHandler
├── marshaler.go          ← CommandMarshaler, EventMarshaler
├── attributes.go         ← WithTypeOf, WithSubject, etc.
├── matcher.go            ← MatchGenericTypeOf
└── pipe.go               ← NewCommandPipe

cqrs/compensation/        ← Compensating sagas (ADR 0008) 📋 Proposed
└── (future)              ← SagaStore, CompensatingSagaCoordinator

cqrs/outbox/              ← Transactional outbox (ADR 0009) 📋 Proposed
└── (future)              ← OutboxStore, OutboxProcessor
```

### Basic Example

```go
import (
    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/message/cqrs"
)

// 1. Define command and event
type CreateOrder struct {
    ID         string
    CustomerID string
    Amount     int
}

type OrderCreated struct {
    ID         string
    CustomerID string
    Amount     int
    CreatedAt  time.Time
}

// 2. Create marshaler
marshaler := cqrs.NewJSONCommandMarshaler(cqrs.WithTypeOf())

// 3. Create command handler (Command → Events)
createOrderHandler := cqrs.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // Business logic
        saveOrder(cmd)

        // Return events
        return []OrderCreated{{
            ID:         cmd.ID,
            CustomerID: cmd.CustomerID,
            Amount:     cmd.Amount,
            CreatedAt:  time.Now(),
        }}, nil
    },
    cqrs.Match(cqrs.MatchSubject("CreateOrder")),
    marshaler,
)

// 4. Create event handler (Event → Side Effects)
emailHandler := cqrs.NewEventHandler(
    func(ctx context.Context, evt OrderCreated) error {
        return emailService.Send(evt.CustomerID, "Order created!")
    },
    cqrs.Match(cqrs.MatchSubject("OrderCreated")),
    cqrs.NewJSONEventMarshaler(),
)

// 5. Wire together with routers
commandRouter := message.NewRouter(message.RouterConfig{})
commandRouter.AddHandler(createOrderHandler)

eventRouter := message.NewRouter(message.RouterConfig{})
eventRouter.AddHandler(emailHandler)

commands := make(chan *message.Message, 10)
events := commandRouter.Start(ctx, commands)
eventRouter.Start(ctx, events)
```

## Key Concepts

### Commands

**What:** Requests to perform an action

**Characteristics:**
- Imperative naming: "CreateOrder", "CancelPayment"
- Single handler per command
- Can succeed or fail
- Return events on success

**Example:**
```go
type CreateOrder struct {
    ID         string
    CustomerID string
    Amount     int
}
```

### Events

**What:** Facts about what happened

**Characteristics:**
- Past tense naming: "OrderCreated", "PaymentCancelled"
- Multiple handlers (subscribers)
- Never fail (already happened)
- Used for notifications and triggering workflows

**Example:**
```go
type OrderCreated struct {
    ID         string
    CustomerID string
    Amount     int
    CreatedAt  time.Time
}
```

### Command Handlers

**What:** Process commands and return events

**Signature:**
```go
func(ctx context.Context, cmd Cmd) ([]Event, error)
```

**Responsibilities:**
- Validate command
- Execute business logic
- Return resulting events
- Pure function (no side effects)

### Event Handlers

**What:** React to events with side effects

**Signature:**
```go
func(ctx context.Context, evt Event) error
```

**Responsibilities:**
- Perform side effects (email, logging, analytics)
- No commands returned (pure side effects)
- Can trigger other events via event bus

## Architecture Patterns

### Pattern 1: Simple CQRS (Basic)

**Use when:** Simple command → event flows

```
Command → Command Handler → Event → Event Handler (side effects)
```

**See:** [ADR 0006: CQRS Implementation](./adr/0006-cqrs-implementation.md)

### Pattern 2: Saga Coordinator (Multi-Step Workflows)

**Use when:** Events need to trigger commands

```
Command → Event → Saga Coordinator → Command → Event → ...
```

**See:** [ADR 0007: Saga Coordinator Pattern](./adr/0007-saga-coordinator-pattern.md)

### Pattern 3: Compensating Saga (Failure Handling)

**Use when:** Need to undo steps on failure

```
Command → Event [compensation stored]
    ↓
Command → ❌ FAILED
    ↓
Execute compensations (LIFO)
```

**See:** [ADR 0008: Compensating Saga Pattern](./adr/0008-compensating-saga-pattern.md) 📋 Proposed

### Pattern 4: Transactional Outbox (Exactly-Once)

**Use when:** Need atomic DB writes + message publishing

```
┌─── DB Transaction ───┐
│ INSERT INTO orders   │
│ INSERT INTO outbox   │ ← Atomic
│ COMMIT               │
└──────────────────────┘
```

**See:** [ADR 0009: Transactional Outbox Pattern](./adr/0009-transactional-outbox-pattern.md) 📋 Proposed

## Progressive Enhancement

gopipe's CQRS implementation follows a **progressive enhancement** model:

```
Level 0: message package      ← Everyone uses (core)
    ↓
Level 1: cqrs package         ← 90% of projects ✅ Implemented
    ↓
Level 2: compensation         ← 10% of projects 📋 Proposed
    ↓
Level 3: outbox               ← <1% of projects 📋 Proposed
```

**Start simple, add complexity only when needed!**

## Best Practices

### 1. Command Naming

✅ **Good:** Imperative verbs
```go
CreateOrder
CancelOrder
ChargePayment
ShipOrder
```

❌ **Bad:** Nouns or past tense
```go
Order          // Not clear what action
OrderCreation  // Verbose
OrderCreated   // This is an event, not command
```

### 2. Event Naming

✅ **Good:** Past tense
```go
OrderCreated
OrderCancelled
PaymentCharged
OrderShipped
```

❌ **Bad:** Imperative or continuous
```go
CreateOrder     // This is a command
OrderCreating   // Not a fact
```

### 3. Command Handlers Should Be Pure

✅ **Good:** Return events, no side effects
```go
func handleCreateOrder(ctx, cmd) ([]OrderCreated, error) {
    saveOrder(cmd)  // Necessary persistence
    return []OrderCreated{{...}}, nil
}
```

❌ **Bad:** Direct side effects in handler
```go
func handleCreateOrder(ctx, cmd) ([]OrderCreated, error) {
    saveOrder(cmd)
    emailService.Send(...)  // ❌ Side effect in command handler!
    return []OrderCreated{{...}}, nil
}
```

### 4. Event Handlers for Side Effects

✅ **Good:** Side effects in event handlers
```go
func handleOrderCreated(ctx, evt) error {
    return emailService.Send(evt.CustomerID, "Order created!")
}
```

### 5. Use Correlation IDs

✅ **Good:** Track end-to-end flows
```go
data, _ := marshaler.Marshal(CreateOrder{...})
msg := message.New(data, message.Attributes{
    message.AttrCorrelationID: "corr-123",
    message.AttrSubject: "CreateOrder",
    message.AttrType: "CreateOrder",
})
```

### 6. One Event Can Have Multiple Handlers

```go
// All listening to "OrderCreated"
emailHandler := cqrs.NewEventHandler(sendEmail, cqrs.MatchSubject("OrderCreated"), marshaler)
analyticsHandler := cqrs.NewEventHandler(trackAnalytics, cqrs.MatchSubject("OrderCreated"), marshaler)
inventoryHandler := cqrs.NewEventHandler(updateInventory, cqrs.MatchSubject("OrderCreated"), marshaler)
sagaCoordinator := &OrderSagaCoordinator{...} // Workflow logic
```

## When to Use CQRS

### Use CQRS When:

✅ Building event-driven architectures
✅ Need clear separation between reads and writes
✅ Want to track all changes as events
✅ Microservices communicating via events
✅ Need audit trail
✅ Different scaling requirements for reads/writes

### Don't Use CQRS When:

❌ Simple CRUD applications
❌ No need for event-driven architecture
❌ Complexity not justified
❌ Team unfamiliar with pattern
❌ Read and write models are identical

## Common Patterns

### Pattern: Event Notification

```
CreateOrder → OrderCreated → Email + Analytics + Logging
```

### Pattern: Event Choreography (Saga)

```
CreateOrder → OrderCreated → ChargePayment → PaymentCharged → ShipOrder
```

### Pattern: Event Sourcing

```
Commands → Events → Event Store → Current State (projection)
```

### Pattern: Read Models

```
Events → Materialized View (denormalized read model)
```

## Related Documentation

### Core Documentation

- [Router and Handlers](./router-and-handlers.md) - Router, Handler, Pipe, and Matcher documentation

### Architecture Decision Records

- [ADR 0006: CQRS Implementation](./adr/0006-cqrs-implementation.md) ✅ Implemented
- [ADR 0007: Saga Coordinator Pattern](./adr/0007-saga-coordinator-pattern.md) 📋 Proposed
- [ADR 0008: Compensating Saga Pattern](./adr/0008-compensating-saga-pattern.md) 📋 Proposed
- [ADR 0009: Transactional Outbox Pattern](./adr/0009-transactional-outbox-pattern.md) 📋 Proposed

### Pattern Guides

- [Saga Pattern Overview](./saga-overview.md)
- [Outbox Pattern Overview](./outbox-overview.md)
- [CQRS Architecture Overview](./cqrs-architecture-overview.md)

### Examples

- [examples/cqrs-package](../examples/cqrs-package/) - Complete CQRS + Saga example

### External Resources

- [Microsoft: CQRS Pattern](https://learn.microsoft.com/en-us/azure/architecture/patterns/cqrs)
- [Martin Fowler: CQRS](https://martinfowler.com/bliki/CQRS.html)
- [Watermill CQRS Component](https://watermill.io/docs/cqrs/)
- [How to use basic CQRS in Go](https://threedots.tech/post/basic-cqrs-in-go/)

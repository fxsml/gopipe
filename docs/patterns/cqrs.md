# Pattern: CQRS (Command Query Responsibility Segregation)

## Intent

Separate read and write operations into different models, enabling independent scaling, optimization, and evolution of each side.

## Structure

```
┌─────────────────────────────────────────────────────────────┐
│                    CQRS Architecture                         │
│                                                              │
│  Write Side                         Read Side               │
│  ──────────                         ─────────               │
│                                                              │
│  ┌─────────┐     ┌─────────────┐     ┌─────────────┐       │
│  │ Command │────►│  Command    │────►│   Event     │       │
│  │         │     │  Handler    │     │   Store     │       │
│  └─────────┘     └─────────────┘     └──────┬──────┘       │
│                                              │               │
│                                              ▼               │
│                                       ┌─────────────┐       │
│  ┌─────────┐     ┌─────────────┐     │ Projection  │       │
│  │  Query  │◄────│   Query     │◄────│             │       │
│  │         │     │   Handler   │     └─────────────┘       │
│  └─────────┘     └─────────────┘            │               │
│                                              ▼               │
│                                       ┌─────────────┐       │
│                                       │ Read Model  │       │
│                                       │ (optimized) │       │
│                                       └─────────────┘       │
└─────────────────────────────────────────────────────────────┘
```

## Participants

| Component | Role |
|-----------|------|
| **Command** | Request to perform an action (imperative: "CreateOrder") |
| **Command Handler** | Validates and executes commands, emits events |
| **Event** | Fact about what happened (past tense: "OrderCreated") |
| **Event Store** | Persists events (optional, for event sourcing) |
| **Projection** | Transforms events into read models |
| **Query Handler** | Retrieves data from read models |
| **Read Model** | Optimized data structure for queries |

## When to Use

**Good fit:**
- Complex domain logic requiring different read/write models
- Different scaling needs for reads vs writes
- Audit trail requirements
- Event sourcing architecture
- Multiple read model representations needed

**Poor fit:**
- Simple CRUD applications
- Strong consistency requirements everywhere
- Small team with simple domain

## Implementation in goengine

### Command Handler

```go
// CommandHandler processes commands and returns events
handler := cqrs.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // Validate
        if cmd.Amount <= 0 {
            return nil, errors.New("invalid amount")
        }

        // Execute business logic
        order := createOrder(cmd)

        // Return events
        return []OrderCreated{{
            OrderID:   order.ID,
            Amount:    cmd.Amount,
            CreatedAt: time.Now(),
        }}, nil
    },
)
```

### Event Handler

```go
// EventHandler reacts to events
handler := cqrs.NewEventHandler(
    func(ctx context.Context, evt OrderCreated) error {
        // Update read model
        return readModel.AddOrder(evt)
    },
)
```

### Router Integration

```go
// Wire handlers with router
router := message.NewRouter()
router.AddHandler(createOrderHandler)
router.AddHandler(cancelOrderHandler)

// Process commands
events := router.Start(ctx, commands)
```

## Naming Conventions

| Type | Convention | Examples |
|------|------------|----------|
| Command | Imperative verb + noun | CreateOrder, CancelPayment, UpdateProfile |
| Event | Noun + past participle | OrderCreated, PaymentCancelled, ProfileUpdated |
| Query | Get/List/Find + noun | GetOrder, ListOrders, FindOrdersByCustomer |

## Related Patterns

- [Saga](saga.md) - Coordinates multiple commands across services
- [Outbox](outbox.md) - Reliable event publishing
- [Event Sourcing](event-sourcing.md) - Store state as event sequence

## Related ADRs

- [ADR 0006: CQRS Implementation](../adr/IMP-0006-cqrs-implementation.md)
- [ADR 0007: Saga Coordinator Pattern](../goengine/adr/PRO-0007-saga-coordinator.md)

## Further Reading

- Martin Fowler: [CQRS](https://martinfowler.com/bliki/CQRS.html)
- Greg Young: [CQRS and Event Sourcing](https://cqrs.files.wordpress.com/2010/11/cqrs_documents.pdf)

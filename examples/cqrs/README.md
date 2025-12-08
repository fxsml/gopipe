# CQRS Example

This example demonstrates how CQRS (Command Query Responsibility Segregation) could be implemented using gopipe's message routing infrastructure.

## Overview

CQRS separates write operations (Commands) from read operations and event notifications (Events):

- **Commands**: Imperative requests for actions (CreateOrder, CancelOrder)
- **Events**: Past-tense notifications of what happened (OrderCreated, OrderCancelled)

## Architecture

```
┌─────────────┐
│ CommandBus  │──┐
└─────────────┘  │
                 ├──> ┌──────────────────┐     ┌─────────────┐
┌─────────────┐  │    │ Command Handlers │────>│  EventBus   │
│   Clients   │──┤    └──────────────────┘     └─────────────┘
└─────────────┘  │                                     │
                 │                                     ├──> Email Handler
┌─────────────┐  │                                     ├──> Analytics Handler
│  EventBus   │──┘                                     └──> Inventory Handler
└─────────────┘
```

## Key Concepts

### 1. Commands (Write Operations)

Commands are imperative and represent requests for actions:

```go
type CreateOrder struct {
    ID         string
    CustomerID string
    Amount     int
}

// Send command
commandBus.Send(ctx, CreateOrder{
    ID:         "order-123",
    CustomerID: "customer-456",
    Amount:     250,
})
```

**Characteristics:**
- Imperative naming (Create, Cancel, Update)
- Single handler per command
- May fail validation
- Publish events on success

### 2. Events (Read/Notification)

Events are past-tense and represent facts about what happened:

```go
type OrderCreated struct {
    ID         string
    CustomerID string
    Amount     int
    CreatedAt  time.Time
}

// Publish event
eventBus.Publish(ctx, OrderCreated{
    ID:         order.ID,
    CustomerID: order.CustomerID,
    Amount:     order.Amount,
    CreatedAt:  time.Now(),
})
```

**Characteristics:**
- Past-tense naming (Created, Cancelled, Updated)
- Multiple handlers can react to the same event
- Cannot fail - they already happened
- Enable decoupled reactions

### 3. Command Handlers

Process commands and publish events:

```go
createOrderHandler := NewCommandHandler(
    "CreateOrder",
    func(ctx context.Context, cmd CreateOrder) error {
        // Validate
        if cmd.Amount <= 0 {
            return fmt.Errorf("invalid amount")
        }

        // Business logic
        saveOrder(cmd)

        // Publish event
        return eventBus.Publish(ctx, OrderCreated{
            ID:         cmd.ID,
            CustomerID: cmd.CustomerID,
            Amount:     cmd.Amount,
            CreatedAt:  time.Now(),
        })
    },
)
```

### 4. Event Handlers

React to events independently:

```go
// Handler 1: Send email
sendEmailHandler := NewEventHandler(
    "OrderCreated",
    func(ctx context.Context, evt OrderCreated) error {
        return emailService.Send(evt.CustomerID, "Order Confirmation", ...)
    },
)

// Handler 2: Update analytics
analyticsHandler := NewEventHandler(
    "OrderCreated",
    func(ctx context.Context, evt OrderCreated) error {
        return analyticsService.Track("order_created", evt)
    },
)

// Handler 3: Update inventory
inventoryHandler := NewEventHandler(
    "OrderCreated",
    func(ctx context.Context, evt OrderCreated) error {
        return inventoryService.Reserve(evt.ID)
    },
)
```

## Benefits

### 1. Separation of Concerns

Commands handle writes, events handle notifications:

```go
// Command: Write operation
CreateOrder → Validates → Saves → Publishes OrderCreated

// Events: Multiple independent reactions
OrderCreated → Send Email
            → Update Analytics
            → Update Inventory
```

### 2. Decoupling

Add new event handlers without modifying existing code:

```go
// Later, add new functionality without changing CreateOrder handler
fraudDetectionHandler := NewEventHandler(
    "OrderCreated",
    func(ctx context.Context, evt OrderCreated) error {
        return fraudService.Analyze(evt)
    },
)
```

### 3. Type Safety

Handlers are strongly typed:

```go
// Compiler enforces correct types
NewCommandHandler(
    "CreateOrder",
    func(ctx context.Context, cmd CreateOrder) error {
        // cmd is CreateOrder, not interface{} or map
        return processOrder(cmd.ID, cmd.Amount)
    },
)
```

### 4. Testability

Easy to test in isolation:

```go
func TestCreateOrderHandler(t *testing.T) {
    mockEventBus := &MockEventBus{}
    handler := NewCreateOrderHandler(mockEventBus)

    err := handler.Handle(ctx, CreateOrder{ID: "test", Amount: 100})

    assert.NoError(t, err)
    assert.True(t, mockEventBus.WasPublished("OrderCreated"))
}
```

## Running the Example

```bash
cd examples/cqrs
go run main.go
```

Expected output:

```
======================================================================
CQRS Example: Order Processing
======================================================================

🚀 Sending CreateOrder command...
📝 Handling command: CreateOrder{ID: order-123, Amount: 250}
✅ Order created: order-123
📧 Sending confirmation email for order order-123 to customer customer-456
✉️  Email sent successfully

---

🚀 Sending CancelOrder command...
📝 Handling command: CancelOrder{ID: order-123, Reason: Customer requested cancellation}
❌ Order cancelled: order-123
📧 Sending cancellation email for order order-123

======================================================================
Demo complete!

Key observations:
  1. Commands are imperative (CreateOrder, CancelOrder)
  2. Events are past tense (OrderCreated, OrderCancelled)
  3. Single command handler per command
  4. Multiple event handlers can react to same event
  5. Clear separation of write (commands) and read (events)
======================================================================
```

## Implementation Notes

This is a **proof-of-concept** showing how a dedicated `cqrs` package could work. The actual implementation would be in a separate package (`github.com/fxsml/gopipe/cqrs`) with:

1. **Marshaler interface** - Pluggable serialization (JSON, Protobuf, etc.)
2. **CommandBus and EventBus** - Type-safe command sending and event publishing
3. **Handler builders** - `NewCommandHandler` and `NewEventHandler`
4. **Integration with gopipe** - Builds on `message.Router` and `message.Handler`

See [ADR 0006](../../docs/adr/0006-cqrs-implementation.md) for the complete proposal.

## Comparison with Watermill

| Feature | Watermill | gopipe CQRS |
|---------|-----------|-------------|
| **CommandBus** | ✅ | ✅ |
| **EventBus** | ✅ | ✅ |
| **Type Safety** | ✅ (via generics) | ✅ (via generics) |
| **Marshaler Interface** | ✅ | ✅ |
| **Configuration** | Complex (struct-based) | Simple (function-based) |
| **Dependencies** | Full Watermill | Just gopipe |
| **Learning Curve** | Moderate | Low (if familiar with gopipe) |

## Next Steps

To implement this proposal:

1. Create `cqrs` package with core types
2. Implement `JSONMarshaler` and marshaler interface
3. Create `CommandBus` and `EventBus`
4. Add helper functions for handler creation
5. Write comprehensive tests
6. Document migration from plain `message.Router`

## References

- [ADR 0006: CQRS Implementation](../../docs/adr/0006-cqrs-implementation.md)
- [Watermill CQRS Component](https://watermill.io/docs/cqrs/)
- [Basic CQRS in Go](https://threedots.tech/post/basic-cqrs-in-go/)
- [Microsoft: CQRS Pattern](https://learn.microsoft.com/en-us/azure/architecture/patterns/cqrs)

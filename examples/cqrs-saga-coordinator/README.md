# CQRS with Saga Coordinator Pattern

This example demonstrates the **Saga Coordinator pattern**, which solves the coupling problem in the initial CQRS design.

## The Problem: Coupling in Event Handlers

### ❌ Bad: Event Handlers Return Commands Directly

```go
// Problem: Event handler is COUPLED to ChargePayment command!
orderCreatedHandler := NewEventHandler(
    "OrderCreated",
    func(ctx, evt OrderCreated) ([]ChargePayment, error) {
        return []ChargePayment{{...}}, nil  // ❌ Knows about ChargePayment!
    },
)
```

**Issues:**
- Event handler knows about specific command types
- Hard to change workflow without modifying event handlers
- Testing requires knowledge of command types
- Violates separation of concerns

## The Solution: Saga Coordinator

### ✅ Good: Separate Workflow Logic from Event Handling

```go
// Pure event handler (side effects only, no commands!)
emailHandler := NewEventHandler(
    "OrderCreated",
    func(ctx, evt OrderCreated) error {
        sendEmail(evt.CustomerID)
        return nil  // ✅ No commands returned!
    },
)

// Saga Coordinator (workflow logic)
type OrderSagaCoordinator struct {}

func (s *OrderSagaCoordinator) OnEvent(ctx, msg *Message) ([]*Message, error) {
    subject, _ := msg.Properties.Subject()

    switch subject {
    case "OrderCreated":
        var evt OrderCreated
        json.Unmarshal(msg.Payload, &evt)

        // ✅ Workflow logic lives HERE, not in event handlers!
        return s.createCommands(
            ChargePayment{OrderID: evt.ID, Amount: evt.Amount},
            ReserveInventory{OrderID: evt.ID},
        )
    }
}
```

## Architecture

```
┌─────────────┐
│   Command   │
└──────┬──────┘
       ↓
┌──────────────┐
│   Command    │
│  Processor   │
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
└─────────────┘ └──────┬──────┘ └──────────────┘
                       ↓
                ┌──────────────┐
                │   Commands   │
                │  (feedback)  │
                └──────────────┘
```

## Key Components

### 1. Command Handlers (Commands → Events)

```go
createOrderHandler := NewCommandHandler(
    "CreateOrder",
    func(ctx, cmd CreateOrder) ([]OrderCreated, error) {
        saveOrder(cmd)
        return []OrderCreated{{...}}, nil
    },
)
```

**Responsibility:** Process commands, produce events

### 2. Event Handlers (Events → Side Effects)

```go
// ✅ Pure side effects, no commands!
emailHandler := NewEventHandler(
    "OrderCreated",
    func(ctx, evt OrderCreated) error {
        return emailService.Send(evt.CustomerID, ...)
    },
)

analyticsHandler := NewEventHandler(
    "OrderCreated",
    func(ctx, evt OrderCreated) error {
        return analyticsService.Track("order_created", evt)
    },
)
```

**Responsibility:** React to events with side effects (emails, logging, analytics)

### 3. Saga Coordinator (Events → Commands)

```go
type OrderSagaCoordinator struct {
    marshaler Marshaler
}

func (s *OrderSagaCoordinator) OnEvent(ctx, msg *Message) ([]*Message, error) {
    subject, _ := msg.Properties.Subject()

    switch subject {
    case "OrderCreated":
        // Workflow logic: what happens next?
        return s.createCommands(
            ChargePayment{...},
            ReserveInventory{...},
        )

    case "PaymentCharged":
        return s.createCommands(ShipOrder{...})

    case "OrderShipped":
        return nil, nil  // Terminal
    }
}
```

**Responsibility:** Define workflow logic, coordinate saga steps

## Benefits

### 1. Decoupling

```go
// ✅ Event handler doesn't know about commands
emailHandler := func(ctx, evt OrderCreated) error {
    return sendEmail(evt)  // No commands!
}

// ✅ Saga coordinator knows about workflow
sagaCoordinator.OnEvent(...)  // Returns commands
```

### 2. Testability

```go
// Test event handler (no mocking!)
func TestEmailHandler(t *testing.T) {
    handler := &EmailHandler{}
    err := handler.HandleOrderCreated(ctx, OrderCreated{...})
    assert.NoError(t, err)
    // Simple!
}

// Test saga coordinator (isolated)
func TestSagaCoordinator(t *testing.T) {
    coordinator := &OrderSagaCoordinator{}
    cmds, err := coordinator.OnEvent(ctx, orderCreatedEvent)

    assert.Len(t, cmds, 2)  // ChargePayment + ReserveInventory
    assert.Equal(t, "ChargePayment", cmds[0].Subject())
}
```

### 3. Flexibility

```go
// Add new side effect (doesn't affect saga)
newHandler := NewEventHandler(
    "OrderCreated",
    func(ctx, evt OrderCreated) error {
        return fraudService.Check(evt)  // Easy to add!
    },
)

// Change workflow (doesn't affect event handlers)
func (s *OrderSagaCoordinator) OnEvent(...) {
    // Add new saga step
    return s.createCommands(
        ChargePayment{...},
        ReserveInventory{...},
        NotifyWarehouse{...},  // New!
    )
}
```

### 4. Multistage Acking

One event can trigger **multiple commands concurrently**:

```go
func (s *OrderSagaCoordinator) OnEvent(...) {
    case "OrderCreated":
        // ✅ Three commands from one event!
        return s.createCommands(
            ChargePayment{...},
            ReserveInventory{...},
            NotifyWarehouse{...},
        )
}
```

## Running the Example

```bash
cd examples/cqrs-saga-coordinator
go run main.go
```

**Expected Output:**

```
======================================================================
CQRS with Saga Coordinator Pattern
✅ Event handlers do NOT return commands (decoupled!)
✅ Saga coordinator manages workflow logic separately
======================================================================

🚀 Sending CreateOrder command...

📝 Command: CreateOrder
   💾 Saving order to database...
   ✅ → Event: OrderCreated
🔄 Saga: OrderCreated → triggering ChargePayment + ReserveInventory
📧 Side Effect: Sending order confirmation email to customer-456
📊 Side Effect: Tracking order_created event (amount: $350)
📝 Command: ChargePayment
   💳 Charging $350...
   ✅ → Event: PaymentCharged
📝 Command: ReserveInventory
   📦 Reserving inventory for SKU-12345...
   ✅ → Event: InventoryReserved
🔄 Saga: InventoryReserved → triggering ShipOrder
📝 Command: ShipOrder
   🚚 Shipping to 123 Main St...
   ✅ → Event: OrderShipped
✅ Saga: OrderShipped → saga complete!
📧 Side Effect: Sending shipping notification (tracking: TRACK-order-789)

======================================================================
Key Benefits:
  ✅ Event handlers are DECOUPLED from commands
  ✅ Workflow logic is in Saga Coordinator (not event handlers)
  ✅ Easy to test: side effects vs saga logic separately
  ✅ Multistage acking: one event → multiple commands
  ✅ Clean separation: side effects vs workflow
======================================================================
```

## Comparison with Other Patterns

See [docs/cqrs-saga-patterns.md](../../docs/cqrs-saga-patterns.md) for detailed comparison of:

1. **Direct Command Return** - Simple but coupled
2. **Saga Coordinator** (this example) - Recommended
3. **Orchestrator** - Central control
4. **Process Manager** - Stateful saga

## When to Use This Pattern

✅ **Use Saga Coordinator when:**
- Multi-step workflows (3+ steps)
- Need to test workflow logic separately
- Want clean separation of concerns
- Workflows change over time
- Need multistage acking

❌ **Don't use when:**
- Very simple workflows (1-2 steps)
- Workflow will never change
- Acceptable to couple event handlers to commands

## Code Structure

```
examples/cqrs-saga-coordinator/
├── main.go
│   ├── Command Handlers (Commands → Events)
│   ├── Event Handlers (Events → Side Effects)
│   └── Saga Coordinator (Events → Commands)
└── README.md
```

## Key Takeaways

1. **Separation of Concerns**:
   - Event handlers: Side effects only
   - Saga coordinator: Workflow logic
   - Clear separation makes code maintainable

2. **No Coupling**:
   - Event handlers don't know about commands
   - Easy to add/remove side effects
   - Easy to change workflow

3. **Testability**:
   - Test side effects independently
   - Test workflow logic independently
   - No mocking required

4. **gopipe-Friendly**:
   - Uses channels naturally
   - Follows gopipe patterns
   - Composable processors

## References

- [Saga Pattern](https://microservices.io/patterns/data/saga.html)
- [Event Choreography](https://www.tenupsoft.com/blog/The-importance-of-cqrs-and-saga-in-microservices-architecture.html)
- [Pattern Comparison](../../docs/cqrs-saga-patterns.md)

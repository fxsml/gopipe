# CQRS with Saga Pattern (Revised Design)

This example demonstrates the **correct** CQRS implementation for gopipe, addressing key design issues from the initial proposal.

## Key Improvements

| Issue | Original Design | Revised Design |
|-------|----------------|----------------|
| **Channel Pattern** | ❌ Buses consume channels | ✅ Processors return channels |
| **Command Handlers** | ❌ Call EventBus directly | ✅ Return events |
| **Event Handlers** | ❌ Can't return commands | ✅ Return commands (saga) |
| **Saga Support** | ❌ No support | ✅ Natural feedback loop |
| **Coupling** | ❌ Handlers depend on buses | ✅ Pure functions |

## Architecture

```
Initial Commands
      ↓
      ├──→ Command Processor ──→ Events
      ↑                              ↓
      |                         Event Processor
      |                              ↓
      └──────── Saga Commands ←──────┘
           (feedback loop)
```

### Flow Example

```
CreateOrder (command)
   ↓
CommandProcessor
   ↓
OrderCreated (event)
   ↓
EventProcessor (saga handler)
   ↓
ChargePayment (command) ←─┐
   ↓                       │
CommandProcessor           │
   ↓                       │
PaymentCharged (event)     │ Feedback
   ↓                       │ Loop
EventProcessor (saga)      │
   ↓                       │
ShipOrder (command) ───────┘
   ↓
CommandProcessor
   ↓
OrderShipped (event)
   ↓
EventProcessor (terminal)
   ↓
Send Email ✓
```

## Core Concepts

### 1. Command Handlers Return Events

```go
// ✅ Correct: Handler returns events
createOrderHandler := NewCommandHandler(
    "CreateOrder",
    marshaler,
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        saveOrder(cmd)

        // Return events (don't send to bus!)
        return []OrderCreated{{
            ID:         cmd.ID,
            CustomerID: cmd.CustomerID,
            Amount:     cmd.Amount,
            CreatedAt:  time.Now(),
        }}, nil
    },
)
```

**Benefits:**
- Handlers are pure functions
- Easy to test (no mocking buses)
- Clear separation of concerns
- Composable

### 2. Event Handlers Return Commands (Saga Pattern)

```go
// Event handler can trigger new commands
orderCreatedSagaHandler := NewEventHandler(
    "OrderCreated",
    marshaler,
    func(ctx context.Context, evt OrderCreated) ([]ChargePayment, error) {
        // Saga: React to event by triggering next command
        return []ChargePayment{{
            OrderID:    evt.ID,
            CustomerID: evt.CustomerID,
            Amount:     evt.Amount,
        }}, nil
    },
)
```

**This enables:**
- Event choreography
- Saga pattern
- Multi-step workflows
- Distributed transactions

### 3. Processors Return Channels

```go
// ✅ Follows gopipe pattern
type CommandProcessor struct {
    router *message.Router
}

func (p *CommandProcessor) Start(ctx context.Context,
    commands <-chan *message.Message) <-chan *message.Message {
    return p.router.Start(ctx, commands)
}
```

**Benefits:**
- Consistent with gopipe API
- Composable via channels
- Natural feedback loops
- Easy to wire together

### 4. Feedback Loop via Channel Merge

```go
// Initial commands
initialCommands := make(chan *message.Message, 10)

// Saga-triggered commands
sagaCommands := make(chan *message.Message, 100)

// Merge: Initial + Saga commands
allCommands := channel.Merge(initialCommands, sagaCommands)

// Commands → Events
events := commandProcessor.Start(ctx, allCommands)

// Events → Commands (saga)
sagaOutputs := eventProcessor.Start(ctx, events)

// Feedback loop: Route saga outputs back to commands
go func() {
    for cmd := range sagaOutputs {
        sagaCommands <- cmd
    }
}()
```

**This creates a continuous flow where:**
1. Commands produce events
2. Events trigger commands (saga)
3. Those commands produce more events
4. And so on...

## Running the Example

```bash
cd examples/cqrs-v2
go run main.go
```

**Expected Output:**

```
======================================================================
CQRS Example: Order Processing with Saga Pattern
======================================================================

Flow: CreateOrder → OrderCreated → ChargePayment → PaymentCharged → ShipOrder → OrderShipped

🚀 Sending CreateOrder command...

📝 Processing command: CreateOrder
   💾 Saving order to database...
✅ Command produced event: OrderCreated
📨 Processing event: OrderCreated
   🔄 Saga: OrderCreated → trigger ChargePayment
🔄 Event handler produced: ChargePayment
📝 Processing command: ChargePayment
   💳 Charging payment...
✅ Command produced event: PaymentCharged
📨 Processing event: PaymentCharged
   🔄 Saga: PaymentCharged → trigger ShipOrder
🔄 Event handler produced: ShipOrder
📝 Processing command: ShipOrder
   📦 Shipping order...
✅ Command produced event: OrderShipped
📨 Processing event: OrderShipped
   📧 Sending shipping confirmation email
   ✉️  Email sent! Tracking: TRACK-order-789

======================================================================
Saga Complete!
======================================================================
```

## Pattern: Saga (Event Choreography)

A **saga** is a sequence of local transactions coordinated through events. Each transaction:
1. Updates the database
2. Publishes an event
3. The event triggers the next transaction

### Why Sagas?

In microservices, you can't use database transactions across services. Sagas provide:
- **Distributed transactions** without 2PC
- **Loose coupling** between services
- **Resilience** (each step can retry independently)
- **Flexibility** (easy to add new reactions to events)

### Types of Sagas

1. **Choreography** (this example): Services react to events autonomously
   - ✅ Decoupled
   - ✅ Scalable
   - ❌ Harder to trace

2. **Orchestration**: Central coordinator directs the flow
   - ✅ Easier to understand
   - ✅ Centralized error handling
   - ❌ Single point of failure

## Comparison with Original Design

### Original (Broken)

```go
// ❌ Command handler calls EventBus directly
handleCreateOrder := func(ctx context.Context, cmd CreateOrder) error {
    saveOrder(cmd)
    return eventBus.Publish(ctx, OrderCreated{...}) // Tight coupling!
}

// ❌ EventBus consumes channel (wrong pattern)
eventBus := NewEventBus(marshaler, outputChan)
```

**Problems:**
- Handlers depend on EventBus (hard to test)
- Doesn't follow gopipe's channel pattern
- No saga support
- Can't compose easily

### Revised (Correct)

```go
// ✅ Command handler returns events
handleCreateOrder := func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
    saveOrder(cmd)
    return []OrderCreated{{...}}, nil // Pure function!
}

// ✅ Processor returns channel (gopipe pattern)
events := commandProcessor.Start(ctx, commands)
```

**Benefits:**
- Handlers are pure functions (easy to test)
- Follows gopipe patterns
- Saga support via feedback loop
- Composable

## Testing Example

```go
func TestCreateOrderHandler(t *testing.T) {
    handler := func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // Business logic
        return []OrderCreated{{
            ID:         cmd.ID,
            CustomerID: cmd.CustomerID,
            Amount:     cmd.Amount,
            CreatedAt:  time.Now(),
        }}, nil
    }

    // Test pure function (no mocks needed!)
    events, err := handler(ctx, CreateOrder{
        ID:         "test-order",
        CustomerID: "test-customer",
        Amount:     100,
    })

    assert.NoError(t, err)
    assert.Len(t, events, 1)
    assert.Equal(t, "test-order", events[0].ID)
    assert.Equal(t, 100, events[0].Amount)
}
```

## Implementation Notes

This is still a proof-of-concept. The actual `cqrs` package would provide:

1. **CommandProcessor** and **EventProcessor** types
2. **NewCommandHandler** and **NewEventHandler** builder functions
3. **Marshaler** interface with JSON and Protobuf implementations
4. **Topic generation** utilities
5. **Correlation ID** propagation
6. **Saga coordinator** helpers (optional)

See the updated [ADR 0006](../../docs/adr/0006-cqrs-implementation.md) for the complete proposal.

## References

- [Saga Pattern](https://microservices.io/patterns/data/saga.html)
- [Event Choreography vs Orchestration](https://www.tenupsoft.com/blog/The-importance-of-cqrs-and-saga-in-microservices-architecture.html)
- [CQRS and Saga in Microservices](https://medium.com/@ingila185/cqrs-and-saga-the-essential-patterns-for-high-performance-microservice-4f23a09889b4)
- [Design Analysis](../../docs/cqrs-design-analysis.md)

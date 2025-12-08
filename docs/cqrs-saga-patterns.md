# CQRS Saga Patterns: Coupling Analysis and Alternatives

## The Coupling Problem

### Current Design (Has Issues)

```go
// Event handler returns commands directly
orderCreatedHandler := NewEventHandler(
    "OrderCreated",
    marshaler,
    func(ctx, evt OrderCreated) ([]ChargePayment, error) {
        // ❌ Problem: Event handler is COUPLED to ChargePayment command!
        return []ChargePayment{{
            OrderID: evt.ID,
            Amount:  evt.Amount,
        }}, nil
    },
)
```

**Issues:**
1. **Tight Coupling**: Event handler knows about specific command types
2. **Business Logic Leakage**: "What happens after OrderCreated" is in the event handler
3. **Hard to Change**: Adding new steps requires modifying event handlers
4. **Testing Complexity**: Must know command types to test event handlers

## Pattern Comparison

| Pattern | Coupling | Complexity | Flexibility | Use Case |
|---------|----------|------------|-------------|----------|
| **1. Direct Command Return** | ❌ High | ✅ Low | ❌ Low | Simple workflows |
| **2. Saga Coordinator** | ✅ Medium | ✅ Medium | ✅ High | Multi-step workflows |
| **3. Orchestrator** | ✅ Low | ❌ High | ✅ High | Complex workflows |
| **4. Process Manager** | ✅ Low | ❌ Very High | ✅ Very High | Long-running sagas |

---

## Pattern 1: Direct Command Return (Current Design)

### Architecture

```
Command → Event → Event Handler → Command (directly)
```

### Implementation

```go
// Command handler returns events
handleCreateOrder := NewCommandHandler(
    "CreateOrder",
    func(ctx, cmd CreateOrder) ([]OrderCreated, error) {
        return []OrderCreated{{...}}, nil
    },
)

// Event handler returns commands (COUPLED!)
handleOrderCreated := NewEventHandler(
    "OrderCreated",
    func(ctx, evt OrderCreated) ([]ChargePayment, error) {
        return []ChargePayment{{...}}, nil  // ❌ Coupling!
    },
)
```

### Pros
- ✅ Simple to understand
- ✅ Low ceremony
- ✅ Direct flow

### Cons
- ❌ **Tight coupling** between event handlers and commands
- ❌ Hard to add new saga steps
- ❌ Business logic in event handlers
- ❌ Testing requires knowing command types

### When to Use
- Simple workflows (2-3 steps max)
- Workflow won't change often
- Acceptable to couple event handlers to commands

---

## Pattern 2: Saga Coordinator (RECOMMENDED)

### Architecture

```
Command → Event → Saga Coordinator → Commands
                ↓
           Event Handler (side effects only)
```

**Key:** Separate "saga coordination" from "event handling"

### Implementation

```go
// ============================================================================
// Pure Event Handlers (No commands, just side effects)
// ============================================================================

type EmailHandler struct{}

func (h *EmailHandler) Handle(ctx context.Context, evt OrderCreated) error {
    log.Printf("📧 Sending confirmation email for order %s", evt.ID)
    return emailService.Send(evt.CustomerID, "Order Confirmation", ...)
}

type AnalyticsHandler struct{}

func (h *AnalyticsHandler) Handle(ctx context.Context, evt OrderCreated) error {
    log.Printf("📊 Tracking order creation: %s", evt.ID)
    return analyticsService.Track("order_created", evt)
}

// ============================================================================
// Saga Coordinator (Manages workflow logic)
// ============================================================================

type OrderSagaCoordinator struct {
    marshaler Marshaler
}

// OnEvent is called for each event, returns commands to trigger
func (s *OrderSagaCoordinator) OnEvent(ctx context.Context, msg *Message) ([]*Message, error) {
    subject, _ := msg.Properties.Subject()

    switch subject {
    case "OrderCreated":
        var evt OrderCreated
        json.Unmarshal(msg.Payload, &evt)

        log.Printf("🔄 Saga: OrderCreated → triggering ChargePayment")

        // Return next command in saga
        return s.createCommands(
            ChargePayment{
                OrderID:    evt.ID,
                CustomerID: evt.CustomerID,
                Amount:     evt.Amount,
            },
        )

    case "PaymentCharged":
        var evt PaymentCharged
        json.Unmarshal(msg.Payload, &evt)

        log.Printf("🔄 Saga: PaymentCharged → triggering ShipOrder")

        return s.createCommands(
            ShipOrder{
                OrderID: evt.OrderID,
                Address: "123 Main St",
            },
        )

    case "OrderShipped":
        // Terminal event - no more commands
        log.Printf("✅ Saga complete for order")
        return nil, nil
    }

    return nil, nil
}

func (s *OrderSagaCoordinator) createCommands(cmds ...any) ([]*Message, error) {
    var msgs []*Message
    for _, cmd := range cmds {
        payload, _ := json.Marshal(cmd)
        name := s.marshaler.Name(cmd)

        msgs = append(msgs, message.New(payload, message.Properties{
            message.PropSubject: name,
            "type":              "command",
        }))
    }
    return msgs, nil
}

// ============================================================================
// Wiring
// ============================================================================

// Event processor for side effects (email, analytics, etc.)
sideEffectsProcessor := NewEventProcessor(
    config,
    marshaler,
    NewEventHandler("OrderCreated", emailHandler.Handle),
    NewEventHandler("OrderCreated", analyticsHandler.Handle),
    NewEventHandler("PaymentCharged", paymentEmailHandler.Handle),
)

// Saga coordinator (workflow logic)
sagaCoordinator := &OrderSagaCoordinator{marshaler: marshaler}
sagaHandler := message.NewHandler(
    sagaCoordinator.OnEvent,
    func(prop Properties) bool {
        msgType, _ := prop["type"].(string)
        return msgType == "event"  // Reacts to all events
    },
)

sagaProcessor := message.NewRouter(config, sagaHandler)

// Wire together
events := commandProcessor.Start(ctx, allCommands)

// Split events to both processors
sideEffects := sideEffectsProcessor.Start(ctx, events)
sagaCommands := sagaProcessor.Start(ctx, events)

// Route saga commands back to command processor
go func() {
    for cmd := range sagaCommands {
        sagaCommands <- cmd
    }
}()
```

### Pros
- ✅ **Decoupled**: Event handlers don't know about commands
- ✅ **Clear separation**: Side effects vs workflow logic
- ✅ **Testable**: Test saga logic separately from event handlers
- ✅ **Flexible**: Easy to add new saga steps
- ✅ **Multistage acking**: One event can trigger multiple commands

### Cons
- ⚠️ Slightly more code
- ⚠️ Need to understand coordinator concept

### When to Use
- **Multi-step workflows** (3+ steps)
- Workflows that **change over time**
- Need to **test workflow logic** separately
- Want **clean separation** of concerns

---

## Pattern 3: Orchestrator (Central Control)

### Architecture

```
Orchestrator (state machine)
    ↓
Commands → Events → Back to Orchestrator
```

### Implementation

```go
type OrderOrchestrator struct {
    commandBus CommandBus
    state      map[string]*OrderState  // Track order states
}

type OrderState struct {
    OrderID     string
    Step        string  // "created", "payment_charged", "shipped"
    RetryCount  int
}

func (o *OrderOrchestrator) HandleCreateOrder(ctx context.Context, cmd CreateOrder) error {
    // Step 1: Create order
    orderID := cmd.ID
    o.state[orderID] = &OrderState{
        OrderID: orderID,
        Step:    "created",
    }

    // Trigger step 2
    return o.commandBus.Send(ctx, ChargePayment{
        OrderID: orderID,
        Amount:  cmd.Amount,
    })
}

func (o *OrderOrchestrator) OnPaymentCharged(ctx context.Context, evt PaymentCharged) error {
    state := o.state[evt.OrderID]
    state.Step = "payment_charged"

    // Trigger step 3
    return o.commandBus.Send(ctx, ShipOrder{
        OrderID: evt.OrderID,
    })
}

func (o *OrderOrchestrator) OnOrderShipped(ctx context.Context, evt OrderShipped) error {
    state := o.state[evt.OrderID]
    state.Step = "completed"

    log.Printf("✅ Order %s orchestration complete", evt.OrderID)
    delete(o.state, evt.OrderID)  // Cleanup
    return nil
}
```

### Pros
- ✅ **Central control**: Easy to understand entire workflow
- ✅ **State tracking**: Knows where each order is in the process
- ✅ **Error handling**: Can implement compensations centrally
- ✅ **No coupling**: Services don't know about each other

### Cons
- ❌ **Single point of failure**: Orchestrator must be reliable
- ❌ **Scalability**: Orchestrator can become bottleneck
- ❌ **Complexity**: Need to manage state
- ❌ **Less flexible**: Changes require updating orchestrator

### When to Use
- **Complex workflows** with many steps
- Need **centralized error handling**
- Need **visibility** into workflow state
- Acceptable to have central coordination point

---

## Pattern 4: Process Manager (Advanced)

### Architecture

```
Process Manager (stateful saga coordinator)
    ↓
Maintains saga state across events
    ↓
Triggers commands based on state machine
```

### Implementation

```go
type OrderProcessManager struct {
    store SagaStore  // Persistent state store
}

type OrderSagaState struct {
    SagaID          string
    OrderID         string
    CurrentStep     string
    CompletedSteps  []string
    FailedSteps     []string
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

func (pm *OrderProcessManager) HandleEvent(ctx context.Context, evt any) error {
    // Load saga state
    sagaID := extractSagaID(evt)
    state, err := pm.store.Load(sagaID)
    if err != nil {
        return err
    }

    // State machine transition
    switch state.CurrentStep {
    case "order_created":
        if _, ok := evt.(OrderCreated); ok {
            state.CurrentStep = "charging_payment"
            state.CompletedSteps = append(state.CompletedSteps, "order_created")

            // Trigger next command
            commandBus.Send(ctx, ChargePayment{...})
        }

    case "charging_payment":
        if _, ok := evt.(PaymentCharged); ok {
            state.CurrentStep = "shipping"
            state.CompletedSteps = append(state.CompletedSteps, "payment_charged")

            commandBus.Send(ctx, ShipOrder{...})

        } else if _, ok := evt.(PaymentFailed); ok {
            // Compensation: Cancel order
            state.FailedSteps = append(state.FailedSteps, "payment_failed")
            commandBus.Send(ctx, CancelOrder{...})
        }

    case "shipping":
        if _, ok := evt.(OrderShipped); ok {
            state.CurrentStep = "completed"
            state.CompletedSteps = append(state.CompletedSteps, "shipped")
        }
    }

    // Persist state
    return pm.store.Save(state)
}
```

### Pros
- ✅ **Stateful**: Can resume after failures
- ✅ **Compensations**: Can undo failed operations
- ✅ **Complex workflows**: Handles branches and loops
- ✅ **Durability**: State persisted across restarts

### Cons
- ❌ **Very complex**: Requires state store
- ❌ **Performance**: Database overhead
- ❌ **Operational**: Need to manage saga state
- ❌ **Overkill**: For simple workflows

### When to Use
- **Long-running sagas** (hours/days)
- Need **compensation logic** (undo operations)
- **Distributed transactions** across services
- Must **survive process restarts**

---

## Multistage Acking Example

Your insight about multistage acking is spot-on! Here's how it works with Saga Coordinator:

```go
type OrderSagaCoordinator struct {
    marshaler Marshaler
}

func (s *OrderSagaCoordinator) OnEvent(ctx context.Context, msg *Message) ([]*Message, error) {
    subject, _ := msg.Properties.Subject()

    if subject == "OrderCreated" {
        var evt OrderCreated
        json.Unmarshal(msg.Payload, &evt)

        log.Printf("🔄 Saga: OrderCreated → triggering MULTIPLE commands")

        // ✅ One event triggers MULTIPLE commands!
        return s.createCommands(
            ChargePayment{OrderID: evt.ID, Amount: evt.Amount},
            ReserveInventory{OrderID: evt.ID, SKU: evt.SKU},
            NotifyWarehouse{OrderID: evt.ID},
        )
    }

    return nil, nil
}
```

**Flow:**
```
OrderCreated event
    ↓
Saga Coordinator
    ↓
├─→ ChargePayment command
├─→ ReserveInventory command
└─→ NotifyWarehouse command
```

All three commands execute concurrently!

---

## Recommendation

For gopipe, I recommend **Pattern 2: Saga Coordinator** because:

1. ✅ **Decoupled**: Event handlers are pure side effects
2. ✅ **Testable**: Saga logic is isolated
3. ✅ **gopipe-friendly**: Uses channels naturally
4. ✅ **Flexible**: Easy to extend
5. ✅ **Right complexity**: Not too simple, not too complex

### API Design for Saga Coordinator

```go
package cqrs

// SagaCoordinator converts events to commands
type SagaCoordinator interface {
    OnEvent(ctx context.Context, msg *Message) ([]*Message, error)
}

// NewSagaHandler creates a handler from a saga coordinator
func NewSagaHandler(coordinator SagaCoordinator) Handler {
    return message.NewHandler(
        coordinator.OnEvent,
        func(prop Properties) bool {
            msgType, _ := prop["type"].(string)
            return msgType == "event"
        },
    )
}
```

---

## Comparison Table

| Aspect | Direct Return | Saga Coordinator | Orchestrator | Process Manager |
|--------|--------------|------------------|--------------|-----------------|
| **Coupling** | ❌ High | ✅ Low | ✅ Low | ✅ Low |
| **Testability** | ⚠️ Medium | ✅ High | ✅ High | ✅ High |
| **Flexibility** | ❌ Low | ✅ High | ⚠️ Medium | ✅ Very High |
| **Complexity** | ✅ Low | ✅ Medium | ⚠️ High | ❌ Very High |
| **State Management** | ❌ None | ⚠️ In-memory | ⚠️ In-memory | ✅ Persistent |
| **Compensations** | ❌ No | ⚠️ Manual | ✅ Yes | ✅ Yes |
| **Scalability** | ✅ High | ✅ High | ⚠️ Limited | ⚠️ Limited |
| **Use Case** | Simple | Multi-step | Complex | Distributed |

---

## References

- [Saga Pattern](https://microservices.io/patterns/data/saga.html)
- [Process Manager vs Saga](https://blog.bernd-ruecker.com/saga-how-to-implement-complex-business-transactions-without-two-phase-commit-e00aa41a1b1b)
- [Choreography vs Orchestration](https://www.tenupsoft.com/blog/The-importance-of-cqrs-and-saga-in-microservices-architecture.html)

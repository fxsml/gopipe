# CQRS Design Analysis

## Issues with Current Proposal

### 1. **Violates gopipe's Channel Pattern**
Current design has buses *consuming* channels instead of *returning* them:
```go
// ❌ Current (anti-pattern)
commandBus := NewCommandBus(marshaler, outputChan)
commandBus.Send(ctx, cmd) // sends to channel

// ✅ Should be (gopipe pattern)
commandBus := NewCommandBus(marshaler, inputChan)
output := commandBus.Start(ctx) // returns channel
```

### 2. **Command Handlers Shouldn't Call EventBus**
Current design couples command handlers to EventBus:
```go
// ❌ Current (tight coupling)
handleCreateOrder := func(ctx context.Context, cmd CreateOrder) error {
    saveOrder(cmd)
    return eventBus.Publish(ctx, OrderCreated{...}) // BAD!
}
```

**Problem**: Handler now depends on EventBus, making it hard to test and compose.

**Better**: Command handlers should *return* events:
```go
// ✅ Better (decoupled)
handleCreateOrder := func(ctx context.Context, cmd CreateOrder) ([]Event, error) {
    saveOrder(cmd)
    return []Event{OrderCreated{...}}, nil // Returns events
}
```

### 3. **No Support for Saga Pattern**
Current design doesn't support event → command chains:

```
Command1 → Event1 → (need to trigger Command2) → Event2
```

This is the **Saga Pattern** (choreography):
- Event handlers react to events
- Event handlers can trigger new commands
- Creates feedback loop: Commands → Events → Commands

### 4. **Pattern Name: Event-Driven Choreography**
The pattern you're describing is:
- **Saga Pattern** (for distributed transactions)
- **Event Choreography** (services react to events autonomously)
- **Process Manager** (coordinates multi-step processes)

In CQRS terms:
- Commands → produce → Events
- Events → trigger → Commands (via saga/event handlers)

## Correct Architecture

### Channel Flow Pattern

```
Commands In → CommandProcessor → Events Out
                                      ↓
                             EventProcessor → Commands Out
                                      ↓
                             (feed back to CommandProcessor)
```

### API Design

```go
// CommandProcessor: Commands → Events
type CommandProcessor struct {
    router *message.Router
}

func (p *CommandProcessor) Start(ctx context.Context, commands <-chan *Message) <-chan *Message {
    // Process commands, return events
    return p.router.Start(ctx, commands)
}

// EventProcessor: Events → Commands (optional, for sagas)
type EventProcessor struct {
    router *message.Router
}

func (p *EventProcessor) Start(ctx context.Context, events <-chan *Message) <-chan *Message {
    // Process events, return commands (or nil for terminal handlers)
    return p.router.Start(ctx, events)
}

// Command handler returns events
func NewCommandHandler[Cmd, Evt any](
    name string,
    handle func(ctx context.Context, cmd Cmd) ([]Evt, error),
) Handler {
    // Handler unmarshals command, calls handle, marshals events
}

// Event handler can return commands (saga pattern)
func NewEventHandler[Evt, Cmd any](
    name string,
    handle func(ctx context.Context, evt Evt) ([]Cmd, error),
) Handler {
    // Handler unmarshals event, calls handle, marshals commands
}
```

### Usage: Simple Flow

```go
// Commands → Events (no saga)
commandProcessor := cqrs.NewCommandProcessor(
    config,
    cqrs.NewCommandHandler("CreateOrder",
        func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
            saveOrder(cmd)
            return []OrderCreated{{ID: cmd.ID, ...}}, nil
        }),
)

eventProcessor := cqrs.NewEventProcessor(
    config,
    cqrs.NewEventHandler("OrderCreated",
        func(ctx context.Context, evt OrderCreated) ([]any, error) {
            sendEmail(evt)
            return nil, nil // Terminal handler
        }),
)

// Wire together
commandChan := getCommandsFromQueue()
events := commandProcessor.Start(ctx, commandChan)
eventProcessor.Start(ctx, events) // Terminal
```

### Usage: Saga Pattern

```go
// Commands → Events → Commands (saga)
commandProcessor := cqrs.NewCommandProcessor(
    config,
    cqrs.NewCommandHandler("CreateOrder",
        func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
            return []OrderCreated{{...}}, nil
        }),
    cqrs.NewCommandHandler("ChargePayment",
        func(ctx context.Context, cmd ChargePayment) ([]PaymentCharged, error) {
            return []PaymentCharged{{...}}, nil
        }),
)

eventProcessor := cqrs.NewEventProcessor(
    config,
    // Saga: OrderCreated → ChargePayment command
    cqrs.NewEventHandler("OrderCreated",
        func(ctx context.Context, evt OrderCreated) ([]ChargePayment, error) {
            return []ChargePayment{{OrderID: evt.ID, ...}}, nil
        }),
    // Terminal: Just send email
    cqrs.NewEventHandler("PaymentCharged",
        func(ctx context.Context, evt PaymentCharged) ([]any, error) {
            sendEmail(evt)
            return nil, nil
        }),
)

// Wire with feedback loop
commandChan := getCommandsFromQueue()
events := commandProcessor.Start(ctx, commandChan)
newCommands := eventProcessor.Start(ctx, events)

// Merge original commands + saga-triggered commands
allCommands := channel.Merge(commandChan, newCommands)
events = commandProcessor.Start(ctx, allCommands) // Feedback loop!
```

## Benefits of Revised Design

1. **Follows gopipe patterns**: Processors return channels
2. **Decoupled**: Command handlers don't depend on EventBus
3. **Testable**: Easy to test handlers in isolation
4. **Saga support**: Natural feedback loop via channels
5. **Composable**: Can wire processors in different topologies
6. **Type-safe**: Handlers are strongly typed

## Comparison

| Aspect | Original Design | Revised Design |
|--------|----------------|----------------|
| Channel pattern | ❌ Buses consume channels | ✅ Processors return channels |
| Command handlers | ❌ Call EventBus | ✅ Return events |
| Event handlers | ❌ No command output | ✅ Can return commands |
| Saga support | ❌ Not supported | ✅ Natural feedback loop |
| Coupling | ❌ Handlers depend on buses | ✅ Handlers are pure functions |
| Testing | ❌ Need to mock buses | ✅ Test handlers directly |

## References

- [Saga Pattern](https://microservices.io/patterns/data/saga.html)
- [Event Choreography](https://www.tenupsoft.com/blog/The-importance-of-cqrs-and-saga-in-microservices-architecture.html)
- [CQRS with Sagas](https://medium.com/@ingila185/cqrs-and-saga-the-essential-patterns-for-high-performance-microservice-4f23a09889b4)

# Pattern: CQRS (Command Query Responsibility Segregation)

## Intent

Separate command handling (writes) from query handling (reads) using typed message handlers.

## When to Use

- Complex domain logic requiring command/event separation
- Event-driven architectures with CloudEvents
- Audit trail requirements

## Implementation

### Command Handler

The `NewCommandHandler` receives typed commands and returns typed events:

```go
// Define command and event types
type CreateOrder struct {
    OrderID string  `json:"order_id"`
    Amount  float64 `json:"amount"`
}

type OrderCreated struct {
    OrderID string `json:"order_id"`
    Status  string `json:"status"`
}

// Create handler: Command -> Events
handler := message.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        if cmd.Amount <= 0 {
            return nil, errors.New("invalid amount")
        }
        return []OrderCreated{{
            OrderID: cmd.OrderID,
            Status:  "created",
        }}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders",
        Naming: message.KebabNaming, // CreateOrder -> "create.order"
    },
)
```

### Event Handler

For event handlers (side effects only), use `NewHandler` with the `*Message` API:

```go
handler := message.NewHandler[OrderCreated](
    func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        evt := msg.Data.(*OrderCreated)
        // Update read model, send notifications, etc.
        log.Printf("Order %s created", evt.OrderID)
        return nil, nil // No output messages
    },
    message.KebabNaming,
)
```

### Engine Integration

```go
engine := message.NewEngine(message.EngineConfig{
    Marshaler: message.NewJSONMarshaler(),
})

engine.AddHandler("create-order", nil, createOrderHandler)
engine.AddHandler("order-projection", nil, orderCreatedHandler)

input := make(chan *message.RawMessage)
engine.AddRawInput("commands", nil, input)
output, _ := engine.AddRawOutput("events", nil)

done, _ := engine.Start(ctx)
```

## Naming Conventions

| Type | Convention | Examples |
|------|------------|----------|
| Command | Imperative verb + noun | CreateOrder, CancelPayment |
| Event | Noun + past participle | OrderCreated, PaymentCancelled |

## Related ADRs

- [ADR 0011: CQRS Implementation](../adr/0011-cqrs-implementation.md)
- [ADR 0020: Message Engine Architecture](../adr/0020-message-engine-architecture.md)

## Further Reading

- Martin Fowler: [CQRS](https://martinfowler.com/bliki/CQRS.html)

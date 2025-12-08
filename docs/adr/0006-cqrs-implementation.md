# ADR 0006: CQRS Implementation

**Date:** 2025-12-08
**Status:** Proposed

## Context

Command Query Responsibility Segregation (CQRS) is a widely-used pattern in event-driven architectures that separates read and write operations. Watermill, a popular Go messaging library, provides a CQRS component that simplifies building event-driven applications.

gopipe currently has a strong foundation for CQRS with:
- Type-safe message routing via `message.Router`
- Generic handlers via `message.NewHandler[In, Out any]()`
- Property-based message matching
- Built-in JSON marshaling support

However, gopipe lacks explicit CQRS semantics and convenience APIs that make the pattern easy to adopt.

### Research: Watermill's CQRS Approach

Watermill's CQRS implementation ([docs](https://watermill.io/docs/cqrs/), [examples](https://github.com/ThreeDotsLabs/watermill/blob/master/_examples/basic/5-cqrs-protobuf/main.go)) provides:

1. **CommandBus** - For sending commands (imperative requests)
2. **EventBus** - For publishing events (declarative facts)
3. **CommandProcessor** / **EventProcessor** - Route messages to handlers
4. **Marshaler Interface** - Pluggable serialization (JSON, Protobuf, etc.)
5. **Topic Generation** - Automatic topic naming from types
6. **Handler Registration** - Type-safe handler functions

**Key Distinctions:**
- **Commands**: Imperative ("BookRoom"), single handler, request for action
- **Events**: Past tense ("RoomBooked"), multiple handlers, notification

**Example from Watermill:**
```go
commandBus, _ := cqrs.NewCommandBusWithConfig(publisher,
  cqrs.CommandBusConfig{
    GeneratePublishTopic: func(params cqrs.CommandBusGeneratePublishTopicParams)
      (string, error) {
      return params.CommandName, nil
    },
    Marshaler: cqrs.JSONMarshaler{},
  })

eventBus, _ := cqrs.NewEventBusWithConfig(publisher,
  cqrs.EventBusConfig{
    GeneratePublishTopic: func(params cqrs.GenerateEventPublishTopicParams)
      (string, error) {
      return params.EventName, nil
    },
    Marshaler: cqrs.JSONMarshaler{},
  })

// Send command
commandBus.Send(ctx, &BookRoom{RoomID: "123"})

// Publish event
eventBus.Publish(ctx, &RoomBooked{RoomID: "123"})
```

## Decision

**Implement a dedicated `cqrs` package** that builds on gopipe's `message` package, providing CQRS-specific semantics while maintaining gopipe's philosophy of simplicity and type safety.

### Design Principles

1. **Type-Safe**: Use generics for commands and events
2. **Simple**: Leverage existing `message.Router` and `message.Handler`
3. **Flexible**: Support custom marshalers via interface
4. **Explicit**: Clear distinction between commands and events
5. **Minimal**: Don't over-engineer - provide just what's needed

### Architecture

```
cqrs/
├── bus.go           # CommandBus and EventBus
├── handler.go       # Command and Event handler builders
├── marshaler.go     # Marshaler interface and implementations
├── processor.go     # CommandProcessor and EventProcessor
└── naming.go        # Name generation utilities
```

### Core Types

```go
package cqrs

// Marshaler serializes commands/events to messages
type Marshaler interface {
    // Marshal converts a command/event to message payload
    Marshal(v any) ([]byte, error)

    // Unmarshal converts message payload to command/event
    Unmarshal(data []byte, v any) error

    // Name returns the name for a command/event type
    Name(v any) string
}

// CommandBus sends commands to handlers
type CommandBus struct {
    marshaler Marshaler
    send      func(ctx context.Context, name string, payload []byte,
                   props message.Properties) error
}

// EventBus publishes events to subscribers
type EventBus struct {
    marshaler Marshaler
    publish   func(ctx context.Context, name string, payload []byte,
                   props message.Properties) error
}

// CommandProcessor routes commands to handlers
type CommandProcessor struct {
    router *message.Router
}

// EventProcessor routes events to handlers
type EventProcessor struct {
    router *message.Router
}
```

### API Design

#### 1. Creating Buses

```go
// CommandBus for sending commands
commandBus := cqrs.NewCommandBus(
    cqrs.JSONMarshaler{},
    func(ctx context.Context, name string, payload []byte,
         props message.Properties) error {
        // Send to message broker/channel
        msg := message.New(payload, props)
        return publisher.Send(ctx, name, msg)
    },
)

// EventBus for publishing events
eventBus := cqrs.NewEventBus(
    cqrs.JSONMarshaler{},
    func(ctx context.Context, name string, payload []byte,
         props message.Properties) error {
        // Publish to message broker/channel
        msg := message.New(payload, props)
        return publisher.Publish(ctx, name, msg)
    },
)
```

#### 2. Sending Commands

```go
type CreateOrder struct {
    ID       string
    Amount   int
    CustomerID string
}

// Send command
err := commandBus.Send(ctx, CreateOrder{
    ID:       "order-123",
    Amount:   100,
    CustomerID: "cust-456",
})
```

#### 3. Publishing Events

```go
type OrderCreated struct {
    ID          string
    Amount      int
    CustomerID  string
    CreatedAt   time.Time
}

// Publish event
err := eventBus.Publish(ctx, OrderCreated{
    ID:         "order-123",
    Amount:     100,
    CustomerID: "cust-456",
    CreatedAt:  time.Now(),
})
```

#### 4. Handling Commands

```go
// Command handler - single responsibility
handleCreateOrder := cqrs.NewCommandHandler(
    "CreateOrder",
    func(ctx context.Context, cmd CreateOrder) error {
        // Process command
        order := createOrder(cmd)

        // Publish event
        return eventBus.Publish(ctx, OrderCreated{
            ID:         order.ID,
            Amount:     order.Amount,
            CustomerID: order.CustomerID,
            CreatedAt:  time.Now(),
        })
    },
)

processor := cqrs.NewCommandProcessor(
    message.RouterConfig{
        Concurrency: 10,
        Recover:     true,
    },
    handleCreateOrder,
)

processor.Start(ctx, commandChannel)
```

#### 5. Handling Events

```go
// Event handler 1 - send confirmation email
handleOrderCreatedEmail := cqrs.NewEventHandler(
    "OrderCreated.Email",
    func(ctx context.Context, evt OrderCreated) error {
        return emailService.SendOrderConfirmation(evt.CustomerID, evt.ID)
    },
)

// Event handler 2 - update analytics
handleOrderCreatedAnalytics := cqrs.NewEventHandler(
    "OrderCreated.Analytics",
    func(ctx context.Context, evt OrderCreated) error {
        return analyticsService.TrackOrder(evt.ID, evt.Amount)
    },
)

processor := cqrs.NewEventProcessor(
    message.RouterConfig{
        Concurrency: 20,
        Recover:     true,
    },
    handleOrderCreatedEmail,
    handleOrderCreatedAnalytics,
)

processor.Start(ctx, eventChannel)
```

#### 6. Custom Marshalers

```go
// JSONMarshaler (built-in)
type JSONMarshaler struct{}

func (m JSONMarshaler) Marshal(v any) ([]byte, error) {
    return json.Marshal(v)
}

func (m JSONMarshaler) Unmarshal(data []byte, v any) error {
    return json.Unmarshal(data, v)
}

func (m JSONMarshaler) Name(v any) string {
    return reflect.TypeOf(v).Name()
}

// ProtobufMarshaler (user-provided)
type ProtobufMarshaler struct{}

func (m ProtobufMarshaler) Marshal(v any) ([]byte, error) {
    msg, ok := v.(proto.Message)
    if !ok {
        return nil, fmt.Errorf("not a proto message")
    }
    return proto.Marshal(msg)
}

func (m ProtobufMarshaler) Unmarshal(data []byte, v any) error {
    msg, ok := v.(proto.Message)
    if !ok {
        return fmt.Errorf("not a proto message")
    }
    return proto.Unmarshal(data, msg)
}

func (m ProtobufMarshaler) Name(v any) string {
    return string(proto.MessageName(v.(proto.Message)))
}
```

## Comparison: Watermill vs gopipe

### Watermill (Complex)

```go
// Configuration heavy
commandBus, err := cqrs.NewCommandBusWithConfig(
    commandsPublisher,
    cqrs.CommandBusConfig{
        GeneratePublishTopic: func(params cqrs.CommandBusGeneratePublishTopicParams)
          (string, error) {
            return generateCommandsTopic(params.CommandName), nil
        },
        Marshaler: cqrsMarshaler,
        Logger: logger,
    })

// Handler registration
err = commandProcessor.AddHandlers(
    cqrs.NewCommandHandler("BookRoomHandler",
        BookRoomHandler{eventBus}.Handle),
)
```

### gopipe CQRS (Simple)

```go
// Simple construction
commandBus := cqrs.NewCommandBus(
    cqrs.JSONMarshaler{},
    publisher.Send,
)

// Handler registration
processor := cqrs.NewCommandProcessor(
    message.RouterConfig{Concurrency: 10},
    cqrs.NewCommandHandler("BookRoom", handleBookRoom),
)
```

## Benefits

### 1. Clear CQRS Semantics

```go
// Clear intent: this is a command
commandBus.Send(ctx, CreateOrder{...})

// Clear intent: this is an event
eventBus.Publish(ctx, OrderCreated{...})
```

### 2. Type Safety

```go
// Type-safe handlers
cqrs.NewCommandHandler(
    "CreateOrder",
    func(ctx context.Context, cmd CreateOrder) error {
        // cmd is strongly typed
        return processOrder(cmd.ID, cmd.Amount)
    },
)
```

### 3. Decoupling

Events can have multiple independent handlers:

```go
// Handler 1: Email
cqrs.NewEventHandler("OrderCreated.Email", sendEmail)

// Handler 2: Analytics
cqrs.NewEventHandler("OrderCreated.Analytics", trackAnalytics)

// Handler 3: Inventory
cqrs.NewEventHandler("OrderCreated.Inventory", updateInventory)
```

### 4. Testability

```go
// Easy to test command handlers in isolation
func TestHandleCreateOrder(t *testing.T) {
    mockBus := &MockEventBus{}
    handler := NewCreateOrderHandler(mockBus)

    err := handler.Handle(ctx, CreateOrder{
        ID: "test-order",
        Amount: 100,
    })

    if err != nil {
        t.Fatal(err)
    }

    // Verify event was published
    if !mockBus.Published("OrderCreated") {
        t.Error("Expected OrderCreated event")
    }
}
```

### 5. Builds on gopipe Foundation

Leverages existing gopipe features:
- Message routing
- Concurrency control
- Retry logic
- Recovery
- Metrics and logging

## Implementation Phases

### Phase 1: Core CQRS Package

- [ ] `cqrs.Marshaler` interface
- [ ] `cqrs.JSONMarshaler` implementation
- [ ] `cqrs.CommandBus` and `cqrs.EventBus`
- [ ] `cqrs.NewCommandHandler` and `cqrs.NewEventHandler`
- [ ] `cqrs.CommandProcessor` and `cqrs.EventProcessor`

### Phase 2: Documentation & Examples

- [ ] Complete example: Order processing with CQRS
- [ ] Example: Event sourcing pattern
- [ ] Example: Saga pattern
- [ ] Migration guide from direct message.Router usage

### Phase 3: Advanced Features (Future)

- [ ] Event sourcing support
- [ ] Saga/process manager support
- [ ] Command validation
- [ ] Event versioning
- [ ] Replay capabilities

## Example: Complete Order Processing System

```go
package main

import (
    "context"
    "time"

    "github.com/fxsml/gopipe/cqrs"
    "github.com/fxsml/gopipe/message"
)

// Commands (imperative)
type CreateOrder struct {
    ID         string
    CustomerID string
    Amount     int
}

type CancelOrder struct {
    ID     string
    Reason string
}

// Events (past tense)
type OrderCreated struct {
    ID         string
    CustomerID string
    Amount     int
    CreatedAt  time.Time
}

type OrderCancelled struct {
    ID          string
    Reason      string
    CancelledAt time.Time
}

type EmailSent struct {
    CustomerID string
    OrderID    string
    Type       string
}

func main() {
    ctx := context.Background()

    // Setup buses
    marshaler := cqrs.JSONMarshaler{}

    commandBus := cqrs.NewCommandBus(marshaler, publishCommand)
    eventBus := cqrs.NewEventBus(marshaler, publishEvent)

    // Command handlers
    createOrderHandler := cqrs.NewCommandHandler(
        "CreateOrder",
        func(ctx context.Context, cmd CreateOrder) error {
            // Validate and process
            if cmd.Amount <= 0 {
                return fmt.Errorf("invalid amount")
            }

            // Store order
            db.CreateOrder(cmd.ID, cmd.CustomerID, cmd.Amount)

            // Publish event
            return eventBus.Publish(ctx, OrderCreated{
                ID:         cmd.ID,
                CustomerID: cmd.CustomerID,
                Amount:     cmd.Amount,
                CreatedAt:  time.Now(),
            })
        },
    )

    cancelOrderHandler := cqrs.NewCommandHandler(
        "CancelOrder",
        func(ctx context.Context, cmd CancelOrder) error {
            // Cancel order
            db.CancelOrder(cmd.ID)

            // Publish event
            return eventBus.Publish(ctx, OrderCancelled{
                ID:          cmd.ID,
                Reason:      cmd.Reason,
                CancelledAt: time.Now(),
            })
        },
    )

    // Event handlers
    sendEmailHandler := cqrs.NewEventHandler(
        "OrderCreated.SendEmail",
        func(ctx context.Context, evt OrderCreated) error {
            err := emailService.Send(
                evt.CustomerID,
                "Order Confirmation",
                fmt.Sprintf("Your order %s has been created", evt.ID),
            )
            if err != nil {
                return err
            }

            // Publish email sent event
            return eventBus.Publish(ctx, EmailSent{
                CustomerID: evt.CustomerID,
                OrderID:    evt.ID,
                Type:       "confirmation",
            })
        },
    )

    updateAnalyticsHandler := cqrs.NewEventHandler(
        "OrderCreated.Analytics",
        func(ctx context.Context, evt OrderCreated) error {
            return analyticsService.Track("order_created", map[string]any{
                "order_id":    evt.ID,
                "customer_id": evt.CustomerID,
                "amount":      evt.Amount,
            })
        },
    )

    // Start processors
    commandProcessor := cqrs.NewCommandProcessor(
        message.RouterConfig{
            Concurrency: 10,
            Recover:     true,
        },
        createOrderHandler,
        cancelOrderHandler,
    )

    eventProcessor := cqrs.NewEventProcessor(
        message.RouterConfig{
            Concurrency: 20,
            Recover:     true,
        },
        sendEmailHandler,
        updateAnalyticsHandler,
    )

    go commandProcessor.Start(ctx, commandChannel)
    go eventProcessor.Start(ctx, eventChannel)

    // Send a command
    err := commandBus.Send(ctx, CreateOrder{
        ID:         "order-123",
        CustomerID: "customer-456",
        Amount:     100,
    })
}
```

## Breaking Changes

None - this is a new package that builds on existing `message` package APIs.

## Alternatives Considered

### Alternative 1: Helper Functions in message Package

Add `message.NewCommandHandler()` and `message.NewEventHandler()` as convenience functions.

**Rejected because:**
- Pollutes the `message` package with CQRS-specific concepts
- Harder to extend with CQRS-specific features later
- Less clear separation of concerns

### Alternative 2: event Package (not cqrs)

Create an `event` package focused only on events, not commands.

**Rejected because:**
- Commands and events are complementary concepts
- Users typically need both for CQRS
- CQRS is a well-known pattern name

### Alternative 3: Copy Watermill's API Exactly

Replicate Watermill's configuration-heavy approach.

**Rejected because:**
- Goes against gopipe's simplicity philosophy
- Too much ceremony for basic use cases
- gopipe already has Router configuration

## Migration Path

Users currently using `message.Router` directly can migrate incrementally:

**Before** (direct Router usage):
```go
handler := message.NewHandler(
    func(ctx context.Context, order Order) ([]OrderConfirmed, error) {
        // ...
    },
    func(prop map[string]any) bool {
        subject, _ := message.SubjectProps(prop)
        return subject == "orders.create"
    },
    func(prop map[string]any) map[string]any {
        return map[string]any{message.PropSubject: "orders.confirmed"}
    },
)
```

**After** (CQRS):
```go
handler := cqrs.NewCommandHandler(
    "CreateOrder",
    func(ctx context.Context, cmd CreateOrder) error {
        // ...
        return eventBus.Publish(ctx, OrderConfirmed{...})
    },
)
```

## References

- [Watermill CQRS Component](https://watermill.io/docs/cqrs/)
- [How to use basic CQRS in Go](https://threedots.tech/post/basic-cqrs-in-go/)
- [Watermill CQRS Examples](https://github.com/ThreeDotsLabs/watermill/blob/master/_examples/basic/5-cqrs-protobuf/main.go)
- [ADR 0005: Remove Functional Options](./0005-remove-functional-options.md)
- Microsoft: [CQRS Pattern](https://learn.microsoft.com/en-us/azure/architecture/patterns/cqrs)

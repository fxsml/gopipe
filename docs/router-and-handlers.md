# Router and Handlers

This document describes the message routing system in gopipe, including Routers, Handlers, Pipes, and Matchers.

## Overview

The routing system allows you to dispatch messages to appropriate handlers based on message attributes. It consists of:

- **Router**: Dispatches messages to handlers and pipes based on attribute matching
- **Handler**: Processes messages and returns output messages
- **Pipe**: A composable processing unit that can be added to routers
- **Matcher**: Functions that determine which messages a handler/pipe should process

## Package Structure

```
message/
├── router.go       # Router, RouterConfig, NewRouter
├── handler.go      # Handler interface, NewHandler
├── pipe.go         # Pipe interface, NewPipe
└── matcher.go      # Matcher type, Match combinators

message/cqrs/
├── handler.go      # NewCommandHandler, NewEventHandler
├── pipe.go         # NewCommandPipe
└── matcher.go      # MatchGenericTypeOf
```

## Router

The Router dispatches incoming messages to registered handlers and pipes.

### Creating a Router

```go
import "github.com/fxsml/gopipe/message"

router := message.NewRouter(message.RouterConfig{
    Concurrency: 10,           // Process up to 10 messages concurrently
    Timeout:     time.Second,  // Timeout per message
    Recover:     true,         // Recover from panics
})
```

### RouterConfig Options

| Option | Type | Description |
|--------|------|-------------|
| `Concurrency` | `int` | Number of concurrent message processors (0 = sequential) |
| `Timeout` | `time.Duration` | Timeout for processing each message |
| `Retry` | `*gopipe.RetryConfig` | Retry configuration for failed messages |
| `Recover` | `bool` | If true, recover from panics in handlers |
| `Middleware` | `[]MiddlewareFunc` | Middleware to apply to all handlers |

### Adding Handlers

```go
handler := message.NewHandler(
    func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        // Process message
        return []*message.Message{msg}, nil
    },
    message.MatchSubject("orders"),
)

router.AddHandler(handler)
```

### Adding Pipes

```go
pipe := gopipe.NewTransformPipe(func(ctx context.Context, msg *message.Message) (*message.Message, error) {
    // Transform message
    return msg, nil
})

router.AddPipe(message.NewPipe(pipe, message.MatchSubject("events")))
```

### Starting the Router

```go
ctx := context.Background()
input := make(chan *message.Message)
output := router.Start(ctx, input)

// Send messages to input, read results from output
```

### Message Routing Priority

1. **Pipes are checked first** - if a message matches a pipe, it goes to that pipe
2. **Handlers are checked second** - if no pipe matches, handlers are checked
3. **No match** - if nothing matches, the message is nacked with "no handler matched"

## Handlers

Handlers process messages and optionally return output messages.

### Handler Interface

```go
type Handler interface {
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
    Match(attrs Attributes) bool
}
```

### Creating a Handler

```go
handler := message.NewHandler(
    func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        // Your processing logic here

        // Return output messages (can be empty)
        return []*message.Message{outputMsg}, nil
    },
    message.MatchSubject("my-subject"), // Matcher function
)
```

### Handler Responsibilities

- **Return output messages**: Zero or more messages to be sent downstream
- **Ack/Nack**: Call `msg.Ack()` on success or `msg.Nack(err)` on failure
- **Error handling**: Return error to indicate processing failure

### Example: Echo Handler

```go
echoHandler := message.NewHandler(
    func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        msg.Ack()
        return []*message.Message{msg}, nil
    },
    func(attrs message.Attributes) bool { return true }, // Match all
)
```

## CQRS Handlers

The `cqrs` package provides type-safe command and event handlers.

### Command Handler

Command handlers process commands and return events:

```go
import "github.com/fxsml/gopipe/message/cqrs"

type CreateOrder struct {
    ID     string
    Amount int
}

type OrderCreated struct {
    ID        string
    CreatedAt time.Time
}

marshaler := cqrs.NewJSONCommandMarshaler(cqrs.WithTypeOf())

handler := cqrs.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        // Business logic
        return []OrderCreated{{
            ID:        cmd.ID,
            CreatedAt: time.Now(),
        }}, nil
    },
    cqrs.Match(
        cqrs.MatchSubject("CreateOrder"),
        cqrs.MatchType("command"),
    ),
    marshaler,
)
```

### Event Handler

Event handlers process events for side effects (no output):

```go
handler := cqrs.NewEventHandler(
    func(ctx context.Context, evt OrderCreated) error {
        // Side effect: send email, log, etc.
        return emailService.Send(evt.ID)
    },
    cqrs.Match(
        cqrs.MatchSubject("OrderCreated"),
        cqrs.MatchType("event"),
    ),
    cqrs.NewJSONEventMarshaler(),
)
```

## Pipes

Pipes are composable processing units that integrate with the router.

### Pipe Interface

```go
type Pipe interface {
    gopipe.Pipe[*Message, *Message]
    Match(attrs Attributes) bool
}
```

### Creating a Pipe

```go
// Create a gopipe pipe
innerPipe := gopipe.NewTransformPipe(func(ctx context.Context, msg *message.Message) (*message.Message, error) {
    // Transform logic
    return msg, nil
})

// Wrap with matcher for use in router
pipe := message.NewPipe(innerPipe, message.MatchSubject("transform"))

router.AddPipe(pipe)
```

### CQRS Command Pipe

For type-safe command processing:

```go
// Create a typed pipe
typedPipe := gopipe.NewTransformPipe(func(ctx context.Context, order Order) (Order, error) {
    order.Amount *= 2
    return order, nil
})

// Wrap for message router
commandPipe := cqrs.NewCommandPipe(typedPipe, cqrs.MatchSubject("Order"), marshaler)

router.AddPipe(commandPipe)
```

## Matchers

Matchers determine which messages a handler or pipe should process.

### Built-in Matchers

```go
// Match by subject
message.MatchSubject("orders")

// Match by type
message.MatchType("command")

// Match by topic
message.MatchTopic("events.orders")

// Match by any attribute
message.MatchAttribute("priority", "high")

// Match if attribute exists
message.MatchHasAttribute("correlation_id")
```

### Combining Matchers

```go
// AND logic - all must match
message.Match(
    message.MatchSubject("CreateOrder"),
    message.MatchType("command"),
)
```

### Custom Matchers

```go
customMatcher := func(attrs message.Attributes) bool {
    priority, ok := attrs["priority"].(int)
    return ok && priority > 5
}
```

### CQRS Generic Type Matcher

```go
// Match by Go type name
cqrs.MatchGenericTypeOf[CreateOrder]() // Matches type="CreateOrder"
```

## Complete Example

```go
package main

import (
    "context"
    "log"

    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/message/cqrs"
)

type CreateOrder struct {
    ID     string `json:"id"`
    Amount int    `json:"amount"`
}

type OrderCreated struct {
    ID        string `json:"id"`
    CreatedAt string `json:"created_at"`
}

func main() {
    ctx := context.Background()
    marshaler := cqrs.NewJSONCommandMarshaler(cqrs.WithTypeOf())

    // Command handler
    createHandler := cqrs.NewCommandHandler(
        func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
            log.Printf("Processing order: %s", cmd.ID)
            return []OrderCreated{{
                ID:        cmd.ID,
                CreatedAt: "2024-01-01T00:00:00Z",
            }}, nil
        },
        cqrs.Match(cqrs.MatchSubject("CreateOrder")),
        marshaler,
    )

    // Event handler
    emailHandler := cqrs.NewEventHandler(
        func(ctx context.Context, evt OrderCreated) error {
            log.Printf("Sending email for order: %s", evt.ID)
            return nil
        },
        cqrs.Match(cqrs.MatchSubject("OrderCreated")),
        cqrs.NewJSONEventMarshaler(),
    )

    // Create routers
    commandRouter := message.NewRouter(message.RouterConfig{
        Concurrency: 5,
        Recover:     true,
    })
    commandRouter.AddHandler(createHandler)

    eventRouter := message.NewRouter(message.RouterConfig{})
    eventRouter.AddHandler(emailHandler)

    // Wire together
    commands := make(chan *message.Message, 10)
    events := commandRouter.Start(ctx, commands)
    eventRouter.Start(ctx, events)

    // Send a command
    data, _ := marshaler.Marshal(CreateOrder{ID: "order-1", Amount: 100})
    commands <- message.New(data, message.Attributes{
        message.AttrSubject: "CreateOrder",
    })

    close(commands)
}
```

## Best Practices

### 1. Use Appropriate Concurrency

```go
// High throughput, independent messages
router := message.NewRouter(message.RouterConfig{
    Concurrency: 50,
})

// Order-sensitive, sequential processing
router := message.NewRouter(message.RouterConfig{
    Concurrency: 1,
})
```

### 2. Enable Recovery in Production

```go
router := message.NewRouter(message.RouterConfig{
    Recover: true, // Prevents panics from crashing the router
})
```

### 3. Use Specific Matchers

```go
// Good: Specific matching
cqrs.Match(
    cqrs.MatchSubject("CreateOrder"),
    cqrs.MatchType("command"),
)

// Avoid: Overly broad matching
func(attrs message.Attributes) bool { return true }
```

### 4. Always Ack/Nack Messages

```go
handler := message.NewHandler(
    func(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
        result, err := process(msg)
        if err != nil {
            msg.Nack(err) // Important!
            return nil, err
        }
        msg.Ack() // Important!
        return result, nil
    },
    matcher,
)
```

### 5. Register Handlers Before Start

```go
router := message.NewRouter(config)
router.AddHandler(handler1) // OK
router.AddHandler(handler2) // OK

output := router.Start(ctx, input)

router.AddHandler(handler3) // Returns false! Too late
```

## Related Documentation

- [CQRS Overview](./cqrs-overview.md) - CQRS patterns and implementation
- [Middleware](../middleware/) - Adding cross-cutting concerns
- [Examples](../examples/) - Working code examples

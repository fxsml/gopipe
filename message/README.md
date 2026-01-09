# message

CloudEvents message handling with type-based routing.

## Overview

The `message` package provides:

- **Message** - CloudEvents-aligned message with typed data and attributes
- **Engine** - Orchestrates message flow between inputs, handlers, and outputs
- **Handler** - Type-safe command/event handlers with automatic marshaling

## Engine Architecture

```
RawInput₁ → Unmarshal ─┐
RawInput₂ → Unmarshal ─┼─→ Merger → Router → Distributor
TypedInput ────────────┘                            │
                                         ┌──────────┴──────────┐
                                   TypedOutput            Marshal
                                                             ↓
                                                         RawOutput
```

The Engine uses a single merger for all message flows:

- **Merger** combines typed inputs and unmarshaled raw inputs
- **Router** routes messages to handlers by CE type
- **Distributor** routes output to consumers using first-match-wins semantics
- **TypedOutput** bypasses marshaling (for internal use)
- **RawOutput** marshals to bytes (for broker integration)

Loopback is not built into the Engine—use `plugin.Loopback` which connects
a TypedOutput back to TypedInput via the existing Add* APIs.

## Usage

### Raw I/O (Broker Integration)

```go
engine := message.NewEngine(message.EngineConfig{
    Marshaler: message.NewJSONMarshaler(),
})

// Register handlers
handler := message.NewCommandHandler(
    func(ctx context.Context, cmd OrderCommand) ([]OrderEvent, error) {
        return []OrderEvent{{ID: cmd.ID, Status: "created"}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders",
        Naming: message.KebabNaming,
    },
)
engine.AddHandler("orders", nil, handler)

// Add raw inputs and outputs (for broker integration)
input := make(chan *message.RawMessage, 100)
engine.AddRawInput("orders-in", nil, input)
output, _ := engine.AddRawOutput("orders-out", nil)

// Start engine
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
done, _ := engine.Start(ctx)

// Send/receive raw messages (bytes)
input <- &message.RawMessage{
    Data:       []byte(`{"id": "123"}`),
    Attributes: message.Attributes{"type": "order.command"},
}

out := <-output
// out.Data contains marshaled OrderEvent as []byte
```

### Typed I/O (Internal Use / Testing)

```go
engine := message.NewEngine(message.EngineConfig{
    Marshaler: message.NewJSONMarshaler(),
})

// Register handlers
handler := message.NewCommandHandler(
    func(ctx context.Context, cmd OrderCommand) ([]OrderEvent, error) {
        return []OrderEvent{{ID: cmd.ID, Status: "created"}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders",
        Naming: message.KebabNaming,
    },
)
engine.AddHandler("orders", nil, handler)

// Add typed inputs and outputs (no marshal/unmarshal)
input := make(chan *message.Message, 100)
engine.AddInput("orders-in", nil, input)
output, _ := engine.AddOutput("orders-out", nil)

// Start engine
ctx, cancel := context.WithCancel(context.Background())
defer cancel()
done, _ := engine.Start(ctx)

// Send/receive typed messages directly
input <- &message.Message{
    Data:       OrderCommand{ID: "123"},
    Attributes: message.Attributes{"type": "order.command"},
}

out := <-output
// out.Data contains OrderEvent as typed struct (any)
event := out.Data.(OrderEvent)
```

## Dynamic Input/Output

Inputs and outputs can be added after Start():

```go
engine.Start(ctx)

// Add new raw input dynamically (broker integration)
newRawInput := make(chan *message.RawMessage, 100)
engine.AddRawInput("new-raw-input", nil, newRawInput)

// Add new typed input dynamically (internal use)
newTypedInput := make(chan *message.Message, 100)
engine.AddInput("new-typed-input", nil, newTypedInput)

// Add new outputs dynamically
newRawOutput, _ := engine.AddRawOutput("orders-out", match.Types("order.%"))
newTypedOutput, _ := engine.AddOutput("internal-out", match.Types("internal.%"))
```

## Loopback

Use the `plugin.Loopback` plugin to route output messages back to the engine for re-processing:

```go
engine.AddPlugin(plugin.Loopback("loopback", match.Types("intermediate.event")))
```

Messages matching the loopback criteria are fed back to the handler pipeline, skipping marshal/unmarshal for efficiency.

**Note:** Loopback creates a cycle in the message flow. The engine cannot detect when processing is "complete" with loopback enabled. You must cancel the context to trigger shutdown.

## Message Types

### RawMessage

Raw bytes with CloudEvents attributes:

```go
type RawMessage = TypedMessage[[]byte]
```

### Message

Typed message with unmarshaled data:

```go
msg := &message.Message{
    Data:       myStruct,
    Attributes: message.Attributes{
        "type":   "order.created",
        "source": "/orders",
    },
}
```

### Attributes

CloudEvents-aligned attribute keys:

```go
const (
    AttrID              = "id"
    AttrType            = "type"
    AttrSource          = "source"
    AttrSubject         = "subject"
    AttrTime            = "time"
    AttrDataContentType = "datacontenttype"
    AttrDataSchema      = "dataschema"
    AttrSpecVersion     = "specversion"
)
```

## Handlers

### CommandHandler

Processes commands and returns events:

```go
handler := message.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        return []OrderCreated{{OrderID: cmd.ID}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders",
        Naming: message.KebabNaming,
    },
)
```

### Handler Interface

```go
type Handler interface {
    EventType() string
    NewInput() any
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}
```


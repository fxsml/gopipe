# message

CloudEvents message handling with type-based routing.

## Overview

The `message` package provides:

- **Message** - CloudEvents-aligned message with typed data and attributes
- **Engine** - Orchestrates message flow between inputs, handlers, and outputs
- **Handler** - Type-safe command/event handlers with automatic marshaling

## Engine Architecture

```
RawInput₁ ─┐
RawInput₂ ─┼→ RawMerger → Unmarshal ─┐
RawInput₃ ─┘                         │
                                     ↓
TypedInput ───────────────→ TypedMerger → Handler → Distributor
                                 ↑                       │
                                 └─── Loopback (typed) ──┤
                                                         ↓
                                                  ┌──────┴──────┐
                                           TypedOutput    Marshal → RawOutput
```

The Engine uses two mergers for raw (broker) and typed (internal) message flows:

- **RawMerger** combines raw input channels, then unmarshals to typed messages
- **TypedMerger** combines typed inputs and loopback messages
- **Distributor** routes messages to outputs using first-match-wins semantics
- **TypedOutput** bypasses marshaling (for internal use)
- **RawOutput** marshals to bytes (for broker integration)

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
engine.AddHandler(handler, message.HandlerConfig{Name: "orders"})

// Add raw inputs and outputs (for broker integration)
input := make(chan *message.RawMessage, 100)
engine.AddRawInput(input, message.RawInputConfig{Name: "orders-in"})
output := engine.AddRawOutput(message.RawOutputConfig{Name: "orders-out"})

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
engine.AddHandler(handler, message.HandlerConfig{Name: "orders"})

// Add typed inputs and outputs (no marshal/unmarshal)
input := make(chan *message.Message, 100)
engine.AddInput(input, message.InputConfig{Name: "orders-in"})
output := engine.AddOutput(message.OutputConfig{Name: "orders-out"})

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
engine.AddRawInput(newRawInput, message.RawInputConfig{Name: "new-raw-input"})

// Add new typed input dynamically (internal use)
newTypedInput := make(chan *message.Message, 100)
engine.AddInput(newTypedInput, message.InputConfig{Name: "new-typed-input"})

// Add new outputs dynamically
newRawOutput := engine.AddRawOutput(message.RawOutputConfig{
    Matcher: match.Types("order.%"),
})
newTypedOutput := engine.AddOutput(message.OutputConfig{
    Matcher: match.Types("internal.%"),
})
```

## Loopback

Loopback allows messages to be re-processed without marshaling:

```go
engine.AddLoopback(message.LoopbackConfig{
    Matcher: &typeMatcher{pattern: "intermediate.event"},
})
```

Messages matching the loopback criteria are fed back to the handler pipeline, skipping marshal/unmarshal for efficiency.

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


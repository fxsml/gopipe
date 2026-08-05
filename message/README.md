# message

CloudEvents message handling with type-based routing.

## Overview

The `message` package provides:

- **Message** - CloudEvents-aligned message with typed data and attributes
- **Router** - Dispatches messages to handlers by CE type
- **Handler** - Type-safe command/event handlers with automatic marshaling

## Router Composition

`Router` is a standalone component: it takes a channel of typed `*Message`
values and returns a channel of typed `*Message` values.

```
RawInput → UnmarshalPipe → Router → MarshalPipe → RawOutput
```

- **Router** routes messages to handlers by CE type
- **UnmarshalPipe**/**MarshalPipe** convert `[]byte` ↔ typed data at the boundary
  (skip them entirely for internal/typed-only messaging)

For fan-in/fan-out across multiple inputs/outputs, compose with
[`channel.Merge`](https://pkg.go.dev/github.com/fxsml/gopipe/channel#Merge) and
[`channel.Switch`](https://pkg.go.dev/github.com/fxsml/gopipe/channel#Switch).

## Usage

### Raw I/O (Broker Integration)

```go
router := message.NewRouter(message.PipeConfig{})

// Register handlers
handler := message.NewCommandHandler(
    func(ctx context.Context, cmd OrderCommand) ([]OrderEvent, error) {
        return []OrderEvent{{ID: cmd.ID, Status: "created"}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders",
        Naming: message.DotNaming,
    },
)
router.AddHandler("orders", nil, handler)

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Raw input → typed, via unmarshal pipe
input := make(chan *message.RawMessage, 100)
marshaler := message.NewJSONMarshaler()
unmarshal := message.NewUnmarshalPipe(router, marshaler, message.PipeConfig{})
typedIn, _ := unmarshal.Pipe(ctx, input)

typedOut, _ := router.Pipe(ctx, typedIn)

// Typed output → raw, via marshal pipe
marshal := message.NewMarshalPipe(marshaler, message.PipeConfig{})
output, _ := marshal.Pipe(ctx, typedOut)

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
router := message.NewRouter(message.PipeConfig{})

// Register handlers
handler := message.NewCommandHandler(
    func(ctx context.Context, cmd OrderCommand) ([]OrderEvent, error) {
        return []OrderEvent{{ID: cmd.ID, Status: "created"}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders",
        Naming: message.DotNaming,
    },
)
router.AddHandler("orders", nil, handler)

// Typed input/output, no marshal/unmarshal
input := make(chan *message.Message, 100)

ctx, cancel := context.WithCancel(context.Background())
defer cancel()
output, _ := router.Pipe(ctx, input)

// Send/receive typed messages directly
input <- &message.Message{
    Data:       OrderCommand{ID: "123"},
    Attributes: message.Attributes{"type": "order.command"},
}

out := <-output
// out.Data contains OrderEvent as typed struct (any)
event := out.Data.(OrderEvent)
```

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
        Naming: message.DotNaming,
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


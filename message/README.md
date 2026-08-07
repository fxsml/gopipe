# message

CloudEvents message handling with type-based routing.

## Overview

The `message` package provides:

- **Message** - CloudEvents-aligned message with typed data and attributes
- **Router** - Dispatches messages to handlers by CE type
- **Handler** - Type-safe command/event handlers with automatic marshaling

## Router Composition

`Router` is pure dispatch: it looks up a handler by CE type and calls
`Handle`, never inspecting `Data` itself. It takes and returns a channel of
`*Message` values whose `Data` may be raw `[]byte` or typed, depending
entirely on the handlers registered.

`NewCommandHandler` marshals by default, so a `Router` built entirely from
command handlers can sit directly on raw broker/HTTP I/O:

```
RawInput → Router (handlers unmarshal/marshal internally) → RawOutput
```

Set `CommandHandlerConfig.DisableMarshaler` per handler to operate
typed-through instead. `UnmarshalPipe`/`MarshalPipe` remain the right tool
for explicit composition — e.g. naming decoupled from dispatch via an
`InputRegistry`, or a `DisableMarshaler: true` handler feeding further typed
processing before an eventual marshal stage:

```
RawInput → UnmarshalPipe → Router (typed-through) → MarshalPipe → RawOutput
```

For fan-in/fan-out across multiple inputs/outputs, compose with
[`channel.Merge`](https://pkg.go.dev/github.com/fxsml/gopipe/channel#Merge) and
[`channel.Switch`](https://pkg.go.dev/github.com/fxsml/gopipe/channel#Switch).

## Usage

### Raw I/O (Broker Integration)

```go
router := message.NewRouter(message.PipeConfig{})

// Register handlers — marshaling is on by default
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

// Router takes/returns raw messages directly
input := make(chan *message.Message, 100)
output, _ := router.Pipe(ctx, input)

// Send/receive raw messages (bytes)
input <- message.NewRaw([]byte(`{"id": "123"}`), message.Attributes{"type": "order.command"}, nil)

out := <-output
// out.Data contains marshaled OrderEvent as []byte
```

### Typed I/O (Internal Use / Testing)

```go
router := message.NewRouter(message.PipeConfig{})

// DisableMarshaler: true opts this handler out of marshal-by-default
handler := message.NewCommandHandler(
    func(ctx context.Context, cmd OrderCommand) ([]OrderEvent, error) {
        return []OrderEvent{{ID: cmd.ID, Status: "created"}}, nil
    },
    message.CommandHandlerConfig{
        Source:           "/orders",
        Naming:           message.DotNaming,
        DisableMarshaler: true,
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

### Message

A single concrete type. `Data` holds either raw `[]byte` (broker boundary)
or a typed Go value, depending on where the message is in a pipeline. Use
`Raw()` to check which state `Data` is currently in:

```go
msg := &message.Message{
    Data:       myStruct,
    Attributes: message.Attributes{
        "type":   "order.created",
        "source": "/orders",
    },
}

if data, ok := msg.Raw(); ok {
    // Data is []byte
} else {
    // Data is a typed Go value
}
```

**Broker boundary contract:** messages crossing the broker boundary (broker
adapters, `UnmarshalPipe`/`MarshalPipe`, `cloudevents.ToCloudEvent`/`FromCloudEvent`)
always have `Data` typed to `[]byte` — `nil` or an empty slice both mean "no
payload," but `Data` must never be a bare untyped `nil`. Use `NewRaw` to
construct these messages instead of `New`: its `[]byte` parameter makes the
guarantee structural, not just conventional.

```go
heartbeat := message.NewRaw(nil, message.Attributes{"type": "heartbeat"}, nil) // no payload
order := message.NewRaw([]byte(`{"id":"123"}`), message.Attributes{"type": "order.created"}, nil)
```

This restriction applies only at the boundary. Purely internal, typed-only
pipelines are free to use `nil` (or any other value) as `Data` — that's an
application decision, not one gopipe imposes.

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

Processes commands and returns events. Unmarshals input from raw `[]byte`
and marshals output to raw `[]byte` by default (`NewJSONMarshaler()`);
set `DisableMarshaler: true` to operate typed-through instead. `Subject`,
if set, derives the CE subject attribute from each typed output event,
before marshaling:

```go
handler := message.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        return []OrderCreated{{OrderID: cmd.ID}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/orders",
        Naming: message.DotNaming,
        Subject: func(data any) string {
            return data.(OrderCreated).OrderID
        },
    },
)
```

### Handler Interface

```go
type Handler interface {
    EventType() string
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}
```


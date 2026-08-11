---
name: building-message-pipelines
description: |
  Provides expertise in the message package architecture for building CloudEvents-based
  pipelines in gopipe. Apply when working with the message package, Router,
  Handler, Matcher, or designing event-driven systems.
user-invocable: false
---

# Building Message Pipelines

## Architecture: Router Composition

```
RawInput → Router (handlers unmarshal/marshal internally) → RawOutput
```

`Router` is pure dispatch: it looks up a handler by CE type and calls `Handle`,
never inspecting `Data`. `NewCommandHandler` marshals by default, so a `Router`
built from command handlers can sit directly on raw broker/HTTP I/O. Set
`CommandHandlerConfig.DisableMarshaler` per handler to operate typed-through
instead, composing `NewUnmarshalPipe`/`NewMarshalPipe` explicitly at the
boundary where raw ([]byte) messages meet typed ones. Use
`channel.Merge`/`channel.Switch` for fan-in/fan-out.

## Router Configuration

```go
router := message.NewRouter(message.PipeConfig{
    ShutdownTimeout: 5 * time.Second,
})
```

## Adding Handlers

```go
router.AddHandler("handler-name", message.NewCommandHandler(
    func(ctx context.Context, cmd InputType) ([]OutputType, error) {
        return []OutputType{{...}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/source",
        Naming: message.DotNaming,
    },
))
```

## Handler Interface

```go
type Handler interface {
    EventType() string          // CE type for routing (e.g., "order.created")
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}
```

## Matcher Interface

```go
type Matcher interface {
    Match(attrs Attributes) bool  // Attributes only — not *Message
}
```

Operates on Attributes only (not `*Message`) to avoid wrapper allocation for raw messages.

## Raw and Typed I/O

```go
// CommandHandler marshals by default — feed the router raw messages directly.
rawInput := make(chan *message.Message, 10)
rawOutput, _ := router.Pipe(ctx, rawInput)

// DisableMarshaler: true opts a handler out, for typed-through composition
// via explicit NewUnmarshalPipe/NewMarshalPipe stages at the boundary:
//
//	unmarshal := message.NewUnmarshalPipe(registry, message.NewJSONMarshaler(), message.PipeConfig{})
//	typedInput, _ := unmarshal.Pipe(ctx, rawInput)
//	typedOutput, _ := router.Pipe(ctx, typedInput)
//	marshal := message.NewMarshalPipe(message.NewJSONMarshaler(), message.PipeConfig{})
//	rawOutput, _ := marshal.Pipe(ctx, typedOutput)
```

## Event Type Naming

| Naming | Output | Example |
|--------|--------|---------|
| `DotNaming` | `type.subtype` | `order.created` |
| `KebabNaming` | `type-subtype` | `order-created` |
| `SnakeNaming` | `type_subtype` | `order_created` |

## Graceful Shutdown

```go
ctx, cancel := context.WithCancel(context.Background())
out, _ := router.Pipe(ctx, input)

close(input)  // Close input first
cancel()      // Then cancel context
for range out {}  // Drain until closed
```

## Rejected Alternatives

**Combined Marshaler with Registry** — rejected: single responsibility. Marshaler is pure serialization; `CommandHandlerConfig` decides whether and how a handler marshals.

**PipeHandler Interface** — rejected: over-engineering. `EventType()` returning `"*"` for multi-type is a hack.

**Named Outputs with RouteOutput** — rejected: pattern matching on CE type is more declarative.

## Reference Procedures

- @../AGENTS.md — full architecture decisions and rejected alternatives
- @../message/doc.go — package documentation
- @../docs/adr/ — Architecture Decision Records

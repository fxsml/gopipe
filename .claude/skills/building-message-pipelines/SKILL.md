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
RawInput → UnmarshalPipe → Router → MarshalPipe → RawOutput
```

`Router` is a standalone component — compose it directly with
[`NewUnmarshalPipe`]/[`NewMarshalPipe`] at the boundary where raw ([]byte) messages
meet typed ones. Use `channel.Merge`/`channel.Switch` for fan-in/fan-out.

## Router Configuration

```go
router := message.NewRouter(message.PipeConfig{
    ShutdownTimeout: 5 * time.Second,
})
```

## Adding Handlers

```go
router.AddHandler("handler-name", matcher, message.NewCommandHandler(
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
    NewInput() any              // Creates instance for unmarshaling
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
// Raw input ([]byte data) → typed, via unmarshal pipe
rawInput := make(chan *message.RawMessage, 10)
unmarshal := message.NewUnmarshalPipe(router, message.NewJSONMarshaler(), message.PipeConfig{})
typedInput, _ := unmarshal.Pipe(ctx, rawInput)

// Or feed the router directly with an already-typed channel, no marshaling:
// typedInput := make(chan *message.Message, 10)

typedOutput, _ := router.Pipe(ctx, typedInput)

// Typed output → raw, via marshal pipe
marshal := message.NewMarshalPipe(message.NewJSONMarshaler(), message.PipeConfig{})
rawOutput, _ := marshal.Pipe(ctx, typedOutput)
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

**Combined Marshaler with Registry** — rejected: single responsibility. Marshaler is pure serialization; `Handler.NewInput()` provides instances.

**PipeHandler Interface** — rejected: over-engineering. `EventType()` returning `"*"` for multi-type is a hack.

**Named Outputs with RouteOutput** — rejected: pattern matching on CE type is more declarative.

## Reference Procedures

- @../AGENTS.md — full architecture decisions and rejected alternatives
- @../message/doc.go — package documentation
- @../docs/adr/ — Architecture Decision Records

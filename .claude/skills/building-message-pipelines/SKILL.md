---
name: building-message-pipelines
description: |
  Provides expertise in the message package architecture for building CloudEvents-based
  pipelines in gopipe. Apply when working with the message package, Engine, Router,
  Handler, Matcher, or designing event-driven systems.
user-invocable: false
---

# Building Message Pipelines

## Architecture: Single Merger

```
RawInputs → Unmarshal ─┐
                       ├→ Merger → Router → Distributor
TypedInputs ───────────┘                          │
                                       ┌──────────┴──────────┐
                                 TypedOutput            RawOutput
```

Each raw input has its own unmarshal pipe feeding into a single shared merger.

## Engine Configuration

```go
engine := message.NewEngine(message.EngineConfig{
    Marshaler:       message.NewJSONMarshaler(),
    ShutdownTimeout: 5 * time.Second,
})
```

## Adding Handlers

```go
engine.AddHandler("handler-name", matcher, message.NewCommandHandler(
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

## Inputs and Outputs

```go
// Raw input ([]byte data)
input := make(chan *message.RawMessage, 10)
engine.AddRawInput("name", matcher, input)

// Raw output
output, _ := engine.AddRawOutput("name", matcher)

// Typed input
typedInput := make(chan *message.Message, 10)
engine.AddInput("name", matcher, typedInput)

// Typed output
typedOutput, _ := engine.AddOutput("name", matcher)
```

## Loopback is a Plugin

Loopback is NOT built into Engine. Use `plugin.Loopback`:

```go
engine.AddPlugin(plugin.Loopback("step1-loop", &typeMatcher{"step1.completed"}))
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
done, _ := engine.Start(ctx)

close(input)  // Close inputs first
cancel()      // Then cancel context
<-done        // Wait for shutdown
```

## Rejected Alternatives

**Combined Marshaler with Registry** — rejected: single responsibility. Marshaler is pure serialization; `Handler.NewInput()` provides instances.

**PipeHandler Interface** — rejected: over-engineering. `EventType()` returning `"*"` for multi-type is a hack.

**Named Outputs with RouteOutput** — rejected: pattern matching on CE type is more declarative.

## Reference Procedures

- @../AGENTS.md — full architecture decisions and rejected alternatives
- @../message/doc.go — package documentation
- @../docs/adr/ — Architecture Decision Records

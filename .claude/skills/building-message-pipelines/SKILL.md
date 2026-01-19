---
name: building-message-pipelines
description: |
  Provides expertise in the message package architecture for building CloudEvents-based
  message pipelines in gopipe. This skill covers Engine, Router, Handler patterns,
  loopback configuration, and message flow design. Use when working with the message
  package or designing event-driven systems.
---

# Building Message Pipelines

This skill provides guidance for building message pipelines using gopipe's message package.

## Key Knowledge

### Architecture: Single Merger

```
RawInputs → Unmarshal ─┐
                       ├→ Merger → Router → Distributor
TypedInputs ───────────┘                          │
                                       ┌──────────┴──────────┐
                                 TypedOutput            RawOutput
```

**Why:** Single merger is simpler. Each raw input has its own unmarshal pipe that feeds typed messages into the shared merger.

### Loopback is a Plugin

Loopback is NOT built into Engine. Use `plugin.Loopback` which connects TypedOutput back to TypedInput via the existing Add* APIs:

```go
engine.AddPlugin(plugin.Loopback("name", matcher))
```

### Engine Configuration

```go
engine := message.NewEngine(message.EngineConfig{
    Marshaler:       message.NewJSONMarshaler(),
    ShutdownTimeout: 5 * time.Second,
})
```

### Adding Handlers

```go
engine.AddHandler("handler-name", matcher, message.NewCommandHandler(
    func(ctx context.Context, cmd InputType) ([]OutputType, error) {
        // Process command, return events
        return []OutputType{{...}}, nil
    },
    message.CommandHandlerConfig{
        Source: "/source",
        Naming: message.KebabNaming,
    },
))
```

### Handler Interface

```go
type Handler interface {
    EventType() string   // CE type for routing (e.g., "order.created")
    NewInput() any       // Creates instance for unmarshaling
    Handle(ctx context.Context, msg *Message) ([]*Message, error)
}
```

### Inputs and Outputs

```go
// Raw input ([]byte data)
input := make(chan *message.RawMessage, 10)
engine.AddRawInput("name", matcher, input)

// Raw output
output, _ := engine.AddRawOutput("name", matcher)

// Typed input (already unmarshaled)
typedInput := make(chan *message.Message, 10)
engine.AddInput("name", matcher, typedInput)

// Typed output
typedOutput, _ := engine.AddOutput("name", matcher)
```

### Event Type Naming

| Naming | Output Format | Example |
|--------|---------------|---------|
| `KebabNaming` | `type.subtype` | `order.created` |
| `SnakeNaming` | `type_subtype` | `order_created` |

### CloudEvents Attributes

Standard attributes:
- `id` - Unique event ID
- `type` - Event type (e.g., "order.created")
- `source` - Event source URI
- `specversion` - CloudEvents spec version ("1.0")
- `time` - Event timestamp

Optional:
- `subject` - Event subject
- `datacontenttype` - Content type of data
- `correlationid` - For request correlation

## Common Patterns

### Multi-Step Pipeline with Loopbacks

```go
// Step 1 handler produces events that loop back for Step 2
engine.AddHandler("step1", nil, handler1)
engine.AddPlugin(plugin.Loopback("step1-loop", &typeMatcher{"step1.completed"}))

// Step 2 handler processes Step 1 output
engine.AddHandler("step2", nil, handler2)
engine.AddPlugin(plugin.Loopback("step2-loop", &typeMatcher{"step2.completed"}))

// Final output
output, _ := engine.AddRawOutput("final", &typeMatcher{"workflow.completed"})
```

### Graceful Shutdown

```go
ctx, cancel := context.WithCancel(context.Background())
done, _ := engine.Start(ctx)

// ... process messages ...

close(input)  // Close input first
cancel()      // Then cancel context

<-done  // Wait for graceful shutdown
```

## Rejected Alternatives

### Combined Marshaler with Registry

```go
// REJECTED
type Marshaler interface {
    Register(goType reflect.Type)
    Unmarshal(data, ceType) (any, error)
}
```

**Why:** Single responsibility. Marshaler is pure serialization. Handler.NewInput() provides instances.

### PipeHandler Interface

```go
// REJECTED
type PipeHandler interface {
    EventType() string
    Pipe(ctx, in <-chan) (<-chan, error)
}
```

**Why:** Over-engineering. EventType() returning "*" for multi-type is a hack.

### Named Outputs with RouteOutput

```go
// REJECTED
engine.AddOutput("shipments", ch)
engine.RouteOutput("handler", "shipments")
```

**Why:** Pattern matching on CE type is more declarative.

## Reference Documents

- [AGENTS.md](AGENTS.md) - Architecture decisions, rejected alternatives
- [message/doc.go](message/doc.go) - Package documentation
- [docs/adr/](docs/adr/) - Architecture Decision Records

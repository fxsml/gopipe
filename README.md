# gopipe

[![CI](https://github.com/fxsml/gopipe/actions/workflows/ci.yml/badge.svg)](https://github.com/fxsml/gopipe/actions/workflows/ci.yml)
[![GoDoc](https://pkg.go.dev/badge/github.com/fxsml/gopipe.svg)](https://pkg.go.dev/github.com/fxsml/gopipe)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Composable data pipelines for Go** — from simple channel operations to CloudEvents message routing, with zero dependencies.

## Why gopipe?

- **Progressive complexity** — Start with simple channel functions, scale to full message engines
- **Type-safe generics** — Full Go 1.18+ generics support throughout
- **Zero dependencies** — Core packages have no external dependencies
- **CloudEvents aligned** — Message package follows CloudEvents specification

## Quick Start

### Transform and Filter Channels

```go
import "github.com/fxsml/gopipe/channel"

// Transform values
doubled := channel.Transform(numbers, func(n int) int {
    return n * 2
})

// Filter values
evens := channel.Filter(numbers, func(n int) bool {
    return n%2 == 0
})

// Fan-out to multiple consumers (returns []<-chan T)
outputs := channel.Broadcast(source, 2)
```

### Merge Dynamic Inputs

```go
import "github.com/fxsml/gopipe/pipe"

merger := pipe.NewMerger[Order](pipe.MergerConfig{Buffer: 100})
merger.AddInput(webOrders)
merger.AddInput(apiOrders) // Can add more at runtime

merged, _ := merger.Merge(ctx)
```

### Route Messages by Type

```go
import "github.com/fxsml/gopipe/message"

engine := message.NewEngine(message.EngineConfig{
    Marshaler: message.NewJSONMarshaler(),
})

handler := message.NewCommandHandler(
    func(ctx context.Context, cmd CreateOrder) ([]OrderCreated, error) {
        return []OrderCreated{{ID: cmd.ID}}, nil
    },
    message.CommandHandlerConfig{Source: "/orders"},
)
engine.AddHandler("orders", nil, handler)
engine.AddRawInput("input", nil, inputCh)
output, _ := engine.AddRawOutput("output", nil)

engine.Start(ctx)
```

## Packages

| Package | Purpose | Use When |
|---------|---------|----------|
| [`channel`](https://pkg.go.dev/github.com/fxsml/gopipe/channel) | Stateless operations | Simple transforms, filters, fan-in/out |
| [`pipe`](https://pkg.go.dev/github.com/fxsml/gopipe/pipe) | Stateful components | Dynamic inputs/outputs, lifecycle control |
| [`message`](https://pkg.go.dev/github.com/fxsml/gopipe/message) | Message routing | CloudEvents, type-based handlers, middleware |

### When to Use What

```
channel.Transform  →  Simple 1:1 mapping
channel.Filter     →  Drop unwanted values
channel.Merge      →  Combine fixed channels
channel.Broadcast  →  Send to multiple consumers

pipe.Merger        →  Add inputs at runtime
pipe.Distributor   →  Route by matcher, first-match wins
pipe.ProcessPipe   →  Stateful processing with lifecycle

message.Engine     →  Full message bus with routing
message.Router     →  Just the handler dispatch part
message.Handler    →  Type-safe command/event handlers
```

## Installation

```bash
go get github.com/fxsml/gopipe
```

## Learning Path

1. **[Channel](examples/01-channel/)** — Filter, Transform, Merge basics
2. **[Pipe](examples/02-pipe/)** — Stateful pipe with lifecycle
3. **[Merger](examples/03-merger/)** — Dynamic input merging
4. **[Message](examples/04-message/)** — CloudEvents message handling
5. **[Generator](examples/05-generator/)** — Producing values from functions

## Documentation

- **[Package Documentation](https://pkg.go.dev/github.com/fxsml/gopipe)** — API reference
- **[Examples](examples/)** — Working code examples
- **[AGENTS.md](AGENTS.md)** — Architecture decisions and design notes

## License

MIT

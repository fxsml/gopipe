# Examples

Learn gopipe step-by-step.

## Learning Path

| # | Example | Concepts |
|---|---------|----------|
| 1 | [channel](01-channel/) | Filter, Transform, Sink, FromRange |
| 2 | [pipe](02-pipe/) | ProcessPipe, Config, Middleware |
| 3 | [merger](03-merger/) | Dynamic inputs, shutdown timeout |
| 4 | [message](04-message/) | CloudEvents engine, handlers, routing |
| 5 | [generator](05-generator/) | Producing values, context cancellation |

## Quick Start

Run any example:

```bash
go run ./examples/01-channel
go run ./examples/02-pipe
go run ./examples/03-merger
go run ./examples/04-message
go run ./examples/05-generator
```

## Package Progression

```
channel → pipe → message
```

- **channel**: Stateless operations (functional, no config)
- **pipe**: Stateful components (lifecycle, config, middleware)
- **message**: CloudEvents messaging (handlers, routing, ack/nack)

## What Each Example Shows

### 01-channel
Basic channel operations without any configuration. Shows how to generate values, filter, transform, and consume them.

### 02-pipe
Adds configuration (concurrency, buffer size) and middleware (recovery from panics). Shows the pipe pattern for processing with more control.

### 03-merger
Demonstrates dynamic fan-in: adding multiple input channels that get merged into a single output. Shows shutdown handling.

### 04-message
CloudEvents message processing with typed handlers. Shows how to create an engine, register handlers, and route messages by type.

### 05-generator
Produces values on demand using a generator function. Shows context cancellation for controlled shutdown.

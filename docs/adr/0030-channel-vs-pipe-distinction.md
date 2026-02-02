# ADR 0030: Channel vs Pipe Package Distinction

**Date:** 2026-02-02
**Status:** Proposed

## Context

The repository has two packages for data processing: `channel` and `pipe`. Users need clear guidance on when to use each. The distinction is fundamental to the library's architecture.

## Decision

### Channel Package: Pure, Stateless Functions

The `channel` package provides **pure functions** that operate on Go channels:

```go
// Pure transformation - no state, no lifecycle
out := channel.Map(in, func(x int) string {
    return strconv.Itoa(x)
})

// Composition via channel chaining
filtered := channel.Filter(in, isValid)
mapped := channel.Map(filtered, transform)
channel.Drain(mapped, process)
```

**Characteristics:**
- Functions, not structs
- No configuration beyond function arguments
- No middleware support
- No lifecycle management
- Stateless operations
- Direct channel manipulation

### Pipe Package: Stateful Components with Lifecycle

The `pipe` package provides **stateful components** with configuration and middleware:

```go
// Stateful component with configuration
processor := pipe.NewProcessor(processFunc)
processor.Use(middleware.Retry(cfg))

// Lifecycle management
pipeline := pipe.Join(source.Pipe(), processor.Pipe(), sink.Pipe())
err := pipeline.Run(ctx)
```

**Characteristics:**
- Structs with configuration
- Middleware support
- Lifecycle management (start/stop)
- Error handling and recovery
- Composition via `Pipe()` interface

### When to Use Each

| Use Case | Package | Why |
|----------|---------|-----|
| Simple transformation | `channel` | No overhead needed |
| One-off data processing | `channel` | Quick and direct |
| Concurrent fan-out/in | `channel` | Built-in channel operations |
| Production pipeline | `pipe` | Needs retry, metrics, logging |
| Long-running service | `pipe` | Lifecycle management |
| Testable business logic | `pipe` | Middleware isolation |

### Naming Alignment

Operations are named consistently across packages:

| Operation | Channel | Pipe |
|-----------|---------|------|
| 1:1 transform | `Map()` | `Mapper.Map()` |
| 1:N expand | `Expand()` | `Expander.Expand()` |
| Filter | `Filter()` | `Filter.Filter()` |
| Consume | `Drain()` | `Sink.Sink()` |
| Fan-out by index | `Switch()` | `Distributor` (matcher-based) |

## Consequences

**Breaking Changes:**
- None (clarification of existing architecture)

**Benefits:**
- Clear mental model for users
- Appropriate tool for each use case
- Consistent naming reduces cognitive load

**Drawbacks:**
- Two packages to learn
- Some overlap in functionality

## Links

- Related: [Plan 0013 - Channel Package Refactoring](../plans/0013-channel-package-refactoring.md)
- Related: [Plan 0014 - Pipe Interface Refactoring](../plans/0014-pipe-interface-refactoring.md)
- Related: ADR 0005 (Channel Package)

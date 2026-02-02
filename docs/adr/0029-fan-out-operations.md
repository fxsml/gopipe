# ADR 0029: Fan-out Operations

**Date:** 2026-02-02
**Status:** Proposed

## Context

Fan-out operations distribute items from one input to multiple outputs. The current naming (`Route`) is ambiguous and conflicts with `message.Router` semantics. Users need clear understanding of which operation to use:

1. Copy to all outputs
2. Select one output by index
3. Select one output by matching criteria

## Decision

Define three distinct fan-out patterns with clear semantic names:

### 1. Broadcast - Copy to ALL Outputs

```go
// channel package
func Broadcast[T any](in <-chan T, n int) []<-chan T
```

Each input item is copied to every output channel. All outputs receive all items.

### 2. Switch - Select ONE Output by Index

```go
// channel package
func Switch[T any](in <-chan T, fn func(T) int, n int) []<-chan T
```

Like a switch statement: the function returns an index, and the item goes to that single output. Named `Switch` because it mirrors the Go `switch` construct - evaluating to select one case.

### 3. Distributor - Select ONE Output by Matcher

```go
// pipe package
type Distributor[In, Out any] struct { ... }

func (d *Distributor) When(matcher MatcherFunc, pipe Pipe[In, Out])
```

First matching handler receives the item. Used for content-based routing where the selection logic is more complex than an index.

### Comparison

| Operation | Package | Selection | Outputs per Item | Use Case |
|-----------|---------|-----------|------------------|----------|
| `Broadcast` | channel | none (all) | N (all) | Fan-out replication |
| `Switch` | channel | by index | 1 (selected) | Round-robin, partitioning |
| `Distributor` | pipe | by matcher | 1 (first match) | Content-based routing |

### Why "Switch" Instead of "Route"

1. **Clarity**: `Switch` immediately evokes "select one by index" (like `switch i { case 0: ... }`)
2. **Differentiation**: `Route` suggests routing tables or message routing, which is `Distributor`'s domain
3. **No conflict**: Avoids confusion with `message.Router` which does event-type routing

### Usage Examples

```go
// Broadcast: metrics to multiple consumers
outputs := channel.Broadcast(metrics, 3)
go storeToDatabase(outputs[0])
go sendToMonitoring(outputs[1])
go logToFile(outputs[2])

// Switch: partition by hash
outputs := channel.Switch(items, func(item Item) int {
    return hash(item.Key) % 4
}, 4)

// Distributor: route by content
dist := pipe.NewDistributor[*Event, *Result]()
dist.When(IsOrderEvent, orderProcessor)
dist.When(IsPaymentEvent, paymentProcessor)
dist.Default(unknownEventHandler)
```

## Consequences

**Breaking Changes:**
- `channel.Route` renamed to `channel.Switch`

**Benefits:**
- Clear semantic distinction between operations
- Self-documenting names
- No confusion with message.Router

**Drawbacks:**
- `Switch` may be unfamiliar as a function name (usually a keyword)
- Breaking change for Route users

## Links

- Related: [Plan 0013 - Channel Package Refactoring](../plans/0013-channel-package-refactoring.md)
- Related: ADR 0026 (Naming Conventions)

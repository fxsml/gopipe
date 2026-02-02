# ADR 0027: Producer/Trigger Separation

**Date:** 2026-02-02
**Status:** Proposed

## Context

Sources produce data, but the *when* of production is conflated with the *what*. A source that reads from a database doesn't inherently know when to poll. A source that generates timestamps doesn't know the interval.

Current approaches either:
1. Embed timing logic in the source (inflexible)
2. Require external coordination (complex)

## Decision

Separate the concerns of **what to produce** (Source) from **when to produce** (Trigger):

### Source - What to Produce

```go
// Source produces items on demand
type Source[Out any] interface {
    Source(ctx context.Context) ([]Out, error)
}
```

### Trigger - When to Produce

```go
// TriggerFunc signals when to invoke the source
type TriggerFunc func(ctx context.Context, trigger func()) error
```

### Producer - Combines Both

```go
// Producer autonomously streams items
type Producer[Out any] struct {
    source  Source[Out]
    trigger TriggerFunc
}

func NewProducer[Out any](source Source[Out], trigger TriggerFunc) *Producer[Out]

func (p *Producer[Out]) Pipe() Pipe[struct{}, Out]
```

### Built-in Triggers

```go
package trigger

// Interval triggers at fixed intervals
func Interval(d time.Duration) TriggerFunc

// Immediate triggers once immediately
func Immediate() TriggerFunc

// Cron triggers on cron schedule
func Cron(spec string) TriggerFunc

// OnDemand triggers when channel receives
func OnDemand(ch <-chan struct{}) TriggerFunc
```

### Usage

```go
// Database poller - polls every 5 seconds
source := NewSource(func(ctx context.Context) ([]Record, error) {
    return db.Query(ctx, "SELECT * FROM events WHERE processed = false")
})

producer := NewProducer(source, trigger.Interval(5*time.Second))

// Connect to pipeline
pipeline := pipe.Join(
    producer.Pipe(),
    processor.Pipe(),
    sink.Pipe(),
)
```

## Consequences

**Breaking Changes:**
- None (new API addition)

**Benefits:**
- Reusable triggers across different sources
- Easy to test sources in isolation (no timing)
- Composable timing strategies
- Clear separation of concerns

**Drawbacks:**
- Additional concept to learn
- Slightly more verbose for simple cases

## Links

- Related: [Plan 0014 - Pipe Interface Refactoring](../plans/0014-pipe-interface-refactoring.md)
- Related: ADR 0024 (Pure vs Impure Operations)

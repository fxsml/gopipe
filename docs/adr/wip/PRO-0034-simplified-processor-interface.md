# ADR 0034: Simplified Processor Interface

**Date:** 2025-12-17
**Status:** Proposed
**Supersedes:** Current Processor interface with Cancel method

## Context

The current Processor interface includes a Cancel method:

```go
type Processor[In, Out any] interface {
    Process(ctx context.Context, in In) (Out, error)
    Cancel(In, error)
}
```

The Cancel method adds complexity:
- Requires goroutine and synchronization
- Mixed responsibility (processing + cancellation handling)
- Most implementations have empty Cancel methods
- Alternative patterns exist (message acknowledgments)

## Decision

Simplify the Processor interface by removing the Cancel method:

```go
// Simplified - no Cancel method
type Processor[In, Out any] interface {
    Process(ctx context.Context, in In) ([]Out, error)
}

// ProcessFunc implements Processor
type ProcessFunc[In, Out any] func(context.Context, In) ([]Out, error)

func (f ProcessFunc[In, Out]) Process(ctx context.Context, in In) ([]Out, error) {
    return f(ctx, in)
}
```

Error handling moves to:
1. **ProcessorConfig.OnError callback** - for logging/metrics
2. **Message acknowledgment** - for retry/dead-letter patterns
3. **Error routing middleware** - for messaging systems

## Consequences

**Positive:**
- Simpler interface, easier to implement
- Removes goroutine and synchronization complexity
- One less method to implement for every processor
- Clearer separation of concerns

**Negative:**
- Breaking change for existing Processor implementations
- Must migrate cancel logic to OnError callback or middleware
- Loss of dedicated cancel draining (mitigated by message acks)

**Migration:**

```go
// Old
type MyProcessor struct{}

func (p *MyProcessor) Process(ctx context.Context, in Order) (ShippingCommand, error) {
    return process(in)
}

func (p *MyProcessor) Cancel(in Order, err error) {
    log.Printf("canceled: %v", err)
}

// New
type MyProcessor struct{}

func (p *MyProcessor) Process(ctx context.Context, in Order) ([]ShippingCommand, error) {
    return []ShippingCommand{process(in)}, nil
}

// Cancel logic moves to config
config := ProcessorConfig{
    OnError: func(in any, err error) {
        log.Printf("error: %v", err)
    },
}
```

## Links

- Extracted from: [PRO-0026](PRO-0026-pipe-processor-simplification.md)
- Related: [PRO-0033](PRO-0033-non-generic-processor-config.md) - Non-Generic ProcessorConfig
- Related: [IMP-0017](IMP-0017-message-acknowledgment.md) - Message Acknowledgment

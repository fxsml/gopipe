# ADR 0039: Flow Control and Backpressure

**Date:** 2025-12-17
**Status:** Proposed
**Related:** PRO-0031 (issue #3)

## Context

Buffer sizing, broker-specific limits, and slow consumer detection are unaddressed. Need configurable flow control with backpressure handling.

## Decision

Introduce `FlowConfig` and `FlowControlledChannel`:

```go
type FlowConfig struct {
    BufferSize         int                // Default: 256
    HighWaterMark      int                // Default: 80%
    LowWaterMark       int                // Default: 50%
    SlowConsumerAction SlowConsumerAction
    OnSlowConsumer     func(bufferUsage float64, pending int)
}

type SlowConsumerAction int

const (
    SlowConsumerWarn       SlowConsumerAction = iota
    SlowConsumerBlock
    SlowConsumerDrop
    SlowConsumerDropOldest
)

type FlowControlledChannel[T any] struct {
    ch     chan T
    config FlowConfig
    stats  FlowStats
}

type FlowStats struct {
    Received     int64
    Sent         int64
    Dropped      int64
    SlowConsumer bool
    BufferUsage  float64
    LastSlowAt   time.Time
}

func NewFlowControlledChannel[T any](config FlowConfig) *FlowControlledChannel[T]
func (fc *FlowControlledChannel[T]) Send(ctx context.Context, v T) error
func (fc *FlowControlledChannel[T]) Receive() <-chan T
func (fc *FlowControlledChannel[T]) Stats() FlowStats
```

## Consequences

**Positive:**
- Configurable backpressure handling
- Slow consumer detection and metrics
- Multiple drop strategies

**Negative:**
- Additional channel wrapper
- Performance overhead for stats tracking

## Links

- Extracted from: [PRO-0031](PRO-0031-solutions-to-known-issues.md)

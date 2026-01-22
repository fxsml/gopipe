# Preserved Message Ordering for Worker Pool

**Status:** Proposed

**Depends on:** [pipe-autoscaling](pipe-autoscaling.md) (PoolConfig naming convention)

## Overview

Add optional preserved message ordering to the worker pool. When enabled, outputs are delivered in the same order as inputs, despite parallel processing by multiple workers.

## Motivation

Concurrent processing improves throughput but loses input order:

```
Without ordering (current behavior):
  Input:  [A, B, C, D]  (A is slow, B/C/D are fast)
  Output: [B, C, D, A]  (completion order)

With ordering (this proposal):
  Input:  [A, B, C, D]
  Output: [A, B, C, D]  (input order preserved)
```

**Use cases:**
- Event sourcing where order matters
- Stream processing with sequence dependencies
- Audit logs requiring chronological order
- Any pipeline where downstream expects ordered data

## Design

### Architecture

```
                    ┌─────────────────────────────────┐
                    │         Worker Pool             │
                    │   ┌─────────┐                   │
Input → [Sequencer] │   │ Worker1 │──┐               │
         (seq++)    │   ├─────────┤  │               │
                    │   │ Worker2 │──┼─→ [Reorderer] │ → Output
                    │   ├─────────┤  │    (buffer)   │
                    │   │ WorkerN │──┘               │
                    │   └─────────┘                  │
                    │    ↑ static or autoscale ↓     │
                    └─────────────────────────────────┘
```

**Components:**
1. **Sequencer**: Single goroutine assigns monotonic sequence numbers to inputs
2. **Workers**: Process in parallel (existing static or autoscale logic)
3. **Reorderer**: Single goroutine buffers out-of-order results, releases in sequence

### Mechanism

```go
// Internal types
type sequenced[T any] struct {
    seq uint64
    val T
}

type sequencedResult[T any] struct {
    seq     uint64
    results []T  // ProcessFunc can return multiple outputs
}
```

**Sequencer** (input side):
```go
func sequenceInputs[T any](in <-chan T) <-chan sequenced[T] {
    out := make(chan sequenced[T])
    go func() {
        defer close(out)
        var seq uint64
        for v := range in {
            out <- sequenced[T]{seq: seq, val: v}
            seq++
        }
    }()
    return out
}
```

**Reorderer** (output side):
```go
type reorderer[T any] struct {
    nextSeq    uint64
    buffer     map[uint64][]T  // or min-heap for efficiency
    bufferSize int
}

func (r *reorderer[T]) receive(item sequencedResult[T]) []T {
    if item.seq == r.nextSeq {
        // In order - emit immediately, then drain consecutive buffered
        return r.emitConsecutive(item.results)
    }
    // Out of order - buffer
    r.buffer[item.seq] = item.results
    return nil
}

func (r *reorderer[T]) emitConsecutive(first []T) []T {
    result := first
    r.nextSeq++
    for {
        if next, ok := r.buffer[r.nextSeq]; ok {
            delete(r.buffer, r.nextSeq)
            result = append(result, next...)
            r.nextSeq++
        } else {
            break
        }
    }
    return result
}
```

### Configuration API

```go
type PoolConfig struct {
    // ... existing fields from pipe-autoscaling.md ...
    Workers         int
    Autoscale       *AutoscaleConfig
    BufferSize      int
    ErrorHandler    func(in any, err error)
    // ...

    // PreserveOrder enables in-order message delivery.
    // When true, outputs are reordered to match input sequence
    // despite parallel processing. Has memory/latency overhead.
    // Default: false
    PreserveOrder bool

    // OrderBufferSize is the max items to buffer while waiting
    // for in-sequence items. Only used when PreserveOrder is true.
    // When buffer fills, workers block (backpressure).
    // Default: max(Workers, MaxWorkers) * 2
    OrderBufferSize int
}
```

### Processing Dispatcher Update

```go
func startProcessing[In, Out any](...) <-chan Out {
    cfg = cfg.parse()

    if cfg.PreserveOrder {
        return startOrderedProcessing(ctx, in, fn, cfg)
    }
    if cfg.Autoscale != nil {
        return startAutoscaledProcessing(ctx, in, fn, cfg)
    }
    return startStaticProcessing(ctx, in, fn, cfg)
}

func startOrderedProcessing[In, Out any](...) <-chan Out {
    // 1. Wrap inputs with sequence numbers
    seqIn := sequenceInputs(in)

    // 2. Wrap ProcessFunc to carry sequence through
    seqFn := func(ctx context.Context, s sequenced[In]) ([]sequencedResult[Out], error) {
        results, err := fn(ctx, s.val)
        if err != nil {
            return nil, err
        }
        return []sequencedResult[Out]{{seq: s.seq, results: results}}, nil
    }

    // 3. Process using existing workers (static or autoscale)
    var seqOut <-chan sequencedResult[Out]
    if cfg.Autoscale != nil {
        seqOut = startAutoscaledProcessing(ctx, seqIn, seqFn, cfg)
    } else {
        seqOut = startStaticProcessing(ctx, seqIn, seqFn, cfg)
    }

    // 4. Reorder outputs
    return reorderOutputs(seqOut, cfg.OrderBufferSize)
}
```

## Performance Characteristics

### Benefits

| Scenario | Impact |
|----------|--------|
| Variable processing times | **High** - fast items complete while slow ones block sequential processing |
| I/O-bound work | **High** - parallel I/O wait amortizes latency |
| CPU-bound with multi-core | **Moderate** - true parallelism benefits |

### Overhead

| Component | Cost |
|-----------|------|
| Sequencer | Minimal - single atomic increment per item |
| Reorderer | O(1) map operations, bounded memory |
| Wrapping | One allocation per item for `sequenced[T]` struct |

### Head-of-Line Blocking

This is the fundamental trade-off of ordered delivery:

```
Scenario: Item 0 takes 5s, items 1-99 take 10ms each

Timeline:
  t=0:     Item 0 starts on Worker1
  t=10ms:  Item 1 done, buffered (waiting for 0)
  t=20ms:  Item 2 done, buffered
  ...
  t=990ms: Buffer full (OrderBufferSize items), workers block
  t=5s:    Item 0 done, buffer drains instantly

Result: ~5s apparent stall, then burst of 100 outputs
```

**This is inherent to ordering guarantees, not a design flaw.**

**Mitigations:**
1. Larger `OrderBufferSize` (trades memory for throughput)
2. Design ProcessFuncs to have bounded execution time
3. Use circuit breakers/timeouts in ProcessFunc for slow operations

## Compatibility

### Works with Static Workers

```go
p := pipe.NewProcessPipe(fn, pipe.PoolConfig{
    Workers:       4,
    PreserveOrder: true,
})
```

### Works with Autoscaling

```go
p := pipe.NewProcessPipe(fn, pipe.PoolConfig{
    Autoscale: &pipe.AutoscaleConfig{
        MinWorkers: 2,
        MaxWorkers: 16,
    },
    PreserveOrder:   true,
    OrderBufferSize: 32,  // 2x MaxWorkers
})
```

**Note:** With autoscaling, `OrderBufferSize` should be based on `MaxWorkers` to accommodate maximum spread of in-flight items.

### Autoscaling Considerations

| Aspect | Impact |
|--------|--------|
| Scale-up trigger | Works normally - scaler sees input queue depth |
| Scale-down | Works normally - idle workers still scale down |
| Buffer sizing | Should use `MaxWorkers`, not current worker count |
| Backpressure | Reorderer buffer full → workers block → input queue grows → may trigger scale-up |

## Implementation Plan

### File Structure

```
pipe/
├── pool.go                   # Add PreserveOrder, OrderBufferSize fields
├── ordering.go               # NEW: sequenced types, sequenceInputs, reorderOutputs
├── processing.go             # Add startOrderedProcessing dispatcher
└── internal/autoscale/
    └── ...                   # No changes needed
```

### Implementation Steps

1. Add `PreserveOrder` and `OrderBufferSize` to `PoolConfig`
2. Create `ordering.go` with sequencer and reorderer
3. Update `startProcessing` dispatcher
4. Add tests for ordered static processing
5. Add tests for ordered autoscale processing
6. Add benchmark comparing ordered vs unordered

## Alternatives Considered

### Partitioned Workers (Kafka-style)

Messages with same key go to same worker, preserving order within partitions.

**Pros:** Efficient, no reorder buffer needed
**Cons:** Only partial ordering (per-key), requires key function

**Verdict:** Could be added as `PartitionKey func(In) string` for use cases that don't need total ordering.

### Single Worker When Ordered

Use `Workers: 1` when ordering is needed.

**Pros:** Trivially correct
**Cons:** Defeats purpose of concurrency, no throughput benefit

**Verdict:** User can already do this; `PreserveOrder` provides ordered + parallel.

### Slot-based Ring Buffer

Pre-allocate slots, workers claim slots before processing.

**Pros:** Fixed memory, no map overhead
**Cons:** More complex coordination, harder to handle multi-output ProcessFunc

**Verdict:** Map-based reorderer is simpler and sufficient for expected use cases.

## Future Extensions (out of scope)

- **PartitionKey**: Order within partitions only (like Kafka consumer groups)
- **OrderTimeout**: Skip sequence gap after timeout (trades correctness for throughput)
- **Metrics**: Buffer utilization, head-of-line blocking duration

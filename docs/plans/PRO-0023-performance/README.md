# PRO-0023: Performance Benchmarks

**Status:** Proposed
**Priority:** Low
**Related ADRs:** PRO-0044

## Overview

Create benchmark suite and performance guidelines.

## Goals

1. Create comprehensive benchmarks
2. Document performance guidelines
3. Implement message pool

## Task 1: Benchmarks

**Files to Create:**
- `benchmark/pipeline_test.go`
- `benchmark/groupby_test.go`

```go
func BenchmarkPipeline_Passthrough(b *testing.B)
func BenchmarkPipeline_WithMiddleware(b *testing.B)
func BenchmarkPipeline_Concurrent(b *testing.B)
func BenchmarkGroupBy_SmallBatches(b *testing.B)
func BenchmarkGroupBy_LargeBatches(b *testing.B)
func BenchmarkGroupBy_ManyGroups(b *testing.B)
```

## Task 2: Message Pool

**Files to Create:**
- `message/pool.go`

```go
var MessagePool = sync.Pool{
    New: func() any {
        return &Message{Attributes: make(Attributes, 8)}
    },
}

func AcquireMessage() *Message
func ReleaseMessage(msg *Message)
```

## Task 3: Performance Documentation

**Files to Create:**
- `docs/performance.md`

Document buffer sizing, concurrency, anti-patterns.

**Acceptance Criteria:**
- [ ] Pipeline benchmarks
- [ ] GroupBy benchmarks
- [ ] Message pool implemented
- [ ] Performance guide documented
- [ ] CHANGELOG updated

## Related

- [PRO-0044](../../adr/PRO-0044-performance-benchmarks.md) - ADR

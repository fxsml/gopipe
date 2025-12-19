# ADR 0044: Performance Benchmarks and Guidelines

**Date:** 2025-12-17
**Status:** Proposed
**Related:** PRO-0031 (issue #8)

## Context

No benchmarks exist and optimization guidelines are unclear. Need established benchmarks and documented performance patterns.

## Decision

### Benchmarks

Create comprehensive benchmark suite:

```go
// benchmark/pipeline_test.go
func BenchmarkPipeline_Passthrough(b *testing.B)
func BenchmarkPipeline_WithMiddleware(b *testing.B)
func BenchmarkPipeline_Concurrent(b *testing.B)
func BenchmarkGroupBy_SmallBatches(b *testing.B)
func BenchmarkGroupBy_LargeBatches(b *testing.B)
func BenchmarkGroupBy_ManyGroups(b *testing.B)
```

### Performance Guidelines

| Scenario | Recommended Buffer |
|----------|-------------------|
| Low latency | 0-10 |
| Balanced | 100-256 |
| High throughput | 1000+ |

**Concurrency:**
- Start with `Concurrency: runtime.NumCPU()`
- For I/O-bound: increase 2-4x
- For CPU-bound: match CPU count

**Common Anti-patterns:**
1. Creating channels in loops
2. Unbounded GroupBy with many unique keys
3. Excessive middleware chaining
4. Blocking operations in handlers

### Zero-Allocation Path

```go
var MessagePool = sync.Pool{
    New: func() any {
        return &Message{
            Attributes: make(Attributes, 8),
        }
    },
}

func AcquireMessage() *Message
func ReleaseMessage(msg *Message)
```

## Consequences

**Positive:**
- Measurable performance
- Clear guidelines
- Optimization path for high-throughput

**Negative:**
- Benchmark maintenance
- Pool complexity

## Links

- Extracted from: [PRO-0031](PRO-0031-solutions-to-known-issues.md)

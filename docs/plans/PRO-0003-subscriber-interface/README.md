# PRO-0003: Subscriber Interface

**Status:** Proposed
**Priority:** High
**Related ADRs:** PRO-0028

## Overview

Replace `Generator[Out]` interface with `Subscriber[Out]` interface, providing a clearer API for message sources.

## Goals

1. Rename Generator to Subscriber for clarity
2. Add factory functions for common patterns
3. Deprecate old Generator interface

## Task

**Goal:** Replace `Generator[Out]` with `Subscriber[Out]`

**Current:**
```go
type Generator[Out any] interface {
    Generate(ctx context.Context) <-chan Out
}
```

**Target:**
```go
type Subscriber[Out any] interface {
    Subscribe(ctx context.Context) <-chan Out
}

// Factory functions for common patterns
func NewTickerSubscriber[Out any](interval time.Duration, fn TickFunc[Out]) Subscriber[Out]
func NewPollingSubscriber[Out any](interval time.Duration, fn PollFunc[Out]) Subscriber[Out]
func NewFuncSubscriber[Out any](fn func(ctx context.Context) ([]Out, error)) Subscriber[Out]
```

**Files to Create/Modify:**
- `subscriber.go` (new) - Subscriber interface and factory functions
- `generator.go` (modify) - Deprecate, alias to Subscriber
- `pipe.go` (modify) - Update to use Subscriber

**Acceptance Criteria:**
- [ ] `Subscriber[T]` interface defined with Subscribe method
- [ ] Factory functions: NewTickerSubscriber, NewPollingSubscriber, NewFuncSubscriber
- [ ] Generator interface deprecated with godoc notice
- [ ] All tests pass
- [ ] CHANGELOG updated

## Related

- [PRO-0028](../../adr/PRO-0028-generator-source-patterns.md) - Generator/Subscriber Patterns ADR

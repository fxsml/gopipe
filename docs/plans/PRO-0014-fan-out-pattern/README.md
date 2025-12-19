# PRO-0014: Fan-Out Pattern

**Status:** Proposed
**Priority:** Medium
**Related ADRs:** PRO-0027

## Overview

Implement fan-out patterns including enhanced Broadcast, FanOutPipe, and RoutingFanOut for message distribution.

## Goals

1. Enhance `channel.Broadcast` with configuration
2. Create `FanOutPipe` for pipe integration
3. Create `RoutingFanOut` for destination-based routing

## Task 1: Enhanced Broadcast

**Goal:** Add configuration to Broadcast function

```go
type BroadcastConfig struct {
    Buffer         int
    OnSlowReceiver func(index int, val any, elapsed time.Duration)
    SlowThreshold  time.Duration
}

func BroadcastWithConfig[T any](
    in <-chan T,
    n int,
    config BroadcastConfig,
) []<-chan T
```

**Files to Create/Modify:**
- `channel/broadcast.go` - Add BroadcastWithConfig

## Task 2: FanOutPipe

**Goal:** Create FanOutPipe for pipe integration

```go
type FanOutPipe[In any] struct {
    outputs []Pipe[In, any]
    config  FanOutConfig
}

type FanOutConfig struct {
    Buffer         int
    OnSlowConsumer func(consumerIndex int, msg any)
    SlowThreshold  time.Duration
    FailFast       bool
}

func NewFanOutPipe[In any](
    config FanOutConfig,
    outputs ...Pipe[In, any],
) Pipe[In, struct{}]
```

**Files to Create/Modify:**
- `fanout_pipe.go` (new) - FanOutPipe implementation

## Task 3: RoutingFanOut

**Goal:** Create RoutingFanOut for destination-based routing

```go
type RoutingFanOut[T any] struct {
    router   RoutingFunc[T]
    outputs  map[string]chan<- T
    config   RoutingFanOutConfig
}

type RoutingFunc[T any] func(msg T) string

func NewRoutingFanOut[T any](
    router RoutingFunc[T],
    config RoutingFanOutConfig,
) *RoutingFanOut[T]
```

**Files to Create/Modify:**
- `channel/routing_fanout.go` (new) - RoutingFanOut implementation

**Acceptance Criteria:**
- [ ] `BroadcastWithConfig` implemented
- [ ] `FanOutPipe` implemented
- [ ] `RoutingFanOut` implemented
- [ ] Slow consumer detection works
- [ ] Tests for all fan-out patterns
- [ ] CHANGELOG updated

## Related

- [PRO-0027](../../adr/PRO-0027-fan-out-pattern.md) - ADR

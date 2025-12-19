# PRO-0015: Subscriber Patterns

**Status:** Proposed
**Priority:** High
**Related ADRs:** PRO-0028

## Overview

Implement unified Subscriber interface and specialized subscriber types for various message sources.

## Goals

1. Define `Subscriber[Out]` interface
2. Create `SubscriberConfig` struct
3. Implement specialized subscriber types
4. Define `BrokerSubscriber` interface

## Task 1: Subscriber Interface

**Goal:** Define base Subscriber interface

```go
type Subscriber[Out any] interface {
    Subscribe(ctx context.Context) <-chan Out
}
```

**Files to Create/Modify:**
- `subscriber.go` (new) - Subscriber interface

## Task 2: SubscriberConfig

**Goal:** Create configuration struct

```go
type SubscriberConfig struct {
    Interval     time.Duration
    MaxPerBatch  int
    OnError      func(err error)
    RetryOnError bool
    MaxRetries   int
    StopOnError  bool
    Buffer       int
}

var DefaultSubscriberConfig = SubscriberConfig{
    Buffer: 0,
}
```

**Files to Create/Modify:**
- `subscriber_config.go` (new) - SubscriberConfig struct

## Task 3: Specialized Subscribers

**Goal:** Implement subscriber factory functions

```go
func NewTickerSubscriber[Out any](
    interval time.Duration,
    generate func(ctx context.Context, tick time.Time) ([]Out, error),
    config SubscriberConfig,
) Subscriber[Out]

func NewPollingSubscriber[Out any](
    interval time.Duration,
    poll func(ctx context.Context) ([]Out, error),
    config SubscriberConfig,
) Subscriber[Out]

func NewChannelSubscriber[Out any](
    ch <-chan Out,
) Subscriber[Out]

func NewFuncSubscriber[Out any](
    generate func(ctx context.Context) ([]Out, error),
    config SubscriberConfig,
) Subscriber[Out]
```

**Files to Create/Modify:**
- `subscriber_ticker.go` (new) - TickerSubscriber
- `subscriber_polling.go` (new) - PollingSubscriber
- `subscriber_channel.go` (new) - ChannelSubscriber
- `subscriber_func.go` (new) - FuncSubscriber

## Task 4: BrokerSubscriber Interface

**Goal:** Define extended interface for message brokers

```go
type BrokerSubscriber interface {
    Subscriber[*Message]
    Acknowledge(ctx context.Context, msg *Message) error
    Seek(ctx context.Context, offset int64) error
    Pause(ctx context.Context) error
    Resume(ctx context.Context) error
}
```

**Files to Create/Modify:**
- `broker_subscriber.go` (new) - BrokerSubscriber interface

**Acceptance Criteria:**
- [ ] `Subscriber[Out]` interface defined
- [ ] `SubscriberConfig` struct defined
- [ ] `TickerSubscriber` implemented
- [ ] `PollingSubscriber` implemented
- [ ] `ChannelSubscriber` implemented
- [ ] `FuncSubscriber` implemented
- [ ] `BrokerSubscriber` interface defined
- [ ] Tests for all subscriber types
- [ ] Migration from Generator documented
- [ ] CHANGELOG updated

## Related

- [PRO-0028](../../adr/PRO-0028-generator-source-patterns.md) - ADR
- [PRO-0003](../PRO-0003-subscriber-interface/) - Previous subscriber work

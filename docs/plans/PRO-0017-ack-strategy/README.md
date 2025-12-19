# PRO-0017: Acknowledgment Strategy Interface

**Status:** Proposed
**Priority:** High
**Related ADRs:** PRO-0038

## Overview

Implement `AckStrategy` interface to handle broker-specific acknowledgment semantics.

## Goals

1. Define `AckStrategy` interface
2. Define `AckMode` constants
3. Define `AckBehavior` configuration

## Task

**Goal:** Create acknowledgment abstraction layer

**Files to Create:**
- `message/ack_strategy.go` - AckStrategy interface and AckMode
- `message/ack_behavior.go` - AckBehavior configuration

**Implementation:**
```go
type AckStrategy interface {
    Mode() AckMode
    OnAck(ctx context.Context, msg *Message) error
    OnNack(ctx context.Context, msg *Message, err error) error
    OnBatchAck(ctx context.Context, msgs []*Message) ([]*Message, error)
}
```

**Acceptance Criteria:**
- [ ] `AckStrategy` interface defined
- [ ] `AckMode` constants defined (None, PerMessage, Offset, Batch)
- [ ] `AckBehavior` config defined
- [ ] Tests for interface compliance
- [ ] CHANGELOG updated

## Related

- [PRO-0038](../../adr/PRO-0038-ack-strategy-interface.md) - ADR

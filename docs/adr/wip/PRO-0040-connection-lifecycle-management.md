# ADR 0040: Connection Lifecycle Management

**Date:** 2025-12-17
**Status:** Proposed
**Related:** PRO-0031 (issue #4)

## Context

NATS auto-reconnects, Kafka needs connection pools, RabbitMQ uses channels. Each broker handles connections differently.

## Decision

Define `ConnectionManager` interface and `ConnectionState` events:

```go
type ConnectionState int

const (
    ConnectionStateDisconnected ConnectionState = iota
    ConnectionStateConnecting
    ConnectionStateConnected
    ConnectionStateReconnecting
    ConnectionStateClosed
)

type ConnectionEvent struct {
    State     ConnectionState
    Error     error
    Attempt   int
    Timestamp time.Time
}

type ConnectionManager interface {
    State() ConnectionState
    Events() <-chan ConnectionEvent
    Reconnect(ctx context.Context) error
    Close() error
}

type ConnectionConfig struct {
    ConnectTimeout  time.Duration
    ReconnectPolicy ReconnectPolicy
    OnStateChange   func(event ConnectionEvent)
}

type ReconnectPolicy struct {
    Enabled         bool
    MaxAttempts     int
    InitialInterval time.Duration
    MaxInterval     time.Duration
    Multiplier      float64
}

func DefaultReconnectPolicy() ReconnectPolicy {
    return ReconnectPolicy{
        Enabled:         true,
        MaxAttempts:     0, // unlimited
        InitialInterval: time.Second,
        MaxInterval:     30 * time.Second,
        Multiplier:      2.0,
    }
}
```

## Consequences

**Positive:**
- Unified connection state handling
- Configurable reconnection
- Observable connection events

**Negative:**
- Each adapter must implement connection manager
- Additional complexity for simple use cases

## Links

- Extracted from: [PRO-0031](PRO-0031-solutions-to-known-issues.md)

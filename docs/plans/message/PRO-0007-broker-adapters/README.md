# PRO-0007: Broker Adapters

**Status:** Proposed
**Priority:** Medium
**Dependencies:** PRO-0003, PRO-0004, PRO-0005

## Overview

Broker adapters connect goengine to external messaging systems, translating between broker-specific protocols and goengine's Message type.

## Adapter Strategy

| Component | Implementation | Rationale |
|-----------|----------------|-----------|
| Connection | Direct broker library | Full control over lifecycle |
| Serialization | CloudEvents SDK | CE spec compliance |
| Message type | goengine Message | Type safety |

## Available Adapters

| Protocol | Library | Status |
|----------|---------|--------|
| NATS | `nats-io/nats.go` | Planned |
| Kafka | `IBM/sarama` | Planned |
| AMQP 1.0 | `Azure/go-amqp` | Planned |
| HTTP | `net/http` | Planned |
| RabbitMQ | `rabbitmq/amqp091-go` | Planned |

## Adapter Interface

```go
type Subscriber[T any] interface {
    Subscribe(ctx context.Context) <-chan T
}

type Publisher interface {
    Publish(ctx context.Context, msg *Message) error
    Close() error
}
```

## Examples

- [amqp_adapter.go](examples/amqp_adapter.go) - AMQP 1.0 adapter
- [http_adapter.go](examples/http_adapter.go) - HTTP adapter

## Related Evaluations

See [analysis/](../../../analysis/) for detailed evaluations:
- ce-adapters-evaluation.md - CloudEvents SDK bindings
- go-amqp-evaluation.md - Azure go-amqp library

## Related

- [PRO-0005](../PRO-0005-engine-orchestration/) - Engine Orchestration
- [PRO-0006](../PRO-0006-event-persistence/) - Event Persistence

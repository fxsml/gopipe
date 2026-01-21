# Plan 0003: CloudEvents SDK Boundary Adapters

**Status:** Complete
**Related ADRs:** [0022](../adr/0022-message-package-redesign.md)
**Depends On:** [Plan 0001](0001-message-engine.md) (Message Engine)

## Overview

Use the official CloudEvents SDK for Go (`cloudevents/sdk-go/v2`) at I/O boundaries for protocol bindings.

## Implementation

The `message/cloudevents/` package uses CE-SDK for boundary adapters:

| Component | Description |
|-----------|-------------|
| `Subscriber` | Wraps `cloudevents.Client` for receiving events |
| `Publisher` | Wraps `cloudevents.Client` for sending events |
| `FromEvent()` | Converts `cloudevents.Event` → `*message.Message` |
| `ToEvent()` | Converts `*message.Message` → `cloudevents.Event` |

### Benefits

- Native protocol bindings (HTTP, Kafka, NATS, AMQP)
- Correct CE serialization/deserialization per spec
- Battle-tested in production (Knative)
- Community-maintained

### Trade-offs

- CE-SDK dependency in adapter package (acceptable - external boundary concern)
- Protocol-specific configuration differs from custom implementation
- Some brokers have their own CE support

## Files

| File | Purpose |
|------|---------|
| `message/cloudevents/subscriber.go` | CE-SDK based subscriber |
| `message/cloudevents/publisher.go` | CE-SDK based publisher |
| `message/cloudevents/convert.go` | FromEvent/ToEvent conversion |

## Future Work

CESQL pattern matching was originally planned as Phase 1 but has been deferred. See [Plan 0014](0014-cesql-pattern-matching.md) for details.

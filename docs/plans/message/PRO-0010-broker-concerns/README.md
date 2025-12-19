# PRO-0010: Broker Concerns

**Status:** Proposed
**Priority:** Medium
**Dependencies:** PRO-0003, PRO-0007

## Overview

Address common broker adapter concerns: acknowledgment models, connection lifecycle, and testing infrastructure.

## Issues to Address

### 1. Acknowledgment Model Differences

Brokers handle acknowledgment differently:

| Broker | Ack Model | Batch Ack | Negative Ack |
|--------|-----------|-----------|--------------|
| **NATS** | Automatic (core), Manual (JetStream) | No | No (core) |
| **Kafka** | Offset-based, manual or auto-commit | Yes | No (don't commit) |
| **RabbitMQ** | Per-message, manual or auto-ack | Yes (multiple flag) | Yes (nack with requeue) |

**Solution:** `AckStrategy` interface with adapter-specific implementations.

### 2. Connection Lifecycle Management

Each broker has different connection requirements:

| Broker | Connection Model | Reconnection |
|--------|-----------------|--------------|
| **NATS** | Single connection, auto-reconnect | Built-in |
| **Kafka** | Per-topic writers, connection pool | Manual |
| **RabbitMQ** | Connection + Channels | Manual reconnect |

**Solution:** `ConnectionManager` interface with state events.

### 3. Serialization Boundary

Where serialization happens:
- Application serializes → Adapter receives `[]byte`
- Adapter serializes → Uses ContentType
- Hybrid → Support both

**Solution:** `Codec` interface with `CodecRegistry` at adapter boundaries.

### 4. Adapter Testing

Testing broker adapters requires:
- Running broker instances (Docker)
- Integration test setup/teardown
- Flaky test handling

**Solution:** `testcontainers-go` infrastructure and mock adapters.

## Tasks

Each issue above becomes a sub-task when implementing broker adapters (PRO-0007).

## Acceptance Criteria

- [ ] AckStrategy interface defined
- [ ] ConnectionManager interface defined
- [ ] Codec interface defined
- [ ] Test infrastructure documented

## Related

- [PRO-0007](../PRO-0007-broker-adapters/) - Broker Adapters
- [PRO-0003](../PRO-0003-message-standardization/) - Message Standardization

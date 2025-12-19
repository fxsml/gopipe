# PRO-0009: CloudEvents Overview

**Date:** 2025-12-13
**Status:** Proposed
**Author:** Claude

## Executive Summary

This plan outlines a phased approach to make CloudEvents a **mandatory specification** for all messaging in gopipe. The goal is to standardize behavior, enable automatic validation, simplify type handling, and create truly composable internal pipelines.

## Motivation

### Current Pain Points

1. **Optional CloudEvents**: CloudEvents attributes are optional, leading to inconsistent behavior
2. **Generic Complexity**: `TypedMessage[T]` generics make composable pipelines awkward (see `NewProcessPipe`)
3. **Manual Serialization**: Developers must handle serialization/deserialization explicitly
4. **Internal Routing Friction**: Building composable internal pipelines requires manual type juggling

### Vision

- **Mandatory CloudEvents**: All messages MUST have valid CloudEvents attributes
- **Simplified Message Type**: Single `Message` type with `Data any` (no generics)
- **Automatic Serialization**: ContentType-driven serialization at system boundaries
- **Topic-Based Internal Routing**: Seamless internal message flow using Topic attribute

## CloudEvents Specification Alignment

Per the [CloudEvents Specification](https://github.com/cloudevents/spec/blob/main/cloudevents/spec.md):

### Required Attributes (MUST be present)
- `id` - Unique identifier
- `source` - URI reference identifying the event source
- `specversion` - CloudEvents version (always "1.0")
- `type` - Event type identifier

### Optional Attributes
- `datacontenttype` - Content type of data (default: `application/json`)
- `subject` - Subject/routing key
- `time` - Timestamp (RFC3339)

### Topic as gopipe Extension

The CloudEvents spec **intentionally excludes destination/routing attributes**:
> "Routing information is not just redundant, it detracts. CloudEvents should increase interoperability and decouple the producer and consumer of events."

**Our approach**: Keep `topic` as a gopipe extension for **routing only**, not forwarded to external systems. This aligns with the spec since routing is protocol-level, not event-level.

## Feature Overview

### Phase 0: Core Pipe Refactoring (ADRs 0026-0028) ← NEW PREREQUISITE
Before implementing CloudEvents changes, simplify core abstractions:
- Replace generic options with non-generic ProcessorConfig struct
- Simplify cancel path (remove dedicated goroutine)
- Clear separation of config vs middleware
- Enhanced fan-out with slow-receiver handling
- Unified Subscriber interface with specialized types (Ticker, Polling, Broker)

### Phase 1: CloudEvents Mandatory (ADR 0019)
Enforce CloudEvents required attributes on all messages.

### Phase 2: Non-Generic Message (ADR 0020)
Remove generics from Message, use `Data any` with type safety via ContentType.

### Phase 3: ContentType Serialization (ADR 0021)
Automatic serialization/deserialization at system boundaries based on ContentType.

### Phase 4: Internal Message Routing (ADR 0022)
Topic-based internal routing for composable pipelines without external systems.

### Phase 5: Destination Attribute (ADR 0024)
URI-based routing with `gopipe://` for internal and scheme-based external routing.

### Phase 6: Internal Message Loop (ADR 0023)
Complete internal messaging system with feedback loop and pluggable transport.

### External Package: NATS Integration
Optional external package for advanced messaging features (persistence, clustering).

### Phase 7: SQL Event Store (ADR 0025)
Durable event persistence with rich querying and transactional outbox support.

### Phase 8: Message Engine (ADR 0029)
Top-level orchestration component that declaratively wires subscribers, routers, and publishers with automatic internal loop handling.

## Implementation Order

```
┌──────────────────────────────────────────────────────────────────┐
│ Phase 0: Core Pipe Refactoring (ADRs 0026-0028) ← PREREQUISITE   │
│ - ProcessorConfig struct (replaces generic options)              │
│ - Simplified cancel path (no dedicated goroutine)                │
│ - Config vs Middleware separation                                │
│ - Enhanced FanOut with BroadcastConfig                           │
│ - Unified Subscriber interface (Ticker, Polling, Broker)         │
│ - SubscriberConfig and factory functions                         │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│ Phase 1: CloudEvents Mandatory (ADR 0019)                        │
│ - Enforce required attributes                                    │
│ - Validation on message creation                                 │
│ - Migration utilities                                            │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│ Phase 2: Non-Generic Message (ADR 0020)                          │
│ - Change Data from []byte to any                                 │
│ - Remove TypedMessage[T]                                         │
│ - Update all consumers                                           │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│ Phase 3: ContentType Serialization (ADR 0021)                    │
│ - Sender serializes based on ContentType                         │
│ - Receiver deserializes based on ContentType                     │
│ - Internal flow uses Go types directly                           │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│ Phase 4: Internal Message Routing (ADR 0022)                     │
│ - Topic-based internal routing                                   │
│ - Composable pipeline flow                                       │
│ - No external system dependency for internal messaging           │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│ Phase 5: Destination Attribute (ADR 0024)                        │
│ - URI-based routing: gopipe://, kafka://, nats://, http://       │
│ - Clear internal (gopipe://) vs external boundary                │
│ - Mirrors source attribute semantics                             │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│ Phase 6: Internal Message Loop (ADR 0023)                        │
│ - MessageChannel interface with pub/sub                          │
│ - NoopChannel (Go channels) - zero dependencies                  │
│ - Publisher/Subscriber adapters                                  │
│ - External break-out via destination scheme                      │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│ External: NATS Integration (gopipe-nats package)                 │
│ - NATSChannel implements MessageChannel                          │
│ - Embedded NATS option (zero infrastructure)                     │
│ - JetStream for persistence                                      │
│ - External sender/receiver for nats:// destinations              │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│ Phase 7: SQL Event Store (ADR 0025)                              │
│ - EventStore interface with pluggable drivers                    │
│ - PostgreSQL and SQLite drivers                                  │
│ - Query by CloudEvents attributes                                │
│ - Transactional outbox support                                   │
│ - Event replay for projections                                   │
└────────────────────────────┬─────────────────────────────────────┘
                             │
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│ Phase 8: Message Engine (ADR 0029)                               │
│ - Engine as top-level orchestrator                               │
│ - Declarative subscriber/router/publisher registration           │
│ - RoutingFanIn for merging subscriber outputs                    │
│ - RoutingFanOut for destination-based routing                    │
│ - Automatic internal loops via gopipe:// destination             │
│ - Source matching for router selection                           │
│ - Simple mode: Subscriber → Router → Publisher chain             │
└──────────────────────────────────────────────────────────────────┘
```

## Architecture Overview

### Before: Current State

```go
// Generic message - awkward for composition
type TypedMessage[T any] struct {
    Data       T
    Attributes map[string]any
}

// Type alias for pub/sub
type Message = TypedMessage[[]byte]

// Pipe with complex generics
func NewProcessPipe[In, Out any](
    handle func(context.Context, In) ([]Out, error),
) Pipe[In, Out]
```

### After: Target State

```go
// Unified message - simple composition
type Message struct {
    Data       any            // Go type internally, []byte at boundaries
    Attributes Attributes     // Mandatory CloudEvents attributes
}

// Validation on creation
func New(data any, attrs Attributes) (*Message, error) {
    if err := ValidateCloudEvents(attrs); err != nil {
        return nil, err
    }
    return &Message{Data: data, Attributes: attrs}, nil
}

// ContentType-driven serialization at boundaries
type Sender interface {
    Send(ctx context.Context, msgs []*Message) error
    // Serializes Data to []byte based on DataContentType
}

type Receiver interface {
    Receive(ctx context.Context) (<-chan *Message, error)
    // Deserializes []byte to registered Go type based on DataContentType
}

// Simple internal routing
type InternalRouter struct {
    handlers map[string]Handler  // topic -> handler
}
```

### Data Flow

```
External System                    gopipe Internal                    External System
     │                                   │                                  │
     ▼                                   │                                  │
┌─────────┐                              │                                  │
│ []byte  │                              │                                  │
└────┬────┘                              │                                  │
     │ Receiver.Receive()                │                                  │
     │ (deserialize by ContentType)      │                                  │
     ▼                                   ▼                                  │
┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐    │
│ Message │ -> │ Handler │ -> │ Message │ -> │ Handler │ -> │ Message │    │
│ Data:   │    └─────────┘    │ Data:   │    └─────────┘    │ Data:   │    │
│ GoType  │                   │ GoType  │                   │ GoType  │    │
└─────────┘                   └─────────┘                   └─────────┘    │
                                                                 │          │
                                                                 │          │
                                   Sender.Send()                 │          │
                                   (serialize by ContentType)    │          │
                                                                 ▼          ▼
                                                            ┌─────────┐
                                                            │ []byte  │
                                                            └─────────┘
```

## Related ADRs

| ADR | Title | Purpose |
|-----|-------|---------|
| [0026](../adr/0026-pipe-processor-simplification.md) | Pipe Simplification | Non-generic config, simplified cancel |
| [0027](../adr/0027-fan-out-pattern.md) | Fan-Out Pattern | Enhanced broadcast with config |
| [0028](../adr/0028-generator-source-patterns.md) | Subscriber Patterns | Unified Subscriber interface |
| [0019](../adr/0019-cloudevents-mandatory.md) | CloudEvents Mandatory | Enforce required CE attributes |
| [0020](../adr/0020-non-generic-message.md) | Non-Generic Message | Simplify message type |
| [0021](../adr/0021-contenttype-serialization.md) | ContentType Serialization | Automatic boundary serialization |
| [0022](../adr/0022-internal-message-routing.md) | Internal Message Routing | Topic-based internal routing |
| [0024](../adr/0024-destination-attribute.md) | Destination Attribute | URI-based routing destinations |
| [0023](../adr/0023-internal-message-loop.md) | Internal Message Loop | Feedback loop with pluggable transport |
| [0025](../adr/0025-sql-event-store.md) | SQL Event Store | Durable persistence with querying |
| [0029](../adr/0029-message-engine.md) | Message Engine | Top-level orchestration component |

## Related Features

| Feature | Title | Purpose |
|---------|-------|---------|
| [16](../features/16-core-pipe-refactoring.md) | Core Pipe Refactoring | Prerequisite simplification |
| [09](../features/09-cloudevents-mandatory.md) | CloudEvents Mandatory | Implementation details |
| [10](../features/10-non-generic-message.md) | Non-Generic Message | Implementation details |
| [11](../features/11-contenttype-serialization.md) | ContentType Serialization | Implementation details |
| [12](../features/12-internal-message-routing.md) | Internal Message Routing | Implementation details |
| [13](../features/13-internal-message-loop.md) | Internal Message Loop | Feedback loop implementation |
| [14](../features/14-nats-integration.md) | NATS Integration | External package plan |
| [15](../features/15-sql-event-store.md) | SQL Event Store | Persistence and querying |
| [17](../features/17-message-engine.md) | Message Engine | Engine orchestration implementation |

## Success Criteria

1. **Validation**: All messages created without required CE attributes fail with clear error
2. **Type Safety**: Internal handlers work with Go types, not `[]byte`
3. **Boundary Serialization**: External communication auto-serializes based on ContentType
4. **Composable Pipelines**: Internal message routing without external system dependency
5. **Backward Compatibility**: Migration path for existing code

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Breaking existing code | Provide migration utilities and deprecation warnings |
| Performance overhead | Lazy validation, efficient type registry |
| Complexity increase | Clear documentation, examples, and feature docs |
| External system compatibility | Boundary serialization maintains wire format |

## Timeline

Each phase can be implemented independently:

1. **Phase 1**: Foundation - can be merged first
2. **Phase 2**: Depends on Phase 1
3. **Phase 3**: Depends on Phase 2
4. **Phase 4**: Depends on Phase 3

## Complete Internal Loop Architecture

### Full Pipeline Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                              gopipe InternalLoop                                         │
│                                                                                          │
│  External Input                                                         External Output  │
│  (HTTP/Kafka)                                                           (Kafka/HTTP)    │
│       │                                                                      ▲          │
│       ▼                                                                      │          │
│  ┌─────────────────────────────────────────────────────────────────────────────────┐   │
│  │                    MessageChannel (NoopChannel or NATSChannel)                   │   │
│  │                                                                                  │   │
│  │   gopipe://        gopipe://        gopipe://        gopipe://                   │   │
│  │   orders           shipping         audit            complete                    │   │
│  │      │                │                │                │                        │   │
│  └──────┼────────────────┼────────────────┼────────────────┼────────────────────────┘   │
│         │                │                │                │                            │
│         ▼                ▼                ▼                ▼                            │
│  ┌────────────┐   ┌────────────┐   ┌────────────┐   ┌────────────┐                     │
│  │  Handler   │   │  Handler   │   │  Handler   │   │  Handler   │                     │
│  │  orders    │   │  shipping  │   │  audit     │   │  complete  │                     │
│  │            │   │            │   │            │   │            │                     │
│  │ type:      │   │ type:      │   │ type:      │   │ type:      │                     │
│  │ order.     │   │ shipping.  │   │ audit.     │   │ order.     │                     │
│  │ created    │   │ requested  │   │ event      │   │ completed  │                     │
│  └─────┬──────┘   └─────┬──────┘   └─────┬──────┘   └─────┬──────┘                     │
│        │                │                │                │                            │
│        ▼                ▼                ▼                │                            │
│   gopipe://        gopipe://        gopipe://             │                            │
│   shipping         audit            (sink)               │                            │
│        │                │                                │                            │
│        └────────────────┘                                │                            │
│               │                                          │                            │
│               │ (feedback via MessageChannel)            │                            │
│               │                                          │                            │
│               ▼                                          │ ◄── Break-out point        │
│        ┌──────────────┐                                  │                            │
│        │ Internal     │                                  │                            │
│        │ Routing      │                                  │                            │
│        └──────────────┘                                  │                            │
│                                                          │                            │
└──────────────────────────────────────────────────────────┼────────────────────────────┘
                                                           │
                                                           │ kafka://notifications/...
                                                           │ http://partner.com/webhook
                                                           │ nats://events.completed
                                                           ▼
                                                  ┌────────────────────┐
                                                  │ ExternalDispatcher │
                                                  │                    │
                                                  │ ┌────────────────┐ │
                                                  │ │ KafkaSender    │─┼──> Kafka Cluster
                                                  │ └────────────────┘ │
                                                  │ ┌────────────────┐ │
                                                  │ │ HTTPSender     │─┼──> HTTP Endpoint
                                                  │ └────────────────┘ │
                                                  │ ┌────────────────┐ │
                                                  │ │ NATSSender     │─┼──> External NATS
                                                  │ └────────────────┘ │
                                                  └────────────────────┘
```

### Destination vs Topic Namespace Decision

**Recommendation: Use `destination` attribute with URI scheme**

See [ADR 0024](../adr/0024-destination-attribute.md) for full analysis.

| Approach | Pros | Cons |
|----------|------|------|
| Topic Namespace (`/gopipe/internal/`) | Simple, uses existing attr | Mixes concerns, limited extensibility |
| **Destination URI (`gopipe://`)** | Clear intent, URI extensibility, mirrors source | New attribute |

**Chosen**: Destination attribute because:
1. **Explicit**: `gopipe://orders` clearly indicates internal routing
2. **Extensible**: `kafka://`, `nats://`, `http://` for external
3. **Mirrors source**: `source` (where from) and `destination` (where to) are symmetric
4. **Protocol encoding**: Destination can include protocol-specific routing info

### Message Attributes Summary

| Attribute | Purpose | Example | Required |
|-----------|---------|---------|----------|
| `id` | Unique event ID | `"550e8400-e29b..."` | Yes (CE) |
| `source` | Event origin | `"/orders/api"` | Yes (CE) |
| `specversion` | CE version | `"1.0"` | Yes (CE) |
| `type` | Event classification | `"order.created"` | Yes (CE) |
| `datacontenttype` | Data format | `"application/json"` | No (CE) |
| `topic` | Pub/sub topic (semantic) | `"orders"` | No (gopipe) |
| `destination` | Routing target (physical) | `"gopipe://shipping"` | No (gopipe) |

### Example: Complete Order Flow

```go
// 1. External HTTP receives order
// POST /orders -> creates message with destination: gopipe://orders

// 2. Orders handler processes
loop.Route("orders", func(ctx context.Context, msg *Message) ([]*Message, error) {
    order := msg.Data.(Order)

    return []*Message{
        // Internal: shipping
        MustNew(ShippingCmd{OrderID: order.ID}, Attributes{
            AttrDestination: "gopipe://shipping",
            AttrType:        "shipping.requested",
            // ...CE attrs
        }),
        // Internal: inventory
        MustNew(InventoryCmd{OrderID: order.ID}, Attributes{
            AttrDestination: "gopipe://inventory",
            AttrType:        "inventory.reserve",
            // ...CE attrs
        }),
    }, nil
})

// 3. Shipping handler processes
loop.Route("shipping", func(ctx context.Context, msg *Message) ([]*Message, error) {
    cmd := msg.Data.(ShippingCmd)

    return []*Message{
        // Internal: back to complete handler
        MustNew(ShippedEvent{OrderID: cmd.OrderID}, Attributes{
            AttrDestination: "gopipe://complete",
            AttrType:        "order.shipped",
        }),
    }, nil
})

// 4. Complete handler breaks out
loop.Route("complete", func(ctx context.Context, msg *Message) ([]*Message, error) {
    event := msg.Data.(ShippedEvent)

    return []*Message{
        // External: Kafka for analytics
        MustNew(AnalyticsEvent{...}, Attributes{
            AttrDestination: "kafka://analytics/order-events",
            AttrType:        "analytics.order.complete",
        }),
        // External: HTTP webhook to partner
        MustNew(WebhookPayload{...}, Attributes{
            AttrDestination: "http://partner.example.com/orders/webhook",
            AttrType:        "webhook.order.complete",
        }),
    }, nil
})
```

## Sources

- [CloudEvents Specification](https://github.com/cloudevents/spec/blob/main/cloudevents/spec.md)
- [CloudEvents Primer](https://github.com/cloudevents/spec/blob/main/cloudevents/primer.md)
- [NL GOV profile for CloudEvents](https://logius-standaarden.github.io/NL-GOV-profile-for-CloudEvents/)
- [NATS Documentation](https://docs.nats.io/)

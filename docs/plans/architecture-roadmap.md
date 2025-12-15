# gopipe Architecture Roadmap

**Date:** 2025-12-15
**Status:** Master Plan
**Author:** Claude

## Executive Summary

This document provides the **big picture** of gopipe's evolution from a channel processing library to a complete CloudEvents-based messaging system. The roadmap is organized into four hierarchical layers, each building on the previous.

## Vision

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              gopipe Vision                                   │
│                                                                              │
│  "A composable, CloudEvents-native messaging framework for Go"              │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                     message.Engine (Layer 3)                          │   │
│  │  Declarative orchestration: Subscribers → Routers → Publishers        │   │
│  └───────────────────────────────────┬──────────────────────────────────┘   │
│                                      │                                       │
│  ┌───────────────────────────────────┴──────────────────────────────────┐   │
│  │                   Routing Infrastructure (Layer 2)                    │   │
│  │  Internal routing, destination URIs, feedback loops                   │   │
│  └───────────────────────────────────┬──────────────────────────────────┘   │
│                                      │                                       │
│  ┌───────────────────────────────────┴──────────────────────────────────┐   │
│  │                  Message Standardization (Layer 1)                    │   │
│  │  CloudEvents mandatory, non-generic Message, auto-serialization       │   │
│  └───────────────────────────────────┬──────────────────────────────────┘   │
│                                      │                                       │
│  ┌───────────────────────────────────┴──────────────────────────────────┐   │
│  │                    Foundation Cleanup (Layer 0)                       │   │
│  │  ProcessorConfig, Subscriber patterns, middleware consolidation       │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                    Persistence (Extension)                            │   │
│  │  SQL Event Store, NATS JetStream                                      │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Current State vs Target State

### Current State

```go
// Generic, verbose, inconsistent
pipe := gopipe.NewProcessPipe(
    handler,
    gopipe.WithConcurrency[Order, ShippingCommand](4),
    gopipe.WithTimeout[Order, ShippingCommand](5*time.Second),
    gopipe.WithRetryConfig[Order, ShippingCommand](RetryConfig{...}),
)

// Message with optional CloudEvents attributes
msg := message.New([]byte(`{"id":"123"}`), message.Attributes{
    "type": "order.created",  // Optional, may be missing
})
```

### Target State

```go
// Clean, declarative, standardized
engine := message.NewEngine()

// Register components
engine.AddSubscriber("kafka", kafkaSub)
engine.AddRouter("orders", ordersRouter)
engine.AddPublisher("kafka://", kafkaPub)

// Handler with mandatory CloudEvents + auto Go types
func handleOrder(ctx context.Context, msg *message.Message) ([]*message.Message, error) {
    order := msg.Data.(Order)  // Already deserialized

    return []*message.Message{
        message.MustNew(ShippingCmd{OrderID: order.ID},
            message.WithType("shipping.requested"),
            message.WithDestination("gopipe://shipping"),
        ),
    }, nil
}

engine.Start(ctx)
```

## Layer Architecture

### Layer 0: Foundation Cleanup (Pre-requisite)

**Goal:** Simplify core abstractions before adding new features

| ADR | Title | Key Changes |
|-----|-------|-------------|
| 0026 | Processor Simplification | `ProcessorConfig` struct, simplified cancel |
| 0027 | Fan-Out Pattern | `BroadcastConfig`, `RoutingFanOut` |
| 0028 | Subscriber Patterns | `Subscriber[Out]` replaces `Generator` |

**Sub-Plan:** [Layer 0: Foundation Cleanup](layer-0-foundation-cleanup.md)

**Breaking Changes:**
- `Option[In, Out]` → `ProcessorConfig` + `Middleware[]`
- `Generator[Out]` → `Subscriber[Out]` hierarchy
- Cancel goroutine removed from processor

---

### Layer 1: Message Standardization

**Goal:** Make CloudEvents mandatory and simplify message handling

| ADR | Title | Key Changes |
|-----|-------|-------------|
| 0019 | CloudEvents Mandatory | Required CE attributes, validation |
| 0020 | Non-Generic Message | `Data any` instead of `TypedMessage[T]` |
| 0021 | ContentType Serialization | Auto-serialization at boundaries |

**Sub-Plan:** [Layer 1: Message Standardization](layer-1-message-standardization.md)

**Breaking Changes:**
- `message.New()` returns error if missing CE attributes
- `TypedMessage[T]` → `Message` with `Data any`
- Serialization moves to Sender/Receiver boundaries

---

### Layer 2: Routing Infrastructure

**Goal:** Enable composable internal pipelines with clear routing

| ADR | Title | Key Changes |
|-----|-------|-------------|
| 0022 | Internal Message Routing | Topic-based handler dispatch |
| 0024 | Destination Attribute | `gopipe://`, `kafka://`, `http://` URIs |
| 0023 | Internal Message Loop | `MessageChannel`, feedback loop |

**Sub-Plan:** [Layer 2: Routing Infrastructure](layer-2-routing-infrastructure.md)

**Breaking Changes:**
- New `destination` attribute for routing
- `topic` for semantic grouping only
- `InternalRouter` added

---

### Layer 3: Orchestration

**Goal:** Provide top-level Engine for complex scenarios

| ADR | Title | Key Changes |
|-----|-------|-------------|
| 0029 | Message Engine | Engine orchestrator, FanIn/FanOut |

**Sub-Plan:** [Layer 3: Engine Orchestration](layer-3-engine-orchestration.md)

**Non-Breaking:** Engine is additive, simple mode preserved

---

### Extension: Persistence

**Goal:** Durable event storage with rich querying

| ADR | Title | Key Changes |
|-----|-------|-------------|
| 0025 | SQL Event Store | `EventStore` interface, SQL drivers |

**Sub-Plan:** [Extension: Event Persistence](extension-event-persistence.md)

**Non-Breaking:** Separate package, opt-in

---

## Release Roadmap

### Release v1.0.0 (Q1 2025)

**Core Framework Release** - All layers implemented

| Component | Layer | Status |
|-----------|-------|--------|
| Foundation Cleanup | Layer 0 | Planned |
| CloudEvents Standardization | Layer 1 | Planned |
| Routing Infrastructure | Layer 2 | Planned |
| Message Engine | Layer 3 | Planned |

**Planning for v1.1:**
- Persistence architecture
- Outbox pattern design
- Transaction handling

---

### Release v1.1.0

**Persistence & Transactions**

| Feature | Description |
|---------|-------------|
| Event Persistence | SQL Event Store implementation |
| Outbox Pattern | Transactional outbox for reliable messaging |
| Transactions | Cross-component transaction support |

**Planning for v1.2:**
- Saga pattern
- Reply-to function for request/response

---

### Release v1.2.0

**Adapters Release** - External system integrations

> **Note:** All adapters are independent plugin modules maintained in separate repositories.

| Adapter | Repository | Features |
|---------|------------|----------|
| File Watcher | `gopipe-filewatcher` | inotify-based file system events |
| CDC (Postgres/MariaDB) | `gopipe-cdc` | Database change data capture via [Debezium CloudEvents](https://debezium.io/documentation/reference/3.4/integrations/cloudevents.html) |
| Redis | `gopipe-redis` | Messaging + Event Store |
| Postgres | `gopipe-postgres` | Messaging + Event Store + Outbox |
| GraphQL | `gopipe-graphql` | Leveraging GraphQL subscriptions |
| Azure Service Bus | `gopipe-azservicebus` | CloudEvents binding + AMQP format |
| CLI | `gopipe-cli` | Command-line adapter |

---

### Release v1.3.0

**Advanced Features**

| Feature | Description |
|---------|-------------|
| Peek Function | Non-destructive message inspection for receivers |
| Saga Pattern | Distributed transaction coordination |
| Reply-To | Request/response messaging pattern |

---

### Example Application: Basic ERP/Ecom

**Reference implementation demonstrating gopipe capabilities**

| Aspect | Implementation |
|--------|----------------|
| Language | Go |
| Messaging | gopipe |
| Default Adapter | NATS.io |
| Patterns | Event Store, CQRS, Outbox |

**Scoped Use Cases:**
- Upsert Product
- Payment Processing
- Order Management
- Inventory Updates

---

## Implementation Phases

```
Phase 0: Foundation                Phase 1: Messages           Phase 2: Routing
┌─────────────────────────┐       ┌──────────────────────┐    ┌──────────────────────┐
│ ✓ ProcessorConfig       │       │ CloudEvents required │    │ Internal routing     │
│ ✓ Middleware package    │──────►│ Non-generic Message  │───►│ Destination URIs     │
│ ✓ Subscriber interface  │       │ Auto serialization   │    │ Message loop         │
│ ✓ BroadcastConfig       │       └──────────────────────┘    └──────────────────────┘
│ ✓ RoutingFanOut         │                │                           │
└─────────────────────────┘                │                           │
                                           ▼                           ▼
                                  Phase 3: Engine            Extensions
                                  ┌──────────────────────┐   ┌─────────────────────┐
                                  │ Engine orchestrator  │   │ SQL Event Store     │
                                  │ FanIn / FanOut       │   │ NATS Integration    │
                                  │ Internal loops       │   │ (separate package)  │
                                  └──────────────────────┘   └─────────────────────┘
```

### Release Mapping

```
v1.0.0 (Q1 2025)          v1.1.0                    v1.2.0                 v1.3.0
┌──────────────────┐      ┌──────────────────┐      ┌──────────────────┐   ┌──────────────┐
│ Layer 0-3        │      │ Persistence      │      │ Adapters         │   │ Advanced     │
│ ─────────────────│      │ ─────────────────│      │ ─────────────────│   │ ─────────────│
│ • Foundation     │      │ • Event Store    │      │ • gopipe-cdc     │   │ • Peek       │
│ • CloudEvents    │─────►│ • Outbox         │─────►│ • gopipe-redis   │──►│ • Saga       │
│ • Routing        │      │ • Transactions   │      │ • gopipe-postgres│   │ • Reply-To   │
│ • Engine         │      │                  │      │ • gopipe-az...   │   │              │
└──────────────────┘      └──────────────────┘      └──────────────────┘   └──────────────┘
```

## Component Taxonomy

### Message Flow Components

| Component | Direction | Use Case |
|-----------|-----------|----------|
| `Subscriber` | Source → | Produces messages (broker, ticker, polling) |
| `Router` | → Handler | Dispatches by type/topic to handlers |
| `Handler` | In → Out | Processes messages, produces outputs |
| `Publisher` | → Sink | Sends messages to external systems |

### Fan Components

| Component | Pattern | Routes To |
|-----------|---------|-----------|
| `Broadcast` | 1→N (all) | ALL outputs |
| `RoutingFanOut` | 1→N (one) | ONE output by function |
| `FanIn` | N→1 | Merges multiple inputs |

### Engine Components

| Component | Purpose |
|-----------|---------|
| `Engine` | Top-level orchestrator |
| `RoutingFanIn` | Merges subscriber outputs |
| `RoutingFanOut` | Routes by destination |
| `LoopChannel` | Internal feedback |

## API Changes (v1.0.0)

### Planned Breaking Changes

These changes are planned for v1.0.0. ADR code examples show the **target API**, not the current implementation.

| Component | Current API | Target API (v1.0.0) | ADR |
|-----------|-------------|---------------------|-----|
| Message type | `Message = TypedMessage[[]byte]` | `Message = TypedMessage[any]` | 0020 |
| Constructor | `New[T]() *TypedMessage[T]` | `New() (*Message, error)` with validation | 0019 |
| Options | `Option[In, Out]` funcs | `ProcessorConfig` struct | 0026 |
| Sources | `Generator[Out]` | `Subscriber[Out]` interface | 0028 |

### TypedMessage Decision

**TypedMessage[T] stays** - useful for non-messaging pipelines:

```go
// Preserved: TypedMessage[T] for pipelines (no validation)
msg := message.NewTyped(order, nil)

// New: Message = TypedMessage[any] for CloudEvents (validates)
msg, err := message.New(order, attrs)  // Returns error if missing CE attrs

// Deprecated: Old generic New[T]
msg := message.New(data, attrs)  // Will be deprecated
```

### ✓ Working Pattern: Simple Mode

For users who don't need Engine, the simple pattern remains:

```go
// This pattern works today and will continue to work
sub := broker.NewSubscriber(config)
router := message.NewRouter(routerConfig)
router.AddHandler(myHandler)
pub := broker.NewPublisher(config)

msgs := sub.Receive(ctx)
processed := router.Start(ctx, msgs)
pub.SendAll(ctx, processed)
```

## Dependency Graph

```
                    ┌──────────────────────────────────────────────────┐
                    │                    Layer 3                       │
                    │                                                  │
                    │    ┌────────────────────────────────────────┐   │
                    │    │         ADR 0029: Engine               │   │
                    │    │   Depends on: 0022, 0023, 0024, 0027,  │   │
                    │    │               0028                      │   │
                    │    └────────────────┬───────────────────────┘   │
                    │                     │                           │
                    └─────────────────────┼───────────────────────────┘
                                          │
           ┌──────────────────────────────┼──────────────────────────────┐
           │                              │                              │
┌──────────┴───────────────────────────────────────────────────────────┐
│                         Layer 2                                       │
│                                                                       │
│  ┌─────────────────┐   ┌─────────────────┐   ┌─────────────────────┐ │
│  │ ADR 0022        │   │ ADR 0024        │   │ ADR 0023            │ │
│  │ Internal        │◄──┤ Destination     │◄──┤ Internal Loop       │ │
│  │ Routing         │   │ Attribute       │   │                     │ │
│  └────────┬────────┘   └────────┬────────┘   └──────────┬──────────┘ │
│           │                     │                       │            │
└───────────┼─────────────────────┼───────────────────────┼────────────┘
            │                     │                       │
            ▼                     ▼                       ▼
┌──────────────────────────────────────────────────────────────────────┐
│                         Layer 1                                       │
│                                                                       │
│  ┌─────────────────┐   ┌─────────────────┐   ┌─────────────────────┐ │
│  │ ADR 0019        │   │ ADR 0020        │   │ ADR 0021            │ │
│  │ CloudEvents     │◄──┤ Non-Generic     │◄──┤ ContentType         │ │
│  │ Mandatory       │   │ Message         │   │ Serialization       │ │
│  └────────┬────────┘   └────────┬────────┘   └──────────┬──────────┘ │
│           │                     │                       │            │
└───────────┼─────────────────────┼───────────────────────┼────────────┘
            │                     │                       │
            ▼                     ▼                       ▼
┌──────────────────────────────────────────────────────────────────────┐
│                         Layer 0                                       │
│                                                                       │
│  ┌─────────────────┐   ┌─────────────────┐   ┌─────────────────────┐ │
│  │ ADR 0026        │   │ ADR 0027        │   │ ADR 0028            │ │
│  │ Processor       │   │ Fan-Out         │   │ Subscriber          │ │
│  │ Simplification  │   │ Pattern         │   │ Patterns            │ │
│  └─────────────────┘   └─────────────────┘   └─────────────────────┘ │
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘
```

## Migration Strategy

### Incremental Adoption

Each layer can be adopted independently:

1. **Layer 0 Only**: Cleaner API, same functionality
2. **+ Layer 1**: Validated messages, simplified types
3. **+ Layer 2**: Internal routing without external brokers
4. **+ Layer 3**: Full orchestration for complex scenarios

### Backward Compatibility

| Component | Strategy |
|-----------|----------|
| `Option[In, Out]` | Deprecated wrappers calling new APIs |
| `TypedMessage[T]` | **Preserved** for pipelines via `NewTyped[T]()` |
| `Message = TypedMessage[[]byte]` | **Deprecated** → `Message = TypedMessage[any]` |
| `New[T]()` | **Deprecated** → `NewTyped[T]()` or `New()` |
| `Generator[Out]` | Deprecated, delegates to `FuncSubscriber` |
| Current `Router` | Preserved, Engine is additive |

## Success Metrics

1. **Simplicity**: No generic parameters for common operations
2. **Validation**: 100% of messages have valid CloudEvents attributes
3. **Composability**: Build pipelines without external dependencies
4. **Flexibility**: Simple mode for basic cases, Engine for complex
5. **Type Safety**: Go types internal, serialization at boundaries

## Related Documents

### Sub-Plans
- [Layer 0: Foundation Cleanup](layer-0-foundation-cleanup.md)
- [Layer 1: Message Standardization](layer-1-message-standardization.md)
- [Layer 2: Routing Infrastructure](layer-2-routing-infrastructure.md)
- [Layer 3: Engine Orchestration](layer-3-engine-orchestration.md)
- [Extension: Event Persistence](extension-event-persistence.md)

### Existing Documentation
- [CloudEvents Standardization Plan](cloudevents-standardization.md)
- [ADR Index](../adr/README.md)
- [Feature Index](../features/README.md)

## Next Steps

### For v1.0.0 (Q1 2026)

1. **Implement Layer 0** - Foundation cleanup
2. **Implement Layer 1** - Message standardization
3. **Implement Layer 2** - Routing infrastructure
4. **Implement Layer 3** - Engine orchestration
5. **Plan persistence** - Design event store and outbox

### For v1.1.0

1. **Event Persistence** - SQL Event Store implementation
2. **Outbox Pattern** - Transactional message delivery
3. **Plan adapters** - Design adapter plugin architecture

### For v1.2.0

1. **Create adapter repos** - gopipe-cdc, gopipe-redis, etc.
2. **Implement core adapters** - PostgreSQL, Redis
3. **External adapters** - Azure Service Bus, CDC

### Future

1. **v1.3.0** - Advanced patterns (Peek, Saga, Reply-To)
2. **Example app** - ERP/Ecom reference implementation

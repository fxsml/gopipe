# CloudEvents Standardization - Current State

**Date:** 2025-12-15
**Branch:** `claude/cloud-events-standardization-plan-012MtyfboE1UBsnd1rZ59Dag`

## Source Documents

This state document consolidates:
- 11 ADRs: 0019-0029
- 9 Feature docs: 09-17
- 6 Plan docs: architecture-roadmap, cloudevents-standardization, layer-0/1/2/3, extension-event-persistence, uuid-integration
- Procedures: documentation, git, go, planning

## Summary

gopipe is evolving from a generic channel processing library to a **CloudEvents-native messaging framework**. The architecture is organized into four layers, each building on the previous, plus an extension for persistence.

```
Layer 3: Engine       → message.Engine orchestrates Subscribers, Routers, Publishers
Layer 2: Routing      → Internal message routing via destination URIs (gopipe://, kafka://)
Layer 1: Messages     → CloudEvents mandatory, non-generic Message, auto-serialization
Layer 0: Foundation   → ProcessorConfig struct, Subscriber interface, middleware consolidation
Extension: Persistence → SQL Event Store with pluggable drivers
```

## Key Decisions

| Decision | Rationale | ADR |
|----------|-----------|-----|
| CloudEvents attributes mandatory | Fail-fast validation, consistent behavior | 0019 |
| Non-generic Message (`Data any`) | Simpler composition, type safety via ContentType | 0020 |
| Serialize at boundaries only | Go types internal, []byte at edges | 0021 |
| Destination URI attribute | `gopipe://` internal, `kafka://` external routing | 0024 |
| Topic for semantics only | Routing via destination, not topic | 0022 |
| Subscriber replaces Generator | Unified interface for all sources (broker, ticker, polling) | 0028 |
| ProcessorConfig struct | Replace generic options, separate config from middleware | 0026 |
| Engine as orchestrator | Declarative wiring of components with auto-loops | 0029 |
| Zero-dependency UUID | Built-in `message.NewID()` with same signature as google/uuid | uuid-integration |

## Current API (Implemented)

```go
// UUID generation (NEW - implemented)
id := message.NewID()  // RFC 4122 UUID v4: "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx"
message.DefaultIDGenerator = uuid.NewString  // Optional: swap for production

// Message creation (existing)
msg := message.New(data, message.Attributes{...})

// CloudEvents attributes (existing)
message.AttrID, AttrSource, AttrSpecVersion, AttrType  // Required
message.AttrSubject, AttrTime, AttrDataContentType     // Optional CE
message.AttrTopic, AttrCorrelationID, AttrDestination  // gopipe extensions
```

## Planned API (Not Yet Implemented)

```go
// Validated message creation
msg, err := message.New(data, attrs)  // Returns error if missing CE attrs
msg := message.MustNew(data, attrs)   // Panics on validation failure

// Builder pattern
builder := message.NewBuilder("/orders")
msg := builder.Build(data, "order.created")

// Engine orchestration
engine := message.NewEngine()
engine.AddSubscriber("kafka", kafkaSub)
engine.AddRouter("orders", ordersRouter)
engine.AddPublisher("kafka://", kafkaPub)
engine.Start(ctx)
```

## Implementation Status

| Component | Status | Location | Notes |
|-----------|--------|----------|-------|
| UUID Generator | **Implemented** | `message/uuid.go` | `NewID()`, `DefaultIDGenerator` |
| CloudEvents Attributes | Implemented | `message/attributes.go` | Constants and accessors |
| TypedMessage[T] | Implemented | `message/message.go` | Will be deprecated for Message |
| Message = TypedMessage[[]byte] | Implemented | `message/message.go` | Will change to TypedMessage[any] |
| CE Validation | Planned | - | ADR 0019 |
| Builder Pattern | Planned | - | ADR 0019 |
| Non-Generic Message | Planned | - | ADR 0020 |
| ContentType Serialization | Planned | - | ADR 0021 |
| Internal Routing | Planned | - | ADR 0022 |
| Destination Attribute | Planned | - | ADR 0024 |
| Message Loop | Planned | - | ADR 0023 |
| SQL Event Store | Planned | - | ADR 0025 |
| Message Engine | Planned | - | ADR 0029 |

## Architecture Vision

```
┌─────────────────────────────────────────────────────────────────┐
│                        message.Engine                            │
│  Declarative: Subscribers → Routers → Publishers                 │
└───────────────────────────────┬─────────────────────────────────┘
                                │
┌───────────────────────────────┴─────────────────────────────────┐
│                    Routing Infrastructure                        │
│  gopipe://internal  kafka://external  http://webhooks            │
└───────────────────────────────┬─────────────────────────────────┘
                                │
┌───────────────────────────────┴─────────────────────────────────┐
│                   Message Standardization                        │
│  CloudEvents mandatory | Data any | Auto-serialization           │
└───────────────────────────────┬─────────────────────────────────┘
                                │
┌───────────────────────────────┴─────────────────────────────────┐
│                     Foundation (Existing)                        │
│  channel/* ops | gopipe.* pipes | message/* types                │
└─────────────────────────────────────────────────────────────────┘
```

## Next Steps (Priority Order)

1. **Layer 1**: Implement CloudEvents validation in `message.New()`
2. **Layer 1**: Create Builder pattern with IDGenerator
3. **Layer 1**: Migrate Message from `TypedMessage[[]byte]` to `TypedMessage[any]`
4. **Layer 0**: Simplify ProcessorConfig (if blocking Layer 1)
5. **Layer 2**: Add destination attribute and internal routing
6. **Layer 3**: Implement Engine orchestrator

## File Organization

```
message/
├── attributes.go      # CE attribute constants and accessors
├── message.go         # TypedMessage, Message, Acking
├── uuid.go            # NEW: NewID(), DefaultIDGenerator
├── uuid_test.go       # NEW: UUID format/uniqueness tests
├── builder.go         # PLANNED: Builder pattern
├── validation.go      # PLANNED: CE validation
└── ...

docs/
├── adr/               # 11 ADRs (0019-0029)
├── features/          # 9 feature docs (09-17)
├── plans/             # Architecture roadmap + layer plans
├── procedures/        # Git, Go, Documentation, Planning
├── state/             # THIS FILE - compressed current state
└── manual/            # User manual
```

## Quick Reference

| Need | Solution |
|------|----------|
| Generate message ID | `message.NewID()` |
| Create message | `message.New(data, attrs)` |
| Access CE attrs | `msg.Attributes.ID()`, `.Type()`, `.Source()` |
| gopipe extensions | `AttrTopic`, `AttrCorrelationID`, `AttrDestination` |
| Architecture overview | `docs/plans/architecture-roadmap.md` |
| ADR index | `docs/adr/README.md` |
| Feature index | `docs/features/README.md` |

# CloudEvents Standardization Plan

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

### Phase 1: CloudEvents Mandatory (ADR 0019)
Enforce CloudEvents required attributes on all messages.

### Phase 2: Non-Generic Message (ADR 0020)
Remove generics from Message, use `Data any` with type safety via ContentType.

### Phase 3: ContentType Serialization (ADR 0021)
Automatic serialization/deserialization at system boundaries based on ContentType.

### Phase 4: Internal Message Routing (ADR 0022)
Topic-based internal routing for composable pipelines without external systems.

## Implementation Order

```
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
| [0019](../adr/0019-cloudevents-mandatory.md) | CloudEvents Mandatory | Enforce required CE attributes |
| [0020](../adr/0020-non-generic-message.md) | Non-Generic Message | Simplify message type |
| [0021](../adr/0021-contenttype-serialization.md) | ContentType Serialization | Automatic boundary serialization |
| [0022](../adr/0022-internal-message-routing.md) | Internal Message Routing | Topic-based internal routing |

## Related Features

| Feature | Title | Purpose |
|---------|-------|---------|
| [09](../features/09-cloudevents-mandatory.md) | CloudEvents Mandatory | Implementation details |
| [10](../features/10-non-generic-message.md) | Non-Generic Message | Implementation details |
| [11](../features/11-contenttype-serialization.md) | ContentType Serialization | Implementation details |
| [12](../features/12-internal-message-routing.md) | Internal Message Routing | Implementation details |

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

## Sources

- [CloudEvents Specification](https://github.com/cloudevents/spec/blob/main/cloudevents/spec.md)
- [CloudEvents Primer](https://github.com/cloudevents/spec/blob/main/cloudevents/primer.md)
- [NL GOV profile for CloudEvents](https://logius-standaarden.github.io/NL-GOV-profile-for-CloudEvents/)

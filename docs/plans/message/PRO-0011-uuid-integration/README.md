# PRO-0011: UUID Integration

**Status:** Proposed
**Priority:** High (required for CloudEvents compliance)
**ADR:** [PRO-0012: Google UUID](../../adr/PRO-0012-google-uuid.md)
**Dependencies:** PRO-0003 (Message Standardization)

## Overview

Integrate `github.com/google/uuid` for automatic message ID generation in goengine. CloudEvents requires a unique `id` attribute for every event.

## Goals

1. Auto-generate UUIDs for message IDs when not provided
2. Support both random (v4) and time-ordered (v7) UUIDs
3. Maintain CloudEvents compliance

## Implementation

### 1. Add Dependency

```bash
go get github.com/google/uuid
```

### 2. Update Message Creation

```go
// message.go
import "github.com/google/uuid"

// New creates a new message with auto-generated ID if not provided
func New(data any, attrs Attributes) (*Message, error) {
    if attrs.ID == "" {
        attrs.ID = uuid.NewString()  // v4 random UUID
    }
    return validate(data, attrs)
}

// NewOrdered creates a message with time-ordered UUID (for event stores)
func NewOrdered(data any, attrs Attributes) (*Message, error) {
    if attrs.ID == "" {
        attrs.ID = uuid.Must(uuid.NewV7()).String()
    }
    return validate(data, attrs)
}
```

### 3. Configuration Option

```go
// For engines that prefer v7 UUIDs globally
type EngineConfig struct {
    UseOrderedUUIDs bool  // Use v7 instead of v4
}
```

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `go.mod` | Add | `github.com/google/uuid` dependency |
| `message/message.go` | Modify | Add ID auto-generation |
| `message/message_test.go` | Modify | Add UUID generation tests |

## Acceptance Criteria

- [ ] `github.com/google/uuid` added to go.mod
- [ ] `New()` auto-generates v4 UUID when ID is empty
- [ ] `NewOrdered()` generates v7 UUID for event stores
- [ ] User-provided IDs are preserved (not overwritten)
- [ ] UUID format is RFC 4122 compliant
- [ ] Tests cover all UUID generation paths
- [ ] CHANGELOG updated

## Migration from gopipe

The partial UUID implementation in gopipe's `develop` branch should be removed. This functionality belongs in goengine.

**gopipe cleanup:**
- Remove `message/uuid.go` (if exists)
- Remove any UUID generation from `message/message.go`

## Related

- [PRO-0012: Google UUID ADR](../../adr/PRO-0012-google-uuid.md)
- [PRO-0003: Message Standardization](../PRO-0003-message-standardization/)
- [PRO-0004: CloudEvents Mandatory ADR](../../adr/PRO-0004-cloudevents-mandatory.md)
- [gopipe PRO-0005](../../../plans/PRO-0005-uuid-integration/) - Won't Do (migrated here)

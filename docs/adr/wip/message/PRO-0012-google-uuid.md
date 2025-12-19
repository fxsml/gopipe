# PRO-0012: Google UUID for Message IDs

**Date:** 2025-12-17
**Status:** Proposed
**Dependencies:** PRO-0004 (CloudEvents Mandatory)

## Context

CloudEvents specification requires an `id` attribute that uniquely identifies the event. The spec recommends RFC 4122 UUIDs.

Options for UUID generation:
1. **Standard library** - `crypto/rand` + manual formatting (no external deps)
2. **google/uuid** - Well-maintained, widely used, RFC 4122 compliant
3. **gofrs/uuid** - Fork of satori/go.uuid, also well-maintained

goengine is designed as a separate package that can use external dependencies (unlike gopipe core).

## Decision

Use `github.com/google/uuid` for message ID generation in goengine.

### Rationale

1. **Maintenance**: Google maintains the package, regular updates
2. **Adoption**: Widely used in Go ecosystem (~50K+ GitHub stars)
3. **Compliance**: RFC 4122 compliant (v1, v4, v6, v7 support)
4. **Performance**: Optimized implementation
5. **API simplicity**: Clean API for common use cases

### Usage

```go
import "github.com/google/uuid"

// In message creation
func New(data any, attrs Attributes) (*Message, error) {
    if attrs.ID == "" {
        attrs.ID = uuid.NewString()  // v4 UUID
    }
    // ... validation
}

// For time-ordered UUIDs (v7 - recommended for databases)
func NewWithOrderedID(data any, attrs Attributes) (*Message, error) {
    if attrs.ID == "" {
        attrs.ID = uuid.Must(uuid.NewV7()).String()
    }
    // ...
}
```

### UUID Version Selection

| Version | Use Case |
|---------|----------|
| v4 (random) | Default for messages - no ordering needed |
| v7 (time-ordered) | Event stores - natural ordering by time |

## Consequences

### Positive

- Standard, well-tested UUID generation
- RFC 4122 compliance for CloudEvents
- Simple API for common cases
- v7 support for time-ordered IDs in event stores

### Negative

- External dependency (acceptable for goengine)
- ~2KB added to binary size

### Neutral

- gopipe remains dependency-free
- Users can still provide custom IDs if needed

## Implementation

See [PRO-0011: UUID Integration Plan](../plans/PRO-0011-uuid-integration/)

## Links

- [google/uuid](https://github.com/google/uuid)
- [RFC 4122](https://datatracker.ietf.org/doc/html/rfc4122)
- [CloudEvents ID attribute](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md#id)
- [PRO-0004: CloudEvents Mandatory](PRO-0004-cloudevents-mandatory.md)

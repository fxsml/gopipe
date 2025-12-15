# UUID Integration Plan for gopipe Message Package

**Date:** 2025-12-15
**Status:** Implemented

## Executive Summary

gopipe should provide a **built-in UUID generator** with zero external dependencies while maintaining API compatibility with `google/uuid.NewString()`. Users needing advanced features can switch to google/uuid with no code changes.

## Research Findings

### CloudEvents ID Requirements

CloudEvents specification v1.0.2 does **NOT** require RFC 4122 UUIDs:

> Producers MUST ensure that `source + id` is unique for each distinct event.

Valid ID options include:
- UUIDs (common practice)
- URNs
- DNS authorities
- Application-specific schemes

**Conclusion:** UUIDs are recommended practice but not mandated. We should provide UUID generation for convenience.

### google/uuid Package Analysis

**API Signature:**
```go
func NewString() string  // Returns "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx"
```

**Pros:**
- RFC 4122/9562 compliant
- Well-tested, production-proven
- Supports all UUID versions (1-7)
- Performance optimized (randomness pool)

**Cons:**
- External dependency (conflicts with "independent as possible" goal)
- Most features unused (only need v4)

### RFC 4122 UUID v4 Format

```
xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
         ^^^^-^   ^
         |    |   |
         |    |   +-- Variant bits (8, 9, A, or B)
         |    +------ Version 4
         +----------- Random
```

- **128 bits total**: 122 random + 6 fixed (4 version + 2 variant)
- **Format**: 8-4-4-4-12 hex characters with hyphens (36 chars)

## Options Analysis

### Option 1: google/uuid Dependency

```go
import "github.com/google/uuid"

var NewID = uuid.NewString
```

| Aspect | Assessment |
|--------|------------|
| Correctness | Excellent (RFC compliant) |
| Independence | Poor (external dependency) |
| Maintenance | Good (Google maintained) |
| Features | Excessive (only need v4) |

### Option 2: Minimal Built-in Implementation (Recommended)

Zero-dependency implementation using Go's `crypto/rand`:

```go
package message

import (
    "crypto/rand"
    "encoding/hex"
)

// NewID generates a RFC 4122 compliant UUID v4 string.
// Returns a string in format "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".
//
// This is a minimal, zero-dependency implementation suitable for
// CloudEvents id attribute generation. For advanced UUID features,
// consider using github.com/google/uuid with the same signature.
func NewID() string {
    var b [16]byte
    _, _ = rand.Read(b[:]) // crypto/rand never fails on modern systems

    // Set version (4) and variant (RFC 4122)
    b[6] = (b[6] & 0x0f) | 0x40 // Version 4
    b[8] = (b[8] & 0x3f) | 0x80 // Variant 10xxxxxx

    var buf [36]byte
    hex.Encode(buf[0:8], b[0:4])
    buf[8] = '-'
    hex.Encode(buf[9:13], b[4:6])
    buf[13] = '-'
    hex.Encode(buf[14:18], b[6:8])
    buf[18] = '-'
    hex.Encode(buf[19:23], b[8:10])
    buf[23] = '-'
    hex.Encode(buf[24:36], b[10:16])

    return string(buf[:])
}
```

| Aspect | Assessment |
|--------|------------|
| Correctness | Excellent (RFC compliant) |
| Independence | Excellent (zero dependencies) |
| Maintenance | Good (simple, stable) |
| Features | Minimal (exactly what's needed) |

### Option 3: Configurable IDGenerator

Allow users to inject custom ID generators:

```go
// IDGenerator generates unique message IDs.
type IDGenerator func() string

// DefaultIDGenerator is the built-in UUID v4 generator.
var DefaultIDGenerator IDGenerator = NewID
```

## Signature Compatibility

Both implementations share the same signature:

```go
func NewID() string          // gopipe built-in
func NewString() string      // google/uuid
```

Migration is trivial:

```go
// Using built-in
import "github.com/fxsml/gopipe/message"
id := message.NewID()

// Switch to google/uuid
import "github.com/google/uuid"
id := uuid.NewString()

// Or: Configure gopipe to use google/uuid
message.DefaultIDGenerator = uuid.NewString
```

## Recommendation

**Use Option 2 (Built-in) + Option 3 (Configurable)**

1. Ship with zero-dependency UUID generator (`message.NewID`)
2. Allow injection via `DefaultIDGenerator` for production use
3. Document google/uuid as recommended for high-volume production

### Rationale

1. **Independence**: gopipe stays dependency-free
2. **Simplicity**: Works out of the box
3. **Flexibility**: Power users can upgrade
4. **API Stability**: Same signature, no breaking changes

## Working Example

See `message/uuid.go` implementation below.

### Implementation

```go
// message/uuid.go
package message

import (
    "crypto/rand"
    "encoding/hex"
)

// IDGenerator generates unique message IDs.
// The default implementation generates RFC 4122 UUID v4 strings.
type IDGenerator func() string

// DefaultIDGenerator is used by Builder and other components to generate IDs.
// Replace with uuid.NewString from github.com/google/uuid for production.
var DefaultIDGenerator IDGenerator = NewID

// NewID generates a RFC 4122 compliant UUID v4 string.
// Returns format: "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx"
//
// Uses crypto/rand for cryptographically secure randomness.
// For high-throughput production systems, consider using
// github.com/google/uuid which implements pooled randomness.
func NewID() string {
    var b [16]byte
    _, _ = rand.Read(b[:])

    // Version 4: set bits 6-7 of byte 6 to 0100
    b[6] = (b[6] & 0x0f) | 0x40
    // Variant RFC 4122: set bits 6-7 of byte 8 to 10
    b[8] = (b[8] & 0x3f) | 0x80

    var buf [36]byte
    hex.Encode(buf[0:8], b[0:4])
    buf[8] = '-'
    hex.Encode(buf[9:13], b[4:6])
    buf[13] = '-'
    hex.Encode(buf[14:18], b[6:8])
    buf[18] = '-'
    hex.Encode(buf[19:23], b[8:10])
    buf[23] = '-'
    hex.Encode(buf[24:36], b[10:16])

    return string(buf[:])
}
```

### Tests

```go
// message/uuid_test.go
package message_test

import (
    "regexp"
    "testing"

    "github.com/fxsml/gopipe/message"
)

func TestNewID_Format(t *testing.T) {
    id := message.NewID()

    // UUID format: 8-4-4-4-12 hex chars
    pattern := `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
    matched, err := regexp.MatchString(pattern, id)
    if err != nil {
        t.Fatalf("regexp error: %v", err)
    }
    if !matched {
        t.Errorf("UUID format invalid: %s", id)
    }
}

func TestNewID_Uniqueness(t *testing.T) {
    seen := make(map[string]bool)
    for i := 0; i < 10000; i++ {
        id := message.NewID()
        if seen[id] {
            t.Errorf("duplicate UUID: %s", id)
        }
        seen[id] = true
    }
}

func TestNewID_Version4(t *testing.T) {
    id := message.NewID()
    // Position 14 should be '4' (version)
    if id[14] != '4' {
        t.Errorf("expected version 4 at position 14, got %c", id[14])
    }
}

func TestNewID_VariantRFC4122(t *testing.T) {
    id := message.NewID()
    // Position 19 should be 8, 9, a, or b (variant)
    variant := id[19]
    if variant != '8' && variant != '9' && variant != 'a' && variant != 'b' {
        t.Errorf("expected variant 8/9/a/b at position 19, got %c", variant)
    }
}

func BenchmarkNewID(b *testing.B) {
    for i := 0; i < b.N; i++ {
        _ = message.NewID()
    }
}
```

## Integration into Builder

Update the Builder pattern from ADR 0019:

```go
// message/builder.go
package message

// Builder constructs messages with CloudEvents defaults.
type Builder struct {
    source      string
    idGenerator IDGenerator
}

// NewBuilder creates a builder with the required source URI.
func NewBuilder(source string) *Builder {
    return &Builder{
        source:      source,
        idGenerator: DefaultIDGenerator,
    }
}

// WithIDGenerator sets a custom ID generator.
func (b *Builder) WithIDGenerator(gen IDGenerator) *Builder {
    b.idGenerator = gen
    return b
}

// Build creates a message with auto-generated id and specversion.
func (b *Builder) Build(data []byte, msgType string, extra ...Attributes) *Message {
    attrs := Attributes{
        AttrID:          b.idGenerator(),
        AttrSource:      b.source,
        AttrSpecVersion: "1.0",
        AttrType:        msgType,
    }
    for _, e := range extra {
        for k, v := range e {
            attrs[k] = v
        }
    }
    return New(data, attrs)
}
```

## Usage Examples

### Basic Usage (Zero Dependencies)

```go
import "github.com/fxsml/gopipe/message"

// Generate ID directly
id := message.NewID()
// Output: "f47ac10b-58cc-4372-a567-0e02b2c3d479"

// Use with builder
builder := message.NewBuilder("/orders")
msg := builder.Build(data, "order.created")
// msg.Attributes[AttrID] is auto-generated
```

### Production Usage (google/uuid)

```go
import (
    "github.com/fxsml/gopipe/message"
    "github.com/google/uuid"
)

// Option 1: Global replacement
func init() {
    message.DefaultIDGenerator = uuid.NewString
}

// Option 2: Per-builder
builder := message.NewBuilder("/orders").
    WithIDGenerator(uuid.NewString)
```

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `message/uuid.go` | Create | UUID generator implementation |
| `message/uuid_test.go` | Create | UUID generator tests |
| `message/builder.go` | Create | Builder pattern (from ADR 0019) |
| `message/builder_test.go` | Create | Builder tests |
| `docs/adr/0030-uuid-generation.md` | Create | ADR documenting decision |

## References

- [CloudEvents Specification v1.0.2](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md)
- [RFC 4122: UUID URN Namespace](https://www.rfc-editor.org/rfc/rfc4122)
- [RFC 9562: UUID Revision](https://www.rfc-editor.org/rfc/rfc9562)
- [google/uuid Package](https://pkg.go.dev/github.com/google/uuid)
- [ADR 0019: CloudEvents Mandatory](../adr/0019-cloudevents-mandatory.md)

# ADR 0024: Destination Attribute

**Date:** 2025-12-13
**Status:** Proposed
**Depends on:** ADR 0019

## Context

To enable internal message routing with clear "break-out" to external systems, we need a mechanism to specify where a message should be routed. Two approaches were considered:

### Option A: Internal Topic Namespace

Use the existing `topic` attribute with a reserved namespace:
```go
msg.Attributes[AttrTopic] = "/gopipe/internal/orders"
msg.Attributes[AttrTopic] = "/gopipe/internal/shipping/commands"
```

### Option B: Destination Attribute

Introduce a new `destination` attribute with URI scheme:
```go
msg.Attributes[AttrDestination] = "gopipe://orders"
msg.Attributes[AttrDestination] = "kafka://cluster/notifications"
msg.Attributes[AttrDestination] = "nats://orders.created"
```

## Evaluation

### Option A: Internal Topic Namespace

**Pros:**
- Uses existing `topic` attribute
- No new attribute needed
- Simpler conceptually

**Cons:**
- Mixes routing destination with topic naming
- Path-based namespace (`/gopipe/internal/`) could conflict with actual topics
- No way to express external destinations explicitly
- `topic` semantically means "what kind" not "where to"
- Harder to distinguish internal vs external at a glance
- CloudEvents alignment: topic is already a gopipe extension for pub/sub routing

### Option B: Destination Attribute

**Pros:**
- **Explicit intent**: Clear separation between topic (what) and destination (where)
- **URI scheme extensibility**: `gopipe://`, `nats://`, `kafka://`, `http://`, `file://`
- **Mirrors source**: `destination` is conceptual opposite of `source` (both URIs)
- **Clear boundaries**: Easy to identify internal (`gopipe://`) vs external
- **Protocol flexibility**: Destination can encode protocol-specific routing
- **Validation**: URI format enables validation and parsing
- **Future-proof**: Can add new schemes without changing semantics

**Cons:**
- New attribute to manage
- Slightly more complex message creation
- Not part of CloudEvents core (but neither is topic)

### Comparison Table

| Aspect | Topic Namespace | Destination Attribute |
|--------|-----------------|----------------------|
| Semantic clarity | Mixed | Clear |
| External routing | Implicit | Explicit |
| Validation | Path parsing | URI parsing |
| Extensibility | Limited | High (URI schemes) |
| CloudEvents alignment | Extension | Extension (like source) |
| Learning curve | Lower | Slightly higher |
| Break-out mechanism | Check prefix | Check scheme |

## Decision

**Adopt Option B: Destination Attribute**

The destination attribute provides clearer semantics, better extensibility, and aligns with how `source` works in CloudEvents.

### 1. Destination Attribute Definition

```go
// AttrDestination is the routing destination URI
// Format: scheme://path
// Examples:
//   - gopipe://orders           (internal)
//   - gopipe://shipping/commands (internal with path)
//   - kafka://cluster/topic     (external Kafka)
//   - nats://subject            (external NATS)
//   - http://host/path          (external HTTP)
const AttrDestination = "destination"
```

### 2. URI Scheme Registry

```go
// DestinationScheme identifies routing destination type
type DestinationScheme string

const (
    // SchemeGopipe is internal gopipe routing
    SchemeGopipe DestinationScheme = "gopipe"

    // SchemeKafka routes to Kafka
    SchemeKafka DestinationScheme = "kafka"

    // SchemeNATS routes to NATS
    SchemeNATS DestinationScheme = "nats"

    // SchemeHTTP routes to HTTP endpoint
    SchemeHTTP DestinationScheme = "http"

    // SchemeHTTPS routes to HTTPS endpoint
    SchemeHTTPS DestinationScheme = "https"
)

// IsInternalScheme returns true for gopipe:// destinations
func IsInternalScheme(dest string) bool {
    return strings.HasPrefix(dest, "gopipe://")
}
```

### 3. Destination Parsing

```go
// Destination represents a parsed routing destination
type Destination struct {
    Scheme DestinationScheme
    Host   string   // Optional host/cluster
    Path   string   // Routing path (topic, subject, etc.)
    Query  url.Values // Optional query parameters
}

// ParseDestination parses a destination URI
func ParseDestination(dest string) (*Destination, error) {
    u, err := url.Parse(dest)
    if err != nil {
        return nil, fmt.Errorf("invalid destination URI: %w", err)
    }

    return &Destination{
        Scheme: DestinationScheme(u.Scheme),
        Host:   u.Host,
        Path:   strings.TrimPrefix(u.Path, "/"),
        Query:  u.Query(),
    }, nil
}

// String returns the URI representation
func (d *Destination) String() string {
    u := &url.URL{
        Scheme:   string(d.Scheme),
        Host:     d.Host,
        Path:     "/" + d.Path,
        RawQuery: d.Query.Encode(),
    }
    return u.String()
}

// IsInternal returns true if destination is internal gopipe routing
func (d *Destination) IsInternal() bool {
    return d.Scheme == SchemeGopipe
}
```

### 4. Message Helpers

```go
// Destination returns the message destination URI
func (m *Message) Destination() string {
    dest, _ := m.Attributes[AttrDestination].(string)
    return dest
}

// SetDestination sets the routing destination
func (m *Message) SetDestination(dest string) {
    m.Attributes[AttrDestination] = dest
}

// IsInternalDestination returns true if message routes internally
func (m *Message) IsInternalDestination() bool {
    return IsInternalScheme(m.Destination())
}

// ParsedDestination returns parsed destination or error
func (m *Message) ParsedDestination() (*Destination, error) {
    return ParseDestination(m.Destination())
}
```

### 5. Builder Support

```go
// WithDestination sets the destination in builder
func (b *Builder) WithDestination(dest string) *Builder {
    b.defaultAttrs[AttrDestination] = dest
    return b
}

// BuildInternal creates message for internal routing
func (b *Builder) BuildInternal(data any, path string, msgType string) *Message {
    return b.Build(data, msgType, Attributes{
        AttrDestination: "gopipe://" + path,
    })
}

// BuildExternal creates message for external routing
func (b *Builder) BuildExternal(data any, dest string, msgType string) *Message {
    return b.Build(data, msgType, Attributes{
        AttrDestination: dest,
    })
}
```

### 6. Destination vs Topic

Both attributes serve different purposes:

| Attribute | Purpose | Example |
|-----------|---------|---------|
| `topic` | Pub/sub topic name (semantic) | `"orders"`, `"user.events"` |
| `destination` | Routing target (physical) | `"gopipe://orders"`, `"kafka://prod/orders"` |

A message can have both:
```go
msg := message.MustNew(order, message.Attributes{
    AttrTopic:       "orders",              // Semantic topic
    AttrDestination: "gopipe://processing", // Where to route
    AttrType:        "order.created",       // Event type
    // ... other CE attributes
})
```

### 7. Routing Flow

```go
// Router checks destination first, then falls back to topic
func (r *Router) Route(ctx context.Context, msg *Message) error {
    dest := msg.Destination()

    if dest != "" {
        // Explicit destination routing
        parsed, err := ParseDestination(dest)
        if err != nil {
            return err
        }

        if parsed.IsInternal() {
            return r.routeInternal(ctx, msg, parsed.Path)
        }
        return r.routeExternal(ctx, msg, parsed)
    }

    // Fall back to topic-based routing
    topic, _ := msg.Attributes[AttrTopic].(string)
    if topic != "" {
        return r.routeByTopic(ctx, msg, topic)
    }

    return ErrNoRoutingInfo
}
```

### 8. Destination Format Examples

```
Internal (gopipe://):
  gopipe://orders                    -> Internal orders handler
  gopipe://shipping/commands         -> Internal shipping commands
  gopipe://audit/events              -> Internal audit events
  gopipe://saga/order-fulfillment    -> Internal saga handler

External (scheme://):
  kafka://prod-cluster/orders        -> Kafka topic on prod cluster
  kafka://orders                     -> Kafka topic (default cluster)
  nats://orders.created              -> NATS subject
  nats://cluster/orders.>            -> NATS with cluster and wildcard
  http://api.example.com/webhook     -> HTTP webhook
  https://events.example.com/ingest  -> HTTPS endpoint
  file:///var/log/events.jsonl       -> File output
```

## Rationale

1. **Semantic Clarity**: `destination` clearly means "where to send"
2. **Mirrors Source**: `source` (where from) and `destination` (where to) are symmetric
3. **URI Extensibility**: New transports via new schemes
4. **Protocol Encoding**: Destination can include protocol-specific info
5. **Clear Boundaries**: `gopipe://` is unambiguous for internal routing
6. **Validation**: URI format enables parsing and validation

## Consequences

### Positive

- Clear separation of concerns (topic vs destination)
- Easy to identify internal vs external routing
- Extensible via URI schemes
- Protocol-specific routing information in destination
- Symmetric with `source` attribute

### Negative

- New attribute to learn and manage
- Messages may need both topic and destination
- Additional parsing overhead (minimal)

### Migration

Existing code using `topic` for routing continues to work; `destination` is optional but recommended for explicit routing:

```go
// Old style (still works via fallback)
msg.Attributes[AttrTopic] = "orders"

// New style (explicit and clear)
msg.Attributes[AttrDestination] = "gopipe://orders"
msg.Attributes[AttrTopic] = "orders"  // Optional semantic topic
```

## Links

- [ADR 0022: Internal Message Routing](0022-internal-message-routing.md)
- [ADR 0023: Internal Message Loop](0023-internal-message-loop.md)
- [CloudEvents Source Attribute](https://github.com/cloudevents/spec/blob/main/cloudevents/spec.md#source)
- [Feature 13: Internal Message Loop](../features/13-internal-message-loop.md)

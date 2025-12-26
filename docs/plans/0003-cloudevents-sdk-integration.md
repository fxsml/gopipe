# Plan 0003: CloudEvents SDK Integration

**Status:** Future (Optional)
**Priority:** Low - Not required for MVP
**Related ADRs:** [0022](../adr/0022-message-package-redesign.md)
**Depends On:** [Plan 0001](0001-message-engine.md) (Message Engine)

## Overview

Evaluate and plan integration of the official CloudEvents SDK for Go (`cloudevents/sdk-go/v2`) into gopipe. This includes using the SDK's CESQL parser for output pattern matching and potentially using the SDK at I/O boundaries.

## Current State

gopipe has a custom CloudEvents implementation in `message/cloudevents/`:
- Custom `Event` struct with JSON marshaling
- `FromMessage()` / `ToMessage()` conversion functions
- No external dependencies in `message/` core

## CE-SDK Capabilities

The CloudEvents SDK provides:

1. **Event Type** - `cloudevents.Event` with all CE attributes
2. **Protocol Bindings** - HTTP, Kafka, NATS, AMQP, etc.
3. **Event Formats** - JSON, Protobuf serialization
4. **CESQL Parser** - `github.com/cloudevents/sdk-go/sql/v2`
   - Parse SQL expressions: `cesqlparser.Parse("type LIKE 'order.%'")`
   - Evaluate against events: `expression.Evaluate(event)`
   - Custom functions via `AddFunction()`

## Integration Options

### Option A: CESQL Only (Recommended)

Use CE-SDK only for CESQL parsing in output pattern matching.

```go
import (
    cesqlparser "github.com/cloudevents/sdk-go/sql/v2/parser"
    cloudevents "github.com/cloudevents/sdk-go/v2"
)

// In match.go
type cesqlMatcher struct {
    expr cesql.Expression
}

func (m *cesqlMatcher) Match(msg *Message) bool {
    // Convert to CE event for CESQL evaluation
    event := toCloudEvent(msg)
    result, err := m.expr.Evaluate(event)
    if err != nil {
        return false
    }
    return result.(bool)
}
```

**Pros:**
- Standardized query language with spec compliance
- Maintained by CloudEvents community
- Custom function support for extensibility
- Handles complex filters: `type LIKE 'order.%' AND data.priority = 'high'`

**Cons:**
- Adds dependency to `message/` package
- Requires conversion to `cloudevents.Event` for CESQL evaluation
- Small performance overhead for conversion

### Option B: CE-SDK at Boundaries

Use CE-SDK for I/O boundaries (Subscriber/Publisher adapters).

```go
import cloudevents "github.com/cloudevents/sdk-go/v2"

// In message/cloudevents/subscriber.go
type Subscriber struct {
    client cloudevents.Client
}

func (s *Subscriber) Subscribe(ctx context.Context, topic string) (<-chan *message.Message, error) {
    ch := make(chan *message.Message)
    go func() {
        defer close(ch)
        s.client.StartReceiver(ctx, func(event cloudevents.Event) {
            msg := fromCEEvent(event)
            ch <- msg
        })
    }()
    return ch, nil
}
```

**Pros:**
- Native protocol bindings (HTTP, Kafka, NATS, AMQP)
- Correct CE serialization/deserialization
- Less custom code to maintain
- Battle-tested in production (Knative)

**Cons:**
- Dependency in adapter package (acceptable - already external concern)
- Protocol-specific configuration differs from current design
- May not fit all broker patterns (some brokers have their own CE support)

### Option C: Full CE-SDK Integration

Replace custom CloudEvents handling entirely with CE-SDK.

**Pros:**
- Consistent with CloudEvents ecosystem
- Community-maintained
- Full spec compliance

**Cons:**
- Breaking change to internal `message.Message` structure
- Tight coupling to CE-SDK
- Loss of flexibility for non-CE use cases

## Critical Evaluation

### CESQL for Pattern Matching

| Criterion | Evaluation |
|-----------|------------|
| **Necessity** | Medium - Simple wildcards cover 80% of cases |
| **Complexity** | Low - Well-documented parser API |
| **Dependency** | Acceptable - isolated to match.go |
| **Performance** | Needs benchmarking - conversion overhead |
| **Alternatives** | Custom glob matching (simpler, fewer features) |

### CE-SDK at Boundaries

| Criterion | Evaluation |
|-----------|------------|
| **Necessity** | Low - Current custom implementation works |
| **Value** | High for multi-protocol support |
| **Dependency** | Acceptable - adapter package already external |
| **Migration** | Medium effort - replace FromMessage/ToMessage |
| **Lock-in** | Low - adapter is replaceable |

### Recommendation

**Phased approach:**

1. **Phase 1: CESQL Parser** (Plan 0003a)
   - Add CE-SDK dependency for CESQL only
   - Use for advanced output pattern matching
   - Keep simple wildcard matching for performance

2. **Phase 2: Boundary Adapters** (Plan 0003b - optional)
   - Migrate `message/cloudevents/` to use CE-SDK
   - Add protocol bindings as needed (Kafka, NATS)
   - Keep custom implementation as fallback

## Implementation: Phase 1 - CESQL

Builds on existing `message/match` package structure:

```
message/
├── matcher.go           # Matcher interface (exists)
├── match/
│   ├── match.go         # All, Any (exists)
│   ├── like.go          # SQL LIKE pattern (exists)
│   ├── sources.go       # Sources matcher (exists)
│   ├── types.go         # Types matcher (exists)
│   └── cesql.go         # CESQL matcher (new, CE-SDK dep)
```

### CESQL Matcher

```go
// message/match/cesql.go
package match

import (
    cesqlparser "github.com/cloudevents/sdk-go/sql/v2/parser"
    cloudevents "github.com/cloudevents/sdk-go/v2"
)

// CESQL creates a matcher from a CESQL expression.
// Example: match.CESQL("type LIKE 'order.%' AND source = '/orders'")
func CESQL(expr string) (message.Matcher, error) {
    parsed, err := cesqlparser.Parse(expr)
    if err != nil {
        return nil, err
    }
    return &cesqlMatcher{expr: parsed}, nil
}

type cesqlMatcher struct {
    expr cesql.Expression
}

func (m *cesqlMatcher) Match(msg *message.Message) bool {
    event := toCloudEvent(msg)
    result, err := m.expr.Evaluate(event)
    if err != nil {
        return false
    }
    return result.(bool)
}

// toCloudEvent converts message to CE event for CESQL evaluation
func toCloudEvent(msg *message.Message) cloudevents.Event {
    event := cloudevents.NewEvent()
    if id, ok := msg.Attributes.ID(); ok {
        event.SetID(id)
    }
    if t, ok := msg.Attributes.Type(); ok {
        event.SetType(t)
    }
    if src, ok := msg.Attributes.Source(); ok {
        event.SetSource(src)
    }
    event.SetData(cloudevents.ApplicationJSON, msg.Data)
    return event
}
```

### Usage

```go
import "github.com/fxsml/gopipe/message/match"

// Simple patterns (no CE-SDK dependency)
engine.AddInput(ch, message.InputConfig{
    Matcher: match.Types("order.%"),
})

// Complex CESQL (requires CE-SDK)
cesqlMatcher, _ := match.CESQL("type LIKE 'order.%' AND source = '/orders'")
engine.AddInput(ch, message.InputConfig{
    Matcher: cesqlMatcher,
})
```

### Files to Create/Modify

- `message/match/cesql.go` - CESQL matcher implementation
- `message/match/go.mod` - Add `github.com/cloudevents/sdk-go/sql/v2` dependency
- `message/match/cesql_test.go` - Tests for CESQL matching

### Test Plan

1. CESQL basic: `type = 'order.created'`
2. CESQL LIKE: `type LIKE 'order.%'`
3. CESQL AND/OR: `type LIKE 'order.%' AND source = '/orders'`
4. CESQL with data access: `data.priority = 'high'` (if supported)
5. Invalid CESQL returns error
6. Benchmark: match.Types vs match.CESQL for simple patterns

## Acceptance Criteria

### Phase 1 (CESQL)
- [ ] match.CESQL function implemented
- [ ] CESQL expressions evaluate correctly
- [ ] CE-SDK dependency isolated to match/cesql.go
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)

### Phase 2 (Boundary Adapters - Future)
- [ ] Replace custom Event struct with CE-SDK
- [ ] HTTP protocol binding adapter
- [ ] Additional protocol bindings as needed

## Open Questions

1. **CESQL data access**: Does CE-SDK support `data.field` in expressions?
2. **Performance**: Benchmark conversion overhead for CESQL evaluation
3. **Custom functions**: What custom functions would be useful? (e.g., `HASPREFIX`)
4. **Error handling**: How to handle CESQL parse errors at AddOutput time?

## Sources

- [CloudEvents SDK-Go](https://github.com/cloudevents/sdk-go)
- [CloudEvents SQL v1 Blog](https://cloudevents.io/blog/2024-07-15/)
- [CESQL Package Docs](https://pkg.go.dev/github.com/cloudevents/sdk-go/sql/v2)

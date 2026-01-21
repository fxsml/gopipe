# Plan 0007: CESQL Pattern Matching

**Status:** Proposed
**Related ADRs:** [0022](../adr/0022-message-package-redesign.md)
**Depends On:** [Plan 0003](archive/0003-cloudevents-sdk-integration.md) (CE-SDK Boundary Adapters)

## Overview

Add CESQL (CloudEvents SQL) support for advanced pattern matching using the CloudEvents SDK parser.

## Problem

Current matchers (`match.Types`, `match.Sources`) use SQL LIKE patterns which cover ~80% of use cases. For complex filtering scenarios, CESQL provides:

- Compound expressions: `type LIKE 'order.%' AND source = '/orders'`
- Data field access: `data.priority = 'high'` (if supported)
- Standard spec-compliant query language

## Proposed Implementation

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

## Files to Create/Modify

| File | Changes |
|------|---------|
| `message/match/cesql.go` | CESQL matcher implementation |
| `message/match/cesql_test.go` | Tests for CESQL matching |
| `go.mod` | Add `github.com/cloudevents/sdk-go/sql/v2` dependency |

## Open Questions

1. **CESQL data access**: Does CE-SDK support `data.field` in expressions?
2. **Performance**: Benchmark conversion overhead for CESQL evaluation
3. **Custom functions**: What custom functions would be useful? (e.g., `HASPREFIX`)
4. **Error handling**: How to handle CESQL parse errors at AddOutput time?

## Test Plan

1. CESQL basic: `type = 'order.created'`
2. CESQL LIKE: `type LIKE 'order.%'`
3. CESQL AND/OR: `type LIKE 'order.%' AND source = '/orders'`
4. CESQL with data access: `data.priority = 'high'` (if supported)
5. Invalid CESQL returns error
6. Benchmark: match.Types vs match.CESQL for simple patterns

## Acceptance Criteria

- [ ] match.CESQL function implemented
- [ ] CESQL expressions evaluate correctly
- [ ] CE-SDK dependency isolated to match/cesql.go
- [ ] Tests pass (`make test`)
- [ ] Build passes (`make build && make vet`)

## Sources

- [CloudEvents SDK-Go](https://github.com/cloudevents/sdk-go)
- [CloudEvents SQL v1 Blog](https://cloudevents.io/blog/2024-07-15/)
- [CESQL Package Docs](https://pkg.go.dev/github.com/cloudevents/sdk-go/sql/v2)

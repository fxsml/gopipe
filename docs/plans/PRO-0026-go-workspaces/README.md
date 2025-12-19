# PRO-0026: Go Workspaces Modularization

**Status:** Proposed
**Priority:** High
**Related ADRs:** PRO-0047

## Overview

Split gopipe into four Go modules using Go workspaces to enforce dependency boundaries.

## Goals

1. Separate modules: pipe, channel, message, engine
2. Keep pipe, channel, message dependency-free
3. Allow external dependencies only in engine

## Task 1: Create Module Structure

```
gopipe/
├── go.work
├── pipe/
│   └── go.mod
├── channel/
│   └── go.mod
├── message/
│   └── go.mod
└── engine/
    └── go.mod
```

**Files to Create:**
- `go.work`
- `pipe/go.mod`
- `channel/go.mod`
- `message/go.mod`
- `engine/go.mod`

## Task 2: Move Files

| Current Location | New Location |
|------------------|--------------|
| `processor.go` | `pipe/processor.go` |
| `pipe.go` | `pipe/pipe.go` |
| `channel/*.go` | `channel/*.go` |
| `message/*.go` | `message/*.go` |
| Engine-related | `engine/*.go` |

## Task 3: Update Imports

Update all import paths:
```go
// Before
import "github.com/fxsml/gopipe"

// After
import "github.com/fxsml/gopipe/pipe"
```

## Task 4: Configure Engine Dependencies

```go
// engine/go.mod
module github.com/fxsml/gopipe/engine

require (
    github.com/cloudevents/sdk-go/v2 v2.15.0
    github.com/segmentio/kafka-go v0.4.47
    github.com/nats-io/nats.go v1.31.0
)
```

**Acceptance Criteria:**
- [ ] Four separate go.mod files
- [ ] go.work configuration
- [ ] pipe, channel, message have zero external deps
- [ ] engine has broker SDKs
- [ ] All tests pass
- [ ] CI updated for multi-module
- [ ] CHANGELOG updated

## Related

- [PRO-0047](../../adr/PRO-0047-go-workspaces-modularization.md) - ADR

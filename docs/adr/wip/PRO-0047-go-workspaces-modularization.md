# ADR 0047: Go Workspaces Modularization

**Date:** 2025-12-17
**Status:** Proposed

## Context

The gopipe codebase has grown to encompass multiple concerns: channel operations, pipeline primitives, message handling, and engine orchestration. These have different dependency requirements:

- Core primitives should remain dependency-free for maximum portability
- Engine/broker integration requires external SDKs

## Decision

Split gopipe into four Go modules using Go workspaces:

```
gopipe/
├── go.work
├── pipe/           # Core pipeline primitives (formerly gopipe root)
│   ├── go.mod      # module github.com/fxsml/gopipe/pipe
│   ├── processor.go
│   ├── pipe.go
│   └── ...
├── channel/        # Channel operations
│   ├── go.mod      # module github.com/fxsml/gopipe/channel
│   ├── groupby.go
│   ├── broadcast.go
│   ├── merge.go
│   └── ...
├── message/        # Message types and handling
│   ├── go.mod      # module github.com/fxsml/gopipe/message
│   ├── message.go
│   ├── attributes.go
│   └── ...
└── engine/         # CloudEvents engine with broker integration
    ├── go.mod      # module github.com/fxsml/gopipe/engine
    ├── engine.go
    ├── router.go
    └── ...
```

### Dependency Rules

| Module | External Dependencies | Rationale |
|--------|----------------------|-----------|
| `pipe` | None | Core primitives, maximum portability |
| `channel` | None | Generic channel operations |
| `message` | None | Message types, no serialization libs |
| `engine` | Allowed | Broker SDKs, CloudEvents SDK, etc. |

### Module Dependencies

```
engine ──> message ──> (none)
   │
   └────> pipe ────> channel ──> (none)
```

### go.work Configuration

```go
// go.work
go 1.21

use (
    ./pipe
    ./channel
    ./message
    ./engine
)
```

### Import Paths

```go
import (
    "github.com/fxsml/gopipe/pipe"
    "github.com/fxsml/gopipe/channel"
    "github.com/fxsml/gopipe/message"
    "github.com/fxsml/gopipe/engine"
)
```

### Engine External Dependencies (Allowed)

```go
// engine/go.mod
module github.com/fxsml/gopipe/engine

require (
    github.com/cloudevents/sdk-go/v2 v2.15.0
    github.com/segmentio/kafka-go v0.4.47
    github.com/nats-io/nats.go v1.31.0
    github.com/rabbitmq/amqp091-go v1.9.0
)
```

## Consequences

**Positive:**
- Clear dependency boundaries
- Core modules remain lightweight and portable
- Independent versioning possible
- Easier testing of core without broker setup
- Users can import only what they need

**Negative:**
- More complex build/release process
- Cross-module refactoring requires coordination
- Version synchronization between modules

## Migration

1. Create module directories
2. Move files to appropriate modules
3. Update import paths
4. Create go.work file
5. Update CI/CD for multi-module builds

## Links

- Related: [PRO-0006](../plans/PRO-0006-package-restructuring/) - Package Restructuring
- Related: [PRO-0030](PRO-0030-remove-sender-receiver.md) - Remove Sender/Receiver

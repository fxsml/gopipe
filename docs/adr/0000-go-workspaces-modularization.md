# ADR 0000: Go Workspaces Modularization

**Date:** 2025-12-17
**Status:** Implemented

## Context

The gopipe codebase has grown to encompass multiple concerns: channel operations, pipeline primitives, and message handling. These have different dependency requirements:

- Core primitives should remain dependency-free for maximum portability
- Message handling builds on top of pipe and channel primitives

We chose `message` as the top-level module name rather than `engine` because `message.NewRouter()` is more explicit about its purpose than `engine.NewRouter()`.

## Decision

Split gopipe into three Go modules using Go workspaces:

```
gopipe/
├── go.work
├── channel/        # Channel operations (standalone, no dependencies)
│   ├── go.mod      # module github.com/fxsml/gopipe/channel
│   ├── groupby.go
│   ├── broadcast.go
│   ├── merge.go
│   └── ...
├── pipe/           # Core pipeline primitives
│   ├── go.mod      # module github.com/fxsml/gopipe/pipe
│   ├── processor.go
│   ├── pipe.go
│   └── ...
└── message/        # Message types, routing, pub/sub
    ├── go.mod      # module github.com/fxsml/gopipe/message
    ├── message.go
    ├── router.go
    ├── publisher.go
    ├── subscriber.go
    ├── middleware/  # Message-specific middleware (not a separate module)
    ├── broker/      # Broker implementations
    ├── cqrs/        # CQRS patterns
    └── ...
```

### Module Dependencies

```
message ──> pipe ──> channel ──> (none)
    │
    └─────> channel
```

### Dependency Rules

| Module | Internal Dependencies | External Dependencies |
|--------|----------------------|----------------------|
| `channel` | None | None |
| `pipe` | `channel` | None |
| `message` | `pipe`, `channel` | None (broker implementations may add deps) |

### go.work Configuration

```go
// go.work
go 1.24.1

use (
    ./channel
    ./pipe
    ./message
)
```

### Import Paths

```go
import (
    "github.com/fxsml/gopipe/channel"
    "github.com/fxsml/gopipe/pipe"
    "github.com/fxsml/gopipe/message"
)
```

### Package Naming

The `pipe` module uses `package pipe` (not `package gopipe`) to match the directory name and follow Go conventions.

## Consequences

**Positive:**
- Clear dependency boundaries
- Core modules remain lightweight and portable
- Independent versioning possible
- Easier testing of core without broker setup
- Users can import only what they need
- `message.NewRouter()` is explicit about working with messages

**Negative:**
- More complex build/release process
- Cross-module refactoring requires coordination
- Version synchronization between modules

## Implementation Notes

- Middleware lives in `message/middleware/` as a subdirectory, not a separate module
- Each module has its own `internal/test` package for test utilities
- The Makefile uses explicit module paths: `./channel/... ./pipe/... ./message/...`

## Links

- Related: [PRO-0006](../plans/PRO-0006-package-restructuring/) - Package Restructuring
- Related: [PRO-0030](PRO-0030-remove-sender-receiver.md) - Remove Sender/Receiver

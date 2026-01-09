# gopipe Implementation Plans

## Current

| Plan | Status | Description |
|------|--------|-------------|
| [message-refactoring](message-refactoring.md) | Implemented | CloudEvents message handling |

## Agent Guidance

For design decisions, common mistakes, and rejected alternatives, see [AGENTS.md](../../AGENTS.md).

## Archive

Historical plans from the message package development are in [archive/](archive/):

| Plan | Title | Status |
|------|-------|--------|
| [0001](archive/0001-message-engine.md) | Message Engine | Implemented |
| [0002](archive/0002-marshaler.md) | Marshaler | Implemented |
| [0003](archive/0003-cloudevents-sdk-integration.md) | CloudEvents SDK | Deferred |
| [0004](archive/0004-router.md) | Router Extraction | Implemented |
| [0005](archive/0005-engine-implementation-review.md) | Engine Fixes | Implemented |
| [0006](archive/0006-engine-raw-api-simplification.md) | Raw API Simplification | Implemented |
| [0007](archive/0007-config-convention.state.md) | Config Convention | Implemented |
| [0008](archive/0008-message-package-release-review.md) | Release Review | Completed |
| [0009](archive/0009-marshal-unmarshal-pipes.state.md) | Marshal Pipes | Implemented |
| [0010](archive/0010-handler-naming-consistency.state.md) | Handler Naming | Implemented |

## Creating New Plans

1. Create `short-title.md` in this directory
2. Add to index above
3. Reference [AGENTS.md](../../AGENTS.md) for design context

See [Planning Procedures](../procedures/planning.md) for details.

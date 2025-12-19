# PRO-0006: Package Restructuring

**Status:** Proposed
**Priority:** Medium
**Dependencies:** PRO-0001, PRO-0003, PRO-0004

## Overview

Clean up gopipe package structure to focus on core pipeline functionality. Messaging components (CloudEvents, brokers, CQRS) will move to goengine.

## Goals

1. Clean separation between core pipelines (gopipe) and messaging (goengine)
2. Simple, clear package hierarchy
3. Prepare codebase for goengine extraction

## Scope

**gopipe focuses on:**
- Channel operations (channel/)
- Pipeline primitives (processor, pipes)
- Subscriber interface for data sources
- Generic middleware

**goengine will own (not in this plan):**
- Message types with CloudEvents
- Broker adapters
- Router, Publisher for messages
- CQRS patterns

## Task

**Target Package Structure:**
```
gopipe/
├── channel/            # Channel operations (GroupBy, Broadcast, Merge, etc.)
│   └── *.go
├── middleware/         # Generic middleware (after PRO-0004)
│   └── *.go
├── processor.go        # Processor with ProcessorConfig
├── subscriber.go       # Subscriber[T] interface
├── pipe.go             # Pipe constructors
└── deprecated.go       # Backward compatibility wrappers
```

**What Stays in gopipe:**
- `channel/` - All channel operations
- `middleware/` - Generic middleware (timeout, retry, recover)
- Core types: Processor, Pipe, Subscriber, ProcessorConfig

**What Moves to goengine:**
- `message/` package (Message, Attributes, CloudEvents)
- `message/broker/` - Broker adapters
- `message/cqrs/` - CQRS patterns
- Router, Publisher, message-specific Subscriber

**Files to Modify:**
- Add deprecation notices to message/ components
- Update imports in examples
- Clean up unused code paths

**Acceptance Criteria:**
- [ ] gopipe package focused on channels/pipelines only
- [ ] No circular dependencies
- [ ] Deprecation notices on message/ components
- [ ] Tests pass
- [ ] CHANGELOG updated

## Dependencies

Execute after:
- PRO-0001: ProcessorConfig
- PRO-0003: Subscriber Interface
- PRO-0004: Middleware Package

## Related

- [goengine PRO-0002](../../goengine/plans/PRO-0002-migration/) - Migration from gopipe

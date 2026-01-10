# gopipe Roadmap

## Vision

A composable, CloudEvents-native messaging framework for Go.

```
┌─────────────────────────────────────────────────────────────────┐
│                        gopipe Vision                             │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                   message.Engine                            │ │
│  │  Inputs → Router → Handlers → Outputs                       │ │
│  └────────────────────────────────────────────────────────────┘ │
│                              │                                   │
│  ┌───────────────────────────┴────────────────────────────────┐ │
│  │                   pipe Package                              │ │
│  │  Stateful pipes, Merger, Distributor, Generator             │ │
│  └────────────────────────────────────────────────────────────┘ │
│                              │                                   │
│  ┌───────────────────────────┴────────────────────────────────┐ │
│  │                  channel Package                            │ │
│  │  Stateless: Transform, Filter, Merge, Broadcast             │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                 External Plugins                            │ │
│  │  gopipe-postgres, gopipe-redis                              │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Current State (v0.11.0)

Released January 2026.

### Packages

| Package | Status | Description |
|---------|--------|-------------|
| `channel` | Stable | Stateless channel operations |
| `pipe` | Stable | Stateful components with lifecycle |
| `message` | Stable | CloudEvents message routing |

### Key Features

- **Message Engine**: Orchestrates inputs, handlers, and outputs
- **Type-based routing**: Handlers registered by CloudEvents type
- **Command/Event handlers**: CQRS pattern support
- **Middleware**: Correlation ID, logging, metrics
- **Multi-module**: Each package is a separate Go module

## Planned Work

### Foundation Cleanup

Improvements to the pipe package:

| Task | Description | Priority |
|------|-------------|----------|
| Generator → Subscriber | Replace Generator with Subscriber interface | Medium |
| BroadcastConfig | Add slow-receiver handling to Broadcast | Low |
| Drop path naming | Rename Cancel → Drop for clarity | Low |

### Future Extensions

External plugins (separate repositories):

| Plugin | Description | Status |
|--------|-------------|--------|
| [Event Persistence](future/event-persistence.md) | SQL Event Store, outbox pattern | Proposed |

## Release History

| Version | Date | Highlights |
|---------|------|------------|
| v0.11.0 | Jan 2026 | Message Engine, multi-module structure |
| v0.10.0 | Dec 2025 | CloudEvents alignment, Router |

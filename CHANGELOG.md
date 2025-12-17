# Changelog

All notable changes to gopipe will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### UUID Generation
- `message.NewID()` - Zero-dependency RFC 4122 UUID v4 generator
- `message.DefaultIDGenerator` - Configurable ID generator variable
- `message.IDGenerator` type for dependency injection
- Same signature as `github.com/google/uuid.NewString()` for easy migration
- See: [docs/plans/uuid-integration.md](docs/plans/uuid-integration.md)

## [0.10.1] - 2025-12-17

### Fixed

- **ChannelBroker.Receive**: Changed from polling with hard-coded 100ms timeout to blocking by default until a message arrives. Added `ReceiveTimeout` config option for optional timeout behavior.

## [0.10.0] - 2025-12-12

Major pub/sub implementation with CloudEvents support, CQRS handlers, and message routing.

### Infrastructure
- GitHub Actions CI workflow with test, lint, and build jobs
- Makefile with essential development targets
- git-semver for semantic versioning
- README badges for CI, Go Report Card, GoDoc, License
- CLAUDE.md with git flow procedures and documentation guidelines

### Added

#### Feature 01: Channel GroupBy
- `channel.GroupBy` function for key-based message batching
- Configurable size-based and time-based flushing
- LRU eviction for concurrent group limits
- See: [docs/features/01-channel-groupby.md](docs/features/01-channel-groupby.md)

#### Feature 02: Message Core Refactor
- Public `Data` and `Attributes` fields on `Message` type
- `TypedMessage[T]` for type-safe message handling
- CloudEvents-aligned attribute names (`AttrID`, `AttrType`, `AttrSource`, etc.)
- Manual acknowledgment support with `NewWithAcking`
- See: [docs/features/02-message-core-refactor.md](docs/features/02-message-core-refactor.md)

#### Feature 03: Message Pub/Sub
- `Publisher` with configurable batching (uses `channel.GroupBy`)
- `Subscriber` with polling and channel output
- Three broker implementations:
  - `broker.NewChannelBroker()` - In-memory channel-based broker
  - `broker.NewHTTPSender/Receiver()` - HTTP webhook broker with CloudEvents
  - `broker.NewIOBroker()` - IO broker for debugging (JSONL format)
- `Sender`, `Receiver`, and `Broker` interfaces
- See: [docs/features/03-message-pubsub.md](docs/features/03-message-pubsub.md)

#### Feature 04: Message Router
- `Router` for attribute-based message dispatch
- `Handler` interface with `Handle` and `Match` methods
- Composable matchers: `MatchAll`, `MatchSubject`, `MatchType`, `And`, `Or`, `Not`
- Middleware support for cross-cutting concerns
- `Generator` for unhandled message responses
- Configurable concurrency for parallel handler execution
- See: [docs/features/04-message-router.md](docs/features/04-message-router.md)

#### Feature 05: Message CQRS
- Type-safe command handlers: `cqrs.NewCommandHandler[Cmd, Evt]()`
- Type-safe event handlers: `cqrs.NewEventHandler[Evt]()`
- `Marshaler` interface for pluggable serialization
- `AttributeProvider` for message metadata enrichment
- JSON marshaler implementation
- See: [docs/features/05-message-cqrs.md](docs/features/05-message-cqrs.md)

#### Feature 06: CloudEvents Support
- CloudEvents v1.0.2 HTTP Protocol Binding
- Binary and structured content modes
- Batching support (`application/cloudevents-batch+json`)
- JSONL format for IO operations
- CloudEvents attribute mapping
- HTTP headers (`ce-*` prefix) support
- See: [docs/features/06-message-cloudevents.md](docs/features/06-message-cloudevents.md)

#### Feature 07: Message Multiplex
- `multiplex.NewSender()` for topic-based sender routing
- `multiplex.NewReceiver()` for topic-based receiver routing
- Default routing for unmatched topics
- Multi-broker fan-out support
- See: [docs/features/07-message-multiplex.md](docs/features/07-message-multiplex.md)

#### Feature 08: Middleware Package
- `middleware.Correlation()` for correlation ID propagation
- `middleware.NewMessageMiddleware()` for message transformation
- Context-based correlation ID storage
- Automatic correlation ID generation
- See: [docs/features/08-middleware-package.md](docs/features/08-middleware-package.md)

#### Documentation
- 8 feature documents in `docs/features/`
- 18 Architecture Decision Records in `docs/adr/`
- Feature integration guide in `docs/features/README.md`
- ADR organization in `docs/adr/README.md`
- Integration procedure in `CLAUDE.md`
- Analysis documents: watermill comparison, CQRS patterns, production readiness
- Working examples: broker, CQRS, saga coordinator, multiplex, router middleware

### Changed

#### Feature 02: Message Core Refactor (Breaking Changes)
- **BREAKING**: `Message` fields are now public: `msg.Data` instead of `msg.Data()`
- **BREAKING**: Removed functional options (`WithID`, `WithSubject`, etc.)
- **BREAKING**: Removed thread-safe property access (use direct map access)
- **BREAKING**: CloudEvents-aligned attribute names (breaking for existing code using old names)
- **BREAKING**: Constructor changes: direct construction instead of options pattern

### Removed

#### Feature 02: Message Core Refactor
- Functional options pattern
- Accessor methods for `Data` and `Attributes`
- Thread-safe property access mechanisms
- "Noisy" properties (complex property getters)

### Proposed (Not Yet Implemented)

These features are documented but not implemented:
- **Saga Coordinator**: Multi-step workflow orchestration (ADR 0007)
- **Compensating Saga**: Rollback for failed workflows (ADR 0008)
- **Transactional Outbox**: Reliable event publishing (ADR 0009)

## Migration Guide

See [docs/features/02-message-core-refactor.md](docs/features/02-message-core-refactor.md) for detailed migration instructions.

### Quick Migration Examples

```go
// Old code
msg := message.New(data,
    message.WithID("123"),
    message.WithSubject("orders"),
)
payload := msg.Data()
attrs := msg.Attributes()

// New code
msg := &message.Message{
    Data: data,
    Attributes: message.Attributes{
        message.AttrID: "123",
        message.AttrSubject: "orders",
    },
}
payload := msg.Data
attrs := msg.Attributes
```

## Integration Order

Features should be integrated in this dependency order:

1. Channel GroupBy (prerequisite)
2. Message Core Refactor (foundation)
3. Message Pub/Sub (with CloudEvents)
4. Message Router
5. Message CQRS
6. Message Multiplex
7. Middleware Package

See [docs/features/README.md](docs/features/README.md) for complete integration guide.

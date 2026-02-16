# Changelog

All notable changes to gopipe will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **message/jsonschema:** JSON Schema validation for CloudEvents messages
  - `Registry` for managing schemas by CloudEvents type (eventType → schema)
  - `RegisterType(v, schema)` derives eventType from Go type via naming strategy
  - Configurable `SchemaURI` function for custom schema URI schemes (defaults to URN)
  - Implements `InputRegistry` for automatic instance creation in pipes
  - `Validate(eventType, data)` validates raw bytes against compiled schemas
  - `Schema(eventType)` and `Schemas()` for HTTP schema serving
  - Three validation middleware types:
    - `NewInputValidationMiddleware` - validates before unmarshaling (fail fast)
    - `NewOutputValidationMiddleware` - validates after marshaling
    - `NewValidationMiddleware` - validates proxy scenarios (RawMessage → RawMessage)
  - Thread-safe for shared use across middleware and pipes
  - Uses JSON Schema Draft 2020-12 via `santhosh-tekuri/jsonschema/v6`
  - Separation of concerns: validation separate from marshaling
  - See ADR 0025 and `examples/07-validating-marshaler` for usage

## [0.17.1] - 2026-02-04

### Added

- **message:** Expose `Use()` method on `UnmarshalPipe` and `MarshalPipe`
  - Allows middleware to be added to unmarshal and marshal pipelines
  - Provides same middleware capabilities as underlying `ProcessPipe`
  - Returns `ErrAlreadyStarted` if called after pipe has been started

## [0.17.0] - 2026-02-03

### Added

- **message/http:** HTTP CloudEvents pub/sub adapter
  - Specialized HTTP adapter using standard library (`http.Handler`, `http.ServeMux`)
  - Binary mode (default): metadata in Ce-* headers, efficient
  - Structured mode: full CloudEvents JSON in body
  - Batch mode: automatic batching with configurable size/duration
  - Graceful shutdown with in-flight request tracking
  - Proper acking semantics (HTTP 2xx = Ack, errors = Nack)
  - Uses CloudEvents SDK for protocol compliance (binary/structured/batch modes)
  - Complements `message/cloudevents` (SDK protocol wrapper) with HTTP-specific optimization
  - See `examples/06-http-cloudevents` for complete usage example

- **pipe:** `ProcessTimeout` configuration field for per-message processing timeouts
  - Works correctly with `ShutdownTimeout` via shutdown context pattern
  - Handler contexts derive from shutdown context, not parent context
  - Handlers continue during grace period with their `ProcessTimeout`
  - On forced shutdown (grace period expired), handlers are cancelled immediately
  - Default: `0` (no timeout)

- **message:** `AckStrategy` for flexible acknowledgment patterns
  - `AckOnSuccess`: automatic ack on success, nack on error (default)
  - `AckManual`: handler controls ack/nack explicitly
  - `AckForward`: ack forwarded to output messages (event sourcing)
  - Uses idiomatic `Done() + Err()` pattern (like `context.Context`)
  - Shared acking enables fan-out patterns
  - Panic protection prevents resource leaks

- **message:** `Context()` method for message-specific context derivation
  - Integrates message `ExpiryTime` with context deadlines
  - Optimized to avoid goroutines when parent context never cancels
  - Separates lifecycle (context) from domain logic (settlement)

### Fixed

- **message:** Signal shutdown to marshal/unmarshal pipes immediately
  - Previously `ShutdownTimeout` was ineffective for marshal/unmarshal pipes
  - Now all engine components see shutdown signal simultaneously

### Removed

- **pipe/middleware:** `Context` middleware (replaced by `ProcessTimeout`)
  - Used anti-pattern: `Background() + Timeout` breaks `ShutdownTimeout`
  - See issue #111 for details

- **message/middleware:** `AutoAck` middleware (replaced by `AckStrategy`)
  - Consolidated into `AckStrategy` configuration
## [0.16.0] - 2026-01-28

### Changed

- **message:** Simplified engine architecture (#104)
  - Removed message tracker and drain detection (timeout-based shutdown only)
  - Removed multi-pool support from Router (single pool configuration)
  - Router no longer auto-acks; use `middleware.AutoAck()` for previous behavior
  - All pipeline components now auto-nack on failure for consistency

- **pipe:** Consistent `ShutdownTimeout` semantics across all components
  - `<= 0`: Forces immediate shutdown (no grace period)
  - `> 0`: Waits up to duration for natural completion, then forces shutdown

### Added

- **message/middleware:** `AutoAck()` middleware for automatic ack on success, nack on error

### Removed

- **message:** Loopback plugins removed due to production deadlock risk
  - `plugin.Loopback`, `plugin.BatchLoopback`, `plugin.GroupLoopback`, `plugin.ProcessLoopback`
  - Use external message queues (Redis, NATS) for message re-routing instead

- **message:** Multi-pool routing APIs
  - `Engine.AddPoolWithConfig()`, `Engine.AddHandlerToPool()`
  - `Router.AddPoolWithConfig()`, `Router.AddHandlerToPool()`
  - Use single pool with higher concurrency, or multiple engines for isolation

- **message:** Loopback and tracker APIs
  - `Engine.AddLoopbackInput()`, `Engine.AddLoopbackOutput()`
  - `Engine.AdjustInFlight()`

### Migration

- **Loopback users:** Use external message queue for re-routing:
  ```go
  output, _ := e.AddOutput("batch-out", matcher)
  go func() {
      for msg := range output {
          externalQueue.Publish(msg)
      }
  }()
  e.AddInput("batch-in", nil, externalQueueConsumer)
  ```

- **Auto-ack users:** Add middleware to restore previous behavior:
  ```go
  engine.Use(middleware.AutoAck())
  ```

- **Multi-pool users:** Use higher concurrency in single pool:
  ```go
  engine := NewEngine(EngineConfig{
      RouterPool: PoolConfig{Workers: 10},
  })
  ```

## [0.15.0] - 2026-01-23

### Added

- **message/cloudevents:** Middleware support for Subscriber and Publisher (#96)
  - `Subscriber.Use()` and `Publisher.Use()` methods for applying `pipe/middleware`
  - Optional variadic middleware params in `SubscriberPlugin` and `PublisherPlugin`
  - Enables retry logic, circuit breaking, and backoff on connection errors

### Fixed

- **message:** Track fan-in/fan-out in loopback plugins for graceful shutdown (#95)
  - Added `Engine.AdjustInFlight()` for loopback plugins to adjust in-flight message count
  - `BatchLoopback`, `GroupLoopback`, and `ProcessLoopback` now track fan-in/fan-out
  - Graceful shutdown completes immediately instead of waiting for timeout when batching

## [0.14.1] - 2026-01-21

### Fixed

- **message:** Merger error logs now use consistent structured format (#92)
  - Added custom ErrorHandler to Engine and Router Mergers
  - Logs include `component`, `error`, and `attributes` fields
  - Removes `[GOPIPE]` prefix and raw JSON dump from error messages

## [0.14.0] - 2026-01-21

### Added

- **message:** Named worker pools for per-handler concurrency control (#90)
  - `PoolConfig` struct for configuring worker pools
  - `RouterConfig.Pool` for default pool configuration
  - `Router.AddPoolWithConfig()` to create named pools
  - `Router.AddHandlerToPool()` to assign handlers to specific pools
  - `Engine.AddPoolWithConfig()` and `Engine.AddHandlerToPool()` passthrough methods
  - `EngineConfig.RouterPool` and `EngineConfig.RouterBufferSize` for pool configuration
  - Enables resource-constrained handlers (e.g., external APIs) to have lower concurrency than fast handlers
- **message/middleware:** `Recover()` middleware for panic recovery (#88)
  - Catches panics in handlers and converts them to errors
  - Logs panic details including stack trace
  - Prevents single handler panic from crashing the entire engine

### Changed

- **message:** Improved logging consistency across module (#87)
  - Standardized log levels and message formats
  - Engine lifecycle events now logged at appropriate levels

## [0.13.3] - 2026-01-19

### Fixed

- **message:** Default `Marshaler` to `JSONMarshaler` when not set in `EngineConfig` (fixes #85)
  - Previously, omitting `Marshaler` caused a nil pointer panic when processing raw inputs
  - `EngineConfig.parse()` now defaults to `NewJSONMarshaler()` like other config fields

## [0.13.2] - 2026-01-19

### Fixed

- **message:** Graceful loopback shutdown - messages in loopback pipelines now drain properly before engine shutdown (fixes #81)
  - Added `messageTracker` for tracking in-flight messages through loopbacks
  - Added `AddLoopbackOutput` and `AddLoopbackInput` methods on Engine for explicit loopback wiring
  - Loopback outputs close only after pipeline is drained, preventing message loss
  - Added comprehensive tests and benchmarks for graceful shutdown scenarios

## [0.13.1] - 2026-01-16

### Fixed

- **message:** `funcName` now correctly handles generic factory functions (fixes #79)
  - Strip generic type parameters `[...]` from runtime function names
  - Return `package.FunctionName` for package-level functions (e.g., `context.Background`)
  - Traverse nested closures to find the actual factory name
  - Correctly distinguish `funcHelper` from closure names like `func1`

## [0.13.0] - 2026-01-15

### Added

- **message:** `AttrCorrelationID` and `AttrExpiryTime` extension attribute constants
- **message:** `IsRequiredAttr()`, `IsOptionalAttr()`, `IsExtensionAttr()` predicate functions
- **message:** `CorrelationID()` and `ExpiryTime()` typed accessor methods
- **message/middleware:** `Deadline()` middleware for expiry time enforcement with context deadline
- **message/middleware:** `ValidateRequired()` middleware for CloudEvents attribute validation
- **message/middleware:** `ErrMessageExpired` error for expired messages
- **message/middleware:** `ErrMissingRequiredAttr` error for validation failures

### Changed

- **message/middleware:** `CorrelationID()` now uses `AttrCorrelationID` constant and `msg.CorrelationID()` accessor

## [0.12.0] - 2026-01-14

### Breaking Changes

- **message:** Rename `ParseRawMessage` to `ParseRaw` for consistency
- **message/plugin:** `BatchLoopback` now takes `BatchLoopbackConfig` instead of individual parameters

### Added

- **message:** CloudEvents attribute accessors: `ID()`, `Type()`, `Source()`, `Subject()`, `Time()`, `DataContentType()`, `DataSchema()`, `SpecVersion()`
- **message:** `MarshalJSON()` for CloudEvents structured JSON format
- **message:** `ParseRaw()` for parsing CloudEvents JSON input
- **message:** `data_base64` support for binary data per CloudEvents spec
- **message:** `NewID()` function for generating UUID v4 message IDs (uses google/uuid)
- **message:** `Attributes` field in `CommandHandlerConfig` for static output attributes
- **message/middleware:** `Subject()` middleware for automatic subject extraction via duck typing
- **message/plugin:** `GroupLoopback` for key-based message batching before transformation
- **message/plugin:** `GroupLoopbackConfig` and `BatchLoopbackConfig` structs
- **message/cloudevents:** CloudEvents protocol integration with `Subscriber` and `Publisher`
- **message/cloudevents:** `SubscriberPlugin` and `PublisherPlugin` for simplified engine registration
- **examples:** CloudEvents HTTP example (06-cloudevents-http)

### Changed

- **message:** `CommandHandlerConfig` now uses `time.Time` directly for `AttrTime` (auto-marshals to RFC3339)

## [0.11.0] - 2025-01-09

### Breaking Changes

#### Message Package Redesign

Complete redesign of the `message` package for simplicity and native CloudEvents support.

**Removed:**
- `Sender`, `Receiver` interfaces
- `Subscriber`, `Publisher` structs
- Old `Router`, `Handler`, `Pipe`, `Generator` types
- Old `Middleware` type
- `broker/` subpackage (`ChannelBroker`, `HTTPBroker`, `IOBroker`)
- `cqrs/` subpackage
- `multiplex/` subpackage
- `cloudevents/` subpackage

**Added:**
- `Engine` — orchestrates message flow between inputs, handlers, and outputs
- `Router` — type-based handler routing with middleware support
- `Handler` interface — self-describing with `EventType()`, `NewInput()`, `Handle()`
- `NewHandler[T]` — generic handler factory
- `NewCommandHandler[C, E]` — command/event handler factory
- `Marshaler` interface with `JSONMarshaler` implementation
- `match/` subpackage — `Types()`, `Sources()`, `All()`, `Any()` matchers
- `middleware/` subpackage — `CorrelationID()` middleware
- `plugin/` subpackage — `Loopback`, `ProcessLoopback`, `BatchLoopback`

**Kept:**
- `Message` (alias for `TypedMessage[any]`)
- `RawMessage` (alias for `TypedMessage[[]byte]`)
- `TypedMessage[T]` with `Data`, `Attributes`, `Ack()`, `Nack()`
- `Attributes` map type
- `Acking` for acknowledgment coordination

#### API Simplification

- **message**: `Add*` methods now take direct parameters instead of config structs
  - `AddHandler(name, matcher, handler)` — was `AddHandler(AddHandlerConfig{})`
  - `AddInput(name, matcher, ch)` — was `AddInput(AddInputConfig{})`
  - `AddRawInput(name, matcher, ch)` — was `AddRawInput(AddRawInputConfig{})`
  - `AddOutput(name, matcher)` — was `AddOutput(AddOutputConfig{})`
  - `AddRawOutput(name, matcher)` — was `AddRawOutput(AddRawOutputConfig{})`
- **message**: `NewAcking(ack, nack)` simplified — removed `expectedCount` parameter
- **message**: Added `NewSharedAcking(ack, nack, expectedCount)` for multi-message acknowledgment
- **message**: Constructors simplified to `New()`, `NewTyped()`, `NewRaw()`
- **pipe**: `Merger.Add()` renamed to `Merger.AddInput()` for symmetry with `Distributor`
- **pipe**: `ApplyMiddleware()` renamed to `Use()` for Go convention

### Added

- **pipe**: `Distributor` for one-to-many message routing with matcher-based output selection
  - First-match-wins routing with optional matcher functions (nil matches all)
  - Dynamic `AddOutput()` during runtime (concurrent-safe)
  - `NoMatchHandler` callback for unmatched messages
  - Graceful shutdown with configurable timeout
- **message**: `Engine.AddInput()` and `Engine.AddOutput()` support dynamic addition after `Start()`
- **message**: `Plugin` mechanism for reusable engine configuration
- **message**: `Use()` method for middleware registration on Engine and Router
- **docs**: `AGENTS.md` for AI coding agent guidance (merged from `CLAUDE.md`)
- **docs**: `doc.go` files for channel, pipe, and message packages
- **examples**: Learning path with 5 numbered examples (01-05)

### Fixed

- **message**: `Copy()` now clones `Attributes` map to avoid shared reference bugs
- **message**: Removed unused `ErrNoMatchingOutput` error
- **pipe**: Improved shutdown semantics for `Merger` and `Distributor`

### Changed

- **docs**: Corrected architecture documentation — Engine uses single merger
- **docs**: Clarified loopback is a plugin (`plugin.Loopback`), not built into Engine
- **docs**: Improved main README with quick start examples and learning path
- **examples**: Removed broken/complex examples, kept 5 essential ones
- **examples**: Fixed message example to use current API

## [0.10.1] - 2024-12-17

### Fixed

- **message/broker**: `ChannelBroker.Receive` changed from polling with hard-coded 100ms timeout to blocking by default. Added `ReceiveTimeout` config option for optional timeout behavior.

## [0.10.0] - 2024-12-12

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

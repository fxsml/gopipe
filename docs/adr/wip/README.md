# gopipe Architecture Decision Records

ADRs documenting decisions for the gopipe core library (channel operations, pipeline primitives).

## Naming Convention

ADR files use status prefixes:

| Prefix | Meaning | Symbol |
|--------|---------|--------|
| `IMP-` | Implemented - Fully in codebase | ✅ |
| `ACC-` | Accepted - Guides development | ✓ |
| `PRO-` | Proposed - Under consideration | 💡 |
| `SUP-` | Superseded - Replaced by newer | ⛔ |

## ADR Index

### Implemented (IMP-)

| ADR | Title | Feature |
|-----|-------|---------|
| [IMP-0001](IMP-0001-public-message-fields.md) | Public Message Fields | Core |
| [IMP-0002](IMP-0002-remove-properties-thread-safety.md) | Remove Properties Thread Safety | Core |
| [IMP-0003](IMP-0003-remove-noisy-properties.md) | Remove Noisy Properties | Core |
| [IMP-0004](IMP-0004-dual-message-types.md) | Dual Message Types | Core |
| [IMP-0005](IMP-0005-remove-functional-options.md) | Remove Functional Options | Core |
| [IMP-0006](IMP-0006-cqrs-implementation.md) | CQRS Implementation | CQRS |
| [IMP-0010](IMP-0010-pubsub-package-structure.md) | Pub/Sub Package Structure | Pub/Sub |
| [IMP-0012](IMP-0012-multiplex-pubsub.md) | Multiplex Pub/Sub | Pub/Sub |
| [IMP-0013](IMP-0013-processor-abstraction.md) | Processor Abstraction | Pipeline |
| [IMP-0014](IMP-0014-composable-pipe.md) | Composable Pipe | Pipeline |
| [IMP-0015](IMP-0015-middleware-pattern.md) | Middleware Pattern | Pipeline |
| [IMP-0016](IMP-0016-channel-package.md) | Channel Package | Channel |
| [IMP-0017](IMP-0017-message-acknowledgment.md) | Message Acknowledgment | Core |

### Accepted (ACC-)

| ADR | Title | Description |
|-----|-------|-------------|
| [ACC-0018](ACC-0018-cloudevents-terminology.md) | CloudEvents Terminology | Aligns attribute naming with CE spec |

### Proposed (PRO-)

#### Pipe Simplification (PRO-0026 Series)

| ADR | Title | Plan | Dependencies |
|-----|-------|------|--------------|
| [PRO-0026](PRO-0026-pipe-processor-simplification.md) | Pipe Simplification (Index) | - | - |
| [PRO-0032](PRO-0032-spearate-middleware-from-config.md) | Separate Middleware from Config | [PRO-0008](../plans/PRO-0008-separate-middleware-from-config/) | PRO-0033 |
| [PRO-0033](PRO-0033-non-generic-processor-config.md) | Non-Generic ProcessorConfig | [PRO-0009](../plans/PRO-0009-non-generic-processor-config/) | - |
| [PRO-0034](PRO-0034-simplified-processor-interface.md) | Simplified Processor Interface | [PRO-0010](../plans/PRO-0010-simplified-processor-interface/) | - |
| [PRO-0035](PRO-0035-message-error-routing.md) | Message Error Routing | [PRO-0011](../plans/PRO-0011-message-error-routing/) | PRO-0037 |
| [PRO-0036](PRO-0036-logging-metrics-middleware.md) | Logging and Metrics Middleware | [PRO-0012](../plans/PRO-0012-logging-metrics-middleware/) | PRO-0037 |
| [PRO-0037](PRO-0037-middleware-package-consolidation.md) | Middleware Package Consolidation | [PRO-0013](../plans/PRO-0013-middleware-package-consolidation/) | PRO-0034 |

#### Distribution Patterns (PRO-0027-0030)

| ADR | Title | Plan | Dependencies |
|-----|-------|------|--------------|
| [PRO-0027](PRO-0027-fan-out-pattern.md) | Fan-Out Pattern | [PRO-0014](../plans/PRO-0014-fan-out-pattern/) | PRO-0026 |
| [PRO-0028](PRO-0028-generator-source-patterns.md) | Subscriber Patterns | [PRO-0015](../plans/PRO-0015-subscriber-patterns/) | PRO-0026 |
| [PRO-0030](PRO-0030-remove-sender-receiver.md) | Remove Sender/Receiver | [PRO-0016](../plans/PRO-0016-remove-sender-receiver/) | PRO-0028 |

#### Known Issues Solutions (PRO-0031 Series)

| ADR | Title | Plan | Priority |
|-----|-------|------|----------|
| [PRO-0031](PRO-0031-solutions-to-known-issues.md) | Known Issues (Index) | - | - |
| [PRO-0038](PRO-0038-ack-strategy-interface.md) | Ack Strategy Interface | [PRO-0017](../plans/PRO-0017-ack-strategy/) | High |
| [PRO-0039](PRO-0039-flow-control-backpressure.md) | Flow Control | [PRO-0018](../plans/PRO-0018-flow-control/) | High |
| [PRO-0040](PRO-0040-connection-lifecycle-management.md) | Connection Management | [PRO-0019](../plans/PRO-0019-connection-lifecycle/) | Medium |
| [PRO-0041](PRO-0041-codec-serialization.md) | Codec Serialization | [PRO-0020](../plans/PRO-0020-codec-system/) | Medium |
| [PRO-0042](PRO-0042-groupby-composite-keys.md) | GroupBy Composite Keys | [PRO-0021](../plans/PRO-0021-groupby-composite/) | Low |
| [PRO-0043](PRO-0043-extension-points-middleware.md) | Extension Points | [PRO-0022](../plans/PRO-0022-extension-points/) | Medium |
| [PRO-0044](PRO-0044-performance-benchmarks.md) | Performance Benchmarks | [PRO-0023](../plans/PRO-0023-performance/) | Low |
| [PRO-0045](PRO-0045-testing-infrastructure.md) | Testing Infrastructure | [PRO-0024](../plans/PRO-0024-testing-infra/) | Low |
| [PRO-0046](PRO-0046-versioning-compatibility.md) | Versioning Policy | [PRO-0025](../plans/PRO-0025-versioning/) | Low |

### Superseded (SUP-)

| ADR | Superseded By | Reason |
|-----|---------------|--------|
| [SUP-0011](SUP-0011-cloudevents-compatibility.md) | ACC-0018 | Integrated CE attributes into message |

## Repository Separation

gopipe is separated from goengine (CloudEvents messaging engine):

| Package | Focus | ADRs Location |
|---------|-------|---------------|
| **gopipe** | Channel operations, pipeline primitives | This directory |
| **goengine** | CloudEvents messaging, broker integration | [../goengine/adr/](../goengine/adr/) |

## Feature Groups

### Core Message (IMP-0001 to IMP-0005)

Foundation for simplified message handling. Implemented 2025-11-01.

### Pipeline (IMP-0013 to IMP-0015)

Composable message processing with processors, pipes, and middleware.

### Pub/Sub (IMP-0010, IMP-0012)

Message broker and routing infrastructure.

### Proposed Refactoring

- **PRO-0026 Series**: Pipe and processor simplification
- **PRO-0027-0030**: Distribution patterns (fan-out, subscriber, pub/sub)
- **PRO-0031 Series**: Solutions to known issues

## Reading Order

1. **Core** (IMP-0001-0005) - Message foundation
2. **Pipeline** (IMP-0013-0015) - Processing abstractions
3. **Pub/Sub** (IMP-0010, IMP-0012) - Distribution
4. **Standards** (ACC-0018) - CloudEvents alignment
5. **Proposed** (PRO-0026-0046) - Future simplification

## Related Documentation

- [goengine ADRs](../goengine/adr/) - CloudEvents engine decisions
- [Patterns](../patterns/) - CQRS, Saga, Outbox patterns
- [Plans](../plans/) - Implementation plans
- [Manual](../manual/) - User documentation

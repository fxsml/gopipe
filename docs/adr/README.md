# gopipe Architecture Decision Records

## Quick Start

1. Get next number from index below
2. Create `NNNN-short-title.md`
3. Add to index

See [ADR Procedures](../procedures/adr.md) for details.

## Index

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-processor-abstraction.md) | Processor Abstraction | Superseded |
| [0002](0002-middleware-pattern.md) | Middleware Pattern | Superseded |
| [0003](0003-composable-pipe.md) | Composable Pipe | Implemented |
| [0004](0004-remove-functional-options.md) | Remove Functional Options | Implemented |
| [0005](0005-channel-package.md) | Channel Package | Implemented |
| [0006](0006-message-acknowledgment.md) | Message Acknowledgment | Implemented |
| [0007](0007-public-message-fields.md) | Public Message Fields | Implemented |
| [0008](0008-remove-properties-thread-safety.md) | Remove Properties Thread Safety | Implemented |
| [0009](0009-remove-noisy-properties.md) | Remove Noisy Properties | Implemented |
| [0010](0010-dual-message-types.md) | Dual Message Types | Implemented |
| [0011](0011-cqrs-implementation.md) | CQRS Implementation | Implemented |
| [0012](0012-pubsub-package-structure.md) | Pub/Sub Package Structure | Implemented |
| [0013](0013-multiplex-pubsub.md) | Multiplex Pub/Sub | Implemented |
| [0014](0014-go-workspaces-modularization.md) | Go Workspaces Modularization | Implemented |
| [0015](0015-remove-cancel-path.md) | Remove Cancel Path | Accepted |
| [0016](0016-processor-config-struct.md) | Processor Config Struct | Accepted |
| [0017](0017-middleware-for-processfunc.md) | Middleware for ProcessFunc | Accepted |
| [0018](0018-interface-naming-conventions.md) | Interface Naming Conventions | Implemented |
| [0019](0019-remove-sender-receiver.md) | Remove Sender and Receiver | Proposed |
| [0020](0020-message-engine-architecture.md) | Message Engine Architecture | Proposed |
| [0021](0021-codec-marshaling-pattern.md) | Codec/Marshaling Pattern | Proposed |
| [0022](0022-message-package-redesign.md) | Message Package Redesign | Proposed |
| [0023](0023-engine-simplification.md) | Engine Simplification | Implemented |
| [0024](0024-pure-vs-impure-operations.md) | Pure vs Impure Operations | Proposed |
| [0025](0025-semantic-interfaces.md) | Semantic Interfaces Pattern | Proposed |
| [0026](0026-naming-conventions.md) | Naming Conventions | Proposed |
| [0027](0027-producer-trigger-separation.md) | Producer/Trigger Separation | Proposed |
| [0028](0028-middleware-type-architecture.md) | Middleware Type Architecture | Proposed |
| [0029](0029-fan-out-operations.md) | Fan-out Operations | Proposed |
| [0030](0030-channel-vs-pipe-distinction.md) | Channel vs Pipe Distinction | Proposed |

## History

ADRs numbered sequentially by creation date since Dec 2025. Previous schemes used status prefixes (IMP-, PRO-, etc.) and issue-based numbering.

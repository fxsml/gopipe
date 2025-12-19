# gopipe Architecture Decision Records

ADRs numbered sequentially by creation date.

## Convention

- ADRs are numbered sequentially (0001, 0002, ...)
- Numbers reflect chronological order of when the decision was made
- No status prefixes in filenames - status is tracked in the ADR content

## ADR Index

| ADR | Created | Title | Status | Issue |
|-----|---------|-------|--------|-------|
| [0001](0001-processor-abstraction.md) | 2025-10-14 | Processor Abstraction | Implemented | [#12](https://github.com/fxsml/gopipe/issues/12) |
| [0002](0002-middleware-pattern.md) | 2025-10-14 | Middleware Pattern | Implemented | [#9](https://github.com/fxsml/gopipe/issues/9) |
| [0003](0003-composable-pipe.md) | 2025-10-16 | Composable Pipe | Implemented | [#8](https://github.com/fxsml/gopipe/issues/8) |
| [0004](0004-remove-functional-options.md) | 2025-10-28 | Remove Functional Options | Implemented | [#3](https://github.com/fxsml/gopipe/issues/3) |
| [0005](0005-channel-package.md) | 2025-10-30 | Channel Package | Implemented | [#23](https://github.com/fxsml/gopipe/issues/23) |
| [0006](0006-message-acknowledgment.md) | 2025-11-14 | Message Acknowledgment | Implemented | [#51](https://github.com/fxsml/gopipe/issues/51) |
| [0007](0007-public-message-fields.md) | 2025-12-06 | Public Message Fields | Implemented | [#52](https://github.com/fxsml/gopipe/issues/52) |
| [0008](0008-remove-properties-thread-safety.md) | 2025-12-06 | Remove Properties Thread Safety | Implemented | [#53](https://github.com/fxsml/gopipe/issues/53) |
| [0009](0009-remove-noisy-properties.md) | 2025-12-06 | Remove Noisy Properties | Implemented | [#54](https://github.com/fxsml/gopipe/issues/54) |
| [0010](0010-dual-message-types.md) | 2025-12-07 | Dual Message Types | Implemented | [#55](https://github.com/fxsml/gopipe/issues/55) |
| [0011](0011-cqrs-implementation.md) | 2025-12-08 | CQRS Implementation | Implemented | [#56](https://github.com/fxsml/gopipe/issues/56) |
| [0012](0012-pubsub-package-structure.md) | 2025-12-08 | Pub/Sub Package Structure | Implemented | [#57](https://github.com/fxsml/gopipe/issues/57) |
| [0013](0013-multiplex-pubsub.md) | 2025-12-09 | Multiplex Pub/Sub | Implemented | [#58](https://github.com/fxsml/gopipe/issues/58) |
| [0014](0014-go-workspaces-modularization.md) | 2025-12-17 | Go Workspaces Modularization | Implemented | [#48](https://github.com/fxsml/gopipe/issues/48) |

## Migration from Previous Numbering

This folder establishes a new sequential numbering scheme. The mapping from previous numbers:

| New | Old (IMP-) | Original (adr-) | Title |
|-----|------------|-----------------|-------|
| 0001 | IMP-0013 | adr-12 | Processor Abstraction |
| 0002 | IMP-0015 | adr-9 | Middleware Pattern |
| 0003 | IMP-0014 | adr-8 | Composable Pipe |
| 0004 | IMP-0005 | adr-3 | Remove Functional Options |
| 0005 | IMP-0016 | adr-23 | Channel Package |
| 0006 | IMP-0017 | adr-24 | Message Acknowledgment |
| 0007 | IMP-0001 | - | Public Message Fields |
| 0008 | IMP-0002 | - | Remove Properties Thread Safety |
| 0009 | IMP-0003 | - | Remove Noisy Properties |
| 0010 | IMP-0004 | - | Dual Message Types |
| 0011 | IMP-0006 | - | CQRS Implementation |
| 0012 | IMP-0010 | adr-25 | Pub/Sub Package Structure |
| 0013 | IMP-0012 | - | Multiplex Pub/Sub |
| 0014 | 0000 / PRO-0047 | - | Go Workspaces Modularization |

## Pending Migration

The following ADRs from `docs/adr/` still need to be reorganized:

### Accepted (to be added)

- ACC-0018: CloudEvents Terminology (2025-12-10)
- ACC-0026: Pipe Processor Simplification

### Proposed (to be reviewed)

- PRO-0026 series: Pipe Simplification
- PRO-0027-0030: Distribution Patterns
- PRO-0031 series: Known Issues Solutions

### Superseded (for reference)

- SUP-0011: CloudEvents Compatibility (superseded by ACC-0018)

### Moved to goengine

- Saga patterns (old 0007-0009) → goengine PRO-0001-0003
- CloudEvents ADRs (old 0019-0029) → goengine PRO-0004-0012

---

## History

<details>
<summary>Previous numbering schemes (click to expand)</summary>

### Original Scheme (Oct-Nov 2025)

ADR numbers matched GitHub issue numbers:
- `docs/adr-N_name.md` format
- Numbers: 3, 8, 9, 12, 15, 23, 24, 25

### v0.10.0 Scheme (Dec 2025)

Sequential numbering in `docs/adr/` folder:
- Numbers: 0001-0018
- Did NOT reflect creation order

### Post-v0.10.0 Scheme

Status prefixes added:
- IMP- (Implemented)
- ACC- (Accepted)
- PRO- (Proposed)
- SUP- (Superseded)

### Current Scheme

Sequential by creation date, no prefixes:
- Numbers: 0001-0014 (implemented)
- Future ADRs continue the sequence

</details>

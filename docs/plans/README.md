# gopipe Roadmap & Plans

Implementation plans for gopipe core library.

## Focus

**gopipe** focuses on:
- Channel operations (GroupBy, Broadcast, Merge, Filter, Collect)
- Pipeline primitives (Processor, Pipe, Subscriber)
- Generic middleware

**goengine** (separate repo) focuses on:
- CloudEvents messaging
- Broker adapters
- Message routing and orchestration

See [goengine/plans/](../goengine/plans/) for messaging plans.

## Plan Index

### Pipe Simplification (PRO-0026 Series)

| Plan | Title | ADR | Priority | Status |
|------|-------|-----|----------|--------|
| [PRO-0001](PRO-0001-processor-config/) | ProcessorConfig | PRO-0026 | High | Proposed |
| [PRO-0002](PRO-0002-cancel-path/) | Cancel Path Simplification | PRO-0034 | High | Proposed |
| [PRO-0008](PRO-0008-separate-middleware-from-config/) | Separate Middleware from Config | [PRO-0032](../adr/PRO-0032-spearate-middleware-from-config.md) | High | Proposed |
| [PRO-0009](PRO-0009-non-generic-processor-config/) | Non-Generic ProcessorConfig | [PRO-0033](../adr/PRO-0033-non-generic-processor-config.md) | High | Proposed |
| [PRO-0010](PRO-0010-simplified-processor-interface/) | Simplified Processor Interface | [PRO-0034](../adr/PRO-0034-simplified-processor-interface.md) | High | Proposed |
| [PRO-0011](PRO-0011-message-error-routing/) | Message Error Routing | [PRO-0035](../adr/PRO-0035-message-error-routing.md) | Medium | Proposed |
| [PRO-0012](PRO-0012-logging-metrics-middleware/) | Logging and Metrics Middleware | [PRO-0036](../adr/PRO-0036-logging-metrics-middleware.md) | Medium | Proposed |
| [PRO-0013](PRO-0013-middleware-package-consolidation/) | Middleware Package Consolidation | [PRO-0037](../adr/PRO-0037-middleware-package-consolidation.md) | High | Proposed |

### Distribution Patterns (PRO-0027-0030)

| Plan | Title | ADR | Priority | Status |
|------|-------|-----|----------|--------|
| [PRO-0003](PRO-0003-subscriber-interface/) | Subscriber Interface | PRO-0028 | High | Proposed |
| [PRO-0014](PRO-0014-fan-out-pattern/) | Fan-Out Pattern | [PRO-0027](../adr/PRO-0027-fan-out-pattern.md) | Medium | Proposed |
| [PRO-0015](PRO-0015-subscriber-patterns/) | Subscriber Patterns | [PRO-0028](../adr/PRO-0028-generator-source-patterns.md) | High | Proposed |
| [PRO-0016](PRO-0016-remove-sender-receiver/) | Remove Sender/Receiver | [PRO-0030](../adr/PRO-0030-remove-sender-receiver.md) | Medium | Proposed |

### Known Issues Solutions (PRO-0031 Series)

| Plan | Title | ADR | Priority | Status |
|------|-------|-----|----------|--------|
| [PRO-0017](PRO-0017-ack-strategy/) | Ack Strategy Interface | [PRO-0038](../adr/PRO-0038-ack-strategy-interface.md) | High | Proposed |
| [PRO-0018](PRO-0018-flow-control/) | Flow Control | [PRO-0039](../adr/PRO-0039-flow-control-backpressure.md) | High | Proposed |
| [PRO-0019](PRO-0019-connection-lifecycle/) | Connection Lifecycle | [PRO-0040](../adr/PRO-0040-connection-lifecycle-management.md) | Medium | Proposed |
| [PRO-0020](PRO-0020-codec-system/) | Codec System | [PRO-0041](../adr/PRO-0041-codec-serialization.md) | Medium | Proposed |
| [PRO-0021](PRO-0021-groupby-composite/) | GroupBy Composite Keys | [PRO-0042](../adr/PRO-0042-groupby-composite-keys.md) | Low | Proposed |
| [PRO-0022](PRO-0022-extension-points/) | Extension Points | [PRO-0043](../adr/PRO-0043-extension-points-middleware.md) | Medium | Proposed |
| [PRO-0023](PRO-0023-performance/) | Performance Benchmarks | [PRO-0044](../adr/PRO-0044-performance-benchmarks.md) | Low | Proposed |
| [PRO-0024](PRO-0024-testing-infra/) | Testing Infrastructure | [PRO-0045](../adr/PRO-0045-testing-infrastructure.md) | Low | Proposed |
| [PRO-0025](PRO-0025-versioning/) | Versioning Policy | [PRO-0046](../adr/PRO-0046-versioning-compatibility.md) | Low | Proposed |

### Other Plans

| Plan | Title | Priority | Status |
|------|-------|----------|--------|
| [PRO-0004](PRO-0004-middleware-package/) | Middleware Package | Medium | Superseded by PRO-0013 |
| [PRO-0005](PRO-0005-uuid-integration/) | UUID Integration | N/A | Won't Do |
| [PRO-0006](PRO-0006-package-restructuring/) | Package Restructuring | Medium | Proposed |
| [PRO-0007](PRO-0007-groupby-flexibility/) | GroupBy Key Flexibility | Low | Superseded by PRO-0021 |

## Implementation Order

### Phase 1: Core Simplification (High Priority)

```
PRO-0009 (ProcessorConfig) ─┬─> PRO-0010 (Simplified Processor)
                            │
                            └─> PRO-0008 (Config/Middleware Separation)
```

### Phase 2: Middleware (High Priority)

```
PRO-0013 (Middleware Package) ─┬─> PRO-0011 (Error Routing)
                               │
                               └─> PRO-0012 (Logging/Metrics)
```

### Phase 3: Distribution (Medium Priority)

```
PRO-0015 (Subscriber) ─┬─> PRO-0014 (Fan-Out)
                       │
                       └─> PRO-0016 (Remove Sender/Receiver)
```

### Phase 4: Infrastructure (Medium/Low Priority)

```
PRO-0017 (Ack Strategy) ──> PRO-0018 (Flow Control)
PRO-0019 (Connection) ──> PRO-0020 (Codec)
PRO-0022 (Extensions) ──> PRO-0023 (Performance)
```

## PR Sequence

1. **PR 1:** PRO-0009 + PRO-0010 (ProcessorConfig + Simplified Processor)
2. **PR 2:** PRO-0008 (Config/Middleware Separation)
3. **PR 3:** PRO-0013 (Middleware Package Consolidation)
4. **PR 4:** PRO-0015 (Subscriber Patterns)
5. **PR 5:** PRO-0017 + PRO-0018 (Ack Strategy + Flow Control)

## Plan Status

| Status | Meaning |
|--------|---------|
| Proposed | Documented, not started |
| In Progress | Work underway |
| Complete | Fully implemented |
| Superseded | Replaced by newer plan |
| Won't Do | Decided against implementation |

## Related Documentation

- [gopipe ADRs](../adr/) - Architecture decisions
- [goengine Plans](../goengine/plans/) - CloudEvents messaging plans
- [Manual](../manual/) - User documentation
- [Planning Procedures](../procedures/planning.md) - How to create plans

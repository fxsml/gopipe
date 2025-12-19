# ADR 0026: Pipe and Processor Simplification (Index)

**Date:** 2025-12-13
**Status:** Accepted
**Supersedes:** Partial aspects of current implementation

## Context

Before implementing the CloudEvents standardization (ADRs 0019-0025), we need to review and simplify the core pipe and processor abstractions. This ADR serves as an index to the individual ADRs that address each concern.

## Issues Identified

### Current State Analysis

1. **Generic Verbosity**: Every option requires explicit generic parameters
2. **Mixed Concerns**: Options mix configuration with middleware behavior
3. **Complex Processor Interface**: Cancel method adds unnecessary complexity
4. **Scattered Middleware**: No central middleware organization

## ADR Index

Each concern has been extracted into its own ADR:

| Concern | ADR | Title | Plan |
|---------|-----|-------|------|
| Config vs Middleware | [PRO-0032](PRO-0032-spearate-middleware-from-config.md) | Separate Middleware from Config | [PRO-0008](../plans/PRO-0008-separate-middleware-from-config/) |
| Generic Config | [PRO-0033](PRO-0033-non-generic-processor-config.md) | Non-Generic ProcessorConfig | [PRO-0009](../plans/PRO-0009-non-generic-processor-config/) |
| Processor Interface | [PRO-0034](PRO-0034-simplified-processor-interface.md) | Simplified Processor Interface | [PRO-0010](../plans/PRO-0010-simplified-processor-interface/) |
| Error Routing | [PRO-0035](PRO-0035-message-error-routing.md) | Message Error Routing | [PRO-0011](../plans/PRO-0011-message-error-routing/) |
| Observability | [PRO-0036](PRO-0036-logging-metrics-middleware.md) | Logging and Metrics Middleware | [PRO-0012](../plans/PRO-0012-logging-metrics-middleware/) |
| Middleware Package | [PRO-0037](PRO-0037-middleware-package-consolidation.md) | Middleware Consolidation | [PRO-0013](../plans/PRO-0013-middleware-package-consolidation/) |

## Related ADRs

| ADR | Title | Description |
|-----|-------|-------------|
| [PRO-0027](PRO-0027-fan-out-pattern.md) | Fan-Out Pattern | Message distribution |
| [PRO-0028](PRO-0028-generator-source-patterns.md) | Subscriber Patterns | Unified subscriber interface |
| [PRO-0030](PRO-0030-remove-sender-receiver.md) | Remove Sender/Receiver | Simplify pub/sub |
| [PRO-0031](PRO-0031-solutions-to-known-issues.md) | Known Issues | Index of solutions |

## Implementation Order

```
PRO-0033 (ProcessorConfig) ─┬─> PRO-0032 (Config/Middleware Separation)
                            │
PRO-0034 (Simplified Processor) ─┬─> PRO-0037 (Middleware Package)
                                 │
                                 ├─> PRO-0035 (Error Routing)
                                 │
                                 └─> PRO-0036 (Logging/Metrics)
```

## Migration Summary

```go
// Old
pipe := NewProcessPipe(
    handler,
    WithConcurrency[In, Out](4),
    WithTimeout[In, Out](5*time.Second),
    WithCancel[In, Out](cancelFunc),
)

// New
pipe := NewPipe(
    handler,
    ProcessorConfig{
        Concurrency: 4,
        OnError:     errorFunc,
    },
    middleware.WithTimeout[In, Out](5*time.Second),
)
```

## Consequences

**Positive:**
- Simpler API with less generic boilerplate
- Clearer mental model (config vs middleware)
- Better messaging alignment
- Easier testing and documentation

**Negative:**
- Breaking change from current API
- Multiple ADRs to coordinate
- Migration effort required

## Links

- Individual ADRs listed in table above
- [ADR 0013: Processor Abstraction](IMP-0013-processor-abstraction.md)
- [ADR 0014: Composable Pipe](IMP-0014-composable-pipe.md)
- [ADR 0015: Middleware Pattern](IMP-0015-middleware-pattern.md)

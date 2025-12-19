# ADR 0031: Solutions to Known Issues (Index)

**Date:** 2025-12-17
**Status:** Proposed (Split into separate ADRs)
**Related:** ADR 0017, ADR 0021, ADR 0028, ADR 0030

## Context

During the implementation of broker adapters (NATS, Kafka, RabbitMQ), we documented several known issues in `docs/known-issues.md`. This ADR serves as an index to the individual ADRs that address each issue.

## Issues Index

Each issue has been extracted into its own ADR for focused implementation:

| Issue | ADR | Title | Priority |
|-------|-----|-------|----------|
| 1. Topic Semantics | ✅ [PRO-0030](PRO-0030-remove-sender-receiver.md) | Remove Sender/Receiver | Done |
| 2. Acknowledgment Models | [PRO-0038](PRO-0038-ack-strategy-interface.md) | Ack Strategy Interface | High |
| 3. Backpressure | [PRO-0039](PRO-0039-flow-control-backpressure.md) | Flow Control | High |
| 4. Connection Lifecycle | [PRO-0040](PRO-0040-connection-lifecycle-management.md) | Connection Management | Medium |
| 5. Serialization | [PRO-0041](PRO-0041-codec-serialization.md) | Codec System | Medium |
| 6. GroupBy Keys | [PRO-0042](PRO-0042-groupby-composite-keys.md) | Composite Keys | Low |
| 7. Extension Points | [PRO-0043](PRO-0043-extension-points-middleware.md) | Extensions Middleware | Medium |
| 8. Performance | [PRO-0044](PRO-0044-performance-benchmarks.md) | Benchmarks | Low |
| 9. Testing | [PRO-0045](PRO-0045-testing-infrastructure.md) | Testing Infrastructure | Low |
| 10. Versioning | [PRO-0046](PRO-0046-versioning-compatibility.md) | Version Policy | Low |

## Implementation Priority

| Priority | ADRs |
|----------|------|
| High | PRO-0038 (Ack Strategy), PRO-0039 (Flow Control) |
| Medium | PRO-0040 (Connection), PRO-0041 (Codec), PRO-0043 (Extensions) |
| Low | PRO-0042 (GroupBy), PRO-0044 (Perf), PRO-0045 (Testing), PRO-0046 (Version) |

## Consequences

**Positive:**
- Each issue has focused ADR
- Clear implementation priority
- Independent implementation possible

**Negative:**
- More ADRs to track
- Cross-references needed

## Links

- [Known Issues](../known-issues.md)
- Individual ADRs listed in table above

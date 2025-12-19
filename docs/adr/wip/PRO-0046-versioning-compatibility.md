# ADR 0046: Versioning and Compatibility Policy

**Date:** 2025-12-17
**Status:** Proposed
**Related:** PRO-0031 (issue #10)

## Context

Go version requirements and breaking changes need clear policy. Need version policy and migration tools.

## Decision

### Supported Go Versions

- Minimum: Go 1.21 (generics maturity)
- Recommended: Go 1.22+
- Development: Latest stable

### Semantic Versioning

- MAJOR: Breaking API changes
- MINOR: New features, backward compatible
- PATCH: Bug fixes only

### Deprecation Policy

1. Mark deprecated in MINOR release with godoc comment
2. Log runtime warning for 2 MINOR releases
3. Remove in next MAJOR release

```go
// Deprecated: Use NewFunction instead. Will be removed in v2.0.
func OldFunction() {}
```

### Migration Tool

```go
// cmd/gopipe-migrate/main.go

// Automated migration for common patterns
// Usage: gopipe-migrate --from v0.x --to v1.x ./...

// Migrations:
// - Sender/Receiver → Subscriber pattern
// - Option[In,Out] → ProcessorConfig
// - Generator → Subscriber
```

### Compatibility Matrix

| gopipe | Go Minimum | Status |
|--------|------------|--------|
| v1.x | 1.21 | Current |
| v0.x | 1.18 | Legacy |

## Consequences

**Positive:**
- Clear upgrade path
- Automated migration
- Predictable deprecation

**Negative:**
- Migration tool maintenance
- Multiple Go versions support

## Links

- Extracted from: [PRO-0031](PRO-0031-solutions-to-known-issues.md)

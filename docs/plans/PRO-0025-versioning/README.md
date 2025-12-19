# PRO-0025: Versioning and Compatibility

**Status:** Proposed
**Priority:** Low
**Related ADRs:** PRO-0046

## Overview

Document versioning policy and create migration tool.

## Goals

1. Document version policy
2. Document deprecation process
3. Create migration tool

## Task 1: Version Policy Documentation

**Files to Create:**
- `docs/versioning.md`

Contents:
- Supported Go versions (1.21+)
- Semantic versioning rules
- Deprecation policy (2 minor releases)

## Task 2: Migration Tool

**Files to Create:**
- `cmd/gopipe-migrate/main.go`

```go
// Usage: gopipe-migrate --from v0.x --to v1.x ./...

// Migrations:
// - Sender/Receiver → Subscriber pattern
// - Option[In,Out] → ProcessorConfig
// - Generator → Subscriber
```

**Acceptance Criteria:**
- [ ] Version policy documented
- [ ] Deprecation process documented
- [ ] Migration tool scaffolded
- [ ] Common migrations identified
- [ ] CHANGELOG updated

## Related

- [PRO-0046](../../adr/PRO-0046-versioning-compatibility.md) - ADR

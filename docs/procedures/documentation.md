# Documentation Procedures

## Required Documentation

Before pushing code:

- [ ] Feature documentation in `docs/features/`
- [ ] CHANGELOG.md updated under `[Unreleased]`
- [ ] ADRs for architectural decisions
- [ ] README.md for core component changes
- [ ] Godoc for public APIs

## Directory Structure

```
docs/
├── adr/           # Architecture Decision Records
├── features/      # Feature documentation
├── manual/        # User manual
├── plans/         # Implementation plans
└── procedures/    # Development procedures (this)
```

## Feature Documentation Template

```markdown
# Feature: <Name>

**Package:** `<package-name>`
**Status:** ✅ Implemented | 🔄 Proposed | ⛔ Superseded
**Related ADRs:**
- [ADR NNNN](../adr/NNNN-name.md) - Description

## Summary

Brief description.

## Implementation

Key types and APIs:

\`\`\`go
type Foo struct { ... }
func NewFoo() *Foo
\`\`\`

## Usage Example

\`\`\`go
foo := NewFoo()
\`\`\`

## Files Changed

- `path/to/file.go` - Description

## Related Features

- [NN-feature](NN-feature.md) - Relationship
```

## ADR Template

```markdown
# ADR NNNN: <Title>

**Date:** YYYY-MM-DD
**Status:** Proposed | Accepted | Implemented | Superseded

## Context

Problem or situation requiring decision.

## Decision

What we decided and why.

## Consequences

Positive and negative outcomes.
```

## ADR Status Categories

| Status | Meaning |
|--------|---------|
| Proposed 💡 | Documented, not implemented |
| Accepted ✓ | Approved, guides development |
| Implemented ✅ | Complete in codebase |
| Superseded ⛔ | Replaced by newer ADR |

## Plan Documentation

See [planning.md](planning.md) for plan file conventions.

# ADR Procedures

Procedures for Architecture Decision Records in gopipe.

## When to Create an ADR

Create an ADR when:
- Changing public API signatures
- Adding/removing interfaces or types
- Changing architectural patterns
- Making decisions with long-term impact

Do NOT create an ADR for:
- Bug fixes
- Internal refactoring without API changes
- Documentation updates

## ADR Lifecycle

```
Proposed → Accepted → Implemented
                ↓
            Superseded (by newer ADR)
```

| Status | Meaning |
|--------|---------|
| Proposed | Under discussion |
| Accepted | Approved, not yet implemented |
| Implemented | Code complete |
| Superseded | Replaced by another ADR |

## Creating an ADR

1. Get next number from `docs/adr/README.md` index
2. Create file: `docs/adr/NNNN-short-title.md`
3. Use template below
4. Add entry to README.md index
5. Link related ADRs

## Updating an ADR

**Status changes:**
- Update `Status:` field in ADR header
- Update status in README.md index

**When superseding:**
- Mark old ADR: `Status: Superseded by ADR NNNN`
- New ADR should reference: `Supersedes: ADR NNNN`

**Content updates:**
- Add `## Updates` section at end
- Format: `**YYYY-MM-DD:** Description of change`

## Naming Convention

Format: `NNNN-short-title.md`

- Sequential 4-digit number
- Lowercase, hyphen-separated
- No status prefix in filename

## Template

```markdown
# ADR NNNN: Title

**Date:** YYYY-MM-DD
**Status:** Proposed | Accepted | Implemented | Superseded by ADR NNNN

## Context

Problem or situation requiring decision.

## Decision

What we decided. Include code examples showing the API:

\`\`\`go
// Example code
\`\`\`

## Consequences

**Breaking Changes:**
- Migration step 1 (if applicable)

**Benefits:**
- Benefit 1

**Drawbacks:**
- Drawback 1

## Links

- Supersedes: ADR NNNN (if applicable)
- Related: ADR NNNN

## Updates

**YYYY-MM-DD:** Description of change (if any updates made after initial creation)
```

## Review Checklist

Before merging ADR changes:

- [ ] Number is sequential and unique
- [ ] Status reflects current state
- [ ] README.md index updated
- [ ] Related ADRs cross-referenced
- [ ] Code examples match actual implementation
- [ ] Superseded ADRs marked correctly

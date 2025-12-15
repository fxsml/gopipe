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

## Docs Compress Procedure

Consolidate multiple related documentation files into a unified state document. Similar to context compression for maintaining clarity.

### When to Use

- Branch has accumulated many incremental docs (ADRs, plans, features)
- Before merging a large feature branch
- Documentation sprawl makes current state unclear

### Procedure

1. **Identify scope**: List all docs created/modified on the branch
   ```bash
   git diff --name-only main -- docs/
   ```

2. **Review each document**: Extract key decisions, APIs, and current state

3. **Create state document**: `docs/state/<topic>-state.md`
   ```markdown
   # <Topic> Current State

   **Date:** YYYY-MM-DD
   **Branch:** <branch-name>
   **Source Documents:** <list of consolidated docs>

   ## Summary

   One paragraph describing the current state.

   ## Key Decisions

   | Decision | Rationale | ADR |
   |----------|-----------|-----|
   | ... | ... | ... |

   ## Current API

   Key types and functions currently implemented or planned.

   ## Implementation Status

   | Component | Status | Notes |
   |-----------|--------|-------|
   | ... | ... | ... |

   ## Next Steps

   Prioritized list of remaining work.
   ```

4. **Archive or retain originals**: Keep original docs for history, reference state doc for current understanding

5. **Update references**: Point CLAUDE.md or README to state document

### Example

```bash
# Find docs on branch
git diff --name-only main -- docs/

# Creates: docs/state/cloudevents-state.md
# From: docs/adr/0018-0029.md, docs/plans/*.md, docs/features/09-15.md
```

## Compression Guidelines

**CRITICAL**: Follow these rules when compressing or consolidating documentation.

### Compression Hierarchy

| Type | Compressibility | Rationale |
|------|-----------------|-----------|
| Procedures | **Minimal** | Must be precise and actionable |
| ADRs | Low | Historical record of decisions |
| Plans | Medium | Can reference state docs |
| Features | Medium | Can reference state docs |
| State docs | High | Summary by design |

### Rules

1. **Never delete procedures without explicit permission**
   - Procedures are essential for repeatability
   - Move to archive if truly obsolete, don't delete

2. **Procedures require precision**
   - Include exact commands and steps
   - Keep all edge cases and caveats
   - Don't summarize to the point of losing detail

3. **Plans and features have more freedom**
   - Can be consolidated into state documents
   - Original files retained for history
   - State doc becomes the reference

4. **Always preserve**:
   - Step-by-step procedures
   - Command-line examples
   - Checklists
   - Templates

# Documentation Procedures

Procedures for maintaining gopipe documentation.

## Directory Structure

```
docs/
├── README.md            # Conventions and structure
├── adr/                 # Architecture Decision Records
├── analysis/            # Historical analysis and comparisons
├── manual/              # User manual and guides
├── patterns/            # CQRS, Saga, Outbox patterns
├── plans/               # Roadmap and implementation plans
├── procedures/          # Development procedures (this)
│
└── goengine/            # goengine documentation
    ├── adr/             # goengine ADRs (PRO-0001+)
    ├── plans/           # goengine plans (numbered folders)
    └── README.md        # goengine overview
```

## Required Documentation

Before pushing code:

- [ ] CHANGELOG.md updated under `[Unreleased]`
- [ ] ADRs for architectural decisions
- [ ] Plans updated if implementing planned work
- [ ] Godoc for public APIs
- [ ] README.md for core component changes

## Naming Conventions

### ADR Files

Use status prefix:

| Prefix | Meaning |
|--------|---------|
| `IMP-` | Implemented |
| `ACC-` | Accepted |
| `PRO-` | Proposed |
| `SUP-` | Superseded |

Format: `{PREFIX}-{NUMBER}-{short-title}.md`

### Plan Folders

Plans are numbered folders with a single README.md (no plan.md):

Format: `PRO-{NUMBER}-{short-title}/`

Contents:
- `README.md` - Complete plan document (required, see template below)
- `examples/` - Working example code (optional)

**Important:** Use only README.md for plans. Do not create separate plan.md files.

## Adding Documentation

### Adding an ADR

1. Determine next number in `docs/adr/`
2. Create file: `docs/adr/PRO-NNNN-short-title.md`
3. Use ADR template (see below)
4. Update `docs/adr/README.md` index

### Adding a Plan

1. Determine next number in `docs/plans/`
2. Create folder: `docs/plans/PRO-NNNN-short-title/`
3. Create `README.md` using plan template
4. Add examples if applicable
5. Update `docs/plans/README.md` index

### Promoting ADR Status

When implementing an ADR:

1. Rename file: `PRO-NNNN-...` → `IMP-NNNN-...`
2. Update `Status:` in file header
3. Update `docs/adr/README.md` index

## Templates

### ADR Template

```markdown
# {PREFIX}-{NUMBER}: Title

**Date:** YYYY-MM-DD
**Status:** Proposed | Accepted | Implemented | Superseded
**Supersedes:** (if applicable)

## Context

Problem or situation requiring decision.

## Decision

What we decided and why.

## Consequences

### Positive
- Benefit 1

### Negative
- Drawback 1

## Related

- [PRO-NNNN](PRO-NNNN-name.md) - Related ADR
- [Plan](../plans/PRO-NNNN-name/) - Implementation plan
```

### Plan Template

Plans must contain enough detail for implementation without external context.

```markdown
# PRO-{NUMBER}: Title

**Status:** Proposed | In Progress | Completed
**Priority:** High | Medium | Low
**Related ADRs:** PRO-NNNN, PRO-NNNN

## Overview

Brief description of what this plan accomplishes and why.

## Goals

1. Goal 1
2. Goal 2

## Tasks

### Task 1: Description

**Goal:** What this task accomplishes

**Current:** (file.go:line if applicable)
```go
// Current implementation
```

**Target:**
```go
// Target implementation
```

**Files to Create/Modify:**
- `path/file.go` (new) - Description
- `path/file.go` (modify) - What changes

**Acceptance Criteria:**
- [ ] Specific, testable criterion
- [ ] Another criterion

---

### Task 2: Description

(Same structure as Task 1)

## Implementation Order

```
1. Task A ──► 2. Task B ──► 3. Task C
                    │
                    └──► 4. Task D (parallel)
```

## PR Sequence

1. **PR 1:** Tasks 1-2 (grouped logically)
2. **PR 2:** Task 3
3. **PR 3:** Tasks 4-5

## Validation Checklist

Before marking this plan complete:

- [ ] All tests pass
- [ ] CHANGELOG updated
- [ ] Deprecated items have godoc notices

## Examples

See [examples/](examples/) for working code (if applicable).

## Related

- [PRO-NNNN](../../adr/PRO-NNNN-name.md) - Related ADR
- [PRO-NNNN](../PRO-NNNN-name/) - Related plan
```

**Key sections for executable plans:**
- **Current/Target code** - Shows exact transformation needed
- **Files to Create/Modify** - Explicit file list with actions
- **Acceptance Criteria per task** - Testable completion criteria
- **Implementation Order** - Dependency graph
- **PR Sequence** - Logical grouping for review

## gopipe vs goengine

| Concern | Location |
|---------|----------|
| Channel operations | `docs/` |
| Pipeline primitives | `docs/` |
| CloudEvents messaging | `docs/goengine/` |
| Broker adapters | `docs/goengine/` |

goengine ADRs and plans are numbered independently starting at 0001.

## Restructuring Documentation

### When to Restructure

- Plans grow beyond single-file readability
- Multiple related components need consolidated view
- Examples need to live next to documentation
- Repository separation creates distinct scopes

### Restructuring Steps

1. **Analyze Current State**
   - List all existing plans and their dependencies
   - Identify which ADRs map to which plans
   - Note any circular dependencies

2. **Design New Structure**
   - Group related components by layer/phase
   - Determine implementation order
   - Plan example code coverage

3. **Migrate Content**
   - Consolidate related plans into layer README
   - Remove redundancy, keep essentials
   - Ensure links and references are current

4. **Create/Update Examples**
   - Create working Go code in `examples/`
   - Add godoc comments explaining the pattern
   - Ensure code compiles

5. **Update Cross-References**
   - Update ADR index with layer mappings
   - Update links in all README files
   - Update CLAUDE.md if procedures change

### Renumbering Plans

When reordering plans to match implementation order:

1. Use temporary names to avoid conflicts: `PRO-NNNN-` → `TMP-NNNN-`
2. Rename to final numbers: `TMP-NNNN-` → `PRO-NNNN-`
3. Update titles in each README.md
4. Update dependency references between plans
5. Update main README.md index tables
6. Use `git add -A` to track renames properly

### Repository Separation

When documentation spans multiple repositories:

1. **Identify Boundaries** - What stays, what moves, what are dependencies
2. **Add Repository Labels** - Mark ADRs/plans by target repository
3. **Create Architecture Diagram** - Show relationship between repos
4. **Document Migration Path** - Help users transition

### Restructuring Validation

After restructuring:

- [ ] All links in README files work
- [ ] ADR index reflects current state
- [ ] Examples compile without errors
- [ ] No orphaned documentation
- [ ] CLAUDE.md references are current
- [ ] Plan numbers match implementation order

## Documentation Lint Procedure

Run via: `make docs-lint`

This procedure simplifies and focuses documentation on gopipe's core purpose and current roadmap.

### Execution Steps

Execute ALL steps in order:

**1. Verify Core Focus**

gopipe's purpose (keep documentation aligned):
- Channel operations (GroupBy, Broadcast, Merge, Filter, Collect)
- Pipeline primitives (Processor, Pipe, Subscriber)
- Generic middleware
- NO CloudEvents (that's goengine)

**2. Review and Reduce**

For each file in `docs/`:

```
□ Remove redundant content (said elsewhere)
□ Remove speculative features (not in current roadmap)
□ Remove verbose explanations (keep concise)
□ Ensure links work
□ Verify file is in correct location (gopipe vs goengine)
```

**3. Validate Structure**

```
□ docs/README.md - Quick reference only, no duplication
□ docs/adr/README.md - Index table matches actual files
□ docs/plans/README.md - Only current roadmap items
□ docs/goengine/ - All messaging content moved here
```

**4. Check Roadmap Alignment**

Current gopipe roadmap (PRO-0001 to PRO-0007):
- PRO-0001: ProcessorConfig
- PRO-0002: Cancel Path
- PRO-0003: Subscriber Interface
- PRO-0004: Middleware Package
- PRO-0005: UUID (Won't Do - moved to goengine)
- PRO-0006: Package Restructuring
- PRO-0007: GroupBy Flexibility

Remove or archive plans not in this list.

**5. Eliminate Duplication**

Check for duplicate content between:
- `docs/README.md` ↔ `docs/procedures/documentation.md`
- `docs/plans/README.md` ↔ individual plan READMEs
- `docs/adr/README.md` ↔ individual ADRs

Keep authoritative source, remove copies.

**6. Output Report**

After running, output:
- Files modified
- Files archived/removed
- Broken links found
- Remaining issues

### Acceptance Criteria

- [ ] No file > 200 lines (split if larger)
- [ ] No duplicate content between files
- [ ] All links valid
- [ ] ADR index matches files
- [ ] Plans index matches folders
- [ ] goengine content not in gopipe docs/

## Maintenance Checklist

Weekly:
- [ ] Review open plans for status updates
- [ ] Verify ADR README indexes are current

Before release:
- [ ] Move `[Unreleased]` to version section in CHANGELOG
- [ ] Promote implemented ADRs (PRO- → IMP-)
- [ ] Update manual with new features

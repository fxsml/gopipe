# Planning Procedures

## Plan File Structure

Implementation plans live in `docs/plans/`:

```
docs/plans/
├── README.md                    # Plan index
├── feature-name.md              # Active plans (descriptive names)
└── archive/                     # Completed/historical plans
    ├── 0001-message-engine.md
    ├── 0002-marshaler.md
    └── ...
```

Active plans use descriptive names. Completed plans are moved to `archive/` with sequential numbering.

## Plan Document Structure

```markdown
# Plan NNNN: Title

**Status:** Proposed | In Progress | Complete
**Related ADRs:** [NNNN](../adr/NNNN-name.md)
**Depends On:** [Plan NNNN](NNNN-name.md) (optional)

## Overview

Brief description of what this plan accomplishes.

## Goals

1. Goal 1
2. Goal 2

## Tasks

### Task 1: Description

**Goal:** What this task accomplishes

**Implementation:**
\`\`\`go
// Code example
\`\`\`

**Files to Create/Modify:**
- `path/file.go` - Description

**Acceptance Criteria:**
- [ ] Criterion 1
- [ ] Criterion 2

## Implementation Order

Diagram or list showing task dependencies.

## Acceptance Criteria

- [ ] All tasks completed
- [ ] Tests pass
- [ ] CHANGELOG updated
```

## Creating a New Plan

1. Determine next number from `docs/plans/README.md` index
2. Create file: `docs/plans/NNNN-short-title.md`
3. Use template above
4. Update `docs/plans/README.md` index
5. Link related ADRs (update ADR Links section too)

## Plan States

| Status | Meaning |
|--------|---------|
| Proposed | Documented, not started |
| In Progress | Work underway |
| Complete | Fully implemented |

## Hierarchical Planning

Large initiatives should be divided into phases using dependencies:

```
0001-foundation.md
0002-feature-a.md (Depends On: 0001)
0003-feature-b.md (Depends On: 0001)
0004-integration.md (Depends On: 0002, 0003)
```

Each plan should be independently implementable after its dependencies.

## Design Evolution Documents

For plans with significant design decisions or options analysis, create companion `.decisions.md` files:

```
docs/plans/archive/
├── NNNN-plan-title.md                  # Main plan (required)
├── NNNN-decision-topic.decisions.md    # Related decision (optional)
```

**Convention:**
- Main plan and decisions files share the same number prefix
- Decisions file name describes the specific topic (may differ from plan title)
- One plan can have multiple decisions files

**Example:**
```
0006-engine-raw-api-simplification.md
0006-config-convention.decisions.md

0008-message-package-release-review.md
0008-marshal-unmarshal-pipes.decisions.md
0008-handler-naming-consistency.decisions.md
```

### Design Evolution Structure

```markdown
# Plan NNNN: Title - Design Evolution

**Status:** Resolved | Open | Superseded
**Related Plan:** [NNNN-title.md](NNNN-title.md)

## Context

Brief description of what design questions were being explored.

## Decision

Summary of what was decided and why (if Resolved).

---

## Options Considered

### Option A: Name
**Pros:** ...
**Cons:** ...

### Option B: Name
...

## Implementation Notes

Any implementation-specific details discovered during development.
```

### When to Create Design Evolution Documents

- Multiple viable approaches were evaluated
- Significant trade-offs needed documentation
- Rejected alternatives should be preserved for future reference
- Design evolved significantly during implementation

Link from main plan using: `**Design Evolution:** [NNNN-title.decisions.md](NNNN-title.decisions.md)`

## Related Documentation

- [Documentation Procedures](documentation.md) - Templates
- [Plans Index](../plans/README.md) - All plans
- [ADRs](../adr/README.md) - Architecture decisions

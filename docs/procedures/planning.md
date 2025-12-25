# Planning Procedures

## Plan File Structure

Implementation plans live in `docs/plans/` as numbered files (matching ADR convention):

```
docs/plans/
├── README.md                    # Plan index
├── 0001-message-engine.md       # Plan document
├── 0002-marshaler.md
└── ...
```

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

## Related Documentation

- [Documentation Procedures](documentation.md) - Templates
- [Plans Index](../plans/README.md) - All plans
- [ADRs](../adr/README.md) - Architecture decisions

# Planning Procedures

## Plan Folder Structure

Implementation plans live in `docs/plans/` as numbered folders:

```
docs/plans/
├── README.md                          # Plan index
├── architecture-roadmap.md            # Master plan overview
├── PRO-0001-cancel-path-refactoring/
│   ├── README.md                      # Main plan document
│   └── main.go                        # Optional example code
├── PRO-0002-middleware-refactoring/
└── ...
```

## Plan Document Structure

```markdown
# PRO-{NUMBER}: Title

**Status:** Proposed | In Progress | Complete
**Priority:** High | Medium | Low
**Related ADRs:** PRO-NNNN

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

1. Determine next number in `docs/plans/`
2. Create folder: `docs/plans/PRO-NNNN-short-title/`
3. Create `README.md` using template above
4. Add example code if helpful
5. Update `docs/plans/README.md` index

## Hierarchical Planning

Large initiatives should be divided into phases:

```
architecture-roadmap.md (Master Overview)
├── PRO-0001-* (Foundation tasks)
├── PRO-0002-* (Depends on 0001)
├── PRO-0003-* (Depends on 0002)
└── ...
```

Each plan should be independently implementable after its dependencies.

## Plan States

| Status | Meaning |
|--------|---------|
| Proposed | Documented, not started |
| In Progress | Work underway |
| Complete | Fully implemented |

## Related Documentation

- [Documentation Procedures](documentation.md) - Templates
- [Plans Index](../plans/README.md) - All plans
- [ADRs](../adr/README.md) - Architecture decisions

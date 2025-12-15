# Planning Procedures

## Plan File Structure

Implementation plans live in `docs/plans/`:

```
docs/plans/
├── README.md                  # Plan index
├── architecture-roadmap.md    # Master plan (big picture)
├── layer-0-*.md               # Hierarchical sub-plans
├── layer-1-*.md
└── <plan>.prompt.md           # Optimized prompts for each plan
```

## Plan Document Structure

```markdown
# <Plan Title>

**Status:** Proposed | In Progress | Complete
**Depends On:** <prerequisite plans>
**Related ADRs:** <ADR numbers>

## Overview

Brief description of what this plan accomplishes.

## Goals

1. Goal 1
2. Goal 2

## Sub-Tasks

### Task N.1: <Name>

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

## Validation Checklist

- [ ] All acceptance criteria met
- [ ] Tests pass
- [ ] CHANGELOG updated
```

## Prompt Files Convention

**For every plan file, create a corresponding `.prompt.md` file.**

### Purpose

The `.prompt.md` file is an optimized, summarized prompt for executing the plan. It answers: *"What info would I need to fulfill this plan?"*

### Location

```
docs/plans/layer-1-message-standardization.md        # Plan
docs/plans/layer-1-message-standardization.prompt.md # Prompt
```

### Prompt File Structure

```markdown
# Prompt: <Plan Title>

## Context
<Brief context needed to understand the task>

## Current State
<Relevant current implementation details with file paths>

## Target State
<Desired outcome with code examples>

## Tasks
1. <Task 1> - see [plan section](#task-n1-name)
2. <Task 2>

## Key Files
- `path/file.go` - <what to change>

## Constraints
- <Important constraints or rules>

## References
- [Full Plan](plan-file.md)
- [ADR NNNN](../adr/NNNN.md)
```

### Guidelines

1. **Concise** - Only essential information
2. **Self-contained** - Executable without reading full plan
3. **Links for depth** - Reference plan/ADRs for details
4. **Code examples** - Show before/after when helpful
5. **File paths** - Always include relevant file paths

### Example

```markdown
# Prompt: Layer 0 - ProcessorConfig

## Context
Replace generic `Option[In, Out]` with struct-based config.

## Current State
`processor.go:59-90` - Functional options with generic params

## Target State
\`\`\`go
type ProcessorConfig struct {
    Concurrency int
    Buffer      int
    Timeout     time.Duration
}
\`\`\`

## Tasks
1. Create ProcessorConfig struct
2. Update StartProcessor to accept config
3. Add deprecated wrappers

## Key Files
- `processor.go` - Config struct, StartProcessor
- `pipe.go` - Pipe constructors
- `deprecated.go` - Backward compat (new file)

## References
- [Full Plan](layer-0-foundation-cleanup.md#task-01-processorconfig-struct)
- [ADR 0026](../adr/0026-pipe-processor-simplification.md)
```

## Hierarchical Planning

Large plans should be divided into layers:

```
architecture-roadmap.md (Master)
├── layer-0-*.md (Foundation)
├── layer-1-*.md (Depends on 0)
├── layer-2-*.md (Depends on 1)
└── layer-3-*.md (Depends on 2)
```

Each layer should be independently implementable after its dependencies.

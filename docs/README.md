# gopipe Documentation

Documentation for the gopipe project. Organized by concern with clear separation between gopipe (core) and goengine (messaging).

## Documentation Structure

```
docs/
├── README.md            # This file - documentation conventions
├── adr/                 # gopipe ADRs (IMP-, ACC-, PRO-, SUP-)
├── analysis/            # Historical analysis and comparisons
├── manual/              # User manual and guides
├── patterns/            # CQRS, Saga, Outbox patterns
├── plans/               # Roadmap and implementation plans
├── procedures/          # Development procedures
│
└── goengine/            # goengine documentation (future separate repo)
    ├── adr/             # goengine ADRs (PRO-0001+)
    ├── plans/           # goengine implementation plans
    └── README.md        # goengine overview
```

## Repository Separation

| Repository | Focus | Documentation |
|------------|-------|---------------|
| **gopipe** | Channel operations, pipeline primitives | `docs/` |
| **goengine** | CloudEvents messaging, broker adapters | `docs/goengine/` → separate repo |

goengine will be a **separate repository** (`fxsml/goengine`). See [goengine plans](goengine/plans/) for migration details.

## Naming Conventions

### ADR Files

| Prefix | Status | Meaning |
|--------|--------|---------|
| `IMP-` | Implemented | Fully in codebase |
| `ACC-` | Accepted | Decision made |
| `PRO-` | Proposed | Under consideration |
| `SUP-` | Superseded | Replaced |

Format: `{PREFIX}-{NUMBER}-{short-title}.md`

Examples:
- `IMP-0001-public-message-fields.md`
- `PRO-0026-processor-simplification.md`

### Plan Folders

Plans are numbered folders:

```
plans/
├── PRO-0001-cancel-path-refactoring/
│   ├── README.md          # Main plan document
│   └── main.go            # Optional example code
├── PRO-0002-middleware-refactoring/
└── ...
```

Format: `PRO-{NUMBER}-{short-title}/`

## Templates

### ADR Template

```markdown
# {PREFIX}-{NUMBER}: Title

**Date:** YYYY-MM-DD
**Status:** Proposed | Accepted | Implemented | Superseded

## Context
What is the issue?

## Decision
What did we decide?

## Consequences
### Positive
- Benefit 1

### Negative
- Drawback 1
```

### Plan Template

```markdown
# PRO-{NUMBER}: Title

**Status:** Proposed | In Progress | Completed
**Priority:** High | Medium | Low
**Related ADRs:** PRO-NNNN, PRO-NNNN

## Overview
Brief description.

## Goals
1. Goal 1
2. Goal 2

## Tasks
### Task 1: Description
Details...

## Acceptance Criteria
- [ ] Criterion 1
- [ ] Criterion 2
```

## Quick Reference

| I want to... | Look in... |
|-------------|------------|
| Understand a decision | `adr/` |
| Learn how to use gopipe | `manual/` |
| See planned work & roadmap | `plans/` |
| Understand a pattern | `patterns/` |
| Follow dev procedures | `procedures/` |
| See historical analysis | `analysis/` |

## Maintenance Procedures

See [procedures/documentation.md](procedures/documentation.md) for detailed maintenance instructions.

### Quick Maintenance

**Adding an ADR:**
1. Create `adr/PRO-NNNN-title.md`
2. Update `adr/README.md`

**Adding a Plan:**
1. Create `plans/PRO-NNNN-title/README.md`
2. Update `plans/README.md`

**Promoting ADR:**
1. Rename `PRO-` → `IMP-`
2. Update status in file
3. Update `adr/README.md`

## Related

- [Procedures](procedures/) - Development workflow
- [Plans](plans/) - Implementation plans and roadmap
- [gopipe README](../README.md) - Project overview

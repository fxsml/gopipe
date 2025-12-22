# Documentation Procedures

Procedures for maintaining gopipe documentation.

## Directory Structure

```
docs/
├── adr/                 # Architecture Decision Records
├── manual/              # User manual and guides
├── plans/               # Implementation plans
├── procedures/          # Development procedures (this)
└── README.md            # Documentation overview
```

## Before Pushing Code

- [ ] CHANGELOG.md updated under `[Unreleased]`
- [ ] ADR created for architectural decisions (see [adr.md](adr.md))
- [ ] Godoc for public APIs
- [ ] README.md for core component changes

## Plan Documentation

Plans are numbered folders with README.md:

Format: `docs/plans/NNNN-short-title/README.md`

### Plan Template

```markdown
# Plan NNNN: Title

**Status:** Proposed | In Progress | Completed

## Overview

Brief description.

## Tasks

### Task 1: Description

**Files:** `path/file.go`

**Changes:**
- What changes

**Acceptance:**
- [ ] Testable criterion

## Implementation Order

1. Task A → 2. Task B → 3. Task C
```

## Maintenance

Weekly:
- [ ] Review open plans for status updates
- [ ] Verify ADR index is current

Before release:
- [ ] Move `[Unreleased]` to version section in CHANGELOG
- [ ] Update ADR statuses (Accepted → Implemented)

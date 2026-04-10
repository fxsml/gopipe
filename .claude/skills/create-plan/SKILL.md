---
name: create-plan
description: Create a new implementation plan document. Usage: /create-plan TITLE (e.g. /create-plan redis-adapter)
allowed-tools:
  - Bash
  - Read
  - Write
  - Edit
  - Glob
---

# Create Plan

Create an implementation plan for: `$ARGUMENTS`

Follow the procedure: @../docs/procedures/planning.md

## Steps

1. Read `docs/plans/README.md` to determine the next number (look at archive/ for highest used)
2. Create `docs/plans/<descriptive-name>.md` (active plans use descriptive names, not numbers)
3. Fill in the plan template:
   - `Status: Proposed`
   - `Overview:` brief description
   - `Goals:` list of goals
   - `Tasks:` one task section per major work item
   - `Implementation Order:` dependency diagram or list
   - `Acceptance Criteria:` completion checklist
4. Update `docs/plans/README.md` index with the new plan under the current initiatives table
5. If related ADRs exist, link them (and update the ADR's Links section)
6. Report the file path created

Use the title from arguments to derive the descriptive filename (lowercase, hyphens).

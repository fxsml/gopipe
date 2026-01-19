# Command: create-plan

Create a new implementation plan document.

## Procedure Reference

This command implements plan creation from `docs/procedures/planning.md`.

## Usage

```
/create-plan short-title
```

## Required Arguments

- `$ARGUMENTS`: Short title for the plan (e.g., `batch-processing`, `graceful-shutdown`)

## Execution

### Step 1: Determine Plan Number

1. List existing plans:
   ```bash
   ls docs/plans/*.md | grep -E '[0-9]{4}-' | sort -r | head -1
   ```

2. Extract highest number and increment by 1.

3. Format as 4-digit number (e.g., `0014`).

### Step 2: Generate Filename

Format: `docs/plans/NNNN-[title].md`

Example: `docs/plans/0014-batch-processing.md`

### Step 3: Create Plan File

Use template:

```markdown
# Plan NNNN: [Title in Title Case]

**Status:** Draft
**Created:** [today's date]
**Author:** [user or Claude]

## Overview

[Brief description of what this plan covers]

## Goals

1. [Primary goal]
2. [Secondary goal]

## Non-Goals

- [What this plan explicitly does not cover]

## Background

[Context and motivation for this work]

## Design

### [Component/Feature 1]

[Description and approach]

### [Component/Feature 2]

[Description and approach]

## Implementation Steps

1. [ ] [Step 1]
2. [ ] [Step 2]
3. [ ] [Step 3]

## Testing Strategy

[How will this be tested?]

## Migration/Rollout

[How will this be deployed/released?]

## Open Questions

- [ ] [Question 1]
- [ ] [Question 2]

## Related

- ADR: [link if applicable]
- Issue: [link if applicable]
```

### Step 4: Update Plans Index

Add entry to `docs/plans/README.md`:

```markdown
| NNNN | [Title] | Draft | [today's date] |
```

### Step 5: Gather Initial Content

Ask user for:
1. Overview: "What is this plan about?"
2. Goals: "What are the main goals?"
3. Related ADRs or issues

Fill in the template with user's answers.

## Completion Summary

Report:
- Created file: `docs/plans/NNNN-[title].md`
- Updated: `docs/plans/README.md`
- Next steps: Flesh out design sections, move to "In Progress" when implementing

## Error Handling

- If title contains invalid characters, sanitize
- If docs/plans/ doesn't exist, create it
- If README.md doesn't exist, create with header

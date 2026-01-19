# Command: create-adr

Create a new Architecture Decision Record.

## Procedure Reference

This command implements ADR creation from `docs/procedures/adr.md`.

## Usage

```
/create-adr short-title
```

## Required Arguments

- `$ARGUMENTS`: Short title for the ADR (e.g., `handler-configuration`, `single-merger-architecture`)

## Execution

### Step 1: Determine Next ADR Number

1. List existing ADRs:
   ```bash
   ls docs/adr/*.md | grep -E '[0-9]{4}-' | sort -r | head -1
   ```

2. Extract highest number and increment by 1.

3. Format as 4-digit number (e.g., `0019`).

### Step 2: Generate Filename

Format: `docs/adr/NNNN-[title].md`

Example: `docs/adr/0019-handler-configuration.md`

### Step 3: Create ADR File

Use template:

```markdown
# ADR NNNN: [Title in Title Case]

**Date:** [today's date]
**Status:** Proposed

## Context

[Describe the context and problem. What forces are at play?]

## Decision

[Describe the decision and rationale.]

## Consequences

**Breaking Changes:**
- None / [list any breaking changes]

**Benefits:**
- [list benefits]

**Drawbacks:**
- [list drawbacks]

## Alternatives Considered

### [Alternative 1]
[Description and why rejected]

## Links

- Related: [links to related ADRs, issues, or docs]
```

### Step 4: Update ADR Index

Add entry to `docs/adr/README.md`:

```markdown
| NNNN | [Title] | Proposed | [today's date] |
```

### Step 5: Gather Content

Ask user for:
1. Context: "What problem are you solving?"
2. Decision: "What did you decide?"
3. Consequences: "What are the benefits and drawbacks?"

Fill in the template with user's answers.

## Completion Summary

Report:
- Created file: `docs/adr/NNNN-[title].md`
- Updated: `docs/adr/README.md`
- Next steps: Fill in remaining sections, change status to "Accepted" when approved

## Error Handling

- If title contains invalid characters, sanitize (lowercase, hyphens only)
- If docs/adr/ doesn't exist, create it
- If README.md doesn't exist, create with header

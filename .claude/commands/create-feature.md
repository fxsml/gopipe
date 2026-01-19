# Command: create-feature

Create a new feature branch following git flow conventions.

## Procedure Reference

This command implements branch creation from `docs/procedures/git.md`.

## Usage

```
/create-feature feature-name
```

Or from GitHub issue:
```
/create-feature #123
```

## Required Arguments

- `$ARGUMENTS`: Feature name (e.g., `add-batch-processing`) or GitHub issue number (e.g., `#123`)

## Pre-Checks

1. Working directory is clean: `git status`
2. If issue number provided, verify issue exists: `gh issue view [number]`

## Execution

### Step 1: Prepare Branch Name

1. If argument is issue number (#NNN):
   - Fetch issue title: `gh issue view NNN --json title`
   - Generate branch name from title:
     - Convert to lowercase
     - Replace spaces with hyphens
     - Remove special characters
     - Truncate to reasonable length (50 chars max)

2. If argument is a name, use it directly.

3. Final branch name format: `feature/[name]`

### Step 2: Create Branch

1. Ensure on develop and up to date:
   ```bash
   git checkout develop
   git pull origin develop
   ```

2. Create and switch to feature branch:
   ```bash
   git checkout -b feature/[name]
   ```

3. Push branch to remote with tracking:
   ```bash
   git push -u origin feature/[name]
   ```

### Step 3: Link to Issue (if applicable)

If created from issue number:
```bash
gh issue develop NNN --name feature/[name]
```

## Completion Summary

Report:
- Branch name created: `feature/[name]`
- Linked issue (if any)
- Next steps:
  1. Make your changes
  2. Commit following conventional commits (`feat:`, `fix:`, etc.)
  3. When ready, use `/release-feature feature/[name]`

## Error Handling

- If branch already exists, suggest a different name or ask to checkout existing
- If issue doesn't exist, abort with message
- If not on develop, offer to switch

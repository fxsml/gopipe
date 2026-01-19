# Command: review-pr

Review a pull request against project standards.

## Procedure References

Combines checks from:
- `docs/procedures/go.md` - Code standards
- `docs/procedures/git.md` - Commit conventions
- `docs/procedures/documentation.md` - Documentation requirements
- `AGENTS.md` - Common mistakes to avoid

## Usage

Review specific PR:
```
/review-pr 123
```

Review current branch's PR:
```
/review-pr
```

## Required Arguments

- `$ARGUMENTS`: PR number (optional, defaults to current branch's PR)

## Execution

### Step 1: Get PR Information

```bash
gh pr view [number] --json title,body,commits,files,reviews,checks
```

If no number provided:
```bash
gh pr view --json title,body,commits,files,reviews,checks
```

### Step 2: Commit Message Review

Check each commit message:

1. **Format**: Must follow conventional commits
   - Pattern: `type(scope): description`
   - Valid types: feat, fix, refactor, docs, test, chore

2. **Scope**: Should match changed packages
   - If changing `message/`, scope should be `message`
   - If changing multiple, can omit scope or use most relevant

3. **Description**: Should be lowercase, imperative mood

Report:
```
Commit Messages:
✓ feat(message): add graceful shutdown
✗ fixed bug - not conventional format
```

### Step 3: Code Review (Common Mistakes)

Check changed files for AGENTS.md common mistakes:

1. **channel.Process for filtering**: Should use Filter instead
2. **Component creation in Start()**: Should create upfront
3. **N goroutines for loopback**: Should combine matchers
4. **Handler.Name() method**: Name should be parameter
5. **Copy() sharing Attributes**: Must clone map

For each file, scan for these patterns and report.

### Step 4: Documentation Review

1. **CHANGELOG.md**: Updated for features/fixes?
2. **Godoc**: New public APIs documented?
3. **ADR**: Architectural changes have decision record?

Report:
```
Documentation:
✓ CHANGELOG.md updated
? New public API - check godoc
✗ No ADR for architectural change
```

### Step 5: Test Review

1. **Tests added**: New features have tests?
2. **Tests pass**: CI checks green?

```bash
gh pr checks [number]
```

### Step 6: Files Changed Summary

List changed files by package:
```
Files Changed:
  channel/ (2 files)
    - filter.go
    - filter_test.go
  message/ (4 files)
    - engine.go
    - engine_test.go
    - handler.go
    - handler_test.go
```

## Output Report

```
PR Review: #123 - feat(message): add graceful shutdown
============================================================

Commit Messages:
✓ All 3 commits follow conventional format
  - feat(message): add graceful shutdown
  - test(message): add shutdown tests
  - docs: update CHANGELOG

Code Review:
✓ No common mistakes found
  Checked: channel.Process misuse, Start() creation, loopback patterns

Documentation:
✓ CHANGELOG.md updated
✓ Godoc present for new APIs

Tests:
✓ Tests added for new functionality
✓ CI checks passing

Files Changed: 6 files in 2 packages

Overall: APPROVED / NEEDS CHANGES / NEEDS REVIEW
```

## Suggestions

After review, offer actionable suggestions:

- If commits need fixing: "Consider squashing WIP commits"
- If docs missing: "Add godoc for NewFunction"
- If tests missing: "Add test for error case X"

## Error Handling

- If PR doesn't exist, report error
- If not on a branch with PR, prompt for PR number
- If CI still running, note "CI in progress" in test section

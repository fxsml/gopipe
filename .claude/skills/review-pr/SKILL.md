---
name: review-pr
description: Review a pull request against gopipe project standards. Usage: /review-pr [NUMBER] (omit for current branch PR)
allowed-tools:
  - Bash
  - Read
  - Glob
  - Grep
---

# Review PR

Review PR `$ARGUMENTS` (or the current branch PR if no number given) against gopipe standards.

## Steps

1. Fetch PR details:
   ```bash
   gh pr view $ARGUMENTS
   gh pr diff $ARGUMENTS
   gh pr checks $ARGUMENTS
   ```

2. Review against each checklist item below and report pass/fail with specific line references

## Review Checklist

### Code Quality
- [ ] No use of `channel.Process` for filtering (use `channel.Filter`)
- [ ] Components created in constructor, not in `Start()`
- [ ] Handler does not own its name (name is passed to `AddHandler`)
- [ ] `Copy()` clones Attributes map (`maps.Clone`)
- [ ] Matcher operates on `Attributes`, not `*Message`
- [ ] Errors returned, not panicked (except `Must*` functions)
- [ ] Errors wrapped with context: `fmt.Errorf("op: %w", err)`

### Tests
- [ ] Table-driven tests for multiple cases
- [ ] Both success and error paths tested
- [ ] `t.Parallel()` used where safe
- [ ] New public APIs have tests

### Documentation
- [ ] Public APIs have godoc (first sentence starts with function name)
- [ ] CHANGELOG.md updated under `[Unreleased]`
- [ ] ADR created for architectural decisions (API changes, new interfaces)
- [ ] No breaking changes without ADR

### Git
- [ ] Conventional commit messages (`feat:`, `fix:`, `docs:`, etc.)
- [ ] No merge commits in feature branch history
- [ ] PR targets `develop`, not `main`

## Output Format

Report as:
```
PR #N Review: <title>

PASS ✓ / FAIL ✗ / N/A for each checklist item

Issues found:
- <specific issue with file:line reference>

Overall: APPROVED / CHANGES REQUESTED
```

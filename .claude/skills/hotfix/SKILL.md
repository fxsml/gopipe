---
name: hotfix
description: Create and release a hotfix branch. Usage: /hotfix NAME (e.g. /hotfix fix-nil-panic)
disable-model-invocation: true
allowed-tools:
  - Bash
  - Read
  - Edit
  - Glob
  - Grep
---

# Hotfix

Create and release a hotfix for: `$ARGUMENTS`

Read the full procedure before starting: @../docs/procedures/release.md

## Steps

**Create hotfix branch:**
1. `git checkout main && git pull origin main`
2. `git checkout -b hotfix/$ARGUMENTS`

**Implement fix:**
3. Explore the issue, implement the minimal fix
4. Add tests if applicable
5. Update CHANGELOG with new version section
6. `make check` must pass

**Commit and PR:**
7. Commit with `fix: description`
8. `git push -u origin hotfix/$ARGUMENTS`
9. `gh pr create --base main --title "fix: description"`

**Release:**
10. **PAUSE: ask for approval before merge**
11. `gh pr merge --merge`
12. `git checkout main && git pull origin main`
13. **PAUSE: ask for approval before creating tags**
14. Determine next patch version (`make version`)
15. Create and push module tags in order: channel → pipe → message
16. **PAUSE: ask for approval before pushing tags**
17. Create GitHub release
18. Merge main back to develop

## Rules

- Hotfixes branch from main, not develop
- Fix only the reported issue — no scope creep
- Never skip an approval gate
- Resolve CHANGELOG conflicts when merging main to develop (keep [Unreleased] at top)

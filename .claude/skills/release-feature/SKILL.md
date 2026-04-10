---
name: release-feature
description: Merge a feature branch to develop following the git flow procedure (history cleanup, PR, verify).
disable-model-invocation: true
allowed-tools:
  - Bash
  - Read
  - Glob
  - Grep
---

# Release Feature

Execute Phases 1–3 of the Feature Branch Release Procedure for branch: `$ARGUMENTS`

Read the full procedure before starting: @../docs/procedures/feature-release.md

## Steps

**Phase 1 — History cleanup:**
1. Review commit history since branching from develop
2. Plan logical, atomic commit structure
3. **PAUSE: ask for approval before rebase**
4. Interactive rebase to clean history
5. Verify `make check` passes after rebase
6. **PAUSE: ask for approval before force push**
7. `git push --force-with-lease origin <branch>`

**Phase 2 — Merge to develop:**
1. Rebase onto latest develop
2. Create PR: `gh pr create --base develop ...`
3. Show PR checks and diff summary
4. **PAUSE: ask for approval before merge**
5. `gh pr merge --merge --delete-branch`

**Phase 3 — Verify develop:**
1. `git checkout develop && git pull origin develop`
2. `make check`
3. Report success

## Rules

- Never skip an approval gate
- Show diffs before merges (`gh pr diff`)
- Show current history before proposing rebase plan
- Use conventional commit format in rebase

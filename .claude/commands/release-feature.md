# Command: release-feature

Merge a feature branch to develop with proper history cleanup following git flow.

## Procedure Reference

This command implements Phases 1-3 of `docs/procedures/feature-release.md`.

## Usage

```
/release-feature feature/branch-name
```

## Required Arguments

- `$ARGUMENTS`: Feature branch name (e.g., `feature/message-refactoring`)

## Approval Gates

**CRITICAL: You MUST pause and ask for explicit approval before:**
1. Interactive rebase or history rewrite
2. Force push to the feature branch
3. Merge to develop

Show the current state and planned action, then wait for "yes" or "approved".
Valid responses: "yes"/"approved" = proceed, "skip" = skip this step, "abort" = stop entirely.

## Pre-Checks

Before starting, verify:
1. The feature branch exists: `git fetch origin && git branch -a | grep $ARGUMENTS`
2. No uncommitted changes: `git status`
3. Tests pass on feature branch: `make check`

## Execution

### Phase 1: History Review and Cleanup

1. Switch to feature branch and fetch latest:
   ```bash
   git checkout $ARGUMENTS
   git fetch origin develop
   ```

2. Review commit history:
   ```bash
   git log --oneline develop..HEAD
   git rev-list --count develop..HEAD
   ```

3. Present commit structure analysis to user. Suggest squash/reword plan following conventional commits:
   - `feat:` for new features
   - `fix:` for bug fixes
   - `refactor:` for code restructuring
   - `docs:` for documentation
   - `test:` for tests

4. **[APPROVAL GATE]** Before rebase, show plan and ask for approval.

5. Perform interactive rebase:
   ```bash
   MERGE_BASE=$(git merge-base develop HEAD)
   git rebase -i $MERGE_BASE
   ```

6. Verify rebase result:
   ```bash
   make check
   ```

7. **[APPROVAL GATE]** Before force push, show new history and ask for approval.

8. Force push:
   ```bash
   git push --force-with-lease origin $ARGUMENTS
   ```

### Phase 2: Merge to Develop

1. Rebase on latest develop:
   ```bash
   git fetch origin develop
   git rebase origin/develop
   git push --force-with-lease origin $ARGUMENTS
   ```

2. Create PR to develop:
   ```bash
   gh pr create --base develop --title "feat: [extracted from commits]" --body "[generated summary]"
   ```

3. Wait for CI checks: `gh pr checks`

4. **[APPROVAL GATE]** Before merge, show PR status and ask for approval.

5. Merge PR:
   ```bash
   gh pr merge --merge --delete-branch
   ```

### Phase 3: Verify on Develop

1. Update local develop:
   ```bash
   git checkout develop
   git pull origin develop
   ```

2. Run verification:
   ```bash
   make check
   ```

## Completion Summary

Report:
- Commits merged (before/after count)
- PR URL
- Verification status on develop

## Error Handling

- If rebase conflicts occur, help user resolve them
- If tests fail after rebase, stop and report
- If CI checks fail, wait for user to fix before merge

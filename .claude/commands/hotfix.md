# Command: hotfix

Create and release a hotfix from main.

## Procedure Reference

This command implements the Hotfix Release Process from `docs/procedures/release.md`.

## Usage

```
/hotfix fix-name
```

Or with explicit version:
```
/hotfix fix-name v0.13.3
```

## Required Arguments

- `$ARGUMENTS`: Hotfix name (e.g., `fix-nil-pointer`) and optionally version

## Approval Gates

**CRITICAL: You MUST pause and ask for explicit approval before:**
1. Creating PR to main
2. Merging to main
3. Creating and pushing tags
4. Creating GitHub release
5. Merging main back to develop

Show the current state and planned action, then wait for "yes" or "approved".
Valid responses: "yes"/"approved" = proceed, "skip" = skip this step, "abort" = stop entirely.

## Pre-Checks

1. Working directory is clean: `git status`
2. On main or can switch to main

## Execution

### Step 1: Create Hotfix Branch

1. Checkout main and create branch:
   ```bash
   git checkout main
   git pull origin main
   git checkout -b hotfix/[name from $ARGUMENTS]
   ```

2. Inform user: "Hotfix branch created. Make your fixes, then say 'ready' when done."

3. Wait for user to make fixes and confirm ready.

### Step 2: Verify and Prepare

1. Verify fixes:
   ```bash
   make check
   ```

2. Determine version (increment patch from current if not provided):
   ```bash
   git describe --tags --abbrev=0
   ```
   Example: if current is `v0.13.2`, next hotfix is `v0.13.3`

3. Update CHANGELOG.md with new version section and fix description.

4. Commit changes:
   ```bash
   git add .
   git commit -m "fix: [description from $ARGUMENTS]"
   git push -u origin hotfix/[name]
   ```

### Step 3: Create and Merge PR

1. **[APPROVAL GATE]** Before creating PR to main.

2. Create PR:
   ```bash
   gh pr create --base main --title "fix: [description]" --body "[hotfix details]"
   ```

3. Wait for CI: `gh pr checks`

4. **[APPROVAL GATE]** Before merge to main.

5. Merge:
   ```bash
   gh pr merge --merge --delete-branch
   ```

### Step 4: Tag and Release

1. Update local main:
   ```bash
   git checkout main
   git pull origin main
   ```

2. **[APPROVAL GATE]** Before creating tags.

3. Create and push module tags (same as release command):
   ```bash
   git tag channel/$VERSION
   git tag pipe/$VERSION
   git tag message/$VERSION
   git push origin channel/$VERSION pipe/$VERSION message/$VERSION
   git tag $VERSION
   git push origin $VERSION
   ```

4. Create GitHub release:
   ```bash
   gh release create $VERSION --title "$VERSION" --notes "[from CHANGELOG]"
   ```

### Step 5: Sync to Develop

1. **[APPROVAL GATE]** Before merging main to develop.

2. Merge main to develop:
   ```bash
   git checkout develop
   git merge main
   ```

3. If CHANGELOG conflicts, resolve by keeping both version sections.

4. Push develop:
   ```bash
   git push origin develop
   ```

## Completion Summary

Report:
- Hotfix branch name
- Version released
- Tags created
- GitHub release URL
- Develop sync status

## Error Handling

- If tests fail, stop and help user fix
- If CHANGELOG conflicts on develop merge, guide user through resolution
- If tag already exists, suggest incrementing version

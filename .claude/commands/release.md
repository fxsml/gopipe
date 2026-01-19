# Command: release

Release develop to main with multi-module tagging.

## Procedure Reference

This command implements Phases 4-6 of `docs/procedures/feature-release.md` and multi-module tagging from `docs/procedures/release.md`.

## Usage

```
/release v0.14.0
```

## Required Arguments

- `$ARGUMENTS`: Version number in semver format (e.g., `v0.14.0`)

## Approval Gates

**CRITICAL: You MUST pause and ask for explicit approval before:**
1. Merge to main
2. Creating module tags
3. Pushing tags to remote
4. Creating GitHub release

Show the current state and planned action, then wait for "yes" or "approved".
Valid responses: "yes"/"approved" = proceed, "skip" = skip this step, "abort" = stop entirely.

## Pre-Checks

1. Version format is valid: `vX.Y.Z`
2. Currently on develop branch
3. Working directory is clean: `git status`
4. Tests pass: `make check`
5. CHANGELOG.md has `[Unreleased]` section with content

## Execution

### Phase 4: Prepare and Merge to Main

1. Verify CHANGELOG.md has unreleased content to release.

2. Update CHANGELOG: Move `[Unreleased]` entries to `[$ARGUMENTS]` section with today's date.

3. Commit CHANGELOG update:
   ```bash
   git add CHANGELOG.md
   git commit -m "docs: prepare release $ARGUMENTS"
   git push origin develop
   ```

4. Create release PR:
   ```bash
   gh pr create --base main --head develop --title "release: $ARGUMENTS" --body "[changelog excerpt]"
   ```

5. Wait for CI checks: `gh pr checks`

6. **[APPROVAL GATE]** Before merge to main, show PR status.

7. Merge to main:
   ```bash
   gh pr merge --merge
   ```

### Phase 5: Tag and Release

1. Update local main:
   ```bash
   git checkout main
   git pull origin main
   ```

2. **[APPROVAL GATE]** Before creating tags, show what will be tagged.

3. Create module tags in dependency order:
   ```bash
   git tag channel/$ARGUMENTS
   git tag pipe/$ARGUMENTS
   git tag message/$ARGUMENTS
   ```

4. **[APPROVAL GATE]** Before pushing tags.

5. Push tags:
   ```bash
   git push origin channel/$ARGUMENTS pipe/$ARGUMENTS message/$ARGUMENTS
   ```

6. Create root version tag:
   ```bash
   git tag $ARGUMENTS
   git push origin $ARGUMENTS
   ```

7. Create GitHub release:
   ```bash
   gh release create $ARGUMENTS --title "$ARGUMENTS" --notes "[from CHANGELOG]"
   ```

### Phase 6: Post-Release

1. Merge main back to develop:
   ```bash
   git checkout develop
   git merge main
   git push origin develop
   ```

2. Verify module publication (may take a few minutes):
   ```bash
   go list -m github.com/fxsml/gopipe/channel@$ARGUMENTS
   go list -m github.com/fxsml/gopipe/pipe@$ARGUMENTS
   go list -m github.com/fxsml/gopipe/message@$ARGUMENTS
   ```

## Completion Summary

Report:
- Tags created and pushed
- GitHub release URL
- Module verification status

## Error Handling

- If CHANGELOG has no unreleased content, abort with message
- If CI fails, wait for fixes before merge
- If tag push fails, investigate and retry
- If module verification fails, check Go proxy status

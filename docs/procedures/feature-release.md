# Feature Branch Release Procedure

This procedure guides the release of a feature branch through git flow with proper history cleanup.

## Overview

```
feature/xxx → (cleanup) → develop → (verify) → main → (tag)
```

**Approval gates** are required before:
- Every rebase/history rewrite
- Every merge
- Every push to remote
- Every tag creation

## Prerequisites

- macOS with zsh/bash
- GitHub CLI (`gh`) installed and authenticated
- Claude Code (`claude`) installed
- `git-semver` installed (`make install-tools`)

## Quick Start

```bash
# Step 1: Merge feature branch to develop (Phases 1-3)
make merge-feature BRANCH=feature/message-refactoring

# Step 2: Release develop to main with tags (Phases 4-6)
make release VERSION=v0.11.0
```

## Step-by-Step Procedure

### Phase 1: History Review and Cleanup

#### 1.1 Review Current History

```bash
# Switch to feature branch
git checkout feature/xxx
git fetch origin develop
git fetch origin main

# Review full commit history since branching from develop
git log --oneline develop..HEAD

# Review commit details
git log --stat develop..HEAD

# Count commits
git rev-list --count develop..HEAD
```

#### 1.2 Plan Commit Structure

For large features, aim for **logical, atomic commits** following conventional commits:

| Type | Description | Example |
|------|-------------|---------|
| `feat` | New feature | `feat(message): add Engine type` |
| `fix` | Bug fix | `fix(message): correct Copy() to clone Attributes` |
| `refactor` | Code restructuring | `refactor(message): simplify Add* methods` |
| `docs` | Documentation | `docs(message): update architecture diagrams` |
| `test` | Tests | `test(message): add Engine integration tests` |
| `chore` | Maintenance | `chore: update dependencies` |

**Target structure for large features:**
1. Core types/interfaces (feat)
2. Implementation (feat)
3. Tests (test)
4. Documentation (docs)
5. Bug fixes found during development (fix)

#### 1.3 Interactive Rebase

**⚠️ APPROVAL REQUIRED before rebase**

```bash
# Find the merge base with develop
MERGE_BASE=$(git merge-base develop HEAD)
echo "Merge base: $MERGE_BASE"

# Start interactive rebase
git rebase -i $MERGE_BASE
```

**Rebase commands:**
- `pick` - Keep commit as-is
- `reword` - Change commit message
- `squash` - Combine with previous commit
- `fixup` - Combine with previous, discard message
- `drop` - Remove commit entirely

**Example rebase plan:**
```
pick abc1234 feat(message): add Engine type with single-merger architecture
squash def5678 wip: engine tests
squash ghi9012 fix: engine edge cases
pick jkl3456 feat(message): add Router and Distributor
pick mno7890 refactor(message): simplify Add* methods to use Options pattern
pick pqr1234 docs(message): update architecture documentation
pick stu5678 test(message): add comprehensive integration tests
```

#### 1.4 Verify Rebase Result

```bash
# Review new history
git log --oneline develop..HEAD

# Verify each commit compiles and tests pass
git rebase -i --exec "make check" $MERGE_BASE

# Or manually verify
make check
```

#### 1.5 Force Push to Feature Branch

**⚠️ APPROVAL REQUIRED before force push**

```bash
# Only after verifying everything works
git push --force-with-lease origin feature/xxx
```

### Phase 2: Merge to Develop

#### 2.1 Create PR to Develop

```bash
# Ensure feature branch is up to date
git fetch origin develop
git rebase origin/develop

# Push updated branch
git push --force-with-lease origin feature/xxx

# Create PR
gh pr create \
  --base develop \
  --title "feat(message): complete message package redesign" \
  --body "$(cat <<'EOF'
## Summary

- Redesigned message package with single-merger architecture
- Simplified Add* methods with Options pattern
- Added Router and Distributor components

## Commits

$(git log --oneline develop..HEAD)

## Test Plan

- [ ] `make check` passes
- [ ] Manual testing of examples
EOF
)"
```

#### 2.2 Review PR

```bash
# View PR status
gh pr view

# View PR checks
gh pr checks

# View PR diff
gh pr diff
```

#### 2.3 Merge PR

**⚠️ APPROVAL REQUIRED before merge**

```bash
# Squash merge (single commit) or merge commit (preserve history)
# For large features with clean history, use merge commit:
gh pr merge --merge --delete-branch

# Or squash if history isn't important:
# gh pr merge --squash --delete-branch
```

### Phase 3: Verify on Develop

#### 3.1 Update Local Develop

```bash
git checkout develop
git pull origin develop
```

#### 3.2 Run Full Verification

```bash
make check
make test-coverage
```

#### 3.3 Test Examples

```bash
cd examples
go run ./01-basic/
go run ./02-broadcast/
# ... etc
```

### Phase 4: Merge to Main

#### 4.1 Create Release PR

```bash
# Create PR from develop to main
gh pr create \
  --base main \
  --head develop \
  --title "release: v0.11.0" \
  --body "$(cat <<'EOF'
## Release v0.11.0

See CHANGELOG.md for full details.

## Pre-release Checklist

- [ ] All tests pass
- [ ] CHANGELOG.md updated
- [ ] Documentation updated
- [ ] Examples work
EOF
)"
```

#### 4.2 Merge to Main

**⚠️ APPROVAL REQUIRED before merge**

```bash
gh pr merge --merge
```

### Phase 5: Tag and Release

#### 5.1 Update Local Main

```bash
git checkout main
git pull origin main
```

#### 5.2 Create Module Tags

**⚠️ APPROVAL REQUIRED before tagging**

```bash
VERSION=v0.11.0

# Create tags in dependency order
git tag channel/$VERSION
git tag pipe/$VERSION
git tag message/$VERSION
git tag examples/$VERSION  # Optional
```

#### 5.3 Push Tags

**⚠️ APPROVAL REQUIRED before push**

```bash
git push origin channel/$VERSION pipe/$VERSION message/$VERSION

# Push release tag for GitHub
git tag $VERSION
git push origin $VERSION
```

#### 5.4 Create GitHub Release

```bash
gh release create $VERSION \
  --title "$VERSION" \
  --notes "$(cat <<EOF
## Modules Released

- \`github.com/fxsml/gopipe/channel@$VERSION\`
- \`github.com/fxsml/gopipe/pipe@$VERSION\`
- \`github.com/fxsml/gopipe/message@$VERSION\`

## Changes

$(sed -n "/## \[$VERSION\]/,/## \[/p" CHANGELOG.md | head -n -1)

## Full Changelog

https://github.com/fxsml/gopipe/compare/v0.10.1...$VERSION
EOF
)"
```

#### 5.5 Verify Module Publication

```bash
# Wait a few minutes for Go proxy to update, then verify
go list -m github.com/fxsml/gopipe/channel@$VERSION
go list -m github.com/fxsml/gopipe/pipe@$VERSION
go list -m github.com/fxsml/gopipe/message@$VERSION
```

### Phase 6: Post-Release

#### 6.1 Merge Main Back to Develop

```bash
git checkout develop
git merge main
git push origin develop
```

#### 6.2 Update Version References (if needed)

Update any hardcoded version references in documentation.

## Claude Code Integration

### Running with Claude

Two make targets split the procedure:

```bash
# Merge feature to develop (Phases 1-3: history cleanup, merge, verify)
make merge-feature BRANCH=feature/xxx

# Release to main with tags (Phases 4-6: merge main, tag, release)
make release VERSION=v0.11.0
```

Claude will:
1. Execute each phase
2. Pause for approval at each gate
3. Show diffs before merges
4. Verify tests pass before proceeding

### Manual Claude Invocation

```bash
# Merge feature to develop
claude -p "Execute Phases 1-3 of the Feature Branch Release Procedure from docs/procedures/feature-release.md for branch 'feature/xxx'. Pause for my approval before every rebase, merge, and push operation."

# Release to main
claude -p "Execute Phases 4-6 of the Feature Branch Release Procedure from docs/procedures/feature-release.md for version 'v0.11.0'. Pause for my approval before every merge, tag, and push operation."
```

## Troubleshooting

### Rebase Conflicts

```bash
# During rebase, if conflicts occur:
git status                    # See conflicted files
# ... resolve conflicts ...
git add <resolved-files>
git rebase --continue

# Or abort and try different strategy:
git rebase --abort
```

### Failed CI After Rebase

```bash
# If tests fail after rebase, fix and amend:
# ... make fixes ...
git add .
git commit --amend --no-edit
git push --force-with-lease origin feature/xxx
```

### Wrong Commit Message Format

```bash
# Reword the last commit:
git commit --amend -m "feat(scope): correct message"

# Reword older commits:
git rebase -i HEAD~N  # Then change 'pick' to 'reword'
```

## Checklist Summary

### Before Starting
- [ ] Feature branch has all changes
- [ ] Tests pass on feature branch
- [ ] CHANGELOG.md drafted

### Phase 1: History Cleanup
- [ ] Reviewed commit history
- [ ] Planned commit structure
- [ ] **APPROVED** rebase
- [ ] Rebased to clean history
- [ ] Verified tests pass after rebase
- [ ] **APPROVED** force push

### Phase 2: Merge to Develop
- [ ] Created PR to develop
- [ ] PR checks pass
- [ ] **APPROVED** merge to develop

### Phase 3: Verify Develop
- [ ] Tests pass on develop
- [ ] Examples work

### Phase 4: Merge to Main
- [ ] Created release PR
- [ ] **APPROVED** merge to main

### Phase 5: Tag and Release
- [ ] **APPROVED** tag creation
- [ ] Created module tags
- [ ] **APPROVED** tag push
- [ ] Pushed tags
- [ ] Created GitHub release
- [ ] Verified module publication

### Phase 6: Post-Release
- [ ] Merged main to develop
- [ ] Updated version references

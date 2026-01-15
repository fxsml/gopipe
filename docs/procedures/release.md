# Release Procedures

This document outlines the procedures for releasing new versions of gopipe.

## Quick Start

```bash
# Release develop to main with tags (Claude-assisted)
make release VERSION=v0.11.0
```

**See also:** [Feature Branch Release](feature-release.md) for merging large feature branches to develop with history cleanup.

## Version Numbering

This project uses [Semantic Versioning](https://semver.org/):

- **Major** (v1.0.0): Breaking changes after v1.0
- **Minor** (v0.X.0): New features, breaking changes before v1.0
- **Patch** (v0.0.X): Bug fixes only

### Checking Version

```bash
# Install git-semver (one time)
make install-tools

# Show current version
make version
```

## Regular Release Process

For planned releases with new features.

### 1. Create Release Branch

```bash
git checkout develop
git pull origin develop
git checkout -b release/vX.Y.Z
```

### 2. Prepare Release

- Update `CHANGELOG.md`: Move [Unreleased] entries to new version section with date
- Run tests: `make test`
- Run build: `make build`
- Run vet: `make vet`

### 3. Create Pull Request

```bash
git push -u origin release/vX.Y.Z
gh pr create --base main --title "release: vX.Y.Z" --body "Release notes..."
```

### 4. Merge and Tag

After PR is approved and merged:

```bash
# Update local main
git checkout main
git pull origin main

# Tag all modules (see "Multi-Module Tagging" section for details)
git tag channel/vX.Y.Z
git tag pipe/vX.Y.Z
git tag message/vX.Y.Z
git tag examples/vX.Y.Z  # Optional
git push origin channel/vX.Y.Z pipe/vX.Y.Z message/vX.Y.Z

# Create GitHub release (uses unprefixed tag for release page)
git tag vX.Y.Z
git push origin vX.Y.Z
gh release create vX.Y.Z --title "vX.Y.Z" --notes "Release notes..."

# Merge back to develop
git checkout develop
git merge main
git push origin develop

# Delete release branch
git branch -d release/vX.Y.Z
git push origin --delete release/vX.Y.Z
```

### 5. Verify Release

```bash
# Verify tags exist on remote
git ls-remote --tags origin | grep vX.Y.Z

# Verify modules resolve via Go proxy (may take a few minutes)
go list -m github.com/fxsml/gopipe/channel@vX.Y.Z
go list -m github.com/fxsml/gopipe/pipe@vX.Y.Z
go list -m github.com/fxsml/gopipe/message@vX.Y.Z

# Verify GitHub release exists
gh release view vX.Y.Z
```

**Optional: Test in fresh project**

```bash
cd $(mktemp -d)
go mod init test
go get github.com/fxsml/gopipe/message@vX.Y.Z
# Verify correct version installed
go list -m github.com/fxsml/gopipe/...
```

## Hotfix Release Process

For critical fixes to production that cannot wait for a regular release.

### 1. Create Hotfix Branch

```bash
git checkout main
git pull origin main
git checkout -b hotfix/<descriptive-name>
```

### 2. Make Fixes

- Implement the fix
- Add tests if applicable
- Update `CHANGELOG.md` with new version section:

```markdown
## [X.Y.Z] - YYYY-MM-DD

### Fixed

- Description of the fix
```

### 3. Commit and Push

```bash
git add .
git commit -m "fix: description of the fix"
git push -u origin hotfix/<descriptive-name>
```

### 4. Create Pull Request to Main

```bash
gh pr create --base main --title "fix: description" --body "$(cat <<'EOF'
## Summary

- Description of what was fixed

## Test plan

- [ ] Tests pass
EOF
)"
```

### 5. After PR is Merged

```bash
# Update local main
git checkout main
git pull origin main

# Tag all modules (see "Multi-Module Tagging" section for details)
git tag channel/vX.Y.Z
git tag pipe/vX.Y.Z
git tag message/vX.Y.Z
git push origin channel/vX.Y.Z pipe/vX.Y.Z message/vX.Y.Z

# Create GitHub release
git tag vX.Y.Z
git push origin vX.Y.Z
gh release create vX.Y.Z --title "vX.Y.Z" --notes "$(cat <<'EOF'
## Fixed

- Description of the fix

## Full Changelog

https://github.com/fxsml/gopipe/compare/vX.Y.Z-1...vX.Y.Z
EOF
)"

# Merge main back to develop
git checkout develop
git merge main
# Resolve any conflicts (typically in CHANGELOG.md)
git push origin develop

# Delete hotfix branch
git branch -d hotfix/<descriptive-name>
git push origin --delete hotfix/<descriptive-name>
```

### 6. Verify Release

```bash
# Verify tags exist on remote
git ls-remote --tags origin | grep vX.Y.Z

# Verify modules resolve via Go proxy (may take a few minutes)
go list -m github.com/fxsml/gopipe/channel@vX.Y.Z
go list -m github.com/fxsml/gopipe/pipe@vX.Y.Z
go list -m github.com/fxsml/gopipe/message@vX.Y.Z

# Verify GitHub release exists
gh release view vX.Y.Z
```

### Resolving CHANGELOG Conflicts

When merging main to develop, CHANGELOG conflicts are common. The resolution should:

1. Keep the `[Unreleased]` section from develop at the top
2. Add the new hotfix version section below `[Unreleased]`
3. Keep all existing released versions below

Example resolution:

```markdown
## [Unreleased]

### Added
- Features in progress on develop...

## [0.10.1] - 2025-12-17   <-- New hotfix version

### Fixed
- The hotfix description...

## [0.10.0] - 2025-12-12   <-- Previous release
...
```

## Multi-Module Tagging

This project uses Go workspaces with multiple modules in subdirectories:

```
gopipe/
├── channel/     # github.com/fxsml/gopipe/channel
├── pipe/        # github.com/fxsml/gopipe/pipe
├── message/     # github.com/fxsml/gopipe/message
└── examples/    # github.com/fxsml/gopipe/examples
```

### Tag Format

For Go modules in subdirectories, tags must be **prefixed with the subdirectory path**:

| Module | Tag Format |
|--------|-----------|
| channel | `channel/vX.Y.Z` |
| pipe | `pipe/vX.Y.Z` |
| message | `message/vX.Y.Z` |
| examples | `examples/vX.Y.Z` (optional) |

### Creating Multi-Module Tags

When releasing, create tags for all modules **in dependency order**:

```bash
# Tag all modules (after PR is merged to main)
git checkout main
git pull origin main

# Create tags in dependency order (channel first, then pipe, then message)
git tag channel/vX.Y.Z
git tag pipe/vX.Y.Z
git tag message/vX.Y.Z
git tag examples/vX.Y.Z  # Optional

# Push all tags at once
git push origin channel/vX.Y.Z pipe/vX.Y.Z message/vX.Y.Z examples/vX.Y.Z

# Or push all tags
git push origin --tags
```

### Dependency Order

Modules must be tagged in dependency order because Go resolves dependencies from published tags:

1. **channel** - No internal dependencies (tag first)
2. **pipe** - Depends on channel
3. **message** - Depends on channel and pipe
4. **examples** - Depends on all modules (tag last, optional)

### go.mod Internal Version References

Internal go.mod files (e.g., `pipe/go.mod` requiring `channel`) reference specific versions. This creates a challenge during releases.

**The Problem:**

When `pipe/go.mod` contains `require github.com/fxsml/gopipe/channel v0.11.0`:
- If we update to `v0.12.0` before tagging → CI fails (tag doesn't exist yet)
- If we keep `v0.11.0` after tagging → users get mixed versions

**When Mixed Versions Are Safe:**

If the release has **no cross-module API changes** (e.g., only doc changes in channel/pipe), keeping old refs is safe:
- `pipe@v0.12.0` + `channel@v0.11.0` works because the API is identical
- Go's MVS resolves correctly

**When You MUST Update Refs:**

If a module uses **new features from a dependency**, you must update refs. For example, if `message@v0.12.0` uses a new `channel.NewFeature()` function:

```bash
# 1. Merge release PR to main
gh pr merge <PR> --merge

# 2. Tag channel first (no dependencies)
git checkout main && git pull origin main
git tag channel/vX.Y.Z
git push origin channel/vX.Y.Z

# 3. Update pipe/go.mod to reference new channel version
# (create commit directly on main)
sed -i 's/channel v0.11.0/channel vX.Y.Z/' pipe/go.mod
git add pipe/go.mod
git commit -m "chore(pipe): update channel dependency to vX.Y.Z"
git push origin main

# 4. Tag pipe
git tag pipe/vX.Y.Z
git push origin pipe/vX.Y.Z

# 5. Update message/go.mod
sed -i 's/channel v0.11.0/channel vX.Y.Z/' message/go.mod
sed -i 's/pipe v0.11.0/pipe vX.Y.Z/' message/go.mod
git add message/go.mod
git commit -m "chore(message): update dependencies to vX.Y.Z"
git push origin main

# 6. Tag message
git tag message/vX.Y.Z
git push origin message/vX.Y.Z

# 7. Create release tag and GitHub release
git tag vX.Y.Z
git push origin vX.Y.Z
gh release create vX.Y.Z --title "vX.Y.Z" --notes "..."

# 8. Merge back to develop
git checkout develop
git merge main
git push origin develop
```

**Determining If Updates Are Needed:**

Before release, check for cross-module changes:
```bash
# Check channel changes
git diff vOLD..HEAD -- channel/

# Check pipe changes
git diff vOLD..HEAD -- pipe/

# If only doc.go or no changes → safe to keep old refs
# If API changes → must update refs as described above
```

### Verifying Tags

After pushing tags, verify modules are accessible:

```bash
# Clear Go module cache (optional, for testing)
go clean -modcache

# Verify each module resolves correctly
go list -m github.com/fxsml/gopipe/channel@vX.Y.Z
go list -m github.com/fxsml/gopipe/pipe@vX.Y.Z
go list -m github.com/fxsml/gopipe/message@vX.Y.Z
```

### GitHub Releases

Create a single GitHub release for the version, documenting all module changes:

```bash
gh release create vX.Y.Z --title "vX.Y.Z" --notes "$(cat <<'EOF'
## Modules Released

- `github.com/fxsml/gopipe/channel@vX.Y.Z`
- `github.com/fxsml/gopipe/pipe@vX.Y.Z`
- `github.com/fxsml/gopipe/message@vX.Y.Z`

## Changes

(Copy from CHANGELOG.md)

## Full Changelog

https://github.com/fxsml/gopipe/compare/vX.Y.Z-1...vX.Y.Z
EOF
)"
```

**Note:** The `vX.Y.Z` tag (without prefix) is for GitHub releases only. The actual Go module tags have subdirectory prefixes.

## Pre-Release Checklist

Before any release:

- [ ] All tests pass (`make test`)
- [ ] Build succeeds (`make build`)
- [ ] Vet passes (`make vet`)
- [ ] CHANGELOG.md updated with version and date
- [ ] No uncommitted changes (`git status`)
- [ ] go.work includes all modules
- [ ] Check for cross-module API changes (see "go.mod Internal Version References")
  - If no API changes in channel/pipe: keep existing go.mod refs
  - If API changes: plan sequential tagging with go.mod updates

## Post-Release Checklist

After any release:

- [ ] Tags visible on remote (`git ls-remote --tags origin | grep vX.Y.Z`)
- [ ] Modules resolve via Go proxy (`go list -m github.com/fxsml/gopipe/message@vX.Y.Z`)
- [ ] GitHub release visible (`gh release view vX.Y.Z`)
- [ ] Main merged back to develop
- [ ] Release/hotfix branch deleted

## GitHub Release Notes

When creating GitHub releases, include:

1. Summary of changes (from CHANGELOG)
2. Link to full changelog comparison
3. Any migration notes for breaking changes

Example:

```markdown
## Added

- New feature description

## Fixed

- Bug fix description

## Full Changelog

https://github.com/fxsml/gopipe/compare/v0.10.0...v0.10.1
```

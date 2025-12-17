# Release Procedures

This document outlines the procedures for releasing new versions of gopipe.

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

# Tag the release
git tag vX.Y.Z
git push origin vX.Y.Z

# Create GitHub release
gh release create vX.Y.Z --title "vX.Y.Z" --notes "Release notes..."

# Merge back to develop
git checkout develop
git merge main
git push origin develop

# Delete release branch
git branch -d release/vX.Y.Z
git push origin --delete release/vX.Y.Z
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

# Tag the release
git tag vX.Y.Z
git push origin vX.Y.Z

# Create GitHub release
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

## Pre-Release Checklist

Before any release:

- [ ] All tests pass (`make test`)
- [ ] Build succeeds (`make build`)
- [ ] Vet passes (`make vet`)
- [ ] CHANGELOG.md updated with version and date
- [ ] No uncommitted changes (`git status`)

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

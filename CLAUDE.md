# Claude Code Procedures for gopipe

This document outlines procedures for common tasks in the gopipe repository, designed for reuse by Claude Code or other AI assistants.

## Git Workflow

This repository uses [git flow](https://www.atlassian.com/git/tutorials/comparing-workflows/gitflow-workflow) for managing branches and releases.

**Branch naming:**
- `main` - Production-ready code
- `develop` - Integration branch for features
- `feature/*` - New features (branch from develop)
- `release/*` - Release preparation (branch from develop)
- `hotfix/*` - Critical fixes (branch from main)

## Commit Messages

Follow [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/) for commit message formatting.

## Version Management

This project uses [semantic versioning](https://semver.org/) (semver) with [git-semver](https://pkg.go.dev/github.com/mdomke/git-semver/v6@v6.10.0) for release management.

```bash
# Show current version
make version

# Install git-semver
make install-tools
```

## Procedure: Feature Branch Documentation and Integration

This procedure documents how to prepare a feature branch for systematic integration into main.

### Overview

When you have a large feature branch (like `feat/pubsub`) with many commits and changes across multiple packages, this procedure helps:

1. Document all features systematically
2. Organize Architecture Decision Records (ADRs)
3. Prepare for feature-by-feature integration
4. Create a clean git history for the final merge

### Step 1: Analyze the Branch

**Goal**: Understand all changes between the feature branch and main.

```bash
# Fetch both branches
git fetch origin main
git fetch origin <feature-branch>

# Compare branches
git log --oneline origin/main..origin/<feature-branch>
git diff origin/main...origin/<feature-branch> --stat

# Identify affected packages
git diff origin/main...origin/<feature-branch> --name-only | cut -d'/' -f1 | sort -u
```

**Output**: List of commits, file changes, and affected packages.

### Step 2: Review Documentation and ADRs

**Goal**: Understand existing architectural decisions and their status.

```bash
# List all ADRs
ls -la docs/adr/

# Extract status from each ADR
for file in docs/adr/*.md; do
    echo "=== $file ==="
    grep -E "^\*\*Status:\*\*" "$file" || echo "No status found"
done
```

**Categorize ADRs**:
- **Implemented**: Features that are complete and tested
- **Accepted**: Architectural decisions guiding development
- **Proposed**: Planned features not yet implemented
- **Superseded**: Decisions replaced by newer ADRs

### Step 3: Create Feature Documentation

**Goal**: Document each distinct feature for systematic integration.

**Structure**: Create `docs/features/` with one file per feature:

```
docs/features/
├── README.md                        # Overview and integration order
├── 01-<prerequisite-feature>.md     # Prerequisites first
├── 02-<core-feature>.md             # Core features
├── 03-<dependent-feature>.md        # Features with dependencies
└── ...
```

**Feature Document Template**:

```markdown
# Feature: <Feature Name>

**Package:** `<package-name>`
**Status:** ✅ Implemented | 🔄 Proposed | ⛔ Superseded
**Related ADRs:**
- [ADR NNNN](../adr/NNNN-name.md) - Description

## Summary

Brief description of what the feature provides and why it's important.

## Implementation

Key types, functions, and APIs with code examples:

\`\`\`go
// Core type or function
type Foo struct { ... }
func NewFoo() *Foo
\`\`\`

**Key features:**
- Feature 1
- Feature 2

## Usage Example

\`\`\`go
// Practical usage example
foo := NewFoo()
\`\`\`

## Files Changed

- `path/to/file.go` - Description
- `path/to/test.go` - Tests

## Related Features

- [NN-other-feature](NN-other-feature.md) - How they relate
```

### Step 4: Organize ADR Documentation

**Goal**: Create a README categorizing ADRs by status.

Create `docs/adr/README.md`:

```markdown
# Architecture Decision Records (ADRs)

## ADR Status

### Implemented ✅
- [ADR 0001](0001-name.md) - Description

### Accepted ✓
- [ADR 0002](0002-name.md) - Description

### Proposed 💡
- [ADR 0003](0003-name.md) - Description

### Superseded ⛔
- [ADR 0004](0004-name.md) - Superseded by ADR NNNN

## Feature Mapping

Map ADRs to features in ../features/

## Reading Order

Recommended order for understanding the architecture.
```

### Step 5: Feature-by-Feature Integration (Git Flow)

**Goal**: Integrate features into develop using git flow.

**For each feature** (in dependency order):

1. **Create feature branch from develop**:
   ```bash
   git checkout develop
   git checkout -b feature/<feature-name>
   ```

2. **Cherry-pick or merge feature commits**

3. **Create Pull Request to develop**:
   ```bash
   gh pr create --base develop --title "feat: <feature-name>" --body "..."
   ```

4. **Merge to develop after review**

5. **Repeat for remaining features**

### Step 6: Release Process

**Goal**: Create and finalize releases using git flow.

1. **Create release branch from develop**:
   ```bash
   git checkout develop
   git checkout -b release/vX.Y.Z
   ```

2. **Finalize release**:
   - Update CHANGELOG.md with release date
   - Bump version numbers if needed
   - Final testing

3. **Merge to main and develop**:
   ```bash
   # Merge to main
   gh pr create --base main --title "release: vX.Y.Z"

   # After merge, tag the release
   git tag vX.Y.Z
   git push origin vX.Y.Z

   # Merge release back to develop
   git checkout develop
   git merge release/vX.Y.Z
   ```

4. **Delete release branch**:
   ```bash
   git branch -d release/vX.Y.Z
   ```

## Documentation Requirements

**CRITICAL**: These requirements must be followed for ALL commits.

### Before Pushing ANY Code

1. **Check Documentation Completeness**:
   - [ ] Feature documentation exists in `docs/features/`
   - [ ] CHANGELOG.md updated
   - [ ] ADRs created for architectural decisions
   - [ ] README.md updated for core component changes
   - [ ] Public API has precise, concise godoc

2. **Verify Documentation Quality**:
   - [ ] Godoc: Precise, concise, NO example implementations
   - [ ] Godoc: References concepts using links to feature docs
   - [ ] Examples: All code examples tested and up-to-date
   - [ ] CHANGELOG: Entry added under [Unreleased]
   - [ ] Feature docs: Include implementation, usage, files changed

### Godoc Standards

**DO:**
```go
// GroupBy aggregates items from the input channel by key, emitting batches
// when size or time limits are reached.
//
// For usage examples and patterns, see docs/features/01-channel-groupby.md.
func GroupBy[K comparable, V any](
    in <-chan V,
    keyFunc func(V) K,
    config GroupByConfig,
) <-chan Group[K, V]
```

**DON'T:**
```go
// GroupBy aggregates items. Example usage:
//
//    grouped := GroupBy(msgs, func(m Msg) string {
//        return m.Topic
//    }, GroupByConfig{MaxBatchSize: 100})
func GroupBy[K comparable, V any](...) <-chan Group[K, V]
```

### Pre-Push Checklist

Run this before EVERY push:

```bash
# 1. Tests pass
make test

# 2. Build passes
make build

# 3. Vet passes
make vet

# 4. Documentation exists
ls docs/features/*.md
grep -q "Unreleased" CHANGELOG.md

# 5. Git check
git status  # No untracked important files
```

## Tips for AI Assistants

### Key Principles

1. **Feature Independence**: Each feature should be independently testable
2. **Dependency Order**: Prerequisites must be integrated before dependents
3. **Clear Documentation**: Each feature gets its own doc for reference
4. **Clean History**: Use git flow branches for clean merges
5. **Test Coverage**: Tests must pass after each feature integration

### Common Pitfalls

1. **Don't**: Cherry-pick commits out of dependency order
   - **Do**: Follow the numbered feature order

2. **Don't**: Create feature docs without understanding ADRs
   - **Do**: Read related ADRs first

3. **Don't**: Squash unrelated features together
   - **Do**: Keep features separate even if they touch same files

4. **Don't**: Skip test runs between features
   - **Do**: Verify tests pass after each integration

### Git Best Practices

- Use descriptive commit messages following conventional commits
- Reference feature docs in commit messages
- Link related ADRs in commits
- Use git flow branch naming conventions

---

**Document Version**: 2.0
**Last Updated**: 2025-12-12
**Maintainer**: gopipe team
- never merge to main yourself
# Claude Code Procedures for gopipe

This document outlines procedures for common tasks in the gopipe repository, designed for reuse by Claude Code or other AI assistants.

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

```go
// Core type or function
type Foo struct { ... }
func NewFoo() *Foo
```

**Key features:**
- Feature 1
- Feature 2

## Usage Example

```go
// Practical usage example
foo := NewFoo()
```

## Files Changed

- `path/to/file.go` - Description
- `path/to/test.go` - Tests

## Related Features

- [NN-other-feature](NN-other-feature.md) - How they relate
```

**Naming Convention**:
- Use numeric prefixes (01-, 02-) for integration order
- Start with prerequisites (e.g., channel utilities)
- Progress through dependencies (core → dependent features)
- Use descriptive kebab-case names

**Example Features**:
1. `01-channel-groupby.md` - Prerequisite batching utility
2. `02-message-core-refactor.md` - Core message type changes
3. `03-message-pubsub.md` - Publisher/Subscriber (depends on 01, 02)
4. `04-message-router.md` - Router (depends on 02)
5. etc.

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

### Step 5: Remove Old/Conflicting Documentation

**Goal**: Clean up documentation that conflicts with new organization.

**Check for**:
- Old ADR files in root `docs/` (vs `docs/adr/`)
- Duplicate or outdated documentation
- Analysis documents that should be archived

**Actions**:
- Move old ADRs if they follow old naming (e.g., `adr-3_name.md` → keep as historical)
- Keep analysis documents (e.g., `critical-analysis-*.md`) for reference
- Update references in README.md

### Step 6: Verify Tests Pass

**Goal**: Ensure the feature branch is stable and ready for integration.

```bash
# Run all tests
go test ./...

# Run with coverage
go test ./... -cover

# Run specific package
go test ./message/... -v
```

**Requirements**:
- All tests must pass
- No compilation errors
- Examples should build successfully

### Step 7: Document the Procedure

**Goal**: Create this CLAUDE.md file for future reuse.

Update or create `CLAUDE.md` with:
- This procedure
- Other common procedures (PR creation, release process, etc.)
- Repository-specific conventions
- Tips for AI assistants

### Step 8: Commit and Push Documentation

**Goal**: Save documentation work to the feature branch.

```bash
# Check current branch
git status

# Review changes
git diff

# Add documentation
git add docs/features/ docs/adr/README.md CLAUDE.md

# Commit with descriptive message
git commit -m "$(cat <<'EOF'
docs: add feature documentation and ADR organization for feat/pubsub

- Create docs/features/ with 8 feature documents
- Add docs/features/README.md with integration order
- Add docs/adr/README.md categorizing ADRs by status
- Document procedure in CLAUDE.md for future reuse

Features documented:
1. Channel GroupBy (prerequisite)
2. Message Core Refactor (foundation)
3. Message Pub/Sub (publisher/subscriber/broker)
4. Message Router (dispatch and handlers)
5. Message CQRS (command/event handlers)
6. Message CloudEvents (v1.0.2 protocol)
7. Message Multiplex (topic routing)
8. Middleware Package (reusable middleware)

All tests passing. Ready for systematic integration into main.
EOF
)"

# Push to remote
git push -u origin <current-branch>
```

### Step 9: Feature-by-Feature Integration

**Goal**: Integrate features into main one at a time with clean commits.

**For each feature** (in dependency order):

1. **Create feature branch from main**:
   ```bash
   git checkout origin/main
   git checkout -b feature/<feature-name>
   ```

2. **Identify commits for the feature**:
   ```bash
   # From feature branch, find commits affecting feature files
   git log --oneline origin/main..origin/<feature-branch> -- <file-paths>
   ```

3. **Cherry-pick commits**:
   ```bash
   # Pick commits in chronological order
   git cherry-pick <commit-hash-1>
   git cherry-pick <commit-hash-2>
   # ... etc
   ```

4. **Squash into single feature commit**:
   ```bash
   # Interactive rebase to squash
   git rebase -i origin/main

   # Or reset and commit
   git reset --soft origin/main
   git add <feature-files>
   git commit -m "feat: <feature-name>

   <Feature summary from feature doc>

   See docs/features/NN-<feature-name>.md for details.

   Related ADRs: #<adr-numbers>
   "
   ```

5. **Run tests**:
   ```bash
   go test ./...
   ```

6. **Push feature branch**:
   ```bash
   git push -u origin feature/<feature-name>
   ```

7. **Create Pull Request** (if using GitHub):
   ```bash
   gh pr create --title "feat: <feature-name>" --body "$(cat <<'EOF'
   ## Summary
   <Feature summary>

   ## Changes
   - Change 1
   - Change 2

   ## Documentation
   See docs/features/NN-<feature-name>.md

   ## Test Plan
   - [ ] All tests pass
   - [ ] Examples run successfully
   - [ ] Documentation reviewed
   EOF
   )"
   ```

8. **Repeat for remaining features**

### Step 10: Handle Non-Essential Features

**Goal**: Add remaining useful changes after core features are integrated.

**Non-essential features** might include:
- Additional examples
- Documentation improvements
- Refactorings not required for core functionality
- Nice-to-have utilities

**Process**:
1. Create commits for related groups of non-essential changes
2. Don't squash - keep as separate logical commits
3. Push in batches for review

### Integration Order Example

For the `feat/pubsub` branch:

1. ✅ Channel GroupBy (prerequisite)
2. ✅ Message Core Refactor (foundation)
3. ✅ Message Pub/Sub (core pub/sub)
4. ✅ Message Router (message dispatch)
5. ✅ Message CQRS (event-driven patterns)
6. ✅ Message CloudEvents (standards compliance)
7. ✅ Message Multiplex (topic routing)
8. ✅ Middleware Package (cross-cutting concerns)
9. 📦 Additional examples
10. 📦 Documentation refinements

## Tips for AI Assistants

### When to Use This Procedure

Use this procedure when:
- A feature branch has many commits (50+)
- Multiple packages are affected
- You need clean git history for main
- Features have clear dependencies
- Documentation needs organization

### Key Principles

1. **Feature Independence**: Each feature should be independently testable
2. **Dependency Order**: Prerequisites must be integrated before dependents
3. **Clear Documentation**: Each feature gets its own doc for reference
4. **Clean History**: One commit per feature in main (via squash)
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

5. **Don't**: Forget to update references when moving docs
   - **Do**: Check all internal links after reorganization

### Git Best Practices

- Use descriptive commit messages following conventional commits
- Reference feature docs in commit messages
- Link related ADRs in commits
- Push regularly to avoid losing work
- Use `-u` flag on first push to track remote branch

### Testing Strategy

After each feature integration:
```bash
# Run all tests
go test ./...

# Run affected package tests with verbose output
go test ./<package> -v

# Check examples build
go build ./examples/...

# Verify documentation examples compile (if using xcode blocks)
```

## Related Procedures

### Creating a New ADR

(To be documented)

### Release Process

(To be documented)

### Dependency Updates

(To be documented)

---

**Document Version**: 1.0
**Last Updated**: 2025-12-11
**Maintainer**: gopipe team

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

3. **On Documentation Deviations**:
   - Refactor documentation to meet standards
   - Do not push until documentation is correct
   - Review all links and references

### Required Documentation Files

For any feature or significant change:

#### 1. Feature Documentation (`docs/features/NN-name.md`)
**When**: Adding new feature or major functionality

Template:
```markdown
# Feature: <Name>

**Package:** `package-name`
**Status:** ✅ Implemented | 🔄 Proposed | ⛔ Superseded
**Related ADRs:** [ADR NNNN](../adr/NNNN-name.md)

## Summary
Brief description (2-3 sentences)

## Implementation
Key types and functions with code examples

## Usage Example
Tested example code

## Files Changed
- path/to/file.go - Description

## Related Features
- [NN-other-feature](NN-other-feature.md)
```

#### 2. CHANGELOG.md Entry
**When**: Every code change

Format:
```markdown
## [Unreleased]

### Added
- New feature X - See [docs/features/NN-name.md](docs/features/NN-name.md)

### Changed  
- **BREAKING**: Description - See migration guide

### Fixed
- Bug fix description
```

#### 3. Architecture Decision Record (`docs/adr/NNNN-name.md`)
**When**: Architectural or design decisions

Template:
```markdown
# ADR NNNN: Title

**Date:** YYYY-MM-DD
**Status:** Proposed | Accepted | Implemented | Superseded

## Context
What problem are we facing?

## Decision
What did we decide?

## Consequences
Positive and negative impacts

## Links
Related features, ADRs, code
```

#### 4. README.md Updates
**When**: Adding/changing core components

Requirements:
- Update Core Components section
- Add tested, working examples
- Link to feature docs for advanced usage
- Keep examples minimal and focused

#### 5. CONTRIBUTING.md
**Purpose**: Human contributor guide (not AI-focused)

Update when:
- Development workflow changes
- New testing requirements
- Documentation standards evolve

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
//
// This will batch messages by topic...
func GroupBy[K comparable, V any](...) <-chan Group[K, V]
```

### Example Validation

**Before pushing, verify all README examples:**

```bash
# Extract example from README
# Save to temporary file
# Attempt to run

# OR: Run all examples
go build ./examples/...
go run ./examples/broker/main.go
go run ./examples/cqrs-package/main.go
```

**In CI/CD**: Add example validation step

### Pre-Push Checklist

Run this before EVERY push:

```bash
# 1. Tests pass
go test ./...

# 2. Examples build
go build ./examples/...

# 3. Documentation exists
ls docs/features/*.md
grep -q "Unreleased" CHANGELOG.md

# 4. Godoc check (manual)
# - No example code in godoc
# - Links to feature docs present
# - Precise and concise

# 5. Git check
git status  # No untracked important files
git diff    # Review all changes
```

### Deviation Handling

**If documentation is incomplete or incorrect:**

1. **STOP** - Do not push
2. **FIX** - Create/update required docs
3. **REVIEW** - Check all requirements
4. **COMMIT** - Separate doc commit if needed
5. **PUSH** - Only when complete

**If discovered after push:**

1. Create immediate follow-up commit
2. Fix all documentation issues
3. Update CHANGELOG with correction
4. Learn and improve process

## Summary: Documentation Workflow

For every feature:

1. **Write code** with tests
2. **Create feature doc** in docs/features/
3. **Update CHANGELOG** with entry
4. **Create ADR** (if architectural)
5. **Update README** (if core component)
6. **Write godoc** (precise, links to docs)
7. **Test examples** in README
8. **Review checklist** above
9. **Commit** with reference to docs
10. **Push** when complete

This workflow ensures:
- Complete documentation
- Tested examples
- Clear API reference
- Easy maintenance
- Future contributor success


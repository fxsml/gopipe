# Git Procedures

## Workflow

Uses [git flow](https://www.atlassian.com/git/tutorials/comparing-workflows/gitflow-workflow).

### Branch Naming

| Branch | Purpose | Base |
|--------|---------|------|
| `main` | Production-ready | - |
| `develop` | Integration | - |
| `feature/*` | New features | develop |
| `release/*` | Release prep | develop |
| `hotfix/*` | Critical fixes | main |

## Commit Messages

Follow [conventional commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

**Types:** `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

**Examples:**
```
feat(message): add CloudEvents validation
fix(router): handle nil handler gracefully
docs: update architecture roadmap
```

## Version Management

Uses [semantic versioning](https://semver.org/) with [git-semver](https://pkg.go.dev/github.com/mdomke/git-semver/v6@v6.10.0).

```bash
make version        # Show current version
make install-tools  # Install git-semver
```

## Feature Integration

### Quick Integration

For small features:

```bash
git checkout develop
git checkout -b feature/<name>
# ... make changes ...
gh pr create --base develop --title "feat: <name>" --body "..."
```

### Large Feature Branch Integration

For large feature branches with many commits across multiple packages.

#### Step 1: Analyze Branch

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

#### Step 2: Review Documentation and ADRs

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
- **Implemented**: Features complete and tested
- **Accepted**: Architectural decisions guiding development
- **Proposed**: Planned features not yet implemented
- **Superseded**: Decisions replaced by newer ADRs

#### Step 3: Create Feature Documentation

Create `docs/features/` with one file per feature:

```
docs/features/
├── README.md                    # Overview and integration order
├── 01-<prerequisite>.md         # Prerequisites first
├── 02-<core-feature>.md         # Core features
└── 03-<dependent>.md            # Features with dependencies
```

**Feature Document Template**:

```markdown
# Feature: <Name>

**Package:** `<package-name>`
**Status:** ✅ Implemented | 🔄 Proposed | ⛔ Superseded
**Related ADRs:**
- [ADR NNNN](../adr/NNNN-name.md)

## Summary

Brief description of what the feature provides.

## Implementation

Key types and functions:

\`\`\`go
type Foo struct { ... }
func NewFoo() *Foo
\`\`\`

## Usage Example

\`\`\`go
foo := NewFoo()
\`\`\`

## Files Changed

- `path/to/file.go` - Description
```

#### Step 4: Feature-by-Feature Integration

For each feature (in dependency order):

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

#### Integration Principles

1. **Feature Independence**: Each feature should be independently testable
2. **Dependency Order**: Prerequisites must be integrated before dependents
3. **Clear Documentation**: Each feature gets its own doc
4. **Clean History**: Use git flow branches for clean merges
5. **Test Coverage**: Tests must pass after each feature integration

#### Common Pitfalls

- **Don't** cherry-pick commits out of dependency order
- **Don't** create feature docs without understanding ADRs
- **Don't** squash unrelated features together
- **Don't** skip test runs between features

## Release Process

1. **Create release branch:**
   ```bash
   git checkout develop
   git checkout -b release/vX.Y.Z
   ```

2. **Finalize:**
   - Update CHANGELOG.md
   - Final testing

3. **Merge and tag:**
   ```bash
   gh pr create --base main --title "release: vX.Y.Z"
   # After merge:
   git tag vX.Y.Z
   git push origin vX.Y.Z
   git checkout develop && git merge release/vX.Y.Z
   ```

4. **Cleanup:**
   ```bash
   git branch -d release/vX.Y.Z
   ```

## Rules

- Never merge to main directly
- Never force push to main/develop
- Always run tests before push

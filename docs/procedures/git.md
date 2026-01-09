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

### Step 1: Analyze Branch

```bash
git fetch origin main
git fetch origin <feature-branch>
git log --oneline origin/main..origin/<feature-branch>
git diff origin/main...origin/<feature-branch> --stat
```

### Step 2: Create Feature Branch

```bash
git checkout develop
git checkout -b feature/<name>
```

### Step 3: Create PR

```bash
gh pr create --base develop --title "feat: <name>" --body "..."
```

## Release Process

See [release.md](release.md) for full release procedures including:
- Multi-module tagging (channel/, pipe/, message/)
- Hotfix releases
- GitHub release creation

**Quick reference:**
```bash
make merge-feature BRANCH=feature/xxx  # Merge feature to develop
make release VERSION=v0.11.0           # Release develop to main
```

## Rules

- Never merge to main directly
- Never force push to main/develop
- Always run tests before push

---
name: create-feature
description: Create a new feature branch from develop. Usage: /create-feature NAME (e.g. /create-feature message-validation)
disable-model-invocation: true
allowed-tools:
  - Bash
---

# Create Feature

Create feature branch from develop for: `$ARGUMENTS`

```bash
git checkout develop
git pull origin develop
git checkout -b feature/$ARGUMENTS
```

Confirm the branch was created and report: `git status` + current branch name.

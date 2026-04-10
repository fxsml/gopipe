---
name: verify
description: Run the full pre-push verification suite (make test, build, vet).
allowed-tools:
  - Bash
---

# Verify

Run the pre-push checklist:

```bash
make test
make build
make vet
```

Report pass/fail for each step. If any step fails, show the relevant error output and suggest the fix.

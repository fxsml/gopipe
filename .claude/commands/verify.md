# Command: verify

Run pre-push verification checks to ensure code quality before pushing.

## Procedure Reference

This command implements the pre-push checklist from `docs/procedures/go.md`.

## Usage

```
/verify
```

No arguments required.

## Pre-Checks

1. Ensure you are in the gopipe repository root
2. Check for any uncommitted changes (warn if present)

## Execution

### Step 1: Run Tests

```bash
make test
```

Report test results (pass/fail count).

### Step 2: Run Build

```bash
make build
```

Report build status.

### Step 3: Run Vet/Lint

```bash
make vet
```

Report any linting issues.

### Step 4: Check Git Status

```bash
git status --short
```

Report if working directory is clean or has uncommitted changes.

## Verification Report

Output a summary:

```
Verification Results
====================
Tests:     PASS / FAIL (X passed, Y failed)
Build:     PASS / FAIL
Vet:       PASS / FAIL (X issues)
Git clean: YES / NO

Ready to push: YES / NO
```

## Error Handling

- If tests fail, report which tests failed
- If build fails, report compilation errors
- If vet fails, report linting issues
- Continue running all checks even if some fail (report all issues)

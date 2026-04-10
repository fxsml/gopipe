#!/usr/bin/env bash
# PreToolUse hook: run make test/build/vet before git commit/push

TMPFILE=$(mktemp)
LOG=$(mktemp)
trap "rm -f $TMPFILE $LOG" EXIT
cat > "$TMPFILE"

CMD=$(python3 -c "
import sys, json
with open(sys.argv[1]) as f:
    data = json.load(f)
print(data.get('tool_input', {}).get('command', ''))
" "$TMPFILE" 2>/dev/null || echo "")

# Only intercept git commit and git push
if [[ "$CMD" != git\ commit* && "$CMD" != git\ push* ]]; then
    exit 0
fi

echo "Running pre-commit checks..." >&2
cd "$CLAUDE_PROJECT_DIR"

if ! make test > "$LOG" 2>&1; then
    echo "=== make test FAILED ===" >&2
    cat "$LOG" >&2
    echo "Fix failing tests before committing." >&2
    exit 2
fi

if ! make build > "$LOG" 2>&1; then
    echo "=== make build FAILED ===" >&2
    cat "$LOG" >&2
    echo "Fix build errors before committing." >&2
    exit 2
fi

if ! make vet > "$LOG" 2>&1; then
    echo "=== make vet FAILED ===" >&2
    cat "$LOG" >&2
    echo "Fix vet issues before committing." >&2
    exit 2
fi

echo "All checks passed." >&2
exit 0

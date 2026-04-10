#!/usr/bin/env bash
# PostToolUse hook: auto-format .go files after Edit/Write

TMPFILE=$(mktemp)
trap "rm -f $TMPFILE" EXIT
cat > "$TMPFILE"

FILE=$(python3 -c "
import sys, json
with open(sys.argv[1]) as f:
    data = json.load(f)
print(data.get('tool_input', {}).get('file_path', ''))
" "$TMPFILE" 2>/dev/null || echo "")

if [[ "$FILE" == *.go && -f "$FILE" ]]; then
    gofmt -w "$FILE"
fi

exit 0

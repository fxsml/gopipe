#!/usr/bin/env bash
# SessionStart hook: warn if the current branch was forked from main instead
# of develop, per docs/procedures/git.md's branch table.
set -u
cd "${CLAUDE_PROJECT_DIR:-$PWD}" || exit 0

BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null) || exit 0

case "$BRANCH" in
  main|develop|release/*|hotfix/*|HEAD) exit 0 ;;
esac

git fetch origin develop main --quiet 2>/dev/null || exit 0

MAIN_TIP=$(git rev-parse origin/main 2>/dev/null) || exit 0
DEVELOP_TIP=$(git rev-parse origin/develop 2>/dev/null) || exit 0

# develop and main coincide (e.g. right after a release) -- any fork point is fine
[[ "$MAIN_TIP" == "$DEVELOP_TIP" ]] && exit 0

FORK_MAIN=$(git merge-base HEAD origin/main 2>/dev/null) || exit 0
FORK_DEVELOP=$(git merge-base HEAD origin/develop 2>/dev/null) || exit 0

# Branch's fork point sits exactly at main's tip, and develop has since
# diverged past that point -- this branch was cut from main, not develop.
if [[ "$FORK_MAIN" == "$MAIN_TIP" && "$FORK_DEVELOP" != "$DEVELOP_TIP" ]]; then
  BEHIND=$(git rev-list --count HEAD..origin/develop 2>/dev/null || echo "?")
  cat <<EOF
{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"BRANCH BASE WARNING: current branch '$BRANCH' appears to be forked from 'main', not 'develop'. docs/procedures/git.md requires feature-type branches (feature/*, and harness-created claude/issue-* branches) to base off 'develop'. origin/develop is $BEHIND commit(s) ahead of this branch's fork point. Before doing any work, rebase onto develop: git fetch origin develop && git rebase origin/develop -- resolve conflicts if any, then continue. If this branch's PR has already been pushed/opened, use --force-with-lease when re-pushing after the rebase."}}
EOF
fi

exit 0

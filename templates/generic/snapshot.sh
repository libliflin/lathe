#!/usr/bin/env bash
# snapshot.sh — Generic project snapshot.
# Override this with project-specific state collection.

set -euo pipefail

echo "# Project Snapshot"
echo "Generated: $(date)"
echo ""

echo "## Git Status"
git status --short 2>/dev/null || echo "(not a git repo)"
echo ""

echo "## Recent Commits (last 10)"
git log --oneline -10 2>/dev/null || echo "(no commits)"
echo ""

echo "## File Structure"
find . -maxdepth 3 -not -path './.git/*' -not -path './.lathe/state/*' -not -path './node_modules/*' -not -path './vendor/*' | head -80 || true
echo ""

echo "## TODOs"
grep -rn 'TODO\|FIXME\|HACK\|XXX' --include='*.go' --include='*.py' --include='*.js' --include='*.ts' --include='*.rs' . 2>/dev/null | head -20 || echo "(none)"
echo ""

echo "## README"
if [[ -f README.md ]]; then
    head -40 README.md
else
    echo "(no README.md)"
fi

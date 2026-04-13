#!/usr/bin/env bash
# snapshot.sh — Collect Go project state for the lathe agent.
# Output goes to stdout as markdown. The engine captures it.
#
# KEEP THIS CRISP. The engine caps snapshot size. If this script produces
# too much output, agents see a truncated snapshot and lose context.
# Summarize (pass/fail counts, not full output). Raw dumps belong in files
# the agent can read on demand, not in the prompt.

set -euo pipefail

# macOS ships without `timeout`; use gtimeout (coreutils) or a no-op fallback
if command -v timeout &>/dev/null; then
    TIMEOUT=timeout
elif command -v gtimeout &>/dev/null; then
    TIMEOUT=gtimeout
else
    TIMEOUT=""
fi
_to() { if [[ -n "$TIMEOUT" ]]; then $TIMEOUT "$@"; else shift; "$@"; fi; }

echo "# Project Snapshot"
echo "Generated: $(date)"
echo ""

# Git state
echo "## Git Status"
git status --short 2>/dev/null || echo "(not a git repo)"
echo ""

echo "## Recent Commits (last 10)"
git log --oneline -10 2>/dev/null || echo "(no commits)"
echo ""

# Go module info
echo "## Go Module"
if [[ -f go.mod ]]; then
    head -3 go.mod
else
    echo "(no go.mod found — pre-modules project)"
fi
echo ""

# Build — only report result, not compiler output
echo "## Build"
if _to 30 go build ./... 2>&1 >/dev/null; then
    echo "OK — builds clean"
else
    echo "FAIL — build errors:"
    echo '```'
    _to 30 go build ./... 2>&1 | head -20
    echo '```'
fi
echo ""

# Tests — summary only (pass/fail counts), not full output
echo "## Tests"
TEST_OUTPUT=$(_to 60 go test -count=1 ./... 2>&1) || true
PASS_COUNT=$(echo "$TEST_OUTPUT" | grep -c '^ok' || true)
FAIL_COUNT=$(echo "$TEST_OUTPUT" | grep -c '^FAIL' || true)
SKIP_COUNT=$(echo "$TEST_OUTPUT" | grep -c '\[no test files\]' || true)
echo "Pass: $PASS_COUNT | Fail: $FAIL_COUNT | No tests: $SKIP_COUNT"
if [[ "$FAIL_COUNT" -gt 0 ]]; then
    echo ""
    echo "Failed packages:"
    echo '```'
    echo "$TEST_OUTPUT" | grep -E '^(FAIL|---\s*FAIL)' | head -15
    echo '```'
fi
echo ""

# Vet — only report if problems found
echo "## Vet"
VET_OUTPUT=$(_to 15 go vet ./... 2>&1) || true
if [[ -z "$VET_OUTPUT" ]]; then
    echo "OK — vet clean"
else
    echo '```'
    echo "$VET_OUTPUT" | head -10
    echo '```'
fi
echo ""

# Package structure
echo "## Packages"
go list ./... 2>/dev/null || echo "(no packages — may need go.mod)"
echo ""

# File overview
echo "## Go Files"
find . -name '*.go' -not -path './vendor/*' -not -path './.lathe/*' | sort | head -50
echo ""

# Coverage — summary line only
echo "## Coverage"
COV_OUTPUT=$(_to 60 go test -count=1 ./... -cover 2>&1) || true
echo "$COV_OUTPUT" | grep -oP 'coverage: [0-9.]+%' | head -5 || echo "(no coverage data)"
echo ""

# CI config
echo "## CI"
if [[ -d .github/workflows ]]; then
    echo "GitHub Actions workflows:"
    ls .github/workflows/ 2>/dev/null
else
    echo "(no CI configuration found)"
fi

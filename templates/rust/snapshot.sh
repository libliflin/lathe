#!/usr/bin/env bash
# snapshot.sh — Collect Rust project state for the lathe agent.
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

# Cargo metadata
echo "## Cargo.toml"
if [[ -f Cargo.toml ]]; then
    head -10 Cargo.toml
else
    echo "(no Cargo.toml found)"
fi
echo ""

# Build — only report result, not compiler output
echo "## Build"
if _to 30 cargo build 2>&1 >/dev/null; then
    echo "OK — builds clean"
else
    echo "FAIL — build errors:"
    echo '```'
    _to 30 cargo build 2>&1 | grep -E '^error' | head -15
    echo '```'
fi
echo ""

# Tests — summary only
echo "## Tests"
TEST_OUTPUT=$(_to 60 cargo test 2>&1) || true
PASS_COUNT=$(echo "$TEST_OUTPUT" | grep -c 'test .* ok$' || true)
FAIL_COUNT=$(echo "$TEST_OUTPUT" | grep -c 'test .* FAILED$' || true)
RESULT_LINE=$(echo "$TEST_OUTPUT" | grep -E '^test result:' | tail -1 || true)
if [[ -n "$RESULT_LINE" ]]; then
    echo "$RESULT_LINE"
else
    echo "Pass: $PASS_COUNT | Fail: $FAIL_COUNT"
fi
if [[ "$FAIL_COUNT" -gt 0 ]]; then
    echo ""
    echo "Failed tests:"
    echo '```'
    echo "$TEST_OUTPUT" | grep -E 'FAILED|panicked' | head -15
    echo '```'
fi
echo ""

# Clippy — only report if problems found
echo "## Clippy"
if cargo clippy --version &>/dev/null 2>&1; then
    CLIPPY_OUTPUT=$(_to 30 cargo clippy -- -D warnings 2>&1) || true
    WARN_COUNT=$(echo "$CLIPPY_OUTPUT" | grep -c '^warning' || true)
    ERR_COUNT=$(echo "$CLIPPY_OUTPUT" | grep -c '^error' || true)
    if [[ "$WARN_COUNT" -eq 0 && "$ERR_COUNT" -eq 0 ]]; then
        echo "OK — clippy clean"
    else
        echo "Warnings: $WARN_COUNT | Errors: $ERR_COUNT"
        echo '```'
        echo "$CLIPPY_OUTPUT" | grep -E '^(warning|error)\[' | head -10
        echo '```'
    fi
else
    echo "(clippy not installed)"
fi
echo ""

# File overview
echo "## Rust Files"
find . -name '*.rs' -not -path './target/*' -not -path './.lathe/*' | sort | head -50
echo ""

# Test count
echo "## Test Count"
grep -rn '#\[test\]' --include='*.rs' . 2>/dev/null | wc -l | xargs -I{} echo "{} test functions"
echo ""

# CI config
echo "## CI"
if [[ -d .github/workflows ]]; then
    echo "GitHub Actions workflows:"
    ls .github/workflows/ 2>/dev/null
else
    echo "(no CI configuration found)"
fi

#!/usr/bin/env bash
set -euo pipefail

# macOS-safe timeout
if command -v gtimeout &>/dev/null; then
  TIMEOUT=gtimeout
elif command -v timeout &>/dev/null; then
  TIMEOUT=timeout
else
  TIMEOUT=""
fi
run_timeout() { if [ -n "$TIMEOUT" ]; then $TIMEOUT "$@"; else shift; "$@"; fi; }

echo "# Project Snapshot"
echo "Generated: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo

# --- Git status ---
echo "## Git Status"
git status --short
echo

# --- Recent commits ---
echo "## Recent Commits"
git log --oneline -10
echo

# --- Build ---
echo "## Build"
BUILD_OUT=$(run_timeout 60 go build ./... 2>&1) && BUILD_STATUS="OK" || BUILD_STATUS="FAILED"
if [ "$BUILD_STATUS" = "OK" ]; then
  echo "OK — builds clean"
else
  echo "FAILED"
  echo '```'
  echo "$BUILD_OUT" | head -20
  echo '```'
fi
echo

# --- Vet ---
echo "## Vet"
VET_OUT=$(run_timeout 60 go vet ./... 2>&1) && VET_STATUS="OK" || VET_STATUS="FAILED"
if [ "$VET_STATUS" = "OK" ]; then
  echo "OK"
else
  echo "FAILED"
  echo '```'
  echo "$VET_OUT" | head -20
  echo '```'
fi
echo

# --- Tests ---
echo "## Tests"
TEST_OUT=$(run_timeout 120 go test -v -count=1 ./... 2>&1) || true
PASS=$(echo "$TEST_OUT" | grep -c '^--- PASS' || true)
FAIL=$(echo "$TEST_OUT" | grep -c '^--- FAIL' || true)
PKG_OK=$(echo "$TEST_OUT" | grep -c '^ok' || true)
PKG_FAIL=$(echo "$TEST_OUT" | grep -c '^FAIL' || true)

echo "Pass: $PASS | Fail: $FAIL | Packages OK: $PKG_OK | Packages FAIL: $PKG_FAIL"
if [ "$FAIL" -gt 0 ] || [ "$PKG_FAIL" -gt 0 ]; then
  echo '```'
  echo "$TEST_OUT" | grep -E "^(--- FAIL|FAIL)" | head -20
  echo '```'
fi
echo

# --- CI ---
echo "## CI"
if ls .github/workflows/*.yml &>/dev/null 2>&1; then
  echo "Workflows: $(ls .github/workflows/*.yml | xargs -n1 basename | tr '\n' ' ')"
else
  echo "No CI workflows found"
fi
echo

# --- Module ---
echo "## Module"
head -3 go.mod

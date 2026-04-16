# Testing in Lathe

## Test Runner

```
go test ./...          # run all tests
go test -v ./...       # verbose output
go test -count=1 ./... # disable test caching
```

All tests are in the root package (`package main`). There are no sub-packages to test separately.

## Test Files

- `init_test.go` — tests for `ensureGitignore` (idempotency, creation)
- `prompt_test.go` — tests for `assembleCommon` (skills, refs, theme, snapshot assembly) and `assembleSessionContext` (branch mode, direct mode)
- `state_test.go` — tests for `readSession`/`writeSession`, `getCycle`/`setCycle`, `archiveCycle`, `archiveGoal`

## Test Helpers

`setupTestState(t)` in `state_test.go` — creates a temp directory, overrides global path vars (`latheDir`, `latheSession`, `latheHistory`, `goalHistory`, `sessionFile`, `latheSkills`), creates the required directory structure. Call this at the start of any test that needs a real `.lathe/` layout.

## What Is and Isn't Tested

**Tested:** Go plumbing — session state read/write, cycle state, archiving, prompt assembly, gitignore management.

**Not tested:** Agent invocation (`invokeAgent`), CI polling, snapshot collection, process management, safety net, the meta-prompt quality itself, or whether the generated agent docs are actually good.

The meta-prompts are the most important part of lathe and have no automated tests. The only way to validate them is to run `lathe init` on a real project and read the output.

## Linux vs macOS Divergence

CI runs on `ubuntu-latest`. Local development is typically macOS. Tests that touch filesystem paths, process management, or shell execution may behave differently. The test helper uses `t.TempDir()` which is safe on both. Avoid tests that assume `/bin/bash` location or macOS-specific tools.

## Adding Tests

Use `setupTestState(t)` to get a clean temp directory with correct global state. Write table-driven tests for any new plumbing functions. For prompt assembly functions, check for expected substrings in the output — don't assert on exact content since it includes runtime-generated sections.

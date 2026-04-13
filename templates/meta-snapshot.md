You are writing a **snapshot script** for the project in the current directory.

The snapshot runs at the start of every cycle and feeds into the agent prompts. It is the agents' only window into the project's current state. If the snapshot is bad, every decision downstream is bad.

## What You Must Produce

Write `.lathe/snapshot.sh` — a bash script that collects project state and writes it to stdout as markdown.

The script must be executable (`chmod +x`), use `#!/usr/bin/env bash`, and `set -euo pipefail`.

## The Budget

The engine hard-caps snapshot output at **6,000 characters**. If your script produces more, agents see a truncated snapshot and lose context from the bottom. Design for ~4,000 characters so there's headroom. This is tight — every line must earn its place.

## What Makes a Good Snapshot

**Summarize, don't dump.** The single most important principle. Agents need health signals, not raw output.

- Tests: "Pass: 12 | Fail: 1 | Skip: 3" — not the full test runner output. Only show failure details if something actually failed.
- Build: "OK — builds clean" — not the compiler output. Only show errors if the build fails.
- Lint/typecheck: "OK" or error count + first few errors. Not the full report.
- Git status: `git status --short` is already a summary. Good.
- Git log: `git log --oneline -10` is fine.

**Only show details on failure.** The happy path should be one line per section. Failure paths can expand to show what's wrong — but still capped (e.g., `| head -10`).

**Include what changes between cycles.** Git status, test results, build status, CI config — these evolve. File listings are stable and waste budget.

**Skip what agents can get on demand.** Agents can `cat` any file, `grep` for TODOs, read the README. Don't waste snapshot budget reproducing things the agent can look up.

## Sections to Include

Read the project and decide which of these apply. Don't include sections that don't apply.

1. **Header** — "# Project Snapshot" + timestamp
2. **Git status** — `git status --short`
3. **Recent commits** — `git log --oneline -10`
4. **Build** — run the project's build command, report pass/fail
5. **Tests** — run the project's test command, report pass/fail counts
6. **Lint / typecheck / static analysis** — if configured, report pass/fail
7. **CI** — list CI config files if they exist, or note absence
8. **Docker / services** — if docker-compose exists, list services and whether they're running
9. **Workspace / monorepo structure** — if applicable, list packages/apps briefly

## Practical Details

- Use timeouts on commands that might hang: `timeout 60 <cmd>` (or `gtimeout` on macOS). Provide a fallback for macOS where `timeout` isn't available.
- Capture output to variables, then extract summaries. Don't pipe raw output to stdout.
- Use `|| true` after commands that might fail — the script should never exit early because a test failed.
- End test/build commands with `2>&1` to capture stderr.
- For test runners: `-count=1` (Go), `--run` (vitest), `--forceExit` (jest) — avoid interactive/watch modes.
- The script runs from the project root directory.

## Bootstrap Test Infrastructure

If the project has no test files, don't just report "no tests found" and move on. The snapshot's tests section is useless without a test to run, and the agents lose a critical health signal.

Find the most logical place for a test file (next to the most important source file, following the project's conventions), and add a single tautological test — something like `test('sanity', () => expect(true).toBe(true))`. The point isn't to test anything real yet; it's to prove the test runner works, give the snapshot something to report, and give the builder a pattern to follow when adding real tests.

## How to Work

1. Read the project: package.json, go.mod, Cargo.toml, Makefile, etc.
2. Identify: build command, test command, lint command, package manager, monorepo tool
3. Check for docker-compose, CI config, workspace config
4. If no test files exist, add a minimal one to bootstrap the test runner
5. Write a snapshot.sh that collects exactly what this project needs — no more, no less
6. Make it executable

Don't write a generic template. Write a script that knows this specific project.

You are writing a **snapshot script** for the project in the current directory.

The snapshot runs at the start of every cycle and feeds into the agent prompts. It is the agents' only window into the project's current state — a clean snapshot grounds every downstream decision in real data.

## What You Must Produce

Write `.lathe/snapshot.sh` — a bash script that collects project state and writes it to stdout as markdown.

The script must be executable (`chmod +x`), use `#!/usr/bin/env bash`, and `set -euo pipefail`.

## The Budget

The engine hard-caps snapshot output at **6,000 characters**. Design for ~4,000 characters so there's headroom — output beyond the cap truncates and cuts context from the bottom. This is tight; every line must earn its place.

## What Makes a Good Snapshot

**Summarize, give signals.** The single most important principle. Agents need health signals, not raw output.

- Tests: "Pass: 12 | Fail: 1 | Skip: 3". Show failure details only when something failed.
- Build: "OK — builds clean". Show errors only when the build fails.
- Lint/typecheck: "OK" or error count + first few errors.
- Git status: `git status --short` is already a summary. Good.
- Git log: `git log --oneline -10` is fine.

**Show details on failure.** Keep the happy path to one line per section. Failure paths expand to show what's wrong — capped (e.g., `| head -10`).

**Include what changes between cycles.** Git status, test results, build status, CI config — these evolve. Skip stable things like file listings; they waste budget.

**Skip what agents can get on demand.** Agents can `cat` any file, `grep` for TODOs, read the README. Let them — keep the snapshot budget for the signals they can only get by running commands.

## Sections to Include

Read the project and include the sections that apply.

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

- Use timeouts on commands that might hang: `timeout 60 <cmd>` (or `gtimeout` on macOS). Include a fallback for macOS where `timeout` is missing.
- Capture output to variables, then extract summaries. Pipe summaries to stdout, not raw output.
- Use `|| true` after commands that might fail — this keeps the script running when a test fails.
- End test/build commands with `2>&1` to capture stderr.
- For test runners: use non-interactive flags — `-count=1` (Go), `--run` (vitest), `--forceExit` (jest).
- The script runs from the project root directory.

## Bootstrap Test Infrastructure

When the project has no test files, add one before moving on. A tests section gains its value when there's a test to run — that's where the agents get a real health signal each cycle.

Find the most logical place for a test file (next to the most important source file, following the project's conventions), and add a single tautological test — something like `test('sanity', () => expect(true).toBe(true))`. The point: prove the test runner works, give the snapshot something to report, and give the builder a pattern to follow when adding real tests.

## How to Work

1. Read the project: package.json, go.mod, Cargo.toml, Makefile, etc.
2. Identify: build command, test command, lint command, package manager, monorepo tool
3. Check for docker-compose, CI config, workspace config
4. When no test files exist, add a minimal one to bootstrap the test runner
5. Write a snapshot.sh that collects exactly what this project needs — every section earns its place in the 6K budget
6. Make it executable

Write a script that knows this specific project — the commands it uses, the tools it has, the places its health signals live.

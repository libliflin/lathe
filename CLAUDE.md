# Lathe — Development Guide

## What This Is

Lathe is an autonomous code improvement loop. It points three AI agents at a repo and runs repeating cycles: a goal-setter picks the highest-value change, a builder implements it, and a verifier checks the work.

The core value proposition: lathe init identifies the stakeholders of a project and encodes their needs into agents that autonomously work on their behalf. Git commits provide oversight.

## The Alignment Model

1. **Identify who the project serves** — `lathe init` reads the project and discovers its real stakeholders, their journeys, and where those needs conflict.
2. **Encode values** — init writes a project-specific snapshot script, three behavioral docs (goal.md, builder.md, verifier.md), and skills that make the agents stakeholder-aware.
3. **Provide ongoing direction** — `--theme` lets the user state a purpose for a session that biases decisions without overriding the stakeholder framework.
4. **Maintain oversight** — every step is a git commit with a changelog that names who benefits and how.

## Architecture

Single Go binary with all templates embedded via `go:embed`. Builds for all platforms via GitHub Actions; self-updates via `lathe update`.

**`lathe init`** — The alignment step. Runs five sequential AI calls:
1. `meta-snapshot.md` → `.lathe/snapshot.sh` — project-specific state collection script. The agent reads the project and writes a snapshot tailored to its build/test/lint tools.
2. `meta-goal.md` → `.lathe/goal.md` — stakeholder map, tensions, ranking guidance. Values manifesto spliced in.
3. `meta-brand.md` → `.lathe/brand.md` — the project's character, cited from real signals (errors, README, CLI output). Loaded into every runtime prompt as a tint on decisions.
4. `meta-builder.md` → `.lathe/builder.md` — implementation quality, CI/PR workflow. Reads goal.md for alignment.
5. `meta-verifier.md` → `.lathe/verifier.md` — adversarial verification themes. Reads builder.md for failure modes.

Use `--agent=snapshot`, `--agent=goal`, `--agent=brand`, `--agent=builder`, or `--agent=verifier` to re-init just one role without touching the others.

**`lathe start`** — The execution loop. One cycle = goal-setter + a dialog between builder and verifier. The builder leans creative/generative; the verifier leans comparative/scrutinizing. Each round both speak: whoever sees something worth adding commits; whoever sees the work as complete from their lens makes no commit. The cycle converges when a round passes with neither committing — no VERDICT, no gate. `roundsPerCycle` (default 4) caps the dialog to prevent oscillation; hitting the cap hands the dialog to the next goal-setter cycle. Each step follows identical plumbing: branch → snapshot → agent → safety net → PR → CI → merge → back to main. The engine tracks convergence by comparing `HEAD` of the base branch before and after each agent step.

**Templates** — Embedded in the binary via `go:embed`, read-only:
- `templates/meta-snapshot.md` — instructions for snapshot script generation
- `templates/meta-goal.md` — instructions for goal-setter init
- `templates/meta-brand.md` — instructions for brand init (character, voice, edge-case behavior)
- `templates/meta-builder.md` — instructions for builder init
- `templates/meta-verifier.md` — instructions for verifier init
- `templates/values-manifesto.md` — design intent, spliced into meta-goal.md via {{VALUES_MANIFESTO}}
- `templates/interactive-preamble.md` — additional instructions for `--interactive` mode

## Key Principle

**The meta-prompts are the whole game.** They determine what init discovers, which determines what the runtime agents know, which determines whether cycles create value. The goal-setter's meta-prompt is the most important — it carries the values manifesto and stakeholder framework.

## File Map

```
main.go                          — CLI entrypoint, path setup
init.go                          — lathe init (generates agent docs, spinner, rate limit retry)
engine.go                        — lathe start/stop/status/logs (background process)
cycle.go                         — Cycle loop (goal + adaptive builder/verifier with verdict)
agent.go                         — Goal-setter, builder, verifier prompt assembly
invoke.go                        — Agent invocation, rate limit detection and sleep
update.go                        — Self-updater (checks GitHub Releases)
prompt.go                        — Shared prompt helpers (skills, refs, snapshot, session context)
snapshot.go                      — Project state collection
state.go                         — Session state management, archiving
ci.go                            — CI polling, auto-merge, CI status collection
safety.go                        — Safety net validation
process.go                       — Process management (kill tree, find agent, is_running)
shell.go                         — Shell execution helpers (run, runCapture, runPipe)
embed.go                         — go:embed for templates/
dashboard/                       — Self-contained web dashboard (isolated package)
  dashboard.go                   — start/stop/status commands, daemon lifecycle
  server.go                      — HTTP server + SSE (/api/stream)
  collector.go                   — Lathe discovery + per-project state reading
  index.html                     — Dashboard UI (embedded)
  embed.go                       — go:embed for index.html
  setsid_*.go                    — Platform-specific detach
templates/
  meta-snapshot.md               — Instructions for snapshot script generation
  meta-goal.md                   — Instructions for goal-setter init
  meta-brand.md                  — Instructions for brand init (project character)
  meta-builder.md                — Instructions for builder init
  meta-verifier.md               — Instructions for verifier init
  values-manifesto.md            — Design intent, spliced into meta-goal.md
  interactive-preamble.md        — Interactive mode behavior
  skill/
    SKILL.md                     — Global Claude Code skill, installed to ~/.claude/skills/lathe/
```

## State Model

**Config** — written by `lathe init`, survives stop, committed by the user:

```
.lathe/goal.md               — Goal-setter behavioral instructions, stakeholder map
.lathe/brand.md              — Project character (voice, edge-case behavior). Loaded into every runtime prompt.
.lathe/builder.md            — Builder behavioral instructions
.lathe/verifier.md           — Verifier behavioral instructions
.lathe/alignment-summary.md  — Plain-English summary of alignment decisions
.lathe/snapshot.sh           — Project state collection script
.lathe/skills/*.md           — Project-specific knowledge
.lathe/refs/*.md             — User-curated reference material
```

**Session** — born on `lathe start`, dies on `lathe stop`, everything wiped:

```
.lathe/session/              — Gitignored. Ephemeral engine runtime.
  session.json               — Branch name, PR number, base branch, mode
  cycle.json                 — Current cycle number and phase
  snapshot.txt               — Latest snapshot output
  changelog.md               — Latest changelog (verifier writes VERDICT here)
  theme.txt                  — Session theme (from --theme flag)
  rate-limited               — Sentinel for rate limit backoff
  lathe.pid                  — Engine process ID
  logs/                      — Per-step agent logs (cycle-001-goal.log, cycle-001-build-1.log, etc.)
  history/                   — Archived cycle changelogs and snapshots
  goal-history/              — Archived goals (goal-setter sees last 4)
```

History lives inside `session/` (gitignored). The real audit trail is the squash merge commits on main.

**`lathe init` (re-init)** wipes everything in `.lathe/` except `refs/` and regenerates the snapshot script and all three behavioral docs. Use `--agent=X` to re-init just one role.

**`lathe stop`** performs full teardown: kills the process tree, closes the PR, discards dirty working tree, checks out the base branch, deletes the local lathe branch, and wipes `session/`.

**`lathe dashboard`** — Machine-wide read-only web UI. `lathe dashboard start` spins up a localhost-only HTTP server (random high port by default, override with `--host`/`--port`) in the background and opens the browser; `lathe dashboard stop` kills it; `lathe dashboard status` reports state. The dashboard discovers every running lathe on the machine via `pgrep lathe _run` + cwd scoping, reads each project's `.lathe/session/` files directly, and streams a fresh snapshot every 2 seconds via SSE. Daemon state lives in `~/.lathe/dashboard.json`. Code is isolated in the `dashboard/` package — it does not import or mutate any main-lathe state.

## Conventions

- `snapshot.sh` is agent-generated at init time, tailored to the specific project. The meta-snapshot prompt teaches the agent to summarize (pass/fail counts, not raw output) and stay within the 6K char budget.
- Skills files are project-specific, written by init. Not generic language references.
- Refs files (`.lathe/refs/`) hold reference material the agents need. Loaded into every step's prompt alongside skills.
- The engine uses `--dangerously-skip-permissions --print` for runtime. Init uses `-p` with `--allowedTools` for controlled writes.
- `.lathe/session/` is gitignored entirely — never blocks branch switches, never committed.
- No fallback templates. Init succeeds or fails.
- Smart decisions belong in the agent prompts, not the engine. The engine is plumbing.
- Each step follows identical plumbing: branch → snapshot → CI status → agent → archive → safety net → PR → CI wait → merge. Teardown works at any point.
- No VERDICT binary. The builder and verifier each have distinct lenses (creative synthesis / comparative scrutiny). Each round they contribute code or stand down plainly in the changelog. The engine reads convergence from `git rev-parse <base_branch>` before and after each step — no commit means no contribution. A round with neither contributing is convergence.

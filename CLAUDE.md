# Lathe — Development Guide

## What This Is

Lathe is an autonomous code improvement loop. It points three AI agents at a repo and runs repeating cycles: a champion picks the highest-value change from lived stakeholder experience, a builder implements it, and a verifier scrutinizes and tightens gaps.

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
2. `meta-champion.md` → `.lathe/agents/champion.md` — the champion's playbook: stakeholder map, tensions, emotional signals, how to rank, the per-cycle output format. Values manifesto spliced in.
3. `meta-brand.md` → `.lathe/brand.md` — the project's character, cited from real signals (errors, README, CLI output). Loaded into every runtime prompt as a tint on decisions. Lives at the root (not under `agents/`) because it's a reference doc, not a loop agent.
4. `meta-builder.md` → `.lathe/agents/builder.md` — implementation quality (creative/synthesis posture), CI/PR workflow. Reads champion.md for alignment.
5. `meta-verifier.md` → `.lathe/agents/verifier.md` — comparative/scrutinizing posture, the shape-specific verification playbook. Reads builder.md for failure modes.

Use `--agent=snapshot`, `--agent=champion`, `--agent=brand`, `--agent=builder`, or `--agent=verifier` to re-init just one role without touching the others. `--agent=goal` is accepted as an alias for `--agent=champion` during the transition.

**`lathe start`** — The execution loop. One cycle = champion + a dialog between builder and verifier. The builder leans creative/generative; the verifier leans comparative/scrutinizing. Each round both speak: whoever sees something worth adding commits; whoever sees the work as complete from their lens makes no commit. The cycle converges when a round passes with neither committing — no VERDICT, no gate. `roundsPerCycle` (default 20) caps the dialog; hitting the cap writes an error state to `.lathe/session/error.md` and halts the engine for human review. Each step follows identical plumbing: branch → snapshot → agent → safety net → PR → CI → merge → back to main. The engine tracks convergence by comparing `HEAD` of the base branch before and after each agent step, and classifies changelog-only or gitignored-file commits as non-substantive (with a 5-minute breathing-room pause).

**Templates** — Embedded in the binary via `go:embed`, read-only:
- `templates/meta-snapshot.md` — instructions for snapshot script generation
- `templates/meta-champion.md` — instructions for champion init
- `templates/meta-brand.md` — instructions for brand init (character, voice, edge-case behavior)
- `templates/meta-builder.md` — instructions for builder init
- `templates/meta-verifier.md` — instructions for verifier init
- `templates/values-manifesto.md` — design intent, spliced into `meta-champion.md` via `{{VALUES_MANIFESTO}}`
- `templates/interactive-preamble.md` — additional instructions for `--interactive` mode

## Key Principle

**The meta-prompts are the whole game.** They determine what init discovers, which determines what the runtime agents know, which determines whether cycles create value. The champion's meta-prompt is the most important — it carries the values manifesto and stakeholder framework.

## File Map

```
main.go                          — CLI entrypoint, path setup
init.go                          — lathe init (generates agent docs, spinner, rate limit retry)
engine.go                        — lathe start/stop/status/logs (background process)
cycle.go                         — Cycle loop (champion + builder/verifier dialog + convergence)
agent.go                         — Champion, builder, verifier prompt assembly
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
  meta-champion.md               — Instructions for champion init
  meta-brand.md                  — Instructions for brand init (project character)
  meta-builder.md                — Instructions for builder init
  meta-verifier.md               — Instructions for verifier init
  values-manifesto.md            — Design intent, spliced into meta-champion.md
  interactive-preamble.md        — Interactive mode behavior
  skill/
    SKILL.md                     — Global Claude Code skill, installed to ~/.claude/skills/lathe/
```

## State Model

**Config** — written by `lathe init`, survives stop, committed by the user:

```
.lathe/agents/               — The three roles that run in the cycle loop:
  champion.md                —   Champion's playbook (stakeholder map, tensions, output format)
  builder.md                 —   Builder behavioral instructions
  verifier.md                —   Verifier behavioral instructions
.lathe/brand.md              — Project character. Reference doc loaded into every runtime
                               prompt as a tint (not a loop agent — no runtime step)
.lathe/alignment-summary.md  — Plain-English summary of alignment decisions (human-facing)
.lathe/snapshot.sh           — Project state collection script
.lathe/skills/*.md           — Project-specific knowledge
.lathe/refs/*.md             — User-curated reference material
```

**Session** — born on `lathe start`, dies on `lathe stop`, everything wiped:

```
.lathe/session/              — Gitignored. Ephemeral engine runtime.
  session.json               — Branch name, PR number, base branch, mode
  cycle.json                 — Current cycle's ID (timestamp) and phase
  journey.md                 — Champion's output for the current cycle (stable during the cycle)
  whiteboard.md              — Shared scratchpad for the current cycle (wiped at cycle boundary)
  snapshot.txt               — Latest snapshot output
  theme.txt                  — Session theme (from --theme flag)
  rate-limited               — Sentinel for rate limit backoff
  lathe.pid                  — Engine process ID
  logs/                      — Per-step agent logs (cycle-<timestamp>-champion.log, etc.)
  history/<cycle-id>/        — Per-cycle archive: journey.md, whiteboard.md, snapshot.txt
                              (cycle-id is a timestamp, e.g. 20260418-083045, globally unique)
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
- Each step follows identical plumbing: stale-PR sweep → branch → snapshot → CI status → agent → archive → safety net → PR → CI wait → merge. Teardown works at any point.
- The stale-PR sweep (`resolveStalePRs`) merges any lathe PR whose CI has turned green and writes `session/stale-prs.txt` (with failure logs + handling instructions) for the ones still failing or pending. `lathe start` runs a one-shot version of this sweep (`preStartCleanup`) to inherit work from prior sessions.
- Step branching: if the session's PR is still open at step start (previous step's CI didn't merge in time), the next step continues on the same branch. A single goal's rounds of dialog share one PR when CI is slow — on merge, the full arc squashes into one commit. At cycle boundaries, `runCycle` clears the session PR so each new goal always cuts fresh.
- No VERDICT binary. The champion runs first (one journey per cycle), then the builder and verifier have a dialog with distinct lenses (creative synthesis / comparative scrutiny). Each round they contribute code or stand down. The engine reads convergence from `git rev-parse <base_branch>` before and after each step — no commit means no contribution. A round with neither contributing is convergence.
- **Two session files that agents touch each cycle:**
  - `.lathe/session/journey.md` — the champion's artifact. Written once at the top of the cycle. Stable throughout. Builder and verifier read it every round. Archived to `history/<cycle-id>/journey.md` at cycle end.
  - `.lathe/session/whiteboard.md` — a shared scratchpad. Any agent in the cycle's loop can read/write/edit/wipe it. Engine wipes it clean at each cycle boundary. Archived to `history/<cycle-id>/whiteboard.md` at cycle end. No prescribed format — treat it like a physical whiteboard in a meeting room.
- Cycle IDs are timestamps (`YYYYMMDD-HHMMSS`, UTC) — globally unique across every `lathe start`. Agents can reference a cycle in code comments without fear that the identifier becomes stale when the counter resets on a new session.

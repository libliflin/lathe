# Lathe — Development Guide

## What This Is

Lathe is an autonomous code improvement loop. It points an AI agent at a repo and runs repeating cycles: snapshot the project state, pick the highest-value change, implement it, validate, commit.

The core value proposition: lathe init identifies the stakeholders of a project and encodes their needs into an agent that autonomously works on their behalf. Every cycle picks the single change that most improves a real person's experience. Git commits provide oversight.

## The Alignment Model

Lathe is an opinionated, automatic approach to the alignment problem for autonomous agents:

1. **Identify who the project serves** — `lathe init` reads the project and discovers its real stakeholders, their journeys, and where those needs conflict.
2. **Encode values** — init writes `agent.md` and skills that make the runtime agent stakeholder-aware. The agent picks the most valuable change each cycle, not the easiest or most obvious.
3. **Provide ongoing direction** — `--theme` lets the user state a purpose for a session ("get the CLI working end-to-end") that biases decisions without overriding the stakeholder framework.
4. **Maintain oversight** — every cycle is a git commit with a changelog that names who benefits and how.

## Architecture

**`lathe init`** — The alignment step. Detects project type, copies `snapshot.sh`, then calls an AI agent with `templates/meta-prompt.md`. The init agent reads the target project deeply and writes:
- `.lathe/agent.md` — behavioral instructions, stakeholder map, tension mapping, priorities
- `.lathe/skills/*.md` — project-specific knowledge (testing conventions, architecture, build process)
- `.lathe/alignment-summary.md` — plain-English summary of alignment decisions for user review

With `--interactive`, the user participates in stakeholder discovery — the init agent pauses at each step (stakeholders, tensions, conventions) and checks in before writing. Default is autonomous.

If AI generation fails, init fails loudly. There are no fallback templates — a generic agent that doesn't understand the project's stakeholders is worse than no agent.

**`lathe start`** — The execution loop. By default runs in branch mode: creates a session branch, and the agent manages PRs and CI through `gh` CLI. The engine is dumb plumbing — it creates the branch, waits for CI (blocks up to 2 min), collects snapshots with CI status, calls the agent, and catches mistakes. All smart decisions (merge PR? fix CI? create new PR? wait out an outage?) live in the agent prompt, not in shell.

The cycle: wait for CI → snapshot (including CI results) → prompt assembly → agent implements one change (commits, pushes, manages PRs) → engine safety net (catches uncommitted changes on wrong branch) → archive → next cycle.

`--direct` flag preserves legacy behavior: commit to current branch, no PRs, no CI integration.

**Templates** — Static mechanics only:
- `templates/meta-prompt.md` — instructions for the init agent (the most important file in the project)
- `templates/interactive-preamble.md` — additional instructions injected when `--interactive` is used
- `templates/*/snapshot.sh` — default state collection scripts
- `templates/*/snapshot.sh` — state collection scripts copied into `.lathe/` at init

## Key Principle

**The meta-prompt is the whole game.** It determines what init discovers, which determines what the runtime agent knows, which determines whether cycles create value. If you want the lathe to do something better, the change almost always belongs in `meta-prompt.md` — or, when the change is about design *intent* rather than mechanics, in `values-manifesto.md`, which the meta-prompt splices in at the top so the init agent reads the *why* before the *how*.

## File Map

```
bin/lathe                        — CLI entrypoint (init, start, stop, status, logs)
engine/loop.sh                   — Cycle engine orchestrator (sources lib/, defines commands + cycle loop)
engine/lib/
  process.sh                     — Process management (kill tree, find agent, is_running)
  state.sh                       — State helpers, session management, teardown
  ci.sh                          — CI polling, auto-merge, CI status collection
  agent.sh                       — Snapshot, falsification, prompt assembly, agent invocation
templates/
  meta-prompt.md                 — Instructions for the init agent
  values-manifesto.md            — The values manifesto, injected into meta-prompt via {{VALUES_MANIFESTO}}. Authoritative source for lathe's design intent; the init agent reads it before the structural rules.
  interactive-preamble.md        — Interactive mode behavior (injected via {{INTERACTIVE}})
  generic/
    snapshot.sh                  — Generic state collection
  go/
    snapshot.sh                  — Go-specific state collection (build, test, vet, coverage)
  rust/
    snapshot.sh                  — Rust-specific state collection (cargo build, test, clippy)
  skill/
    SKILL.md                     — Global Claude Code skill, installed to ~/.claude/skills/lathe/ on init
```

## State Model

There are exactly two categories of state under `.lathe/`:

**Config** — written by `lathe init`, survives stop, committed by the user:

```
.lathe/agent.md              — Behavioral instructions, stakeholder map, priorities
.lathe/alignment-summary.md  — Plain-English summary of alignment decisions
.lathe/snapshot.sh           — Project state collection script
.lathe/claims.md             — Registry of load-bearing promises, per stakeholder
.lathe/falsify.sh            — Adversarial suite, run every cycle by the engine
.lathe/skills/*.md           — Project-specific knowledge
.lathe/refs/*.md             — User-curated reference material
```

The falsification suite (`claims.md` + `falsify.sh`) is the structural defense against Goodhart and metric-gaming. The engine runs `falsify.sh` from `collect_falsification` each cycle and appends a `## Falsification` block to the snapshot. Every cycle, `run_agent` injects a "Red-Team Review" section that tells the agent to review and strengthen claims alongside its normal work (CI output is referenced so the agent sees build health as part of the adversarial picture). Both files are config — the agent extends them as the project grows.

**Session** — born on `lathe start`, dies on `lathe stop`, everything wiped:

```
.lathe/session/              — Gitignored. Ephemeral engine runtime.
  session.json               — Branch name, PR number, base branch, mode
  cycle.json                 — Current cycle number and status
  snapshot.txt               — Latest snapshot output
  changelog.md               — Latest cycle changelog
  theme.txt                  — Session theme (from --theme flag)
  rate-limited               — Sentinel for rate limit backoff
  lathe.pid                  — Engine process ID
  logs/                      — Per-cycle agent logs and stream log
  history/                   — Archived cycle changelogs and snapshots
```

History lives inside `session/` (gitignored). The real audit trail is the squash merge commit on main.

**`lathe init` (re-init)** wipes everything in `.lathe/` except `refs/` and regenerates config. Old history, decisions, and session state are discarded — the new agent shouldn't be constrained by the old one's decisions.

**`lathe stop`** performs full teardown: kills the process tree (recursive, handles claude daemon clients), closes the PR (with `--delete-branch`), discards dirty working tree, checks out the base branch, deletes the local lathe branch, and wipes `session/`.

## Conventions

- `snapshot.sh` uses `-count=1` on test commands — snapshots must reflect real state, not cache.
- Skills files are project-specific, written by init. Not generic language references.
- Refs files (`.lathe/refs/`) hold reference material the agent needs to read to do its work. Loaded into every cycle's prompt alongside skills.
- The engine uses `--dangerously-skip-permissions --print` for runtime (non-interactive). Init uses `-p` with `--allowedTools` for controlled writes.
- `.lathe/session/` is gitignored entirely — never blocks branch switches, never committed.
- No fallback templates. Init succeeds or fails — the user should see and fix failures.
- Smart decisions (PRs, merges, CI fixes) belong in the agent prompt, not shell. The engine is plumbing.
- `gh` CLI is optional but enables PR/CI workflow. Without it, branch mode still works (agent pushes, no PR management).
- CI wait timeout is 5 minutes. Container pulls alone can take 1-2 min. If CI doesn't finish, the agent sees "timed out" in the snapshot and can address it.
- The cycle order is: snapshot → CI status → falsification → agent → archive → safety net → discover PR → CI wait → auto-merge. Each cycle is self-contained: do work, then land it. Teardown only closes work that didn't pass CI.
- Falsification failures appear in the snapshot and are treated by the agent as top-priority, like CI failures. The agent must never weaken `falsify.sh` to make it pass.

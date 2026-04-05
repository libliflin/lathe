# Lathe — Development Guide

## What This Is

Lathe is an autonomous code improvement loop. It points an AI agent at a repo and runs repeating cycles: snapshot the project state, pick the highest-value change, implement it, validate, commit.

The core value proposition: lathe init identifies the stakeholders of a project and encodes their needs into an agent that autonomously works on their behalf. Every cycle picks the single change that most improves a real person's experience. Git commits provide oversight.

## The Alignment Model

Lathe is an opinionated, automatic approach to the alignment problem for autonomous agents:

1. **Identify who the project serves** — `lathe init` reads the project and discovers its real stakeholders, their journeys, and where those needs conflict.
2. **Encode values** — init writes `agent.md` and skills that make the runtime agent stakeholder-aware. The agent picks the most valuable change each cycle, not the easiest or most obvious.
3. **Provide ongoing direction** — `--theme` lets the user state a purpose for a session ("get the CLI working end-to-end") that biases decisions without overriding the stakeholder framework.
4. **Detect drift** — every 5 cycles, a retro checks which stakeholders benefited and whether anyone is being neglected.
5. **Maintain oversight** — every cycle is a git commit with a changelog that names who benefits and how.

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
- `templates/*/priority-stack.md` — layer ordering injected into meta-prompt

## Key Principle

**The meta-prompt is the whole game.** It determines what init discovers, which determines what the runtime agent knows, which determines whether cycles create value. If you want the lathe to do something better, the change almost always belongs in `meta-prompt.md`.

## File Map

```
bin/lathe                        — CLI entrypoint (init, start, stop, status, logs)
engine/loop.sh                   — Cycle engine (snapshot, prompt assembly, commit, retro)
templates/
  meta-prompt.md                 — Instructions for the init agent
  interactive-preamble.md        — Interactive mode behavior (injected via {{INTERACTIVE}})
  generic/
    snapshot.sh                  — Generic state collection
    priority-stack.md            — Generic priority layers
  go/
    snapshot.sh                  — Go-specific state collection (build, test, vet, coverage)
    priority-stack.md            — Go-specific priority layers
  rust/
    snapshot.sh                  — Rust-specific state collection (cargo build, test, clippy)
    priority-stack.md            — Rust-specific priority layers
```

## Runtime State

```
.lathe/state/session.json    — Session state (branch, PR number, mode)
.lathe/state/theme.txt       — Current session theme (set by --theme, optional)
.lathe/state/decisions.md    — Permanent decisions the agent shouldn't revisit
.lathe/state/cycle.json      — Current cycle number and status
.lathe/state/snapshot.txt    — Latest project snapshot (includes CI status)
.lathe/state/changelog.md    — Latest cycle changelog
.lathe/state/history/        — Archived cycle snapshots and changelogs
.lathe/state/logs/           — Per-cycle agent logs and stream log
```

## Conventions

- `snapshot.sh` uses `-count=1` on test commands — snapshots must reflect real state, not cache.
- Skills files are project-specific, written by init. Not generic language references.
- Refs files (`.lathe/refs/`) hold reference material the agent needs to read to do its work. Loaded into every cycle's prompt alongside skills.
- The engine uses `--dangerously-skip-permissions --print` for runtime (non-interactive). Init uses `-p` with `--allowedTools` for controlled writes.
- State lives in `.lathe/state/` (gitignored). Config lives in `.lathe/` root (committed).
- No fallback templates. Init succeeds or fails — the user should see and fix failures.
- Smart decisions (PRs, merges, CI fixes) belong in the agent prompt, not shell. The engine is plumbing.
- `gh` CLI is optional but enables PR/CI workflow. Without it, branch mode still works (agent pushes, no PR management).
- CI wait timeout is 2 minutes. If CI doesn't finish, the agent sees "timed out" in the snapshot and can address it.

# Lathe — Development Guide

## What This Is

Lathe is an autonomous code improvement loop. It points three AI agents at a repo and runs repeating cycles: a goal-setter picks the highest-value change, a builder implements it, and a verifier checks the work.

The core value proposition: lathe init identifies the stakeholders of a project and encodes their needs into agents that autonomously work on their behalf. Git commits provide oversight.

## The Alignment Model

1. **Identify who the project serves** — `lathe init` reads the project and discovers its real stakeholders, their journeys, and where those needs conflict.
2. **Encode values** — init writes three behavioral docs (goal.md, builder.md, verifier.md) and skills that make the agents stakeholder-aware.
3. **Provide ongoing direction** — `--theme` lets the user state a purpose for a session that biases decisions without overriding the stakeholder framework.
4. **Maintain oversight** — every step is a git commit with a changelog that names who benefits and how.

## Architecture

**`lathe init`** — The alignment step. Runs three sequential AI calls, each producing a behavioral doc:
1. `meta-goal.md` → `.lathe/goal.md` — stakeholder map, tensions, ranking guidance. Values manifesto spliced in.
2. `meta-builder.md` → `.lathe/builder.md` — implementation quality, CI/PR workflow. Reads goal.md for alignment.
3. `meta-verifier.md` → `.lathe/verifier.md` — adversarial verification themes. Reads builder.md for failure modes.

Use `--agent=goal`, `--agent=builder`, or `--agent=verifier` to re-init just one role without touching the others.

**`lathe start`** — The execution loop. One cycle = goal-setter + 4 rounds of (builder + verifier). Each of the 9 steps follows identical plumbing: branch → snapshot → agent → safety net → PR → CI → merge → back to main. The engine is dumb plumbing; smart decisions live in the agent prompts.

**Templates** — Static mechanics only:
- `templates/meta-goal.md` — instructions for goal-setter init
- `templates/meta-builder.md` — instructions for builder init
- `templates/meta-verifier.md` — instructions for verifier init
- `templates/values-manifesto.md` — design intent, spliced into meta-goal.md via {{VALUES_MANIFESTO}}
- `templates/interactive-preamble.md` — additional instructions for `--interactive` mode
- `templates/*/snapshot.sh` — state collection scripts copied into `.lathe/` at init

## Key Principle

**The meta-prompts are the whole game.** They determine what init discovers, which determines what the runtime agents know, which determines whether cycles create value. The goal-setter's meta-prompt is the most important — it carries the values manifesto and stakeholder framework.

## File Map

```
bin/lathe                        — CLI entrypoint (init, start, stop, status, logs)
engine/loop.sh                   — Cycle engine orchestrator (goal + 4x builder/verifier)
engine/lib/
  process.sh                     — Process management (kill tree, find agent, is_running)
  state.sh                       — State helpers, session management, teardown
  ci.sh                          — CI polling, auto-merge, CI status collection
  agent.sh                       — Snapshot, prompt assembly, three agent functions
templates/
  meta-goal.md                   — Instructions for goal-setter init
  meta-builder.md                — Instructions for builder init
  meta-verifier.md               — Instructions for verifier init
  values-manifesto.md            — Design intent, spliced into meta-goal.md
  interactive-preamble.md        — Interactive mode behavior
  generic|go|rust/
    snapshot.sh                  — State collection per project type
  skill/
    SKILL.md                     — Global Claude Code skill, installed to ~/.claude/skills/lathe/
```

## State Model

**Config** — written by `lathe init`, survives stop, committed by the user:

```
.lathe/goal.md               — Goal-setter behavioral instructions, stakeholder map
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
  changelog.md               — Latest changelog
  theme.txt                  — Session theme (from --theme flag)
  rate-limited               — Sentinel for rate limit backoff
  lathe.pid                  — Engine process ID
  logs/                      — Per-step agent logs (cycle-001-goal.log, cycle-001-build-1.log, etc.)
  history/                   — Archived cycle changelogs and snapshots
  goal-history/              — Archived goals (goal-setter sees last 4)
```

History lives inside `session/` (gitignored). The real audit trail is the squash merge commits on main.

**`lathe init` (re-init)** wipes everything in `.lathe/` except `refs/` and regenerates all three behavioral docs. Use `--agent=X` to re-init just one role.

**`lathe stop`** performs full teardown: kills the process tree, closes the PR, discards dirty working tree, checks out the base branch, deletes the local lathe branch, and wipes `session/`.

## Conventions

- `snapshot.sh` uses `-count=1` on test commands — snapshots must reflect real state, not cache.
- Skills files are project-specific, written by init. Not generic language references.
- Refs files (`.lathe/refs/`) hold reference material the agents need. Loaded into every step's prompt alongside skills.
- The engine uses `--dangerously-skip-permissions --print` for runtime. Init uses `-p` with `--allowedTools` for controlled writes.
- `.lathe/session/` is gitignored entirely — never blocks branch switches, never committed.
- No fallback templates. Init succeeds or fails.
- Smart decisions belong in the agent prompts, not shell. The engine is plumbing.
- Each step follows identical plumbing: branch → snapshot → CI status → agent → archive → safety net → PR → CI wait → merge. Teardown works at any point.
- One cycle = 9 merge steps: 1 goal + 4 builder + 4 verifier. Each returns to main before the next starts.

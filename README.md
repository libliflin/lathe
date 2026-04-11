# Lathe

An autonomous code-improvement loop. Point it at a repo, and it runs repeating cycles: a goal-setter picks the highest-value change, a builder implements it, a verifier checks the work and tightens gaps.

Lathe is an opinionated take on the alignment problem for autonomous coding agents. Instead of asking one agent "what should you do?", `lathe init` identifies *who the project serves* and encodes their needs into three specialized agents. Every cycle asks: **which stakeholder's experience can I make noticeably better right now?**

## The Alignment Model

1. **Identify who the project serves.** `lathe init` reads the project deeply and discovers its real stakeholders — the actual people who use, build on, or operate this code — along with where their needs conflict.
2. **Encode values.** Init writes three behavioral docs (`goal.md`, `builder.md`, `verifier.md`) and skills that make the agents stakeholder-aware from the first cycle.
3. **Provide direction.** `--theme` lets you state a purpose for a session ("get the CLI working end-to-end") that biases decisions without overriding the stakeholder framework.
4. **Maintain oversight.** Every step is a git commit (and, in branch mode, a squash-merged PR) with a changelog naming who benefits and how.

## Three Roles

**Goal-setter** — The values agent. Reads the project snapshot, stakeholder map, and last 4 goals. Picks the single highest-value change. Commits a goal file. Doesn't implement — decides.

**Builder** — The implementer. Reads the goal and snapshot. Makes one focused change: implement, validate, commit. Follows the project's patterns.

**Verifier** — The adversarial reviewer. Reads the builder's diff against the goal. Asks: did the builder do what was asked? What could break? Commits real fixes — tests, edge cases, error handling.

## Two Phases

### `lathe init` — the alignment step

Runs three sequential AI calls, each producing a behavioral doc:

1. `goal.md` — stakeholder map, tensions, ranking guidance (values manifesto spliced in)
2. `builder.md` — implementation quality, CI/PR workflow (reads goal.md for alignment)
3. `verifier.md` — verification themes, failure modes (reads builder.md)

Also writes: `skills/*.md`, `refs/*.md` (optional), `alignment-summary.md`.

Use `--agent=goal`, `--agent=builder`, or `--agent=verifier` to re-init just one role.

### `lathe start` — the execution loop

One cycle = goal-setter + adaptive rounds of builder/verifier. The verifier decides when the goal is met (`VERDICT: PASS`) or sends feedback to the builder (`VERDICT: NEEDS_WORK`). Max 4 rounds per goal as a safety cap. Each step follows identical plumbing:

```
create branch → snapshot + CI status → agent works → safety net
  → discover PR → wait for CI → auto-merge → back to main
```

`--direct` flag: commit straight to the current branch, no PRs/CI integration.

## Quick Start

```bash
# Install (requires Go)
go install github.com/libliflin/lathe@latest

# Or download a binary from GitHub Releases:
# https://github.com/libliflin/lathe/releases

# Initialize (reads your project, generates stakeholder-aware agents)
cd your-project
lathe init                # autonomous
lathe init --interactive  # participate in stakeholder discovery

# Verify alignment
cat .lathe/alignment-summary.md

# Run
lathe start --cycles 10 --theme "harden edge cases"
lathe logs --follow

# Update to latest version
lathe update
```

## Workflow

- **Start with init, then review the diff.** Read `alignment-summary.md` first. Then review the full `.lathe/` diff. If something is off, use `--interactive` or `--agent=goal` to re-init just the goal-setter.
- **Run in short bursts.** A milestone usually takes 5–10 cycles.
- **Use themes for direction.** A theme biases the goal-setter without overriding stakeholder priorities.
- **Re-init after milestones.** Stakeholders don't change, but what they need does. Re-init wipes `.lathe/` except `refs/`.
- **Review and steer.** Read the commit log. If cycles feel like busywork, goal.md needs work.

## Commands

```bash
lathe init                              # generate all three agent docs
lathe init --interactive                # participate in stakeholder discovery
lathe init --agent=goal                 # re-init just the goal-setter
lathe init --agent=builder              # re-init just the builder
lathe init --agent=verifier             # re-init just the verifier

lathe start                             # run in background (branch mode + PRs + CI)
lathe start --cycles 10                 # stop after 10 cycles
lathe start --theme "harden edge cases" # give the session a purpose
lathe start --direct                    # commit straight to current branch
lathe start --tool amp                  # use AMP instead of Claude CLI

lathe status                            # current cycle, phase, branch, PR
lathe logs                              # latest step log
lathe logs --follow                     # stream logs live
lathe stop                              # full teardown

lathe update                            # self-update to latest release
lathe version                           # show current version
```

## Architecture

Single Go binary with all templates embedded via `go:embed`.

```
main.go                    — CLI entrypoint, path setup
init.go                    — lathe init (generates agent docs)
engine.go                  — lathe start/stop/status/logs
cycle.go                   — Cycle loop (goal + adaptive builder/verifier)
agent.go                   — Goal-setter, builder, verifier prompt assembly
invoke.go                  — Agent invocation, rate limit handling
update.go                  — Self-updater (checks GitHub Releases)
prompt.go                  — Shared prompt helpers (skills, refs, snapshot)
snapshot.go                — Project state collection
state.go                   — Session state management
ci.go                      — CI polling, auto-merge
safety.go                  — Safety net validation
process.go                 — Process management (kill tree, find agent)
shell.go                   — Shell execution helpers
embed.go                   — go:embed for templates/
templates/
  meta-goal.md             — Instructions for goal-setter init
  meta-builder.md          — Instructions for builder init
  meta-verifier.md         — Instructions for verifier init
  values-manifesto.md      — Design intent, spliced into meta-goal.md
  interactive-preamble.md  — Behavior injected when --interactive is used
  generic|go|rust/
    snapshot.sh            — State collection per project type
  skill/SKILL.md           — Global Claude Code skill
```

## State Model

**Config** — written by `lathe init`, survives stop, committed by the user:

```
.lathe/goal.md               — Goal-setter behavioral instructions
.lathe/builder.md            — Builder behavioral instructions
.lathe/verifier.md           — Verifier behavioral instructions
.lathe/alignment-summary.md  — Plain-English summary for the user
.lathe/snapshot.sh           — Project state collection script
.lathe/skills/*.md           — Project-specific knowledge
.lathe/refs/*.md             — User-curated reference material
```

**Session** — born on `lathe start`, dies on `lathe stop`, everything wiped:

```
.lathe/session/              — Gitignored. Ephemeral engine runtime.
  session.json               — Branch, PR, base, mode
  cycle.json                 — Current cycle number and phase
  snapshot.txt               — Latest snapshot
  changelog.md               — Latest changelog
  theme.txt                  — Session theme
  goal-history/              — Archived goals (goal-setter sees last 4)
  history/                   — Archived changelogs/snapshots
  logs/                      — Per-step agent logs
  lathe.pid                  — Engine PID
```

The real audit trail is the squash-merge commits on main.

## Security Model

The snapshot is fed directly into the LLM prompt, which makes everything fetched from GitHub a potential prompt-injection vector. The engine follows two rules:

1. **Only fetch structured fields** from `gh` — numbers, statuses, booleans, timestamps. Never free-text fields like PR titles, bodies, comments, or commit messages.
2. **Only list PRs authored by the current `gh` user.**

`lathe init` audits the repo's security posture (branch protection, `pull_request_target` workflows, public/private status) and flags weaknesses in the alignment summary.

## Requirements

- **Git**
- **Claude CLI** (`claude`) or **AMP** (`amp`)
- **`gh` CLI** (optional, enables PR/CI workflow)
- The relevant toolchain for your project

## Supported Project Types

Go, Rust, Python, Node, and Kubernetes are auto-detected. Any project works with the generic template.

## License

Apache 2.0

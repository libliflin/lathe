# Lathe

An autonomous code-improvement loop. Point it at a repo, and it runs repeating cycles: snapshot the project, pick the single highest-value change, implement it, validate it, commit it.

Lathe is an opinionated take on the alignment problem for autonomous coding agents. Instead of asking the agent "what should you do?", `lathe init` first identifies *who the project serves* and encodes their needs into a project-specific agent. Every cycle then asks the same question: **which stakeholder's experience can I make noticeably better right now, and where?**

## The Alignment Model

1. **Identify who the project serves.** `lathe init` reads the project deeply and discovers its real stakeholders — the actual people who use, build on, or operate this code — along with where their needs conflict.
2. **Encode values.** Init writes `agent.md`, skills, and an `alignment-summary.md` so the runtime agent is stakeholder-aware from the first cycle.
3. **Provide direction.** `--theme` lets you state a purpose for a session ("get the CLI working end-to-end") that biases decisions without overriding the stakeholder framework.
4. **Detect drift.** Every 5 cycles, a retro asks which stakeholders benefited and whether anyone is being neglected.
5. **Maintain oversight.** Every cycle is a git commit (and, in branch mode, a squash-merged PR) with a changelog naming who benefits and how.

## Two Phases

### `lathe init` — the alignment step

Detects project type (Go, Rust, Python, Node, Kubernetes, generic), copies the matching `snapshot.sh`, then calls an AI agent with `templates/meta-prompt.md`. The init agent reads the target project and writes:

- `.lathe/agent.md` — identity, stakeholder map, tensions, priorities, behavioral rules
- `.lathe/skills/*.md` — project-specific knowledge (testing conventions, architecture, build process)
- `.lathe/refs/*.md` (optional) — external reference material the agent needs to read each cycle
- `.lathe/alignment-summary.md` — plain-English summary of alignment decisions for the user to gut-check

`--interactive` makes init a conversation: it pauses at each discovery step (stakeholders, tensions, conventions) before writing. Default is autonomous.

If AI generation fails, init fails loudly. There are no fallback templates — a generic agent that does not understand the project's stakeholders is worse than no agent at all.

### `lathe start` — the execution loop

By default runs in **branch mode**: creates a `lathe/<timestamp>` session branch, and the agent manages PRs and CI through the `gh` CLI. The engine is dumb plumbing — it creates the branch, waits for CI (up to 5 minutes), collects the snapshot (including CI status), calls the agent, catches mistakes, and merges PRs when CI is green. All smart decisions (merge? fix CI? wait out an outage? create a new PR?) live in the agent prompt, not in shell.

Each cycle is self-contained:

```
create branch (if on base) → snapshot + CI status → agent implements one change
  → archive → safety net → discover PR → wait for CI → auto-merge if green
```

`--direct` flag preserves legacy behavior: commit straight to the current branch, no PRs, no CI integration.

## Quick Start

```bash
# Install
git clone https://github.com/libliflin/lathe.git
export PATH="$PATH:$(pwd)/lathe/bin"

# Initialize (reads your project, generates a stakeholder-aware agent)
cd your-project
lathe init                # autonomous
lathe init --interactive  # participate in stakeholder discovery

# Verify alignment — two ways, use both
cat .lathe/alignment-summary.md
# Then: ask your Claude Code agent (or any capable reviewer) to read the diff
# of .lathe/ and confirm the stakeholders, tensions, and claims match the
# project you actually have. Init writes a lot of files in one pass; a fresh
# pair of eyes on the diff catches subtle mismatches (wrong stakeholder
# priorities, claims that are documentation dressed as structure, bash bugs
# in falsify.sh) that the summary alone can hide.

# Run
lathe start --cycles 10 --theme "harden edge cases"
lathe logs --follow
```

## Workflow

Lathe is for **quick turning** — short, focused sessions that accomplish a specific milestone.

- **Start with init, then review the diff.** Read `.lathe/alignment-summary.md` first — it's the 30-second briefing. Then have a capable reviewer (Claude Code in the target project works well) read the full diff of `.lathe/` and confirm the stakeholders, tensions, and claims match the project you actually have. The summary is curated; the diff is the whole story, and subtle issues (a claim that checks documentation instead of structure, a bash bug in `falsify.sh`, a stakeholder you wouldn't have named) surface faster in review than by running cycles and waiting for them to manifest. If something is off, re-run with `--interactive`.
- **Run in short bursts.** A milestone usually takes 5–10 cycles. The lathe is most effective in its first ~10 cycles on a given focus area; after that it tends toward diminishing returns.
- **Use themes for direction.** A theme biases the pick step without overriding stakeholder priorities.
- **Re-init after milestones.** Once a phase of work is done (core implementation, test hardening, API stabilization), re-run `lathe init`. Stakeholders do not change, but what they need from the project does. Re-init wipes everything in `.lathe/` except `refs/` and starts fresh — the new agent should not be constrained by the old one's decisions.
- **Review and steer.** Read the commit log. If the lathe is making small polish changes (README tweaks, doc alignment) when there is real work outstanding, it is either done with the current phase or needs a theme to point at the next one.

## Commands

```bash
lathe init                              # auto-detect project type, generate agent
lathe init --interactive                # participate in stakeholder discovery
lathe init --type go                    # specify project type explicitly

lathe start                             # run in background (branch mode + PRs + CI)
lathe start --cycles 10                 # stop after 10 cycles
lathe start --theme "harden edge cases" # give the session a purpose
lathe start --direct                    # commit straight to current branch (no PR/CI)
lathe start --tool amp                  # use AMP instead of Claude CLI

lathe status                            # current cycle, branch, PR, agent process
lathe logs                              # latest cycle log
lathe logs --follow                     # stream logs live
lathe stop                              # full teardown: kill, close PR, return to base, wipe
```

## Architecture

```
bin/lathe                  — CLI entrypoint (init, start, stop, status, logs)
engine/loop.sh             — Cycle engine orchestrator (sources lib/, defines commands + cycle loop)
engine/lib/
  process.sh               — Process management (kill tree, find agent, is_running)
  state.sh                 — State helpers, session management, teardown
  ci.sh                    — CI polling, auto-merge, CI status collection
  agent.sh                 — Snapshot, falsification, prompt assembly, agent invocation
templates/
  meta-prompt.md           — Instructions for the init agent (the most important file in the project)
  values-manifesto.md      — The values manifesto, spliced into the meta-prompt so the init agent reads the *why* before the *how*
  interactive-preamble.md  — Behavior injected when --interactive is used
  generic|go|rust/
    snapshot.sh            — State collection per project type
  skill/SKILL.md           — Global Claude Code skill installed to ~/.claude/skills/lathe/
```

**The meta-prompt is the whole game.** It determines what init discovers, which determines what the runtime agent knows, which determines whether cycles create value. If you want the lathe to do something better, the change almost always belongs in `templates/meta-prompt.md`.

## State Model

There are exactly two categories of state under `.lathe/`:

**Config** — written by `lathe init`, survives stop, committed by the user:

```
.lathe/agent.md              — Behavioral instructions, stakeholder map, tensions, ranking guidance
.lathe/alignment-summary.md  — Plain-English summary for the user
.lathe/snapshot.sh           — Project state collection script
.lathe/claims.md             — Registry of load-bearing promises, per stakeholder
.lathe/falsify.sh            — Adversarial suite that tests the claims each cycle
.lathe/skills/*.md           — Project-specific knowledge
.lathe/refs/*.md             — User-curated reference material
```

## Falsification Suite

Lathe's structural defense against Goodhart's Law and metric-gaming. Init writes `claims.md` (the load-bearing promises this project makes to its stakeholders) and `falsify.sh` (an adversarial script that tries to break those promises). The engine runs `falsify.sh` every cycle as part of snapshot collection — a failing claim shows up under `## Falsification` in the snapshot and is treated by the agent the same way as a failing CI check: top priority, fix first.

Every fourth cycle is a **red-team cycle**: the engine injects instructions telling the agent to falsify rather than build. The agent picks one claim that has not been adversarially tested recently, tries to break it, and either fixes the break or strengthens the suite. This is the rhythm that prevents falsification work from being deferred indefinitely when the snapshot looks clean.

The agent can extend `claims.md` and `falsify.sh` as the project grows — when a new feature creates a new promise, the agent adds it to the registry and writes a falsification case for it. The suite is meant to grow with the project, not stay frozen at init.

**Session** — born on `lathe start`, dies on `lathe stop`, everything wiped:

```
.lathe/session/              — Gitignored. Ephemeral engine runtime.
  session.json               — Branch, PR, base, mode
  cycle.json                 — Current cycle number and status
  snapshot.txt               — Latest snapshot
  changelog.md               — Latest cycle changelog
  theme.txt                  — Session theme (--theme)
  rate-limited               — Sentinel for rate-limit backoff
  lathe.pid                  — Engine process ID
  logs/                      — Per-cycle agent logs and stream log
  history/                   — Archived changelogs/snapshots (feeds the retro)

.lathe/decisions.md          — Tracked. Agent-written permanent decisions. Wiped on stop.
```

The real audit trail is the squash-merge commit on main. History inside `session/` only exists to feed the retro every 5 cycles.

## Security Model

The snapshot is fed directly into the LLM prompt, which makes everything fetched from GitHub a potential prompt-injection vector. The engine follows two rules:

1. **Only fetch structured fields** from `gh` — numbers, statuses, booleans, timestamps. Never free-text fields like PR titles, bodies, comments, commit messages, or `displayTitle`.
2. **Only list PRs authored by the current `gh` user.**

`lathe init` is also instructed to audit the repo's security posture (branch protection, `pull_request_target` workflows, public/private status) and flag weaknesses in the alignment summary before cycles begin. The agent is only as trustworthy as the validation that runs against it.

## Conventions

- `snapshot.sh` uses `-count=1` on test commands — snapshots must reflect real state, not cache.
- Skills are project-specific. Do not write generic language references.
- Refs hold reference material the agent reads to do its work; they are loaded into every cycle's prompt alongside skills.
- The engine uses `--dangerously-skip-permissions --print` for runtime (non-interactive). Init uses `-p` with `--allowedTools` for controlled writes.
- `.lathe/session/` is gitignored entirely — never blocks branch switches, never committed.
- No fallback templates. Init succeeds or fails loudly.
- Smart decisions (PRs, merges, CI fixes) belong in the agent prompt, not shell. The engine is plumbing.
- `gh` CLI is optional but enables the PR/CI workflow. Without it, branch mode still works (agent pushes, no PR management).
- CI wait timeout is 5 minutes — container pulls alone can take 1–2 minutes. If CI does not finish, the agent sees `timeout` in the snapshot and can address it.

## Requirements

- **Bash 4+**
- **Python 3** (for state management)
- **Git**
- **Claude CLI** (`claude`) or **AMP** (`amp`)
- **`gh` CLI** (optional, enables PR/CI workflow)
- The relevant toolchain for your project (e.g., `go` for Go projects)

## Supported Project Types

Go, Rust, Python, Node, and Kubernetes are auto-detected. Any project works with the generic template. The difference is `snapshot.sh` — Go projects get build/test/vet/coverage collection, Rust gets `cargo build/test/clippy`, generic projects get file structure and git state.

## License

Apache 2.0

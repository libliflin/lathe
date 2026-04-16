# Lathe

An autonomous code-improvement loop. Point it at a repo, and it runs repeating cycles driven by real stakeholder experiences — not backlogs, not story points, not groomed lists of work.

Lathe is an opinionated take on the alignment problem for autonomous coding agents. Instead of asking "what should we build next?", lathe simulates real stakeholder experiences, discovers where the project fails them, and fixes the most important friction. Every cycle asks: **what's the worst experience a real stakeholder is having right now?**

## The Alignment Model

1. **Identify who the project serves.** `lathe init` reads the project and discovers its real stakeholders — the actual people who use, build on, or operate this code.
2. **Experience the project as they would.** Each cycle, a stakeholder simulation tries to use the project from a clean workspace, following their real journey. No insider knowledge. Black-box.
3. **Champion their needs.** A stakeholder champion watches the simulation, takes notes, and picks the single most impactful friction point — filtered through the session's theme and project scope.
4. **Build until it's fixed.** The builder and verifier loop with full autonomy to refactor, prototype, and experiment until the stakeholder's experience is genuinely better.
5. **Maintain oversight.** Every step is a git commit with a changelog. The real audit trail is the squash-merge commits on main.

## The Loop

**Stakeholder sim** — A simulated stakeholder tries to use the project. Clean workspace, no insider knowledge, following their real journey. Hits real friction. Reports what happened.

**Stakeholder champion** — Watches the simulation, understands the friction, keeps project scope and theme in mind. Picks the one thing that matters most. Translates stakeholder pain into an actionable goal. Friendly, empathetic, service-oriented — the stakeholder's advocate inside the development process.

**Builder** — The engineer. Has full autonomy over technical decisions — refactoring, tooling, prototyping, whatever it takes to fix the friction. Makes the tool to make the change easy, then makes the easy change. Owns the how, not the what.

**Verifier** — Two jobs. First: bridge between internal quality and external experience — did the builder actually fix the stakeholder's friction? Second: owns the non-negotiable floor — security, performance, reliability. The sim surfaces what to build; the verifier ensures it's built to a standard. The builder doesn't get to trade security for features or ship something 10x slower. Has empathy for both the builder's technical choices and the stakeholder's needs.

The builder and verifier loop until the friction is resolved. Cycles are no longer small one-off stories — they're as big as the problem requires.

## Where This Is Heading (Workshop)

The model above is aspirational. The current implementation still uses the older goal-setter/builder/verifier pattern (see Architecture below). Key open questions we're workshopping:

- **Stakeholder simulation design.** The sim IS the prioritization engine — there is no backlog, no priority matrix. The sim must cover functional use, performance, trust evaluation, competitive assessment, integration scenarios, and long-term reliability. Stakeholder experiences aren't just "try feature X and it breaks" — they include evaluating whether to trust this project, waiting too long for a result, or trying to integrate with the API. If the sim can't surface a problem, the problem doesn't get prioritized. (See [workshop doc](docs/next-session-prompt.md) for full breakdown.)
- **Champion role.** Relational, not analytical. The stakeholder's advocate inside the process.
- **Cycle scope.** Bigger cycles with more builder autonomy. What are the right safety caps?
- **No backlog.** Priority is discovered live each cycle. No grooming, no maintenance.

## Two Phases

### `lathe init` — the alignment step

Analyzes your project and generates behavioral docs that the runtime agents read every cycle. The binary contains meta-prompts (templates you never see) that tell an AI how to study your project and produce these files:

```
.lathe/goal.md      — Instructions for the goal-setter: who the project serves,
                      stakeholder tensions, how to rank work each cycle.
.lathe/builder.md   — Instructions for the builder: implementation quality,
                      CI/PR workflow, project-specific conventions.
.lathe/verifier.md  — Instructions for the verifier: what to check, verification
                      themes, failure modes.
```

Also writes: `skills/*.md` (project knowledge), `alignment-summary.md` (human-readable summary).

Each doc is generated in sequence — the builder's meta-prompt reads `goal.md` for alignment, the verifier's reads `builder.md` for failure modes. Use `--agent=goal` to re-generate just one.

### `lathe start` — the execution loop

Each cycle, the goal-setter reads `.lathe/goal.md` + the project snapshot and picks one change. It commits a per-cycle goal file that the builder reads. The builder implements it. The verifier checks the work and writes a verdict:

- `VERDICT: PASS` — goal met, advance to the next cycle
- `VERDICT: NEEDS_WORK` — issues remain, loop the builder with feedback

Max 4 rounds per goal as a safety cap. CI failures override PASS. Each step follows identical plumbing:

```
create branch → snapshot + CI status → agent works → safety net
  → push → discover PR → wait for CI → auto-merge → back to main
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

Single Go binary with all templates embedded via `go:embed`. Two layers of prompts:

**Meta-prompts** (embedded in the binary, used only during `lathe init`):
```
templates/meta-goal.md       — Tells an AI how to analyze the project and write goal.md
templates/meta-builder.md    — Same for builder.md
templates/meta-verifier.md   — Same for verifier.md
templates/values-manifesto.md — Design philosophy, spliced into meta-goal.md
```

**Behavioral docs** (generated by init, read by agents every cycle):
```
.lathe/goal.md               — Goal-setter reads this to decide what to work on
.lathe/builder.md            — Builder reads this to know how to implement
.lathe/verifier.md           — Verifier reads this to know what to check
```

**Source files:**
```
main.go         — CLI entrypoint       invoke.go   — Agent invocation, rate limits
init.go         — lathe init           update.go   — Self-updater
engine.go       — start/stop/status    prompt.go   — Prompt assembly helpers
cycle.go        — Cycle loop + verdict state.go    — Session state, teardown
agent.go        — Agent prompt builders ci.go       — CI polling, auto-merge
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

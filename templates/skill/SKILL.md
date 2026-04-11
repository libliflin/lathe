---
name: lathe
description: Knowledge about the Lathe autonomous code improvement system. Trigger when the user mentions lathe, .lathe directory, lathe cycles, lathe init, lathe start, goal.md, builder.md, verifier.md, lathe agent, lathe snapshot, reviewing lathe's work, checking what lathe did, evaluating lathe output, or when the current project has a .lathe/ directory and the user asks about autonomous changes, changelogs, or cycle history.
---

You have knowledge about Lathe, an autonomous code improvement loop. This skill helps you understand what lathe is, what the `.lathe/` directory in your project means, and how to assess whether lathe is doing good work.

## What Lathe Is

Lathe points three AI agents at a repo and runs repeating cycles. A **goal-setter** picks the highest-value change (stakeholder-first), a **builder** implements it, and a **verifier** checks the work and tightens gaps. The core idea: `lathe init` reads a project, identifies its real stakeholders, and encodes their needs into agents that autonomously work on their behalf.

It's a single Go binary with all templates embedded. Install with `go install github.com/libliflin/lathe@latest` or download from GitHub Releases. Self-updates via `lathe update`.

## What's In .lathe/

If your project has a `.lathe/` directory, lathe has been initialized on it. Here's what the files mean:

```
.lathe/
  goal.md               — Goal-setter instructions. Stakeholder map, tensions, how to
                          rank work. This is the "values brain" of the system. READ THIS
                          to understand what lathe is optimizing for.
  builder.md            — Builder instructions. Implementation quality, CI/PR workflow,
                          project-specific conventions.
  verifier.md           — Verifier instructions. Adversarial review themes, what to
                          check, how to tighten gaps.
  skills/*.md           — Project-specific knowledge (testing conventions, architecture).
  refs/*.md             — External reference material (language docs, standards).
  snapshot.sh           — Script that collects project state each cycle.
  alignment-summary.md  — Plain-English summary of alignment decisions for human review.
  session/              — Ephemeral engine runtime (gitignored, wiped on stop):
    session.json        — Current session (branch, PR number, mode)
    theme.txt           — Session purpose set by user via --theme
    cycle.json          — Current cycle number and phase
    snapshot.txt        — Latest snapshot output
    changelog.md        — Latest changelog (verifier writes VERDICT here)
    history/            — Archived cycle changelogs and snapshots
    goal-history/       — Archived goals (goal-setter sees last 4)
    logs/               — Per-step agent logs
```

## How a Cycle Works

One cycle = goal-setter + adaptive rounds of builder/verifier. The verifier decides when to advance.

1. Goal-setter reads snapshot + git log + last 4 goals, picks the highest-value change
2. Goal-setter commits goal, PR merges → back on main
3. Builder reads goal + snapshot (+ verifier feedback on round 2+), implements the change, commits
4. Builder's PR merges → back on main
5. Verifier reads builder's diff + goal, checks work, commits fixes/tests
6. Verifier writes changelog with structured template and a verdict:
   - `VERDICT: PASS` — goal is met, advance to the next goal
   - `VERDICT: NEEDS_WORK` — issues remain, loop builder again with feedback
7. On NEEDS_WORK: builder gets verifier's changelog as feedback, tries again (max 4 rounds)
8. On PASS (or max rounds reached): cycle complete, next goal-setter runs

## How to Review Lathe's Work

When asked to evaluate what lathe has done:

1. **Read the changelogs** in `.lathe/session/history/cycle-NNN/changelog.md` (while running) or check git log for the squash merge commits on main.
2. **Check git log** for the lathe's commits — are they coherent? Do they build on each other?
3. **Read goal.md** to understand what lathe is optimizing for, then judge whether the changes serve those stakeholders.
4. **Look at test results** — is the project in better shape than before?
5. **Check for drift** — is lathe stuck polishing low-value things, or is it advancing the project?

## How to Give Feedback About Lathe

If you're evaluating lathe as a tool (not just its output on your project):

- **Is goal.md good?** Does it identify real stakeholders? Are the tensions genuine? Does it give the goal-setter a clear framework for ranking work instead of a frozen layer ladder?
- **Are cycles delivering value?** Each cycle should make one person's experience noticeably better. If cycles feel like busywork, goal.md probably needs work.
- **Is the verifier catching real issues?** Or is it rubber-stamping? A verifier that only adds trivial tests isn't earning its keep. Is it using VERDICT correctly?
- **Is the snapshot useful?** Does snapshot.sh capture what the agents actually need to make good decisions?

## Commands

```
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

## Re-initializing

- `lathe init` — full re-init, regenerates all three agent docs (preserves `refs/`)
- `lathe init --agent=goal` — re-init only the goal-setter
- `lathe init --agent=builder` — re-init only the builder
- `lathe init --agent=verifier` — re-init only the verifier

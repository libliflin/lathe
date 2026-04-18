---
name: lathe
description: Knowledge about the Lathe autonomous code improvement system. Trigger when the user mentions lathe, .lathe directory, lathe cycles, lathe init, lathe start, champion.md, builder.md, verifier.md, lathe agent, lathe snapshot, reviewing lathe's work, checking what lathe did, evaluating lathe output, or when the current project has a .lathe/ directory and the user asks about autonomous changes, changelogs, or cycle history.
---

You have knowledge about Lathe, an autonomous code improvement loop. This skill helps you understand what lathe is, what the `.lathe/` directory in your project means, and how to assess whether lathe is doing good work.

## What Lathe Is

Lathe points three AI agents at a repo and runs repeating cycles. A **champion** picks the highest-value change from lived stakeholder experience, a **builder** implements it, and a **verifier** scrutinizes and tightens gaps. The core idea: `lathe init` reads a project, identifies its real stakeholders, and encodes their needs into agents that autonomously work on their behalf.

It's a single Go binary with all templates embedded. Install with `go install github.com/libliflin/lathe@latest` or download from GitHub Releases. Self-updates via `lathe update`.

## What's In .lathe/

If your project has a `.lathe/` directory, lathe has been initialized on it. Here's what the files mean:

```
.lathe/
  champion.md           — The champion's playbook (formerly goal.md). Stakeholder map,
                          tensions, emotional signals, how to rank, the output format.
                          This is the "values brain" of the system. READ THIS to understand
                          what lathe is optimizing for.
  brand.md              — The project's character (how it speaks). Loaded into every
                          runtime prompt as a tint on decisions.
  builder.md            — Builder instructions. Creative/synthesis posture, implementation
                          quality, CI/PR workflow, project-specific conventions.
  verifier.md           — Verifier instructions. Comparative/scrutinizing posture, the
                          verification playbook adapted to the project's shape.
  skills/*.md           — Project-specific knowledge (testing conventions, architecture,
                          stakeholder journeys).
  refs/*.md             — External reference material (language docs, standards).
  snapshot.sh           — Script that collects project state each cycle.
  alignment-summary.md  — Plain-English summary of alignment decisions for human review.
  session/              — Ephemeral engine runtime (gitignored, wiped on stop):
    session.json        — Current session (branch, PR number, mode)
    theme.txt           — Session purpose set by user via --theme
    cycle.json          — Current cycle number and phase
    snapshot.txt        — Latest snapshot output
    changelog.md        — Per-step output file; each agent writes their report here
    stale-prs.txt       — Context on orphan PRs across cycles
    error.md            — Written if the dialog hits the oscillation cap (human review)
    history/            — Archived per-cycle changelogs and snapshots
    champion-history/   — Archived champion reports (the champion sees last 4)
    logs/               — Per-step agent logs
```

## How a Cycle Works

One cycle = champion + dialog between builder and verifier. Convergence is the signal, not a verdict.

1. Champion reads snapshot + git log + last 4 cycles + stakeholder playbook; picks one stakeholder, becomes them, walks their first-encounter journey, writes a report.
2. Champion's report archives; builder reads it next.
3. Builder creates a PR implementing the change; CI runs; merge lands on main.
4. Verifier reads the builder's diff, runs the verification playbook, adds what's missing (tests, edges, error handling) and pushes.
5. Next round: builder reads what verifier added, refines or stands down. Verifier reads that, adds more or stands down.
6. Convergence = a round where neither commits substantively. Cycle advances.
7. Safety cap: 20 rounds without convergence → error state written to `.lathe/session/error.md`, engine halts for human review.

## How to Review Lathe's Work

When asked to evaluate what lathe has done:

1. **Read the champion-history archive** in `.lathe/session/champion-history/cycle-NNN.md` while the session is alive, or the squash-merge commit messages on main for completed cycles.
2. **Check git log** — are the commits coherent? Do they build on each other?
3. **Read champion.md** to understand what lathe is optimizing for, then judge whether the changes serve those stakeholders.
4. **Look at test results** — is the project in better shape than before?
5. **Check for drift** — is lathe stuck polishing low-value things, or is it advancing the project?

## How to Give Feedback About Lathe

If you're evaluating lathe as a tool (not just its output on your project):

- **Is champion.md good?** Does it identify real stakeholders? Are the tensions genuine? Does it give the champion a clear framework for ranking work instead of a frozen layer ladder?
- **Are cycles delivering value?** Each cycle should make one person's experience noticeably better. If cycles feel like busywork, champion.md probably needs work.
- **Is the verifier catching real issues?** Or is it rubber-stamping? A verifier that only adds trivial tests isn't earning its keep.
- **Is the snapshot useful?** Does snapshot.sh capture what the agents actually need to make good decisions?

## Commands

```
lathe init                              # generate all agent docs
lathe init --interactive                # participate in stakeholder discovery
lathe init --agent=champion             # re-init just the champion
lathe init --agent=brand                # re-init just the brand agent
lathe init --agent=builder              # re-init just the builder
lathe init --agent=verifier             # re-init just the verifier
lathe init --agent=snapshot             # re-init just the snapshot script

lathe start                             # run in background (branch mode + PRs + CI)
lathe start --cycles 10                 # stop after 10 cycles
lathe start --theme "harden edge cases" # give the session a purpose
lathe start --direct                    # commit straight to current branch
lathe start --tool amp                  # use AMP instead of Claude CLI

lathe status                            # current cycle, phase, branch, PR
lathe logs                              # latest step log
lathe logs --follow                     # stream logs live
lathe stop                              # full teardown

lathe dashboard start                   # machine-wide web dashboard
lathe dashboard stop

lathe update                            # self-update to latest release
lathe version                           # show current version
```

## Re-initializing

- `lathe init` — full re-init, regenerates all agent docs (preserves `refs/`)
- `lathe init --agent=champion` — re-init only the champion
- `lathe init --agent=brand` — re-init only the brand
- `lathe init --agent=builder` — re-init only the builder
- `lathe init --agent=verifier` — re-init only the verifier
- `lathe init --agent=snapshot` — re-init only the snapshot script

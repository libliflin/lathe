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
  agents/               — The three loop agents live here (champion, builder, verifier).
    champion.md         — The champion's playbook. Stakeholder map, tensions, emotional
                          signals, how to rank, the output format. This is the "values
                          brain" of the system. READ THIS to understand what lathe is
                          optimizing for.
    builder.md          — Builder instructions. Creative/synthesis posture, implementation
                          quality, CI/PR workflow, project-specific conventions.
    verifier.md         — Verifier instructions. Comparative/scrutinizing posture, the
                          verification playbook adapted to the project's shape.
  brand.md              — The project's character (how it speaks). A reference doc, loaded
                          into every runtime prompt as a tint. Not a loop agent — no
                          runtime step of its own.
  skills/*.md           — Project-specific knowledge (testing conventions, architecture,
                          stakeholder journeys).
  refs/*.md             — External reference material (language docs, standards).
  snapshot.sh           — Script that collects project state each cycle.
  alignment-summary.md  — Plain-English summary of alignment decisions for human review.
  session/              — Ephemeral engine runtime (gitignored, wiped on stop):
    session.json        — Current session (branch, PR number, mode)
    theme.txt           — Session purpose set by user via --theme
    cycle.json          — Current cycle's ID (timestamp) and phase
    snapshot.txt        — Latest snapshot output
    journey.md          — Champion's artifact for THIS cycle (stable during the cycle)
    whiteboard.md       — Shared scratchpad for THIS cycle (wiped at cycle boundary,
                          any loop agent may read/write/edit/wipe freely)
    stale-prs.txt       — Context on orphan PRs across cycles
    error.md            — Written if the dialog hits the oscillation cap (human review)
    history/<cycle-id>/ — Per-cycle archive: journey.md, whiteboard.md, snapshot.txt
                          (cycle-id is a timestamp like 20260418-083045, globally unique)
    logs/               — Per-step agent logs
```

## How a Cycle Works

One cycle = champion + dialog between builder and verifier. Convergence is the signal, not a verdict. Each cycle has a timestamp ID (`YYYYMMDD-HHMMSS`, UTC — globally unique, safe to reference in code comments).

1. Champion reads `agents/champion.md` + snapshot + git log + last 4 cycles' journeys; picks one stakeholder, becomes them, walks their first-encounter journey, writes `session/journey.md` (stable for this cycle).
2. Builder creates a PR implementing the journey's goal; CI runs; merge lands on main.
3. Verifier reads the builder's diff, runs the verification playbook, adds what's missing (tests, edges, error handling) and pushes.
4. Next round: builder reads what verifier added and the shared whiteboard, refines or stands down. Verifier reads builder's round and the whiteboard, adds more or stands down.
5. Convergence = a round where neither commits substantively. The engine measures this from `git rev-parse <base>` deltas. Cycle advances.
6. Safety cap: 20 rounds without convergence → error state written to `.lathe/session/error.md`, engine halts for human review.
7. At cycle boundary: whiteboard wipes clean, journey archives to `session/history/<cycle-id>/`, a new cycle starts.

The **whiteboard** (`session/whiteboard.md`) is a shared scratchpad — any loop agent may read, write, edit, append, or wipe it. No prescribed format. Engine wipes at cycle boundary. Non-substantive commits (changelog-only, gitignored, or under `.lathe/*`) don't count toward convergence.

## How to Review Lathe's Work

When asked to evaluate what lathe has done:

1. **Read the per-cycle archives** in `.lathe/session/history/<timestamp>/journey.md` to see what each champion asked for, or the squash-merge commit messages on main for shipped work.
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

---
name: lathe
description: Knowledge about the Lathe autonomous code improvement system. Trigger when the user mentions lathe, .lathe directory, lathe cycles, lathe init, lathe start, agent.md, lathe agent, lathe snapshot, reviewing lathe's work, checking what lathe did, evaluating lathe output, or when the current project has a .lathe/ directory and the user asks about autonomous changes, changelogs, or cycle history.
---

You have knowledge about Lathe, an autonomous code improvement loop. This skill helps you understand what lathe is, what the `.lathe/` directory in your project means, and how to assess whether lathe is doing good work.

## What Lathe Is

Lathe points an AI agent at a repo and runs repeating cycles: snapshot the project state, pick the single highest-value change, implement it, validate, commit. The core idea: `lathe init` reads a project, identifies its real stakeholders, and encodes their needs into an agent that autonomously works on their behalf.

## What's In .lathe/

If your project has a `.lathe/` directory, lathe has been initialized on it. Here's what the files mean:

```
.lathe/
  agent.md              — The runtime agent's instructions. Contains stakeholder map,
                          tensions, rules, and how the agent ranks per-cycle work.
                          This is the "brain" of the autonomous loop. READ THIS to
                          understand what lathe is optimizing for.
  skills/*.md           — Project-specific knowledge lathe uses each cycle (testing
                          conventions, architecture, build process).
  refs/*.md             — External reference material the agent reads each cycle
                          (language docs, standards, API contracts). Not process —
                          just material the agent needs to understand the domain.
  snapshot.sh           — Script that collects project state each cycle (build status,
                          test results, git state). Runs at the start of every cycle.
  alignment-summary.md  — Plain-English summary of alignment decisions for human review.
  session/              — Ephemeral engine runtime (gitignored, wiped on stop):
    session.json        — Current session (branch, PR number, mode)
    theme.txt           — Session purpose set by user via --theme
    cycle.json          — Current cycle number
    snapshot.txt        — Latest snapshot output
    changelog.md        — Latest cycle's changelog
    history/            — Archived cycle changelogs and snapshots
    logs/               — Per-cycle agent logs
```

## How a Cycle Works

1. Engine runs snapshot.sh to collect project state and CI status
2. Engine assembles prompt: agent.md + skills + refs + theme + snapshot + session context
3. Agent picks the single highest-value change and implements it
4. Agent commits, pushes, creates PR if needed
5. Engine archives the cycle (changelog + snapshot to history/)
6. Engine waits for CI on the PR, auto-merges if green, creates fresh branch for next cycle

## How to Review Lathe's Work

When asked to evaluate what lathe has done:

1. **Read the changelogs** in `.lathe/session/history/cycle-NNN/changelog.md` (while running) or check git log for the squash merge commits on main.
2. **Check git log** for the lathe's commits — are they coherent? Do they build on each other?
3. **Read agent.md** to understand what lathe was told to optimize for, then judge whether the changes actually serve those stakeholders.
4. **Look at test results** — is the project in better shape than before?
5. **Check for drift** — is lathe stuck polishing low-value things, or is it advancing the project?

## How to Give Feedback About Lathe

If you're evaluating lathe as a tool (not just its output on your project):

- **Is the agent.md good?** Does it identify real stakeholders? Are the tensions genuine? Does the falsification suite cover the load-bearing claims, so the agent has a real floor instead of a frozen layer ladder?
- **Are cycles delivering value?** Each cycle should make one person's experience noticeably better. If cycles feel like busywork, the agent.md probably needs work.
- **Is the snapshot useful?** Does snapshot.sh capture what the agent actually needs to make good decisions?
- **Are refs helping?** If the project needs external reference material, is it in `.lathe/refs/` and is the agent using it effectively?

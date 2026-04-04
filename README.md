# Lathe

An autonomous code improvement loop. Point it at a repo, tell it who the project serves, and it makes one high-value change per cycle -- snapshot, pick, implement, validate, commit.

## How It Works

Lathe has two phases:

**`lathe init`** reads your project and identifies its stakeholders -- the real people who use, build on, or operate this code. It maps where their needs conflict, assesses the project's maturity, and writes tailored instructions that encode these values into an autonomous agent.

**`lathe start`** runs the improvement loop. Each cycle: collect project state, pick the single change that most improves a stakeholder's experience, implement it, validate it, commit it. Every 5 cycles, a retrospective checks for drift -- is any stakeholder being neglected?

Git commits provide oversight. Every commit includes a changelog naming who benefits and how.

## Quick Start

```bash
# Install
git clone https://github.com/libliflin/lathe.git
export PATH="$PATH:$(pwd)/lathe/bin"

# Initialize (reads your project, generates stakeholder-aware agent)
cd your-project
lathe init

# Review the alignment summary
cat .lathe/alignment-summary.md

# Run 10 cycles
lathe start --cycles 10
lathe logs --follow
```

## Workflow

Lathe is for quick turning -- short, focused sessions that accomplish a specific milestone. Here's how to get the most out of it:

**Start with init.** Init reads your project and writes the agent instructions. Review `.lathe/alignment-summary.md` to verify it understood your stakeholders. If something's off, run `lathe init --interactive` to participate in the discovery process.

**Run in short bursts.** A milestone usually takes 5-10 cycles. Start with `--cycles 10` and review what happened. The lathe is most effective in its first ~10 cycles on a given focus area -- after that it tends toward diminishing returns.

**Use themes for direction.** If you know what matters today, say so:

```bash
lathe start --cycles 10 --theme "get the CLI working end-to-end"
```

The theme biases the agent's decisions without overriding its stakeholder framework. Without a theme, the agent uses its own judgment.

**Re-init after milestones.** Once the lathe has accomplished a phase of work (core implementation, test hardening, API stabilization), run `lathe init` again. Init reassesses the project's maturity and writes fresh guidance. The stakeholders don't change, but what they need from the project does.

**Review and steer.** Read the commit log. If the lathe is making small polish changes (README tweaks, doc alignment) instead of substantive work, it's either done with the current phase or needs a theme to point it at the next one.

## Commands

```bash
lathe init                              # auto-detect project type, generate agent
lathe init --interactive                # participate in stakeholder discovery
lathe init --type go                    # specify project type explicitly

lathe start                             # run in background
lathe start --cycles 10                 # stop after 10 cycles
lathe start --theme "harden edge cases" # give the session a purpose
lathe start --tool amp                  # use AMP instead of Claude CLI

lathe status                            # current cycle state
lathe logs                              # latest cycle log
lathe logs --follow                     # stream logs live
lathe stop                              # stop the loop
```

## What Init Creates

```
.lathe/
  agent.md              -- stakeholder map, priorities, behavioral instructions
  skills/*.md           -- project-specific knowledge (testing, architecture, build)
  alignment-summary.md  -- plain-English summary of alignment decisions
  snapshot.sh           -- state collection script (copied from template)
  state/                -- runtime state, logs, cycle history (gitignored)
```

Init writes all of this by reading your project -- there are no generic templates. If init fails, it fails loudly so you can fix the issue rather than running with a generic agent that doesn't understand your project.

## Requirements

- **Bash 4+**
- **Python 3** (for state management)
- **Git**
- **Claude CLI** (`claude`) or **AMP** (`amp`)
- The relevant toolchain for your project (e.g., `go` for Go projects)

## Supported Project Types

Go, Python, Node, Rust, and Kubernetes are auto-detected. Any project works with the generic template. The difference is the snapshot script -- Go projects get build/test/vet collection, generic projects get file structure and git state.

## License

Apache 2.0

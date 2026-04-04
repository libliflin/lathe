# Lathe

An autonomous code improvement loop. Lathe continuously analyzes, improves, validates, and commits changes to your project using Claude AI -- like a lathe shaping material, each cycle takes another pass to make the code better.

## What It Does

Lathe runs a repeating cycle:

1. **Snapshot** -- collects project state (build output, tests, lint, code structure)
2. **Analyze** -- Claude AI identifies the single highest-value improvement
3. **Implement** -- makes one focused change per cycle
4. **Validate** -- confirms the change works (builds, tests pass)
5. **Commit** -- documents the improvement and commits it

Changes follow a strict priority stack: things must compile before tests are fixed, tests must pass before style is cleaned up, etc. Every 5 cycles, a retrospective reviews recent changes for patterns.

## Requirements

- **Bash 4+**
- **Python 3** (for JSON state management)
- **Git**
- **Claude CLI** (`claude`) or **AMP** (`amp`) -- the AI agent that performs improvements

For project-specific templates, you'll also need the relevant toolchain (e.g., `go` for Go projects).

## Supported Project Types

- **Go** -- full template with testing and quality skills
- **Generic** -- works with any project
- Python, Node, Rust, Kubernetes detection is built in (uses generic template)

## Install

```bash
git clone https://github.com/libliflin/lathe.git
export PATH="$PATH:$(pwd)/lathe/bin"
```

Or add `lathe/bin` to your `PATH` permanently.

## Usage

```bash
# Initialize in your project directory
cd your-project
lathe init                      # auto-detects project type
lathe init --type go            # or specify explicitly

# Run the improvement loop
lathe start                     # runs in background
lathe start --cycles 10         # stop after 10 cycles
lathe start --tool amp          # use AMP instead of Claude CLI

# Monitor and control
lathe status                    # current cycle state
lathe logs                      # view cycle logs
lathe logs --follow             # stream logs live
lathe stop                      # stop the loop
```

### What Gets Created

`lathe init` creates a `.lathe/` directory in your project with:

- `agent.md` -- agent instructions tailored to your project type
- `snapshot.sh` -- script that collects project state each cycle
- `skills/*.md` -- domain-specific guidance (e.g., Go testing patterns)
- `state/` -- runtime state, logs, and cycle history

## License

Apache 2.0

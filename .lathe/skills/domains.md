# Domain Boundaries

Lathe operates across several distinct domains. Bugs that look like one domain's problem are often actually another domain's problem.

## Domain Map

### Go Language / Standard Library
**What it covers:** Language semantics, stdlib APIs (os, filepath, strings, time, encoding/json), goroutines, channels, embed.
**Authoritative source:** pkg.go.dev, the Go spec.
**Where confusion arises:** Cross-platform path handling (filepath.Join vs. hardcoded slashes), process management (setsid on Unix vs. Windows), file permission bits on Windows. The repo has `setsid_unix.go` and `setsid_windows.go` for platform-specific process group management — this boundary is real and must be maintained.

### LLM CLI Interface (claude / amp)
**What it covers:** How lathe invokes the `claude` and `amp` CLI tools, what flags they accept, how they signal rate limits, how they handle stdin/stdout.
**Authoritative source:** The claude CLI and amp CLI documentation / behavior. Lathe does not control these tools.
**Where confusion arises:** Rate limit detection is text-matching ("You've hit your limit") — fragile by nature. Exit codes from LLM CLIs are not always meaningful. The `--dangerously-skip-permissions --print` flags for runtime vs. `--allowedTools` for init are distinct modes with different safety models.

### GitHub / gh CLI
**What it covers:** PR creation, CI status polling, auto-merge, branch protection. Lathe uses the `gh` CLI for all GitHub interaction.
**Authoritative source:** gh CLI docs, GitHub API docs.
**Where confusion arises:** Prompt injection — PR titles, bodies, comments, and commit messages are free text and can contain instructions. The engine only reads structured fields (numbers, statuses, booleans) from gh output. Never pass free-text GitHub content into agent prompts.

### Agent Prompt Design
**What it covers:** How LLMs read and act on meta-prompts and behavioral docs. What makes a good goal file. What signals agents respond to.
**Authoritative source:** Empirical — run init, read the output, see if the agents behave correctly. No formal spec.
**Where confusion arises:** Agents reading init-time observations as permanent facts ("CI doesn't exist yet") rather than reading snapshot state. Agents inventing priority ladders when none was specified. Agents encoding project state into behavioral docs instead of teaching themselves to read state. These are prompt design bugs, not Go bugs.

### Git
**What it covers:** Branch management, commits, pushes, working tree state. Lathe uses git extensively for its commit-as-audit-trail model.
**Authoritative source:** git documentation.
**Where confusion arises:** The engine creates and deletes branches, discards working tree changes on stop, and squash-merges via GitHub. Code that runs git commands must handle the case where the working tree is dirty (agent left uncommitted changes) or where the branch doesn't exist on remote yet.

### Shell / OS Process Model
**What it covers:** How lathe forks background processes, how it kills process trees, how snapshot.sh executes.
**Authoritative source:** POSIX for Unix behavior, Windows process model for Windows.
**Where confusion arises:** The background engine (`lathe _run`) must survive its parent exiting. This is why setsid is used on Unix. Process tree killing on stop must kill the agent subprocess, not just the engine. `snapshot.sh` is bash and must work on both macOS (where `timeout` may not exist without gtimeout) and Linux.

## Common Misattributions

"The snapshot is truncated" → usually a snapshot.sh problem (too verbose), not an engine problem.
"The agent did something wrong" → usually a meta-prompt problem, not agent.go.
"CI is failing on Linux but not macOS" → usually a shell compatibility issue in snapshot.sh or a test that assumes macOS paths.
"The goal-setter picked a generic goal" → usually a meta-goal.md problem, not engine code.
"Rate limit handling seems flaky" → invoke.go does text-matching on CLI output — the fragility is structural, the fix is either a more reliable signal from the CLI or a different detection strategy.

# Stakeholder Sim — Interface Spec (for handoff)

> **Status (2026-04-17): lathe is no longer planning to consume this.** The external-sim project was spun out and went elsewhere. Lathe committed to an in-agent model instead: the customer champion (the role formerly called goal-setter) uses the project directly each cycle. This spec is kept as reference for the external project, not as a forward plan for lathe.

## What This Is

An interface definition for a stakeholder simulation system. Lathe will be a consumer of this system, but doesn't own the implementation. Another project should be able to pick this up and build it independently.

## The Pitch

You have heard of AI coworkers. Have you considered AI customers?

Stakeholder sim gives agent systems synthetic customers — simulated stakeholders dropped into realistic environments, running real scenarios, narrating their experience as they go. Continuous synthetic experimentation for product development, like what Apple/Amazon/Netflix do at scale, available to any project.

## The Hypothesis

Claude (or any sufficiently empathetic LLM) is a good enough role player that a well-prompted agent in an honest environment produces experience data that correlates with real stakeholder behavior. Not perfect — but better than reading code and guessing what matters.

## The Interface

A consumer calls the sim with three inputs and gets one output.

### Inputs

**1. Environment spec**

What the stakeholder's world looks like. Composable layers — a consumer picks the combination that matches their stakeholder.

```
environment:
  base: ubuntu-24.04
  language: rust-1.78
  editor: neovim-lsp
  project: medium-sized-cli-app
  extras:
    - cargo-watch
    - just
```

The sim project owns the template library and orchestration (Docker, QEMU, whatever). Consumers reference templates by name. The sim spins up the environment, runs the scenario, tears it down.

**2. Scenario**

What the stakeholder is trying to do. A situation with identity, goal, stakes, and exit condition.

```
scenario:
  identity: "Rust developer, 3 years experience, comfortable with cargo and traits, skeptical of new dependencies"
  goal: "Add this library to my existing CLI project to replace hand-rolled error handling"
  stakes: "If this takes more than 30 minutes or the API is confusing, I'll stick with what I have"
  entry_point: "https://github.com/example/repo"  # or a package registry, docs site, etc.
```

The consumer generates this. In lathe's case, the champion writes it. Other consumers might generate it from user research, product specs, whatever.

**3. Emotional signal definition**

What "good" feels like for this project's stakeholders. Derived by the consumer from their stakeholder map.

```
signal:
  tracking: "confidence and predictability"
  good: "I don't have to think about this. Types are correct, docs match behavior, no surprises."
  bad: "I'm guessing. The API surprised me. I had to read source to understand the docs."
  report_on: "Where did you feel certain? Where did you feel uncertain? Would you trust this in production?"
```

Different projects define different signals. A dev tool tracks excitement. A library tracks confidence. A consumer app tracks delight. The sim doesn't judge — it tracks whatever the consumer asks it to track.

### Output

**Experience log** — a time-stamped narrative stream from the sim agent.

```
[00:00] Starting. Reading the GitHub page. README is short, has a quickstart section.
[00:45] `cargo add` worked. Good sign.
[01:30] Looking at the example... this type signature is dense. Not sure what the lifetime parameter does here.
[02:15] Tried to compile the example. Error message actually told me what to fix. That's nice.
[03:00] Modified it for my project. The builder pattern feels natural.
[04:20] Hit a confusing edge case with async. Docs don't mention this. Reading source.
[05:10] Found it. The source is clean but I shouldn't have had to go there.
[06:00] It works. I'd use this. I wouldn't mass-recommend it yet — the async story needs docs.
[SIGNAL] Confidence was high through setup, dipped at the async edge case, recovered when source was readable. Would trust for sync use cases. Wouldn't trust async without more docs.
```

That's it. The consumer decides what to do with the log. Lathe feeds it to a champion. A CI pipeline might parse the signal summary. A dashboard might track signal trends over time.

## What the Sim Project Owns

- Environment template library and orchestration
- Scenario execution runtime (drop agent into environment, collect log)
- Log format specification
- The agent prompt that makes the sim narrate faithfully and track the requested signal

## What the Sim Project Does NOT Own

- What consumers do with the log
- How scenarios are generated (that's the consumer's domain knowledge)
- How emotional signals are derived (that's the consumer's stakeholder understanding)
- Prioritization, goal-setting, building, verifying — none of that

## Design Principles

- **The environment is the honesty mechanism.** A real workspace with no insider knowledge. The sim can't cheat if it genuinely starts from the entry point in a clean environment.
- **Emotional signal is project-specific, not universal.** The meta-sim doesn't assume excitement is good or friction is bad. The consumer defines what matters.
- **Scenarios are disposable.** Generated fresh, run once, data extracted, environment torn down. No state between runs unless the consumer explicitly designs for it.
- **The sim narrates continuously, not retrospectively.** The emotional arc is captured live, not reconstructed from memory.
- **Composable environments, not bespoke ones.** Most of the environment is reusable across projects. The project-specific layer is thin.

## Open Questions (for the implementing project)

- Docker vs QEMU vs something else for isolation? QEMU has a better isolation story, Docker is simpler. The interface doesn't care.
- How does the sim agent actually execute? Claude Code in the environment? A custom harness? The interface just needs the log back.
- Template distribution — git repo of Dockerfiles? A registry? OCI images?
- How rich does the environment template library need to be at launch? Start with 3-5 common stacks or try to be comprehensive?
- Cost management — spinning up VMs per cycle adds up. Caching, pooling, or just accepting the cost?

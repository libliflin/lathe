# Next Session: Stakeholder Simulation Model

> **Status (2026-04-17): superseded.** The two-agent split described below (sim + champion) was collapsed into a single **customer champion** role that *is* the goal-setter. The champion picks one stakeholder each cycle, uses the project as them directly, and names the goal from lived experience. No separate sim agent. No external sim system (the spun-out external project went elsewhere). See `templates/meta-goal.md` and the README for the current model. This doc is kept as workshop history — the open questions on cycle scope, projection, rotation, and sim-vs-verifier separation of concerns are still relevant.

## Context

Lathe is an autonomous code improvement loop. We've been through several iterations:

1. **Sprint agents** — failed. Sprints spent 80% of time on process improvement (grooming, SMART stories, adding more process). Naval-gazing attractor.
2. **Goal-setter / builder / verifier** — current implementation. Better, but two failure modes:
   - **Progress theater** — goal-setter picks easy busywork instead of hard important work
   - **Stale prompts** — init encodes project state ("X is aspirational") that becomes false within a few cycles, requiring manual re-init every ~10 cycles
3. **Document-driven goals** — explored and rejected. Maintaining a shared document that represents project state becomes backlog grooming by another name. 80% of the work becomes maintaining the artifact, not improving the project.

We arrived at a new model: **stakeholder simulation**.

## The Model

Instead of an agent reading code and deciding what to work on, simulate a real stakeholder trying to use the project. The friction they hit IS the priority, discovered live, not curated.

**Stakeholder sim** — A simulated stakeholder in a clean workspace, no insider knowledge, following their journey faithfully. Tries to do the thing they'd actually try to do. Reports what happened — what worked, what didn't, where they got stuck.

**Stakeholder champion** (replaces goal-setter) — Watches the simulation. Takes notes. Keeps project scope and theme in mind. Picks the single most impactful friction point. Translates stakeholder pain into an actionable goal. Service-oriented — Chick-fil-A style, the stakeholder's advocate.

**Builder** — Full autonomy over technical decisions. Bigger scope than before — cycles are as large as the problem requires, not one-off stories. YAGNI/XP ethos: make the tool to make the change easy, then make the easy change. Owns the how entirely.

**Verifier** — Two jobs. First, bridge between internal quality and external experience — did the builder actually fix the stakeholder's friction? Has empathy for both sides. Second, owns the non-negotiable floor: security, performance, reliability. The sim surfaces what to build; the verifier ensures it's built to a standard. The builder doesn't get to trade security for features. The verifier won't let it through. Can say "you fixed the friction but introduced an injection vulnerability" or "this works but it's 10x slower than before — NEEDS_WORK."

### Why This Might Work

- **No backlog to groom.** Priority is discovered live each cycle. No artifact to maintain.
- **No stale state claims.** The sim reveals actual project state by use, not by inspection. Nobody writes "error handling is aspirational" — the stakeholder tries to trigger an error and reports what happened.
- **Progress theater is harder.** You can't fake stakeholder friction. The sim either hits a wall or it doesn't.
- **Builder gets real autonomy.** Instead of small prescribed stories, the builder has room to refactor, prototype, and experiment — because the goal is "fix this experience" not "add this line of code."
- **Calibration-free.** Doesn't matter if the project is a compiler or a landing page. The sim tries to use it and reports what happens. No assumptions about what's "hard" or "easy."

## Open Questions to Workshop

### 1. Stakeholder Simulation Mechanics (Critical — this is the prioritization engine)
The sim is doing double duty: it's both the quality signal AND the prioritization mechanism. If the sim is shallow, the whole system produces shallow work. This is where most of the design effort belongs.

How does the sim actually work?
- Init generates persona scripts? A library of stakeholder journeys?
- Does the sim get a fresh one each cycle, or does init create a set that rotate?
- How detailed are the scripts? "Try to compile a hello world" vs "You are a systems programmer evaluating this compiler for a new embedded project"?
- How do we keep it honest? If the sim has access to insider knowledge it stops being a real stakeholder test.
- Clean workspace — literally a fresh clone? A docker container? Just "pretend you've never seen this code"?
- How rich is the persona library? It needs to cover functional use, performance evaluation, trust assessment, competitive evaluation, long-term reliability, integration scenarios — because these are all real stakeholder experiences and the sim is the only way priorities get discovered.
- How do we avoid the sim itself becoming a grooming exercise? If writing and maintaining persona scripts takes 80% of the time, we've recreated the backlog problem in a different shape.

### 2. Champion Role Design
The champion is relational, not analytical. Different from the current goal-setter.
- Does the champion interact with the sim in real-time, or review a report after?
- How does the champion balance "stakeholder wants the world" vs "here's our project scope and theme"?
- The champion needs to be empathetic to the stakeholder but pragmatic about what the builder can accomplish in one cycle. How big is "one cycle" now?
- Does the champion write the goal differently than the current goal-setter? The current format is "what/who/why" — does it need to carry the stakeholder's experience narrative?

### 3. Cycle Scope and Safety
Cycles are bigger now. More autonomy = more risk.
- What are the right safety caps? Current max is 4 builder/verifier rounds.
- Should cycles have a token/cost budget instead of a round count?
- The "4 cycles and revert" pattern: if the goal can't be achieved, revert the goal but keep the code improvements. How does the engine handle partial progress?
- Does the verifier need to check more frequently, or is the current loop sufficient?

### 4. The Projection Problem
Agents can only be as good as the view of reality they receive. A prompt is a fixed-size window.
- The stakeholder sim produces a narrative of their experience — that's naturally bounded (one session, one journey).
- The champion compresses that into a goal — also bounded.
- The builder still needs to understand the codebase — that's the existing snapshot + skills problem, unchanged.
- Does this model actually help with the projection problem, or just move it?

### 5. Multiple Stakeholders
Init identifies multiple stakeholders. Each cycle simulates one.
- How do we rotate? Random? Round-robin? Champion picks based on who's been neglected?
- The current goal-setter sees its last 4 goals to avoid repeating itself. The champion would need to see which stakeholders have been served recently.
- Some stakeholders have overlapping journeys — fixing friction for one might fix it for another. How does the champion account for this?

### 6. Separation of Concerns: Sim vs Verifier
The sim and the verifier own different parts of quality.

**Sim + champion own prioritization.** What to build. What friction to fix. Purely experiential — a stakeholder tries to use the project and hits real problems. The sim doesn't need to be contorted into "the security evaluator persona" or "the performance benchmarker persona." It stays focused on what it's good at: experiencing the project as a real user.

**Verifier owns the non-negotiable floor.** Security, performance, reliability. These aren't prioritized by stakeholder experience — they're standards that every cycle's output must meet. The verifier sees every diff and blocks work that trades security for features or introduces performance regressions. CVE-style thinking: likelihood of compromise is weighed heavily. A SQL injection on a public form blocks the cycle. A theoretical race condition in an internal tool with three users is noted but doesn't block.

**Why this split works:**
- The sim stays clean and experiential. No awkward "pretend you're a pentester" personas.
- Security and performance aren't deprioritized — they're enforced on every cycle, not waiting their turn in a priority queue.
- The builder knows the rules: fix the stakeholder's friction, but you can't ship something insecure or slow to do it.
- The verifier's empathy role is richer: it protects stakeholders (did the friction get fixed?), protects the codebase (security, performance, reliability), and respects the builder (good structural choices even when the goal isn't met yet).

**Resolved: the meta-verifier derives the floor from stakeholders and project scope.** The chain is: project → stakeholders → risk profile → verifier standards. The meta-verifier (init template) reads the stakeholders and asks: what are they risking by depending on this project?

- Library that other apps depend on → API stability, semver, input validation, no breaking changes
- Internal data pipeline someone can restart → correctness of output, less concern about graceful recovery
- Government contractor tool → SBOM, dependency auditing, CVE scanning, compliance
- Public-facing web service → injection prevention, rate limiting, data privacy

This means the verifier's behavioral doc stays relevant longer than current — "this project serves government contractors who need supply chain accountability" doesn't go stale the way "error handling is aspirational" does. The risk profile is tied to who the stakeholders are, not where the project is today.

## Constraints (Unchanged)

- Single Go binary, templates embedded via go:embed
- No external dependencies (no databases, no vector stores)
- Simplicity is a feature — lathe should be `lathe init && lathe start`
- Smart decisions in prompts, dumb plumbing in engine
- Empowerment over cages — give agents better information, don't restrict them

## Current Implementation State

The golang rewrite with builder/verifier is not yet solid. Before implementing the stakeholder sim model, we need:
1. Agents that know current, up-to-date project state (not stale init claims)
2. Agent prompts that don't encode state
3. Stable builder/verifier loop

The stakeholder sim is the direction. Getting the foundation right is the immediate work.

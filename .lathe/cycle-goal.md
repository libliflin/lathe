# Goal: Add a stakeholder sim step before the goal-setter

## What to change

Add a lightweight stakeholder sim phase to each lathe cycle, running before the goal-setter. A sim agent reads `goal.md` to understand the stakeholders, picks one, simulates their first encounter with the project using the current snapshot, and writes a friction report to `.lathe/session/friction.md`. The goal-setter then reads `friction.md` alongside the snapshot and picks which friction to fix.

### Concrete scope

1. **`agent.go`** — add `runSim(cycle int, tool string) error`. The sim prompt:
   - Reads `goal.md` (stakeholder descriptions)
   - Reads the snapshot
   - Picks one stakeholder for this cycle
   - Simulates their experience: what would they try to do right now? what would work? what would fail or confuse them?
   - Writes `.lathe/session/friction.md` — a brief, honest narrative (not analysis) of what the stakeholder encountered

2. **`cycle.go`** — call `runSim` before `runGoalSetter` in `runCycle`. The friction report is ephemeral per cycle; no archiving needed for now.

3. **`prompt.go`** — include `friction.md` in the goal-setter's prompt context (after the snapshot, before goal history). Label it clearly: "# Stakeholder Friction Report (this cycle)".

4. **Paths** — define `frictionFile` alongside the other session path vars in `main.go` (or wherever global paths live).

### What the sim does NOT do

- No real infrastructure (no docker, no fresh clone, no shell execution)
- No new init step or template needed — the sim reads the existing `goal.md` stakeholder map
- No new CLI flags
- The sim is an in-context simulation: the agent imagines the stakeholder's experience based on the snapshot and its own knowledge of the project

## Which stakeholder this helps

**Lathe users** running `lathe start` — people trusting the loop to make their project genuinely better. Right now the goal-setter picks goals by reading code and reasoning analytically. That mode is prone to progress theater: it finds things to polish, not things that actually hurt. The sim gives the goal-setter a different kind of signal: "a real person hit this wall" vs "this could theoretically be improved." The change makes every cycle's goal more likely to matter to someone real.

**The future stakeholder sim design** — `docs/next-session-prompt.md` and `docs/stakeholder-sim-interface.md` describe a richer model that this project is building toward. Implementing the sim step for lathe itself is a prototype of that model in its simplest form: no external infrastructure, no interface spec plumbing, just the core idea running. This grounds the design work in something concrete and reveals where the open questions become real problems.

## Why now

The build is clean. Tests pass. The engine is stable. The current model works but the goal-setter is operating on analysis alone. The theme "be the change you want to be" is explicit: lathe should adopt the pattern it's been designing for other projects.

The design in `docs/` is rich but hasn't touched the engine yet. This is the minimal viable version — small enough for one cycle, meaningful enough to validate the core idea. If the sim produces genuinely better goals, that's the signal to invest in the richer interface. If it produces noise, that's the signal to redesign the sim prompt before building infrastructure around it.

The change is contained: two new function bodies, one new path var, one new section in the goal-setter's prompt. No API changes, no template changes, no init changes. The builder can do this in one cycle.

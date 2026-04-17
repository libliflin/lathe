# values

*A manifesto on value-driven development. Lathe's design derives from this document — read it before writing agent docs. Source: https://libliflin.github.io/values/*

## The settled question

Agents can figure out how to build things. That question is answered.

When someone types "build me a SaaS landing page with Stripe checkout" into Bolt, they aren't writing a spec. They're stating a goal and trusting the system to derive the implementation. And it works, millions of times a day, across every zero-shot agentic tool on the market. Bolt, Lovable, Cursor, Claude Code. You describe what you want, not how to build it, and the system reads the codebase, picks the files, chooses the approach, writes the tests, and ships the change.

Specs were invented to do that decomposition for the reader, because the reader couldn't be trusted with it. The reader can be trusted with it now. That part is settled.

## The unsettled question

That works for throwaway prototypes. What happens on a project that matters?

A project with an operator who gets paged at 3am and needs the error message to tell her something useful. A project where a downstream team wired you into their build last quarter and now depends on a behavior you never wrote down. A project where "technically correct and quietly wrong" has real costs.

The agent still has the implementation judgment. What it doesn't have is the frame: who does this project serve, what have we promised them, and what are we trying to accomplish this week?

Nobody is writing that down. Specs don't carry it, because specs were never designed to carry it. They answer "how should this be built?" and leave "why does this matter and to whom?" to oral tradition and tribal knowledge. That worked when the reader was a human who could absorb context from hallway conversations and code review threads. An agent has no hallway. It has what you write down and the code.

So the question isn't whether agents have judgment. They do. The question is what to point that judgment at.

## The frame

Instead of a spec, give the agent two things: **stakeholders** and a **theme**.

**Stakeholders** are the specific people the project serves, written as prose, not a checklist. Each one has a first encounter with the project, a notion of what success looks like, and a reason they'd walk away. The stakeholder prose isn't an introduction that precedes the real document. It is the document. There's no hidden spec behind it.

**Themes** are what you're trying to accomplish this week. A theme is narrower than priorities: where, inside everything that matters, you're going to spend today. *Get the CLI working end-to-end.* *Stop bleeding contributors on the onboarding path.*

Between stakeholders and a theme there's usually enough context for the agent to figure out what to do next, and enough for it to notice when it was wrong about what to do next.

What about the promises each stakeholder is relying on — the claims? Those belong in your test suite, not in a document. A claim you can't run in CI is just a spec by another name. Write tests that encode stakeholder promises, not just implementation correctness: `stop` always leaves the working tree on the base branch. Every public type in `ir.rs` has a `cache_line` field. If CI can't break it, you don't really value it yet. Your CI pipeline is already the falsification loop — you just have to be intentional about what you put in it.

## The practice

Value-driven development is a daily practice, not a document you write once.

Each cycle has three phases. A **customer champion** picks one of the stakeholders and actually uses the project as them — running the commands, reading the output, hitting the friction, noticing the moments that feel good or hollow — then names the single change that would most improve the next encounter. A builder implements it. A verifier runs an adversarial pass against the builder's work — did it actually meet the goal? What could break? — and the builder and verifier loop until the verifier says the goal has been sufficiently met. CI gates every step.

The champion is not a separate simulator agent handing off a report. The champion is the role that used to be called "goal-setter" — same place in the loop, same file on disk (`.lathe/goal.md`), different posture. The champion inhabits a stakeholder, walks their journey, and lets lived experience drive the goal. Lived experience is what gives it the courage to name what's valuable and what's painful specifically — to speak with the weight of being there.

The champion reads its own recent history each cycle — which stakeholder each prior goal served — so it can notice when one stakeholder has been getting all the attention and another has been quietly neglected. The balance is built into the loop, not bolted on after the fact.

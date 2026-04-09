# values

*A manifesto on value-driven development, by William Laffin. Lathe's design derives from this document — read it before writing `agent.md`. Source: https://libliflin.github.io/values/*

## The thesis

I write specs for a living. Requirements docs, design docs, tickets, the whole kit. For a human team they mostly work, and I don't see myself putting down the pen anytime soon.

What I've started to notice is that when the reader is an agent pushing an existing project forward, the documents I've been writing don't translate. I hand over a clean ticket and a link to three CLAUDE.md files and an architecture sketch, and the work comes back technically correct and quietly wrong. The way a new hire's first PR can be correct and quietly wrong: they followed the instructions, and nobody told them what the instructions were for.

After a while I started to suspect the problem lived in document shape, not document quality.

## What this is for

A narrow thing: pushing an existing, scoped project forward with an agent doing the work.

Big questions like microservices versus monolith, which market to target, or whether to build this thing at all, all belong to the owner. They get decided in other rooms. My question is smaller: what do you hand an agent each cycle so it can push the project in a direction the owner would recognize as progress, without drowning it in documentation?

This also isn't an argument against specs in general. Specs remain the right tool whenever you need a document you can cite in a review: regulated industries, audited interfaces, cross-team contracts that have to survive personnel changes. The manifesto isn't a replacement for any of that. It's about the daily work of pushing a scoped project forward, where no external audit is coming and the real cost is documentation overhead slowing the agent down.

The short answer is that you need much less than you'd think, and it needs to be in a different shape than you'd expect.

## What a spec actually is

A spec is a frozen answer to a question that is still being asked. You write it before the work starts, which is the moment you know the least about the work. Then the work teaches you something, as it always does, and now you have two problems: the thing you learned, and the spec that disagrees with it.

You can update the spec. Most of us don't. The spec drifts, the code moves, and the document that was supposed to align everyone becomes the document nobody trusts.

A human reader catches that drift. They push back on stale parts, they ask clarifying questions, they mentally skip the sentences that don't make sense anymore. An agent reads every sentence as authoritative text and runs with it. To an agent, a clear spec looks trustworthy regardless of whether it still corresponds to reality. Clarity beats correctness when the reader has no other frame.

And scattering five clean specs across a repo doesn't solve the problem. It makes it worse. Now you have five drifted documents to satisfy at once, and the agent will dutifully try to satisfy all of them. The documentation overload people complain about when working with agents isn't a volume problem. It's a shape problem.

So a spec, by itself, doesn't carry the thing I actually need the agent to carry: why any of this matters, and to whom.

## What I'd rather give them

**Stakeholders.** The specific people the project serves, named concretely enough that I can picture one of them having a bad morning. Maybe it's the operator who gets paged at 3am and needs the error message to tell her something useful. Or the contributor opening the repo for the first time on a Sunday afternoon, deciding in ninety seconds whether this project is worth her weekend. Or the downstream team that wired us into their build last quarter and now depends on a behavior we never wrote down.

Each stakeholder has a first encounter with the project and some notion of what success looks like for them. They also have a reason they'd walk away. That last one is the one I most often forget to write down. If I can't name it, I'm not ready to ask an agent to act on her behalf.

Personas aren't a new idea. What's different here is that the stakeholder prose isn't an introduction that precedes the real document. It *is* the document. There's no hidden spec behind it that the prose is summarizing. The stakeholders, the claims, and this week's theme are all the agent gets, and they have to be load-bearing on their own.

**Claims.** The promises each stakeholder is relying on, written concretely enough that something can actively try to break them.

Good claims look like: *stop always leaves the working tree on the base branch*. *Every public type in ir.rs has a cache_line field*. Something where I can write an adversarial test that asks whether the promise is still holding and get back a yes or a no. "The CLI should be reliable" fails that bar. It's a mood dressed up as a promise.

The falsification loop is the load-bearing part. If you write claims down but nothing is periodically trying to break them, you don't have claims. You have sincerely-held beliefs, which is the category the original specs were in. A claim without that loop around it will drift the same way a spec drifts, for the same reason: nothing is keeping it honest.

You might notice that the falsification suite is itself a kind of spec, a specification of the claims that must hold. That's true, and worth sitting with. The important difference is that it runs. A drifted test script fails loudly, which forces either a fix or a conscious decision to update the claim. A drifted human-facing spec is silent, and silence is the enemy here. The reason to prefer executable claims over written ones isn't that one is a spec and the other isn't. It's that one breaks loudly and the other lies quietly.

I find I don't really know what I value until I try to write a test that would prove I've stopped valuing it.

**Themes.** What I'm trying to accomplish this week. Priorities come from the stakeholders. A theme is narrower: it says *where*, inside those priorities, I'm going to spend today. *Get the CLI working end-to-end*. *Stop bleeding contributors on the onboarding path*. A theme biases attention. It answers the question: where, within everything that matters, should I be looking this afternoon?

Between those three, there's usually enough context for the agent to figure out what to do next, and enough for it to notice when it was wrong about what to do next.

## Judgment is the bet

The word I want is *judgment*. The ability to read a situation and pick a reasonable next move. Specs were invented to avoid needing it: the whole point of a spec was to move judgment out of the reader's hands and into the writer's, because the reader couldn't be trusted with it.

This is where spec-cage believers and I part ways, and I want to be honest about what I'm claiming. The claim is that the current generation of coding agents can exercise judgment on an existing, scoped project well enough that providing a *frame* beats providing a *cage*. That's an empirical claim about 2026, not a philosophical one about agents in principle. If you've been burned by older agents that needed spec cages to produce anything coherent, the burn was real, and caging them was the right call at the time. What I'm saying is that for this narrow job, pushing an existing project forward one day at a time, the cage now costs more than it saves.

The cheapest way to check is to take the spec away from one of your agents, hand it the stakeholder prose, the claims, and a theme, and watch what happens for a week. That's how I figured it out. A week of honest trial will tell you whether the claim holds on your project.

If it does, the writer's job is to provide the frame the reader needs in order to decide well.

## This is a daily practice

Value-driven development is a daily practice. The unit of work is the question "what meaningful improvement can I make right now?" You show up, you make today's version a little better, you commit, you go home.

This is local gradient by design. The agent walks one project forward, one step at a time, under direction from a human who is steering. The method is for patient, aligned collaborators pushing an existing project forward. A blank-page architect needs a different tool, and that tool is not this one.

## What the loop looks like

In practice, value-driven development is more rhythm than document.

You start by reading the project hard enough to figure out who it actually serves and what you've promised them. You write that down as prose, not as a checklist. You work in small cycles, and each cycle the agent picks the one change that most improves some specific stakeholder's life, then writes down who benefited and how. Periodically it stops building and runs the adversarial pass: does every claim still hold? Did the last few cycles quietly break something? A claim that can't be broken on purpose is either load-bearing or decorative, and you want to know which before you ship.

Every few cycles you look back over the recent work and ask which stakeholder has been getting all the attention, and which one has been quietly neglected. The owner reads those retros and decides whether the tool is earning its keep. That's the feedback loop, and it lives outside the system, held by the human whose project it is. Nothing in this method tries to self-grade. The grading is the owner's job and stays that way.

The agent's input is the frame, the current state, and the question: *which stakeholder's day can I make noticeably better right now?*

That's the whole idea.

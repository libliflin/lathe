# Lathe World Model — Dimensions of Project Understanding

Every agent needs a shared, evolving model of the project. Init seeds it; agents refine it every cycle. These are not questions you ask once — they're dimensions of understanding that update as the project changes.

## Dimensions

### 1. Who does this project serve?
- Who are the stakeholders?
- What does their journey look like?
- Where do their needs conflict?
- What signals resolve those conflicts?

### 2. What is this project?
- What domains of knowledge does it span?
- What's the authority for each domain?
- Where do the boundaries create confusion?
- What does the code actually do vs what it aspires to do?

### 3. How does this project work?
- How do you build it?
- How do you test it?
- How do you deploy/release it?
- What's the CI setup?
- What are the conventions and patterns?

### 4. What does good look like here?
- What's the project's quality bar?
- What patterns does the codebase follow?
- What are the non-obvious gotchas?
- What does the language/framework encourage that this project should lean into?

### 5. What's the current state?
- What works? What's broken?
- What's been attempted and failed?
- Where is the project in its lifecycle?

### 6. What should we measure?
- What goes in the snapshot?
- What signals matter for decision-making?
- What's noise vs signal for this project?

## Observations

- These dimensions all evolve with the project. Understanding them is what makes someone a valuable coworker vs a task executor.
- Each dimension influences the others — a shift in stakeholders changes what "good" looks like, which changes what we measure, which changes current state assessment.
- Agents need to understand the flow of contributors and where they're heading — like network prediction in game engines (Quake-style), predicting future state to make the best decision now.
- Currently scattered across goal.md, builder.md, verifier.md, skills/, refs/, and snapshot.sh. No agent thinks of these as a shared model they're maintaining.
- Init seeds the model. Agents should refine it continuously. The model is the institutional knowledge of the team.

## Open Questions

- How should this model be represented? Files? Structured data? A single document?
- How do agents signal when a dimension needs updating vs when they should just act on current understanding?
- How does the snapshot relate to the model? Is it a view of dimension 5, or something separate?
- Should the model be versioned/historied so agents can see how understanding has evolved?

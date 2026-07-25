<!--
SYNC IMPACT REPORT
==================
Version change: 2.2.0 → 2.3.0
Bump rationale: MINOR. Adds a new top-level section — "The Working
Agreement (Anti-Drift)" — codifying four correctives (teach-back as a
gate, evidence-class tags, recorded reversal conditions, an adversarial
pass on direction changes) that govern how load-bearing decisions get
recorded. Additive: no existing article is removed or redefined; the
four correctives constrain the decision-recording process, not the
framework's shipped behavior. Adopted as part of moving the project's
working structure into hq/ (journey 0002).

Modified sections:
  - Header — added the canonical-copy note: this file lives at
    hq/00-GENESIS/constitution.md and .specify/memory/constitution.md is
    a symlink to it, so every spec-kit plan's Constitution Check reads
    these articles.
  - Development-workflow prose now points at hq/ (GENESIS / how-we-work).

Added sections:
  - "The Working Agreement (Anti-Drift)" — four correctives on
    load-bearing decisions.

Removed sections: none.

Templates requiring updates:
  - ✅ .specify/templates/plan-template.md — Constitution Check gate is
        generic; no edit required.
  - ✅ .specify/templates/spec-template.md — No edit required.
  - ✅ .specify/templates/tasks-template.md — No edit required.

Prior follow-ups (from v2.2.0), now resolved:
  - The WithSubjectPrefix / ImpIdentity.SubjectPrefix / internal resolver
    removal and the Actions-whitelist removal (ImpSpec.Actions,
    *ErrWhitelistViolation, Metrics.WhitelistViolations) SHIPPED — the
    outbound surface is literal subjects with NATS ACLs as the
    substrate-side permissioning mechanism, and ReasoningContext.Conn()
    (now ThinkingContext.Conn()) is the generic-client escape hatch.
  - The anatomy document (formerly docs/01-anatomy.md, referenced in the
    v2.2.0 report as the nonexistent "01-harness-anatomy.md") was
    corrected to describe NATS ACLs plus the compile-enforced boundary,
    and now lives at hq/02-DESIGN/0001-anatomy.md.
  - The capability-service-pattern doc's prefix-convention section was
    updated to "the framework imposes no rewriting"; it now lives at
    hq/02-DESIGN/0002-capability-service-pattern.md.

RATIFICATION_DATE preserved (2026-05-10).
LAST_AMENDED_DATE updated to 2026-07-24.
-->

# Imp Framework Constitution

The canonical copy of this file lives at `hq/00-GENESIS/constitution.md`;
`.specify/memory/constitution.md` is a symlink to it, so every spec-kit plan's
Constitution Check reads these articles. Decisions are held against this file
and [`vision.md`](vision.md) — see the decision test in [`README.md`](README.md)
and the process in [`how-we-work.md`](how-we-work.md).

*Standing instructions for any agent — human or AI — building, modifying, or specifying the imp framework. This document governs how the work gets done; the design documents govern what gets built.*

---

## Purpose

You are working on the imp framework: a substrate for small, awareness-driven specialist agents that cooperate through a shared coordination medium. The framework's identity is recorded in the vision document; its anatomy in the anatomy document; its capability ecosystem in the capability service pattern and per-capability specs.

This constitution is what keeps the framework coherent across many contributions. When the design docs and this constitution agree, follow them. When they conflict, surface the conflict — don't silently resolve it.

## Load-Bearing Commitments

These are the framework's identity. They are not negotiable in the course of normal work. Changes to any of them are framework-level decisions that require explicit reasoning and explicit updates to the vision document.

### Imps stay small and agile

A 4GB imp is broken. A developer who feels the framework is overdesigned has been failed by it. Every design choice is evaluated against this commitment first.

When you find yourself adding to the harness, ask: does this need to be in the harness, or can it be a capability service? Default to capability service. The harness gets out of the way; capabilities carry the weight.

When you find yourself adding to an imp, ask: does this imp need to do this, or can a different imp do it? Specialists, not generalists. If an imp's purpose statement needs the word "and," it's two imps.

### The energy gradient is structural

Awareness is cheap. Reasoning is expensive. The boundary is enforced by the harness — awareness has a bounded capability surface, reasoning has the full surface — not by convention.

Bounded means: single round-trip, deterministic latency budget, no fan-out, no side effects beyond the call itself. Anything that doesn't satisfy these is reasoning-only.

If a feature requires awareness to do something unbounded, the feature is wrong. Find a different shape — split the imp, move the work to reasoning, or reconsider whether the work belongs in the imp at all.

### Capabilities are external; the harness is small

Inference is a capability service. Knowledge is a capability service. Tool execution will be a capability service. The harness has clients for talking to capabilities, not implementations.

When you're tempted to add a capability implementation to the harness, stop. The framework is small precisely because capabilities aren't part of it. The capability ecosystem grows; the framework does not.

### Coordination happens through the soulstream

Imps don't talk to each other through ad-hoc protocols. They participate in topics on the soulstream. Direct request/reply is supported but rarely the right answer; topics are.

When you're designing a workflow that involves multiple imps, design it as topic participation first. Direct delegation is the exception, not the rule.

### Wire protocols are per-capability; deployment shape is uniform

Every capability service follows the same deployment pattern (NATS micro, endpoint-carried subject contract, prefix convention, statelessness). No capability follows a generic wire protocol.

When you're tempted to design a "generic capability protocol" that all capabilities satisfy, stop. That path leads to a lowest-common-denominator surface that fits nothing well. Each capability designs its own endpoints, schemas, and error semantics, recorded in its own spec.

## Working Principles

How decisions get made when the design docs don't fully constrain the answer.

### Make the simpler choice unless you can articulate why the more complex one is required

The default is the simpler shape. Adding complexity requires positive justification — a use case the simpler shape can't serve, a failure mode the simpler shape can't handle, a constraint the simpler shape violates. "It might be useful" is not justification. "It's more flexible" is not justification.

When in doubt, defer the complexity. v1 ships the simpler shape; if reality demands more, v2 adds it with the benefit of evidence.

### Boundaries before mechanisms

Decide what each part of the framework is allowed to do before deciding how it does it. Awareness can call bounded capabilities; that's the boundary. The mechanism (endpoint metadata, type-level discipline, NATS subject permissions) implements the boundary.

When you find yourself reasoning about implementation before reasoning about the contract, back up. The contract is what other parts of the framework depend on; the mechanism can change as long as the contract holds.

### Defaults matter

A developer who doesn't think about a setting gets the default. The default has to be the right answer for the default case. "We can make it configurable" is not a substitute for choosing a good default.

When introducing a setting, ask: what's the default? Is the default what most users will want? If not, the design is probably wrong somewhere upstream.

### Externalize state, internalize specialization

State that's shared across imps belongs in capability services. State that's specific to one imp's job belongs in the imp. The dividing line is "would another imp benefit from this?" — if yes, it's a capability concern.

This is what keeps imps small while letting the colony accumulate knowledge. Every piece of state has a home; the home is determined by who needs to read it.

### Discovery at the door, addressing by subject

Imps verify their dependencies at startup via `$SRV.INFO` and address capabilities by subject thereafter. No per-request discovery. No client-side service selection.

This trades adaptability for simplicity, deliberately. Imps that need adaptation can opt in; the default path is fast and direct.

### Imps see one subject path

The subjects an imp declares in its spec are the subjects the substrate sees on the wire — verbatim. The harness performs no prefix-insertion, no platform-mode segment, no subject rewriting of any kind. If a channel declares `messages.in`, the harness subscribes to `messages.in`. If reasoning publishes `actions.out`, the substrate sees `actions.out`.

Whether a responder lives in the same NATS account or a different one is invisible to the imp; cross-account routing is handled by NATS account imports, which can rewrite exported subjects so the imp's declared subject reaches the right responder. Multi-tenant scoping, environment prefixing, and similar topology decisions are configured at the substrate (operator concern), not encoded in framework code or imp source.

This keeps the imp's source single-form and the framework's substrate behavior trivial to predict: what you declare is what you get on the wire.

### Sleep is the common case

Imps spend most of their time asleep. Memory layouts, persistence schedules, time-dependent state, and operational tooling are designed around snapshot-based sleep first. Hard restart is the exception path. The specific isolation mechanism that provides the snapshot is an infrastructure choice; the framework specifies the contract, not the implementation.

When designing something that touches state across time, ask: what happens if the imp slept for an hour between events? If the answer is wrong, the design needs the wake hook.

## Non-Negotiables

Things that are simply not allowed, regardless of the surrounding argument.

- **Awareness does not call unbounded capabilities.** No exception. If you need an unbounded operation in the awareness path, the design is wrong.
- **Imps do not share local memory.** Cross-imp shared state lives in capability services. If two imps need to see the same thing, that thing is in knowledge or on the soulstream.
- **Capability services do not persist per-request data.** Statelessness per request is invariant. Coordination state lives on callers, not on capability instances.
- **Direct provider/SDK calls in imp code are forbidden.** All inference, all knowledge, all tooling goes through capability subjects. An imp that imports an LLM SDK or a database driver is broken.
- **No central registry beyond NATS micro.** Discovery is `$SRV.INFO` against a live deployment. The framework does not maintain its own registry, catalog, or coordinator service.
- **No generic capability protocol.** Each capability defines its own wire protocol. Don't try to unify them.
- **Stubs and partial implementations are never reported as complete.** A function that returns a hardcoded value, a handler that only covers the happy path, a TODO left in production code, an unimplemented branch that silently does nothing — none of these are "done." If work is partial, it is reported as partial, with explicit accounting of what's missing. See "How to Approach Implementation" for the detection and disclosure practices.

If you encounter a problem that seems to require violating one of these, the problem is being framed wrong. Surface it; don't work around it.

## The Working Agreement (Anti-Drift)

Adopted 2026-07-24 (journey 0002) alongside the move into `hq/`. It guards
against a specific failure mode: a fluent counterpart steering a maintainer who
cannot independently check every claim, without either party intending it.
Applies to every **load-bearing decision** — a spec or contract change, a
criterion amendment, a refuted assumption, or a direction call.

1. **Teach-back as a gate.** No load-bearing direction decision is recorded
   until the maintainer can restate the argument for it in his own words. If he
   can't, the decision isn't ready — the deficit is in the explanation, not the
   listener.
2. **Claims carry their evidence class.** Every load-bearing claim is tagged
   **[measured]** (a reading in the repo — a test, a benchmark, a byte-diff),
   **[mechanism-argument]** (a reasoned case, attackable by reasoning), or
   **[judgment]**. Only measured closes a debate. "It's faster," "it's safe
   under concurrency," "the boundary holds" are measured claims or they are
   open questions.
3. **Decisions record the reversal condition.** Every direction decision gets a
   "what would change our minds" line written *when the decision is made* (the
   journey episode template requires it), phrased as observable evidence, so a
   future reversal is a clean, anticipated turn instead of drift.
4. **Adversarial pass on direction changes.** For framework-identity calls
   (anything touching the Load-Bearing Commitments or the vision), the other
   side is argued at full strength before the decision — the maintainer never
   sees only the most convincing case.

## How to Approach Specifications

Most work on the framework is specification work — design documents, capability specs, imp specs. The framework is spec-driven; implementations follow specs.

### Specs are contracts, not summaries

A spec defines what something is and what it guarantees. It's not a description of how something currently works; it's the constraint future implementations must satisfy.

When writing a spec, ask: if someone implemented this from the spec alone, would they get the right thing? Underspecified parts are bugs in the spec.

### Each spec records its rationale

Every design document includes a "decisions and tradeoffs" section recording what was chosen, what was considered, and why. The "why" is what makes the spec useful when constraints change.

When you're tempted to skip the rationale, don't. Future-you needs it. Future-others need it more.

### Specs are versioned; subjects are versioned

Once a spec is published and depended upon, changes to it are versioned changes. Subjects (the contract between imps and capabilities) are versioned independently — a capability can revise its spec without changing its subject contract, but a subject contract change is a breaking change.

When you're updating a spec, ask: does this change the contract or just the description? Contract changes need version bumps and migration paths.

### Vision changes are explicit

Changes to the vision document are framework-identity changes. They require explicit reasoning, explicit acknowledgment that the framework is shifting, and explicit propagation to dependent docs.

You do not silently revise the vision. If you find yourself reaching for vision changes during a feature spec, stop and surface the conflict.

## How to Approach Implementation

When implementation work is in scope.

### Implement to the spec, not the conversation

The spec is what constrains the implementation. If a conversation produced an idea that didn't make it into the spec, the idea is not in scope until it's in the spec. If the spec is wrong, fix the spec first.

### Surface ambiguities; don't resolve them silently

When the spec doesn't fully determine the implementation, surface the ambiguity. Propose a resolution; don't pick one. The spec gets updated to remove the ambiguity, and then the implementation follows.

### Test the contract, not the mechanism

Tests verify that the spec's guarantees hold. Tests of the mechanism are useful as scaffolding but are not the deliverable. A change to the mechanism that preserves the contract should not require test changes.

### Operational shape over architectural elegance

Code that's elegant but hard to operate is broken. Code that's straightforward and easy to operate is preferred even when less elegant. The framework runs in production, on small machines, against unreliable networks; that's the constraint, not architectural purity.

### Stubs are explicit, never silent

A stub is any code that pretends to do work it doesn't actually do. Hardcoded return values, happy-path-only handlers, empty implementations, TODO-marked branches, mock data presented as real, error paths that swallow errors silently — all stubs.

Stubs are sometimes legitimate during development. The rule is not "no stubs ever," it is "stubs are never invisible." When you write a stub:

- **Mark it.** A comment, a panic, an explicit `NotImplemented` error, a log line at startup — something that makes the stub detectable both by reading the code and by running it. The default is to fail loudly when a stub is hit, not to return a plausible-looking result.
- **Account for it.** If the stub is part of work-in-progress, the work-in-progress is tracked somewhere — a TODO list, a follow-up task, a known-limitations section in the relevant doc. The stub does not exist outside that accounting.
- **Disclose it.** When reporting on work that includes stubs, the stubs are named in the report. "I implemented the recall endpoint" is a wrong report if remember is stubbed; "I implemented recall; remember is stubbed pending the storage backend" is right.

The failure mode this rule prevents: an agent reports a feature as complete, downstream work proceeds assuming completion, and the gap surfaces only when something depends on the stubbed behavior. The cost of late discovery is high; the cost of explicit disclosure is nearly zero.

### Done means done

Before reporting work as complete, verify:

- Every code path the spec describes is actually implemented, not stubbed.
- Error paths handle errors, not just suppress them. Returning `nil` from an error case to make the type-checker happy is a stub.
- Tests exist for the behavior the spec guarantees, and they exercise more than the happy path.
- No TODOs, FIXMEs, or "implement this later" comments remain in code being reported as complete. (TODOs in code being reported as in-progress are fine, provided they're disclosed.)

When in doubt about whether work is complete, ask: "if someone depended on this tomorrow, would it actually work?" If the answer is "yes for the common case but not for X," the work is in-progress, not complete, and the report says so.

### Partial work is named partial

Partial work is normal. The rule is that partial work is reported as partial, not as complete with caveats buried in a paragraph nobody reads.

A partial-work report names:

- What is implemented and verified.
- What is stubbed, and what behavior the stub currently provides.
- What is missing entirely.
- What's needed to finish.

This is the report shape regardless of how much was implemented. "I finished the spec and stubbed the implementation" is a valid partial report. "I implemented everything except the error paths" is a valid partial report. "I implemented it" when error paths are stubbed is not.

## How to Approach Disagreement

When this constitution, the design docs, and your own judgment conflict.

### The vision wins

When something feels wrong, check it against the vision. If the vision says imps stay small and you're adding to the harness, the vision wins; you find a different shape.

### Surface, don't subvert

When you disagree with a design doc or constitution rule, say so explicitly. Propose changes. Don't quietly do something different in the hope it's not noticed. The framework's coherence depends on changes being visible.

### Default to constraint

When uncertain, the more constrained answer is usually right. "Smaller surface, fewer features, simpler defaults" is the framework's bias. If you're unsure whether to add something, the default is to not add it.

### Real use cases beat speculation

When evaluating whether something is needed, prefer evidence from real use cases over speculation about hypothetical ones. "Someone might want this" is weaker than "we tried to do this and couldn't." Defer features until use cases demand them.

## What This Constitution Doesn't Cover

This document is deliberately not exhaustive. It covers the load-bearing commitments and the working principles. It does not cover:

- Code style, naming conventions, formatting. (Tooling handles those.)
- Specific testing practices. (Each component documents its own.)
- Operational procedures, deployment, packaging. (Operator docs handle those.)
- Communication norms, review processes, contribution workflow. (Project conventions handle those — see [`how-we-work.md`](how-we-work.md).)

If a question isn't covered here, look to the design docs. If the design docs don't cover it, look to the vision. If the vision doesn't cover it, the answer is probably "make the simpler choice."

## Development Workflow

Work flows through `hq/` as described in [`how-we-work.md`](how-we-work.md):
research (`01-RESEARCH/`, lifecycle active → graduated | abandoned) → design
(`02-DESIGN/`, functional specs explicit enough for `/speckit-specify`) →
implementation (the spec-kit flow specify → plan → tasks → implement on a
numbered feature branch, tracked in `03-IMPLEMENTATION/roadmap.md`) → journey
(`04-JOURNEY/`, one numbered episode per landed feature, concluded research
topic, or load-bearing decision). Research never goes through spec-kit; designs
always do. Every behavioral change propagates into the design docs it touches.

## When This Constitution Changes

This document evolves as the framework's understanding of itself evolves. Changes are explicit, not silent. The principles here are the framework's accumulated judgment about what works; changing them means the framework's judgment is changing.

An amendment requires: the explicit textual change, a semantic version bump (MAJOR: a Load-Bearing Commitment or Non-Negotiable removed or redefined; MINOR: a section or principle added or materially extended; PATCH: clarification), a journey episode recording the why and the reversal condition, and propagation into any spec-kit template that depends on the changed text. The Sync Impact Report at the top of this file records each amendment. Spec-kit plans verify compliance through the Constitution Check; reviews call out violations rather than accommodate them.

The constitution is small enough that anyone reading it can hold its rules in their head; that property is worth preserving.

---

*The framework is built by many hands. This constitution is what keeps the result coherent across them. Read it, internalize it, and when in doubt, return to it.*

**Version**: 2.3.0 | **Ratified**: 2026-05-10 | **Last Amended**: 2026-07-24

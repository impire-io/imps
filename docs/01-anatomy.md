# Imp Anatomy

*The five parts of an imp, what each does, and the boundaries between them.*

---

An imp is what holds itself together. This document defines what an imp is made of, what each part is allowed to do, and how the parts fit together. The vision document explains *why*; this document explains *what*.

An imp has five parts: **channels**, **awareness**, **reasoning**, **memory**, **action**. Each has a defined surface, defined invariants, and a defined relationship to the others. Nothing in the framework's vocabulary exists outside these five.

## Channels

A channel is an inbound message subscription. Channels are how messages reach an imp.

An imp declares its channels in its spec — a list of NATS subjects (or subject patterns) it subscribes to. The framework establishes the subscriptions at startup and dispatches arriving messages into the imp's awareness layer.

Three things a channel does:

- **Subscribes.** The framework manages the NATS subscription's lifecycle, including consumer position across sleep/wake cycles.
- **Decodes.** Messages arrive on a channel as a typed value the imp understands. Channel declarations include a decode step (a schema reference, a parsing function) so awareness sees structured data, not raw bytes.
- **Dispatches.** The framework calls the awareness layer with the decoded message, an entity identifier (extracted by the channel from the message), and a channel-scoped context.

Channels can be of three kinds:

- **External channels** — subjects outside the imp's own namespace (e.g. `bridge.emails.>`, `sensors.temperature.>`). The imp watches the world here.
- **Soulstream channels** — subjects under `$soulstream.*` for topics the imp participates in. Joining a topic adds a soulstream channel; leaving removes it.
- **Schedule channels** — subjects fed by NATS server-side scheduling. The imp registers a schedule (cadence, target subject), and the framework subscribes the schedule's target as a channel. Periodic work flows through here.

All three kinds use the same dispatch mechanism. The awareness layer doesn't see which kind a message came from; it sees a message, an entity, and a context.

A channel is not allowed to call out to capabilities or do expensive work. Channels are pure subscribe-decode-dispatch. Anything more is awareness's job.

## Awareness

Awareness is cheap, local interpretation. It runs on every message a channel delivers. It decides whether reasoning needs to run.

An awareness function takes a decoded message, an entity identifier, and an awareness context. It updates the imp's local state for that entity, and returns a verdict:

- **`Ignore`** — nothing to do. The state may have updated but no further action is warranted.
- **`Note`** — the state changed in a way worth recording in memory or visible to other imps via the soulstream, but no reasoning needed.
- **`Think`** — escalate to reasoning. Carries a reason and the entity to focus on.

Awareness is allowed to:

- Read and write the imp's local state for the entity.
- Issue a single `Request` — one publish, one reply, one deadline. Bounded by call shape.
- Emit lightweight notes to the soulstream (a comment, an attachment, a status update on an existing topic the imp is in).

Awareness is *not* allowed to:

- Fan out (`RequestMany`), fire-and-forget (`Publish`), or reach the raw NATS connection (`Conn()`).
- Open new soulstream topics or send DMs.
- Recurse into reasoning directly. (Returning `Think` is how reasoning is invoked.)

The structural enforcement: the awareness context's type exposes only the bounded outbound surface. `RequestMany`, `Publish`, and `Conn` do not exist on it. A developer cannot accidentally make awareness expensive because the methods aren't reachable. NATS subject permissions on the imp's connection enforce the same boundary as a backstop at the substrate.

The boundedness criterion is **call shape**, not endpoint metadata. A single request/reply is bounded by virtue of being a single request/reply with a deadline; the framework does not need to inspect what's on the other end of the subject to know the call is bounded.

### Why "Note" exists

An awareness verdict of `Note` is the difference between "the imp's interpretation changed but the colony doesn't need to know" and "something happened that other imps or keepers should be able to see without running my reasoning." A complaint-watcher seeing an inbound email about an existing topic might `Note` it (post the email as a turn) without escalating; only when something about the email warrants investigation does it `Think`.

Without `Note`, awareness can't contribute to the soulstream without going through reasoning, which would force the energy gradient to collapse for any participation.

## Reasoning

Reasoning is expensive deliberation. It runs only when awareness escalates with `Think`.

A reasoning function takes the reason, the entity, and a reasoning context. It does whatever the imp's purpose requires — investigates, decides, drafts, escalates — and emits actions.

Reasoning is allowed to:

- Read and write the imp's local state.
- Issue `Request`, `RequestMany`, and `Publish` against any subject the substrate's ACLs permit.
- Pull the raw `*nats.Conn` via `Conn()` and pass it to any generic NATS-based client library.
- Open soulstream topics, post turns, mention other imps or keepers.
- Delegate work via inter-imp request/reply.

Reasoning is *not* allowed to:

- Run synchronously inside a channel dispatch. Reasoning is async and runs in its own goroutine; the channel dispatch returns as soon as awareness yields `Think`.
- Block awareness. Awareness for new messages continues to run while reasoning is in progress for an earlier wake-up.
- Reach into another imp's local state directly. Inter-imp coordination goes through the soulstream or explicit delegation.

A reasoning function may be called concurrently for different entities. Reasoning code is responsible for being safe under concurrency for the same entity if multiple escalations pile up.

### The reasoning context

The reasoning context is the imp's gateway to the world outside its local state. It exposes:

- **Outbound NATS** — `Request`, `RequestMany`, `Publish`, and the raw `Conn()`. Whatever is on the other end of a subject — an inference service, a knowledge store, a tool runner, a peer imp — is the operator's design. The framework consumes bytes.
- **Soulstream operations** — open a topic, post a turn, mention, attach.
- **Local state access** — read and write the imp's per-entity state.
- **Delegation** — request/reply against other imps via the coordination pattern.

The reasoning context is the imp's single point of contact with anything external. Awareness has the bounded subset of the same context.

## Memory

Memory in the framework is **local**. It holds the state the imp needs to do its job for the entities it's currently tracking.

Local memory has three kinds, distinguished by what they hold and how they're indexed:

- **Per-entity state** — the running interpretation of one entity. Statistics, current state, recent observations, in-flight work. Indexed by `(name, entity)` where `name` identifies the kind of state and `entity` identifies which instance.
- **Imp-scoped facts** — typed records the imp looks up by key. Configuration, recent records, working sets. Not statistically interpreted.
- **Imp-scoped indexes** — secondary views over imp-scoped facts (filter, group-by). Cheap to maintain, useful for in-process lookups.

Local memory is bounded. The framework enforces retention and eviction by default — per-entity state has a configured lifetime, eviction sends cold entities to the persistence backend, on rehydration they come back. A developer who wants unbounded local memory has to opt in explicitly. The default is small and stays small.

Local memory does *not* hold:

- Cross-imp shared state.
- Long-term episodic history (closed topics, prior actions, completed investigations).
- Heavy indexes over large corpora.
- Anything other instances of the same imp class would benefit from sharing.

All of those live in external services. The imp queries them during reasoning when context from outside the immediate moment is needed.

### Persistence and sleep

Local memory survives sleep. When the framework suspends an imp via the configured isolation mechanism's snapshot facility, the full memory image is preserved; on wake, the imp resumes with state intact. Wall-clock time, however, has passed — the framework exposes a wake-hook that signals to the imp how long it slept, and projections that depend on wall-clock decay (EMA, "stable for", "idle since") use the hook to advance their internal state.

Persistence across hard restarts (process death, host loss) uses the standard snapshot/restore cycle: per-entity state serializes to the persistence backend on a configurable schedule, and on cold start the imp replays state from the backend plus any messages since the snapshot.

Sleep is the common case; hard restart is the exception. Both are handled, but sleep is what's optimized for.

## Action

Action is the imp's outbound surface. It's how the imp affects the world.

An imp has three kinds of action:

- **NATS sends** — `Request`, `RequestMany`, `Publish`, and `Conn()`-driven calls from reasoning. Subject permissioning is a substrate concern — operators constrain what an imp can publish on via NATS account ACLs on the imp's connection, not via a framework-side whitelist.
- **Soulstream operations** — opening topics, posting turns, mentioning. These flow through the soulstream's subject conventions.

Action is reasoning-only. Awareness can `Note` (which produces lightweight soulstream activity) and `Request` (a single bounded round-trip) but cannot fan out, fire-and-forget, or invoke side-effecting NATS calls outside the bounded shape. The energy gradient is preserved here too: cheap interpretation doesn't produce expensive outputs.

Every action emission produces an audit record — what was emitted, by which imp, against which entity, in response to which escalation. The framework writes audit records to a tenant-scoped audit stream so the colony's behavior is reconstructable.

## How the parts fit together

A typical message flow:

1. A message arrives on a channel.
2. The channel decodes it, extracts an entity, and dispatches into awareness.
3. Awareness updates per-entity state, decides:
   - `Ignore` — done.
   - `Note` — emit a lightweight soulstream contribution, done.
   - `Think` — return; the framework queues a reasoning invocation.
4. If `Think`, the framework invokes reasoning asynchronously with the reason and entity.
5. Reasoning runs, makes NATS calls as needed, emits actions, updates state.
6. The imp goes back to waiting for the next message (and possibly to sleep, if no work is pending).

The energy gradient is visible in the flow: every message goes through awareness; only some messages produce a `Think`; only `Think`s invoke reasoning; only reasoning produces unbounded NATS sends.

## The outbound surface

An imp's outbound work is byte-shaped NATS sends:

- **`Request(subject, payload)`** — single round-trip request/reply. Available on both awareness and reasoning. Bounded by call shape: one publish, one reply, one deadline.
- **`RequestMany(subject, payload)`** — fan-out collect-with-window. Returns all replies that arrive within a configurable window, up to a configurable cap. Reasoning only.
- **`Publish(subject, payload)`** — fire-and-forget. Reasoning only.
- **`Conn() *nats.Conn`** — the raw NATS connection, for generic NATS-based client libraries. Reasoning only.

Subjects are **literal**. The framework performs no prefix, platform-mode, or account transformation — what the imp declares is what hits the wire (constitution "Imps see one subject path"). Cross-account routing and multi-tenant scoping are operator concerns, configured via NATS account imports and ACLs on the connection the imp uses.

The framework holds no "capability" abstraction. Whatever is on the other end of a subject — an inference service, a knowledge store, a tool runner, a peer imp — is the operator's design. The imp talks to subjects; the framework consumes bytes.

There is no startup discovery, no resolved surface, no declared capabilities on the imp's spec, no `HasCapability` check, no `$SRV.INFO` round-trip performed by the framework on the imp's behalf. The framework is smaller than this — and the structural enforcement of the energy gradient does not depend on knowing what's on the other end of a subject.

## The wake-hook

When the framework restores an imp from a snapshot, it calls the imp's wake hook with the wall-clock duration that elapsed during sleep. The wake hook is optional; imps that don't need it ignore it.

Imps that do need it — anything with EMA decay, "stable for" / "idle since" semantics, debounce windows, time-to-live expirations — use the hook to advance the affected state by the elapsed duration before processing the wake-up message.

The hook is a single call, before any channel dispatch resumes after wake. Implementations are typically short:

```
on_wake(slept_for):
    for each entity in self.state:
        entity.advance_time(slept_for)
```

Without the wake hook, time-dependent state silently produces wrong answers after sleep. Bake it in early.

## What's not in the framework

Several things you might expect are deliberately absent:

- **No internal goroutine pool for periodic work.** Periodic work is schedule channels; the framework has no `RunEvery` primitive of its own.
- **No internal LLM client, embedding cache, vector index, or knowledge store.** These are external services; the framework consumes bytes through the NATS surface, not implementations.
- **No capability declaration on the imp's spec.** No `Capabilities` field, no required/optional dependency resolution, no discovery at startup.
- **No imp-to-imp shared memory.** Coordination goes through the soulstream.
- **No retry, backoff, or circuit-breaker on NATS calls.** A timeout produces a typed error; the imp's code decides what to do. The framework imposes no policy.
- **No codec at the framework level.** `Request` and `RequestMany` take and return `[]byte`. Typed payloads are the imp author's concern.

These omissions are deliberate. Each thing the framework *doesn't* do is a thing it can't get wrong, can't gold-plate, and can't impose on developers who don't need it.

## Decisions and tradeoffs

A record of choices and their rationale, for future reference:

- **Five parts, not three or seven.** Three (awareness/reasoning/action) was too few — memory and channels needed naming. Seven (the current projection/derivation/reactor/knowledge/index/etc. taxonomy) was too many for the developer surface. Five is what survived after pruning.
- **Awareness can issue a single `Request`.** Earlier drafts had awareness as fully-local. That broke as soon as classification needed an external call (embed, classify). A single bounded request/reply is structurally bounded by call shape; that's what landed.
- **Call-shape boundedness, not endpoint-metadata boundedness.** Considered metadata-driven enforcement (capability endpoints declare themselves bounded; the framework consults metadata). Rejected: it requires the framework to model capabilities, perform startup discovery, and maintain a resolved surface. Call-shape enforcement (awareness has `Request` only; reasoning has the full set) gets the same guarantee with no framework-side bookkeeping.
- **One connection per imp, not two.** Considered separate awareness and reasoning NATS connections with different subject permissions. Rejected as overengineering — type-level discipline plus subject permissions on a single connection is enough.
- **No discovery, no `$SRV.INFO` round-trip at startup.** Considered required-capability resolution that fails imp startup if dependencies are missing. Rejected: the framework holds no capability concept at all. Whatever is on the other end of a subject is the operator's design.
- **Subjects are literal.** Each subject the imp declares is the wire subject verbatim. The framework imposes no prefix, no platform-mode segment, no rewrites. Cross-account routing and tenant scoping are configured at the substrate via NATS account imports (constitution v2.2.0 "Imps see one subject path").
- **Sleep is the common case, hard restart is the exception.** Memory and persistence are designed around snapshot-based sleep first; replay-from-backend is the cold-start path, not the hot path. The framework specifies the sleep/wake contract; the isolation mechanism that implements it (microVMs, containers, processes, simulation) is an infrastructure choice.
- **The wake-hook is mandatory in design even if optional in implementation.** Time-skip after sleep is a real bug class; designing it in early is cheaper than retrofitting.

If a future change conflicts with anything here, it gets explicit treatment — what changed, why, what other docs need to update. The boundaries this document defines are what makes the rest of the framework coherent.

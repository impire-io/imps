# Imp Anatomy

*The five parts of an imp, what each does, and the boundaries between them.*

---

An imp is what holds itself together. This document defines what an imp is made of, what each part is allowed to do, and how the parts fit together. The vision document explains *why*; this document explains *what*.

An imp has five parts: **channels**, **awareness**, **thinking**, **memory**, **action**. Each has a defined surface, defined invariants, and a defined relationship to the others. Nothing in the framework's vocabulary exists outside these five.

## Maturity

The five parts are all specified here; not all of each part is built yet (see the [`README.md`](README.md) status legend and [`../03-IMPLEMENTATION/roadmap.md`](../03-IMPLEMENTATION/roadmap.md)).

- **[V] Built and shipped** (features 001–002): core-subject and JetStream stream channels (subscribe / decode / dispatch), awareness with the three-verdict return, thinking in its own goroutine, per-entity local state (get / set / update), and the outbound action surface — `Request`, `RequestMany`, `Publish`, and `Conn`. The awareness/thinking boundary is compile-enforced (see below). Feature 004 added soulstream channels and `Note`-driven contributions as the `imps/soulstream` glue module; feature 005 added durable per-entity memory, eviction/rehydration, the per-entity rehydration wake, and the restart clock as the `imps/persist` package — both with the harness core untouched.
- **[D] Designed, not yet built:** schedule channels, per-action audit records, and whole-imp snapshot sleep/wake — the runtime-owned suspend/resume path and its mid-process wake delivery, co-designed with the soulrealm runtime (episode 0007; roadmap M2b). Each has a defined seam and ships as its own numbered feature.

Every requirement below is mandatory when its part is built; the maturity tag says *when*, not *whether*.

## Channels

A channel is an inbound message subscription. Channels are how messages reach an imp.

An imp declares its channels in its spec — a list of NATS subjects (or subject patterns) it subscribes to. The framework establishes the subscriptions at startup and dispatches arriving messages into the imp's awareness layer.

Three things a channel does:

- **Subscribes.** The framework manages the NATS subscription's lifecycle, including consumer position across sleep/wake cycles.
- **Decodes.** Messages arrive on a channel as a typed value the imp understands. Channel declarations include a decode step (a schema reference, a parsing function) so awareness sees structured data, not raw bytes.
- **Dispatches.** The framework calls the awareness layer with the decoded message, an entity identifier (extracted by the channel from the message), and a channel-scoped context.

Channels can be of three kinds:

- **External channels** [V] — subjects outside the imp's own namespace (e.g. `bridge.emails.>`, `sensors.temperature.>`), either a core NATS subject or a JetStream stream. The imp watches the world here.
- **Soulstream channels** [V] — the topic op-log read as a stream channel (`SOULSTREAM.TOPICS.OPS.<topic-path>`; the protocol has no join/leave — presence *is* the consumer, so joining is declaring the channel and leaving is stopping it). Shipped as the `imps/soulstream` glue module; specified in [`0003-soulstream-participation.md`](0003-soulstream-participation.md).
- **Schedule channels** [D] — subjects fed by NATS server-side scheduling. The imp registers a schedule (cadence, target subject), and the framework subscribes the schedule's target as a channel. Periodic work flows through here.

All three kinds use the same dispatch mechanism. The awareness layer doesn't see which kind a message came from; it sees a message, an entity, and a context.

A channel is not allowed to call out to capabilities or do expensive work. Channels are pure subscribe-decode-dispatch. Anything more is awareness's job.

## Awareness

Awareness is cheap, local interpretation. It runs on every message a channel delivers. It decides whether thinking needs to run.

An awareness function takes a decoded message, an entity identifier, and an awareness context. It updates the imp's local state for that entity, and returns a verdict:

- **`Ignore`** — nothing to do. The state may have updated but no further action is warranted.
- **`Note`** — the state changed in a way worth recording in memory or visible to other imps via the soulstream, but no thinking needed.
- **`Think`** — escalate to thinking. Carries a reason and the entity to focus on.

Awareness is allowed to:

- Read and write the imp's local state for the entity.
- Issue a single `Request` — one publish, one reply, one deadline. Bounded by call shape.
- Emit lightweight notes to the soulstream (a comment, an attachment, a status update on an existing topic the imp is in).

Awareness is *not* allowed to:

- Fan out (`RequestMany`), fire-and-forget (`Publish`), or reach the raw NATS connection (`Conn()`).
- Open new soulstream topics or send DMs.
- Recurse into thinking directly. (Returning `Think` is how thinking is invoked.)

The structural enforcement: the awareness context's type (`AwarenessContext`) exposes only the bounded outbound surface — `State` and `Request`. `RequestMany`, `Publish`, and `Conn` do not exist on it. A developer cannot accidentally make awareness expensive because the methods aren't reachable. The three absences are asserted mechanically: build-tagged files under `integration/compiletest/` each reference one of `AwarenessContext.RequestMany`, `.Publish`, or `.Conn`, and `make compile-deny` requires every one of them to *fail* to compile — a successful build under any deny tag is a regression. NATS subject permissions on the imp's connection enforce the same boundary as a backstop at the substrate.

The boundedness criterion is **call shape**, not endpoint metadata. A single request/reply is bounded by virtue of being a single request/reply with a deadline; the framework does not need to inspect what's on the other end of the subject to know the call is bounded.

### Why "Note" exists

An awareness verdict of `Note` is the difference between "the imp's interpretation changed but the colony doesn't need to know" and "something happened that other imps or keepers should be able to see without running my thinking." A complaint-watcher seeing an inbound email about an existing topic might `Note` it (post the email as a turn) without escalating; only when something about the email warrants investigation does it `Think`.

Without `Note`, awareness can't contribute to the soulstream without going through thinking, which would force the energy gradient to collapse for any participation.

## Thinking

Thinking is expensive deliberation. It runs only when awareness escalates with `Think`.

A thinking function takes the reason, the entity, and a thinking context. It does whatever the imp's purpose requires — investigates, decides, drafts, escalates — and emits actions.

Thinking is allowed to:

- Read and write the imp's local state.
- Issue `Request`, `RequestMany`, and `Publish` against any subject the substrate's ACLs permit.
- Pull the raw `*nats.Conn` via `Conn()` and pass it to any generic NATS-based client library.
- Open soulstream topics, post turns, mention other imps or keepers.
- Delegate work via inter-imp request/reply.

Thinking is *not* allowed to:

- Run synchronously inside a channel dispatch. Thinking is async and runs in its own goroutine; the channel dispatch returns as soon as awareness yields `Think`.
- Block awareness. Awareness for new messages continues to run while thinking is in progress for an earlier wake-up.
- Reach into another imp's local state directly. Inter-imp coordination goes through the soulstream or explicit delegation.

A thinking function may be called concurrently for different entities. Thinking code is responsible for being safe under concurrency for the same entity if multiple escalations pile up.

### The thinking context

The thinking context (`ThinkingContext`) is the imp's gateway to the world outside its local state. It exposes:

- **Outbound NATS** — `Request`, `RequestMany`, `Publish`, and the raw `Conn()`. Whatever is on the other end of a subject — an inference service, a knowledge store, a tool runner, a peer imp — is the operator's design. The framework consumes bytes.
- **Soulstream operations** — open a topic, post a turn, mention, attach.
- **Local state access** — read and write the imp's per-entity state.
- **Delegation** — request/reply against other imps via the coordination pattern.

The thinking context is the imp's single point of contact with anything external. Awareness has the bounded subset (`State` and `Request` only) of the same surface.

## Memory

Memory in the framework is **local**. It holds the state the imp needs to do its job for the entities it's currently tracking.

Local memory has three kinds, distinguished by what they hold and how they're indexed:

- **Per-entity state** — the running interpretation of one entity. Statistics, current state, recent observations, in-flight work. Indexed by `(name, entity)` where `name` identifies the kind of state and `entity` identifies which instance.
- **Imp-scoped facts** — typed records the imp looks up by key. Configuration, recent records, working sets. Not statistically interpreted.
- **Imp-scoped indexes** — secondary views over imp-scoped facts (filter, group-by). Cheap to maintain, useful for in-process lookups.

Local memory is bounded, in two tiers [V]. The ephemeral tier (`ImpSpec.States`) is cap-bounded, in-memory, and rebuildable from the stream — it rejects past its cap and never silently evicts. The durable tier (`imps/persist`, [`0004-sleep-wake-persistence.md`](0004-sleep-wake-persistence.md)) is where loss-on-restart would be a bug: write-through persistence means every update is already on the backend, so LRU eviction of cold entities is a lossless drop and rehydration on access brings them back. One tier per concern; the default resident bound is small (256) and stays small.

Local memory does *not* hold:

- Cross-imp shared state.
- Long-term episodic history (closed topics, prior actions, completed investigations).
- Heavy indexes over large corpora.
- Anything other instances of the same imp class would benefit from sharing.

All of those live in external services. The imp queries them during thinking when context from outside the immediate moment is needed.

### Persistence and sleep [restart path V, snapshot path D]

Sleep proper — snapshot-based suspension of the whole imp, "the imp doesn't know it was asleep" — is the **runtime's** act: the isolation mechanism (in the Impire family, the soulrealm runtime and its backends) freezes and resumes the memory image, and only it can know the suspension interval authoritatively. The framework owns the *contract*: an elapsed reading delivered to imp code before dispatch resumes. That contract's mid-process delivery is deliberately unbuilt **[D]** — it must be co-designed with the runtime's suspend capability (episode 0007; roadmap M2b) — so at the whole-imp level, "sleep is the common case" is the design target, not yet the shipped reality.

What *is* shipped **[V]** is the restart path: durable-tier state is write-through, so stopping the imp is always safe — the snapshot is continuous and there is nothing to flush. Across stops, deploys, and crashes, state rehydrates lazily from the backend on first access, channel positions replay via durable consumers (feature 004), and the `imps/persist` package supplies two wake readings: the per-entity hook fired on rehydration with the elapsed time since that entity's last activity, and the `Beacon` **restart clock** — a self-reported stamp whose `SleptFor` gates `main()` before `Run`. The Beacon is the interim imp-level elapsed source; the runtime's signal supersedes it when M2b lands.

Sleep is the common case; hard restart is the exception. The restart path is shipped; the sleep path is specified and waits on its runtime half.

## Action

Action is the imp's outbound surface. It's how the imp affects the world.

An imp has three kinds of action:

- **NATS sends** [V] — `Request`, `RequestMany`, `Publish`, and `Conn()`-driven calls from thinking. Subject permissioning is a substrate concern — operators constrain what an imp can publish on via NATS account ACLs on the imp's connection, not via a framework-side whitelist.
- **Soulstream operations** [V] — opening topics, posting turns, mentioning, via the `imps/soulstream` glue module's `Participant` (the owner library does the wire). These flow through the soulstream's subject conventions.

Action is thinking-only. Awareness can `Note` (which produces lightweight soulstream activity) and `Request` (a single bounded round-trip) but cannot fan out, fire-and-forget, or invoke side-effecting NATS calls outside the bounded shape. The energy gradient is preserved here too: cheap interpretation doesn't produce expensive outputs.

Every action emission produces an audit record [D] — what was emitted, by which imp, against which entity, in response to which escalation. The framework writes audit records to a tenant-scoped audit stream so the colony's behavior is reconstructable.

## How the parts fit together

A typical message flow:

1. A message arrives on a channel.
2. The channel decodes it, extracts an entity, and dispatches into awareness.
3. Awareness updates per-entity state, decides:
   - `Ignore` — done.
   - `Note` — emit a lightweight soulstream contribution, done.
   - `Think` — return; the framework queues a thinking invocation.
4. If `Think`, the framework invokes thinking asynchronously with the reason and entity.
5. Thinking runs, makes NATS calls as needed, emits actions, updates state.
6. The imp goes back to waiting for the next message (and possibly to sleep, if no work is pending).

The energy gradient is visible in the flow: every message goes through awareness; only some messages produce a `Think`; only `Think`s invoke thinking; only thinking produces unbounded NATS sends.

## The outbound surface

An imp's outbound work is byte-shaped NATS sends:

- **`Request(subject, payload)`** — single round-trip request/reply. Available on both awareness and thinking. Bounded by call shape: one publish, one reply, one deadline.
- **`RequestMany(subject, payload)`** — fan-out collect-with-window. Returns all replies that arrive within a configurable window, up to a configurable cap. Thinking only.
- **`Publish(subject, payload)`** — fire-and-forget. Thinking only.
- **`Conn() *nats.Conn`** — the raw NATS connection, for generic NATS-based client libraries. Thinking only.

Subjects are **literal**. The framework performs no prefix, platform-mode, or account transformation — what the imp declares is what hits the wire (constitution "Imps see one subject path"). Cross-account routing and multi-tenant scoping are operator concerns, configured via NATS account imports and ACLs on the connection the imp uses.

The framework holds no "capability" abstraction. Whatever is on the other end of a subject — an inference service, a knowledge store, a tool runner, a peer imp — is the operator's design. The imp talks to subjects; the framework consumes bytes.

There is no startup discovery, no resolved surface, no declared capabilities on the imp's spec, no `HasCapability` check, no `$SRV.INFO` round-trip performed by the framework on the imp's behalf. The framework is smaller than this — and the structural enforcement of the energy gradient does not depend on knowing what's on the other end of a subject.

## The wake-hook [per-entity V, whole-imp snapshot wake D]

The per-entity level is shipped in `imps/persist`: the durable store fires a wake hook on every rehydration with the elapsed time since that entity's last activity (exactly once, before the state is observable, never writing back). At the imp level, the shipped surface is the `Beacon` restart clock — self-reported elapsed across graceful stops, deploys, and (heartbeat-bounded) crashes, asked in `main()` before `Run`. The snapshot-sleep wake — an authoritative, runtime-delivered elapsed reaching imp code *mid-process* after a resume — is **[D]**, gated on the soulrealm runtime growing suspend/resume (roadmap M2b): no self-stamp can be authoritative when no imp code runs at suspend time. The wake hook is optional; imps that don't need it ignore it.

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

- **Five parts, not three or seven.** Three (awareness/thinking/action) was too few — memory and channels needed naming. Seven (the current projection/derivation/reactor/knowledge/index/etc. taxonomy) was too many for the developer surface. Five is what survived after pruning.
- **Awareness can issue a single `Request`.** Earlier drafts had awareness as fully-local. That broke as soon as classification needed an external call (embed, classify). A single bounded request/reply is structurally bounded by call shape; that's what landed.
- **Call-shape boundedness, not endpoint-metadata boundedness.** Considered metadata-driven enforcement (capability endpoints declare themselves bounded; the framework consults metadata). Rejected: it requires the framework to model capabilities, perform startup discovery, and maintain a resolved surface. Call-shape enforcement (awareness has `Request` only; thinking has the full set) gets the same guarantee with no framework-side bookkeeping.
- **The boundary is compile-enforced, not whitelist-enforced.** Earlier drafts constrained the outbound surface with a framework-side action whitelist (`ImpSpec.Actions`, rejected publishes, whitelist-violation metrics). Removed (constitution v2.2.0): subject permissioning is a substrate concern (NATS account ACLs), and the awareness/thinking boundary is enforced by *which methods exist on each context type*, proven by the `integration/compiletest/` build-tag assertions. The framework holds no whitelist.
- **One connection per imp, not two.** Considered separate awareness and thinking NATS connections with different subject permissions. Rejected as overengineering — type-level discipline plus subject permissions on a single connection is enough.
- **No discovery, no `$SRV.INFO` round-trip at startup.** Considered required-capability resolution that fails imp startup if dependencies are missing. Rejected: the framework holds no capability concept at all. Whatever is on the other end of a subject is the operator's design.
- **Subjects are literal.** Each subject the imp declares is the wire subject verbatim. The framework imposes no prefix, no platform-mode segment, no rewrites. Cross-account routing and tenant scoping are configured at the substrate via NATS account imports (constitution "Imps see one subject path").
- **Sleep is the common case, hard restart is the exception.** Memory and persistence are designed around snapshot-based sleep first; replay-from-backend is the cold-start path, not the hot path. The framework specifies the sleep/wake contract; the isolation mechanism that implements it (microVMs, containers, processes, simulation) is an infrastructure choice.
- **The wake-hook is mandatory in design even if optional in implementation.** Time-skip after sleep is a real bug class; designing it in early is cheaper than retrofitting.

If a future change conflicts with anything here, it gets explicit treatment — what changed, why, what other docs need to update. The boundaries this document defines are what makes the rest of the framework coherent.

# Harness Anatomy

*The five concepts, what each does, and the boundaries between them.*

---

The harness is what holds an imp together. It defines what an imp is made of, what each part is allowed to do, and how the parts fit together. This document is the contract. The vision document explains *why*; this document explains *what*.

The harness has five concepts: **channels**, **awareness**, **reasoning**, **memory**, **action**. Each has a defined surface, defined invariants, and a defined relationship to the others. Nothing in the harness exists outside these five.

## Channels

A channel is an inbound message subscription. Channels are how messages reach an imp.

An imp declares its channels in its spec — a list of NATS subjects (or subject patterns) it subscribes to. The harness establishes the subscriptions at startup and dispatches arriving messages into the imp's awareness layer.

Three things a channel does:

- **Subscribes.** The harness manages the NATS subscription's lifecycle, including consumer position across sleep/wake cycles.
- **Decodes.** Messages arrive on a channel as a typed value the imp understands. Channel declarations include a decode step (a schema reference, a parsing function) so awareness sees structured data, not raw bytes.
- **Dispatches.** The harness calls the awareness layer with the decoded message, an entity identifier (extracted by the channel from the message), and a channel-scoped context.

Channels can be of three kinds:

- **External channels** — subjects outside the imp's own namespace (e.g. `bridge.emails.>`, `sensors.temperature.>`). The imp watches the world here.
- **Soulstream channels** — subjects under `$soulstream.*` for topics the imp participates in. Joining a topic adds a soulstream channel; leaving removes it.
- **Schedule channels** — subjects fed by NATS server-side scheduling. The imp registers a schedule (cadence, target subject), and the harness subscribes the schedule's target as a channel. Periodic work flows through here.

All three kinds use the same dispatch mechanism. The awareness layer doesn't see which kind a message came from; it sees a message, an entity, and a context.

A channel is not allowed to call out to capabilities or do expensive work. Channels are pure subscribe-decode-dispatch. Anything more is awareness's job.

## Awareness

Awareness is cheap, local interpretation. It runs on every message a channel delivers. It decides whether reasoning needs to wake.

An awareness function takes a decoded message, an entity identifier, and an awareness context. It updates the imp's local state for that entity, and returns a verdict:

- **`Ignore`** — nothing to do. The state may have updated but no further action is warranted.
- **`Note`** — the state changed in a way worth recording in memory or visible to other imps via the soulstream, but no reasoning needed.
- **`Wake`** — escalate to reasoning. Carries a reason and the entity to focus on.

Awareness is allowed to:

- Read and write the imp's local state for the entity.
- Call **bounded** capabilities — embed, short classification, key-only knowledge lookup. Bounded means the response is deterministic in its resource envelope: single round-trip, capped latency, no fan-out, no side effects beyond the call itself.
- Emit lightweight notes to the soulstream (a comment, an attachment, a status update on an existing topic the imp is in).

Awareness is *not* allowed to:

- Call unbounded capabilities (semantic search, agentic loops, tool execution).
- Open new soulstream topics or send DMs.
- Publish to action subjects.
- Recurse into reasoning directly. (Returning `Wake` is how reasoning is invoked.)

The structural enforcement: the awareness context's type exposes only the bounded capability surface. The methods for unbounded operations don't exist on the awareness context. A developer cannot accidentally make awareness expensive because the methods aren't reachable. NATS subject permissions on the imp's connection enforce the same boundary as a backstop — a stray publish to an unbounded subject is rejected at the substrate.

The boundedness of a capability endpoint is not a property of the imp framework. It's declared in the capability service's endpoint metadata. The harness reads endpoint metadata at startup, builds the awareness-callable subject set from endpoints flagged bounded, and exposes only those through the awareness context. New bounded capabilities slot in without framework changes; capability authors decide what bounded means for their endpoint by virtue of how they implement it.

### Why "Note" exists

An awareness verdict of `Note` is the difference between "the imp's interpretation changed but the colony doesn't need to know" and "something happened that other imps or keepers should be able to see without waking my reasoning." A complaint-watcher seeing an inbound email about an existing topic might `Note` it (post the email as a turn) without escalating; only when something about the email warrants investigation does it `Wake`.

Without `Note`, awareness can't contribute to the soulstream without going through reasoning, which would force the energy gradient to collapse for any participation.

## Reasoning

Reasoning is expensive deliberation. It runs only when awareness escalates with `Wake`.

A reasoning function takes the wake reason, the entity, and a reasoning context. It does whatever the imp's purpose requires — investigates, decides, drafts, escalates — and emits actions.

Reasoning is allowed to:

- Read and write the imp's local state.
- Call **all** capabilities — bounded or unbounded.
- Open soulstream topics, post turns, mention other imps or keepers.
- Publish to declared action subjects.
- Delegate work via inter-imp request/reply.

Reasoning is *not* allowed to:

- Run synchronously inside a channel dispatch. Reasoning is async and runs in its own goroutine; the channel dispatch returns as soon as awareness yields `Wake`.
- Block awareness. Awareness for new messages continues to run while reasoning is in progress for an earlier wake.
- Reach into another imp's local state directly. Inter-imp coordination goes through the soulstream or explicit delegation.

A reasoning function may be called concurrently for different entities. Reasoning code is responsible for being safe under concurrency for the same entity if multiple wakes pile up; the harness offers per-entity serialization as an opt-in for cases where it matters.

### The reasoning context

The reasoning context is the imp's gateway to the world outside its local state. It exposes:

- **Capability calls** — `inference.complete(...)`, `knowledge.recall(...)`, `tools.invoke(...)`, etc. Each capability is a typed surface backed by NATS subjects, resolved at imp startup based on declared dependencies and discovery.
- **Soulstream operations** — open a topic, post a turn, mention, attach.
- **Action publishes** — publish on action subjects declared in the imp spec.
- **Local state access** — read and write the imp's per-entity state.
- **Delegation** — request/reply against other imps via the coordination pattern.

The reasoning context is the imp's single point of contact with anything external. Awareness has the bounded subset of the same context.

## Memory

Memory in the harness is **local**. It holds the state the imp needs to do its job for the entities it's currently tracking.

Local memory has three kinds, distinguished by what they hold and how they're indexed:

- **Per-entity state** — the running interpretation of one entity. Statistics, current state, recent observations, in-flight work. Indexed by `(name, entity)` where `name` identifies the kind of state and `entity` identifies which instance.
- **Imp-scoped facts** — typed records the imp looks up by key. Configuration, recent records, working sets. Not statistically interpreted.
- **Imp-scoped indexes** — secondary views over imp-scoped facts (filter, group-by). Cheap to maintain, useful for in-process lookups.

Local memory is bounded. The harness enforces retention and eviction by default — per-entity state has a configured lifetime, eviction sends cold entities to the persistence backend, on rehydration they come back. A developer who wants unbounded local memory has to opt in explicitly. The default is small and stays small.

Local memory does *not* hold:

- Cross-imp shared state.
- Long-term episodic history (closed topics, prior actions, completed investigations).
- Heavy indexes over large corpora.
- Anything other instances of the same imp class would benefit from sharing.

All of those live in the knowledge capability. The imp queries knowledge during reasoning when context from outside the immediate moment is needed.

### Persistence and sleep

Local memory survives sleep. When the harness suspends an imp via the configured isolation mechanism's snapshot facility, the full memory image is preserved; on wake, the imp resumes with state intact. Wall-clock time, however, has passed — the harness exposes a wake-hook that signals to the imp how long it slept, and projections that depend on wall-clock decay (EMA, "stable for", "idle since") use the hook to advance their internal state.

Persistence across hard restarts (process death, host loss) uses the standard snapshot/restore cycle: per-entity state serializes to the persistence backend on a configurable schedule, and on cold start the imp replays state from the backend plus any messages since the snapshot.

Sleep is the common case; hard restart is the exception. Both are handled, but sleep is what's optimized for.

## Action

Action is the harness's outbound surface. It's how the imp affects the world.

An imp has three kinds of action:

- **Channel publishes** — publishing on NATS subjects via the reasoning context. Subject permissioning is a substrate concern — operators constrain what an imp can publish on via NATS account ACLs on the imp's connection, not via a framework-side whitelist.
- **Soulstream operations** — opening topics, posting turns, mentioning. These flow through the soulstream's subject conventions.
- **Capability calls with side effects** — tool execution, delegation. These go through the reasoning context's capability surface.

Action is reasoning-only. Awareness can `Note` (which produces lightweight soulstream activity) but cannot publish to action subjects or invoke side-effecting capabilities. The energy gradient is preserved here too: cheap interpretation doesn't produce expensive outputs.

Every action emission produces an audit record — what was emitted, by which imp, against which entity, in response to which wake. The harness writes audit records to a tenant-scoped audit stream so the colony's behavior is reconstructable.

## How the parts fit together

A typical message flow:

1. A message arrives on a channel.
2. The channel decodes it, extracts an entity, and dispatches into awareness.
3. Awareness updates per-entity state, decides:
   - `Ignore` — done.
   - `Note` — emit a lightweight soulstream contribution, done.
   - `Wake` — return; the harness queues a reasoning invocation.
4. If `Wake`, the harness invokes reasoning asynchronously with the wake reason and entity.
5. Reasoning runs, calls capabilities as needed, emits actions, updates state.
6. The imp goes back to waiting for the next message (and possibly to sleep, if no work is pending).

The energy gradient is visible in the flow: every message goes through awareness; only some messages produce a `Wake`; only `Wake`s invoke reasoning; only reasoning produces actions or capability calls.

## Capability surfaces, formalized

An imp's spec declares which capabilities it depends on, expressed as required and optional subject prefixes:

```
capabilities:
  required:
    - prefix: knowledge.cache.*
    - prefix: inference.embed
  optional:
    - prefix: knowledge.episode.*
    - prefix: tools.email.*
```

At startup, the harness:

1. Issues `$SRV.INFO` against the deployment.
2. Aggregates endpoint metadata from all replies.
3. Verifies every required prefix has at least one matching endpoint with declared metadata.
4. Records which optional prefixes are satisfied (for the imp to check at runtime if it adapts behavior).
5. Builds two capability surfaces from the satisfied endpoints:
   - The **awareness surface** — endpoints whose metadata declares them bounded.
   - The **reasoning surface** — all satisfied endpoints.
6. Exposes the awareness surface through the awareness context type and the reasoning surface through the reasoning context type.

Required dependencies that aren't satisfied cause startup failure with a clear error naming the missing prefix and what *was* available. Optional dependencies that aren't satisfied are recorded; the imp checks for them via `ctx.HasCapability(prefix)` before calling.

Boundedness lives in the capability service's endpoint metadata. The harness consumes that declaration; it does not maintain its own list of "which subjects are awareness-callable." This means the framework doesn't need to know about new capabilities to support them — a new capability service registers endpoints with appropriate metadata, imps declare dependencies on its subject prefixes, the harness routes everything else.

## The wake-hook

When the harness restores an imp from a snapshot, it calls the imp's wake hook with the wall-clock duration that elapsed during sleep. The wake hook is optional; imps that don't need it ignore it.

Imps that do need it — anything with EMA decay, "stable for" / "idle since" semantics, debounce windows, time-to-live expirations — use the hook to advance the affected state by the elapsed duration before processing the wake-up message.

The hook is a single call, before any channel dispatch resumes after wake. Implementations are typically short:

```
on_wake(slept_for):
    for each entity in self.state:
        entity.advance_time(slept_for)
```

Without the wake hook, time-dependent state silently produces wrong answers after sleep. Bake it in early.

## What's not in the harness

Several things you might expect are deliberately absent:

- **No internal goroutine pool for periodic work.** Periodic work is schedule channels; the harness has no `RunEvery` primitive of its own.
- **No internal LLM client, embedding cache, vector index, or knowledge store.** These are capability services; the harness has clients for talking to them, not implementations.
- **No spec registry or central coordination service.** Discovery is `$SRV.INFO` against a live deployment.
- **No imp-to-imp shared memory.** Coordination goes through the soulstream.
- **No retry/circuit-breaker logic on capability calls beyond simple timeouts.** Imps handle degradation explicitly. (The harness offers a thin helper for the common timeout pattern, but the framework doesn't impose policy.)

These omissions are deliberate. Each thing the harness *doesn't* do is a thing it can't get wrong, can't gold-plate, and can't impose on developers who don't need it.

## Decisions and tradeoffs

A record of choices and their rationale, for future reference:

- **Five concepts, not three or seven.** Three (awareness/reasoning/action) was too few — memory and channels needed naming. Seven (the current projection/derivation/reactor/knowledge/index/etc. taxonomy) was too many for the developer surface. Five is what survived after pruning.
- **Awareness can call bounded capabilities.** Earlier drafts had awareness as fully-local. That broke as soon as classification needed embeddings. Bounded-by-construction with metadata-driven enforcement is what landed.
- **One connection per imp, not two.** Considered separate awareness and reasoning NATS connections with different subject permissions. Rejected as overengineering — type-level discipline plus subject permissions on a single connection is enough.
- **No dedicated capability-discovery endpoint.** Considered but rejected: `$SRV.INFO` plus endpoint metadata already carry the contract. Adding a parallel endpoint duplicates state and creates drift potential.
- **Subject taxonomy is per-capability, not framework-wide.** Each capability designs its own subjects. The framework does not impose a prefix or platform-mode segment — the imp's declared subjects are the substrate subjects verbatim (constitution "Imps see one subject path"). Cross-account routing and tenant scoping are configured at the substrate via NATS account imports.
- **Sleep is the common case, hard restart is the exception.** Memory and persistence are designed around snapshot-based sleep first; replay-from-backend is the cold-start path, not the hot path. The framework specifies the sleep/wake contract; the isolation mechanism that implements it (microVMs, containers, processes, simulation) is an infrastructure choice.
- **The wake-hook is mandatory in design even if optional in implementation.** Time-skip after sleep is a real bug class; designing it in early is cheaper than retrofitting.

If a future change conflicts with anything here, it gets explicit treatment — what changed, why, what other docs need to update. The boundaries this document defines are what makes the rest of the framework coherent.
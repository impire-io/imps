# Vision

*What an imp is, what it isn't, and why the framework is shaped the way it is.*

---

An **imp** is a small, focused, awareness-driven agent. It watches a slice of the world, builds an interpretation of what it sees, and acts when its interpretation crosses a threshold worth acting on. One imp does one thing.

What this framework gives you is the structure an imp is built around: receiving messages, interpreting them cheaply, thinking about them when thinking is warranted, and emitting actions. The framework stays small. The capabilities an imp can reach for — inference, knowledge, tooling — live outside it as separate services.

This document records the framing the rest of the design hangs from. Everything else is a consequence of these choices.

## Imps are specialists, not generalists

An imp does one thing well. It watches one kind of input, holds one kind of interpretation, takes one kind of action. A complaint-watcher watches emails for complaints; it doesn't also watch invoices for late payments. If you want both, that's two imps.

The reason is operational, not aesthetic. A specialist imp has a small, bounded surface — a fixed set of channels it subscribes to, a known shape of state it maintains, a defined set of actions it can take. That makes it cheap to reason about, cheap to test, cheap to deploy, cheap to keep awake or put to sleep. A generalist imp is the opposite of all those things.

A consequence: many imps cooperate to produce outcomes a single imp couldn't. The framework's coordination story (the soulstream) is what makes that possible. Specialization at the imp level requires good collaboration at the colony level.

## The energy gradient is structural

A human brain spends almost no energy on most of what reaches the senses. Attention surfaces things worth thinking about; thinking happens only on the small subset that earned it. This is the principle the framework is built on.

Inside an imp:

- **Awareness** is cheap. Local interpretation, bounded resource use, deterministic latency. Awareness runs continuously, on every message, and decides whether thinking is warranted.
- **Thinking** is expensive. LLM calls, multi-step tool use, semantic recall over large corpora. Thinking runs only when awareness escalates.

This isn't a coding convention — it's a structural invariant the framework enforces. Awareness has a tightly-bounded surface for the capabilities it can call (a single embed, a budget-capped classification, a key-only lookup); thinking has the full surface (agentic loops, semantic search, tool invocation, delegation). A developer cannot accidentally make awareness expensive because the surface for expensive operations isn't available there.

The boundary lives at the framework level, not the implementation level. What awareness *is* is "the part of the imp that runs cheaply on every message." What thinking *is* is "the part that runs occasionally, and is allowed to do expensive things."

## The framework is small; capabilities are external

The framework gives an imp:

- Channels — how messages reach the imp.
- Awareness — cheap, local interpretation; decides when thinking runs.
- Thinking — expensive deliberation; allowed to call capabilities.
- Memory — *local*, bounded state per entity the imp is currently tracking.
- Action — outbound publishing to channels.

That's the whole vocabulary. Five concepts, each with a small, bounded surface.

What the framework does *not* give an imp:

- Historical knowledge across the colony.
- Inference (LLM completion, embeddings).
- Tool execution surfaces beyond simple NATS publishes.
- Cross-imp shared state.
- Heavy indexes (vector, graph) over large corpora.

Those live in **capability services** — separate NATS services that imps reach for during thinking. A capability service is a regular NATS micro service that handles a specific kind of work. Knowledge is a capability. Inference is a capability. Tool execution is a capability. The framework is small precisely because these aren't part of it.

The shape this gives the system: an imp is a thin orchestrator. It sees messages, holds enough state to decide what to do with them, and reaches for capabilities when the work warrants. The intelligence isn't in the imp; it's in what the imp can call.

## Capability services follow shared deployment patterns; their wire protocols are their own

Every capability service registers as a NATS micro service, declares what it offers via standard `$SRV.*` discovery, follows the platform/non-platform subject prefix convention, and stays stateless per request. This is the *deployment shape*, and it's uniform across capabilities.

What's not uniform is the wire protocol. Inference's `prompt`/`embed` with the streaming-empty-terminator pattern is specific to inference. Knowledge's `recall`/`remember` will look different. A future tool-execution capability will look different again. There is no generic "capability protocol" that all of them satisfy — and trying to invent one would force every capability into a lowest-common-denominator surface that fits none of them well.

This split — uniform deployment, capability-specific wire protocol — is what lets the ecosystem of capabilities grow without the framework growing.

## Imps sleep when they're not working

An imp that has nothing to do consumes no compute. The framework uses snapshot-based suspension to preserve an imp's full memory image; NATS holds messages destined for the imp on its subscribed subjects; on arrival, the imp wakes, processes, and goes back to sleep. The imp doesn't know it was asleep. The specific isolation mechanism — microVMs, containers, processes, or in-process simulation — is an infrastructure choice the framework does not dictate; what the framework specifies is the contract that any isolation mechanism satisfies.

Periodic work uses NATS server-side scheduling: the imp registers a recurring schedule on a subject it subscribes to, and the schedule fires whether the imp is currently warm or cold. Schedule TTLs control whether stale ticks accumulate during long sleeps.

The architectural consequence: there's no meaningful distinction between "running" and "ready-to-run" imps. An imp is a message-handler that happens to be currently warm or currently cold. Operationally, most of the colony is cold at any given moment; semantically, the colony is always there.

This collapses an earlier-considered distinction between persistent and ephemeral imps. Every imp is persistent in identity and procedural memory; every imp is ephemeral in compute footprint when idle. There's one shape, not two.

## Coordination happens through the soulstream

Imps don't talk to each other directly. They participate in **topics** on the soulstream — a shared NATS-backed coordination surface where work that involves multiple imps (and humans) gets organized. A topic has a subject, expected participants, an op-log of contributions, and a lifecycle. Imps join topics, contribute, and leave; humans (keepers) participate as peers through their cockpit.

The soulstream is *the* collaboration medium. Direct request/response between specific named imps is supported but rarely the right tool; topics are. This is what makes specialization at the imp level produce intelligence at the colony level — small specialists posting findings to a shared surface where the right next specialist picks them up.

The framework keeps the soulstream's mechanics simple by default — sequential turn-taking, ordered op-logs, lightweight participation — and reserves richer machinery (concurrent merge via CRDTs, etc.) for cases that genuinely need it.

## Memory is layered, with most of it outside

An imp holds local state for the entities it's currently tracking — recent messages, running statistics, current state, in-flight work. That's it. The imp does not hold the colony's history.

Long-term memory — closed topics, completed actions, prior episodes, accumulated facts — lives in the **knowledge capability**. Imps reach for it during thinking when context from outside the immediate moment is needed. The knowledge service can be backed by anything that fits the scope (Elasticsearch, vector store, graph store, combinations); imps see a uniform protocol regardless of the backing.

Two scopes that matter and are usefully distinct:

- **Tenant-scoped knowledge** — what the tenant knows about its world. Customers, products, prior incidents.
- **Imp-class-scoped knowledge** — what a class of imp has learned about doing its job. The procedural memory of "we tried this remediation and it worked." Visible to all instances of the same imp class, isolated from other classes.

Both are deployment configurations of the same capability protocol. An imp discovers which knowledge endpoints are available and uses what it has access to.

## What the framework optimizes for

Three constraints, in priority order:

1. **Imps stay small and agile.** A 4GB imp is broken. A developer who feels the framework is overdesigned has been failed by it. The framework gets out of the way.
2. **Specialization composes.** Many small imps cooperating produce outcomes one imp couldn't. The framework makes coordination cheap.
3. **The capability ecosystem is open-ended.** New capabilities slot in without framework changes. The framework defines deployment patterns, not capability implementations.

Where these tensions arise — and they will — the small-and-agile constraint wins. The framework is not the place to solve every problem; it's the place to make solving problems straightforward.

## What this isn't

An imp is not a chatbot. It's not a request handler. It's not a long-running agentic loop chewing on a task until done. It's a small specialist watching a slice of the world, with structure that lets it reach for help when the work warrants.

The framework is not a platform for building one big intelligent thing. It's a substrate for many small specialists cooperating, each of which is dumb on its own and smart in aggregate. If that aesthetic is what you want, the framework fits. If you want a single agent that does everything, you want something else.

---

The rest of the documents in this set work out the consequences. The anatomy document specifies what awareness and thinking are, what they can do, and what the boundary between them looks like. The capability service pattern specifies the shared deployment shape. Per-capability specs define their wire protocols. The soulstream document specifies coordination. The developer-surface document specifies what writing an imp actually looks like in code.

If a future change conflicts with anything in this document, this document is what changes — not silently, not by drift, but explicitly, with the rationale. The vision is the load-bearing layer; everything else is implementation of it.
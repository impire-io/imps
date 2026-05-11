# Feature Specification: Harness Core

**Feature Branch**: `001-harness-core`
**Created**: 2026-05-10
**Status**: Draft
**Input**: User description: "The harness core. The minimal substrate that holds an imp together — channel subscription, awareness dispatch, reasoning invocation, local per-entity memory, and action publishing."

## Overview

This feature delivers the smallest substrate that makes an imp an imp: a way to declare what an imp is made of (channels, awareness, reasoning, local memory, actions), a runtime that wires those declarations to the messaging substrate, and structural enforcement of the energy gradient (cheap awareness, expensive reasoning) and the action whitelist.

The harness core is the first feature of the imp framework rewrite. Capability clients, the bounded capability surface, the soulstream, sleep/wake, schedule channels, persistence, and audit emission are deliberately out of scope and ship as separate features. Each load-bearing commitment from the constitution applies: the harness stays small, the awareness/reasoning boundary is structural rather than policy, and capabilities remain external.

## Clarifications

### Session 2026-05-10

- Q: What inbound channel source kinds ship in v1? → A: Subject and stream (with durable consumer optional per channel). KV channels are deferred to a follow-up feature.
- Q: When does the harness ack a stream-channel message? → A: After the awareness function returns any verdict (Ignore, Note, or Wake). Decode/extraction failure or awareness panic/error → NAK and substrate-governed redelivery (consumer max-deliveries). Reasoning success or failure does not affect the originating message's ack state.
- Q: Does the harness bound in-flight reasoning concurrency per imp? → A: No bound in v1. Keeping wake rate × reasoning latency within the imp's footprint is the developer's responsibility. The harness MUST expose the current in-flight reasoning count via its observability surface so developers can monitor and react. Bounded-concurrency policies and overflow handling are deferred to a follow-up feature.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - End-to-end imp dispatch (Priority: P1)

A developer declares an imp with one channel (a NATS subject pattern with a decoder and entity extractor), an awareness function that returns `Wake` when an entity crosses a threshold, a reasoning function that publishes an action, and one declared action subject. They start the harness against a NATS connection. A producer publishes a message on the channel subject; the message is decoded, the entity extracted, awareness invoked, reasoning queued, and an action emitted on the declared subject — all without the developer writing subscription, dispatch, or queueing code.

**Why this priority**: This is the core promise of the framework. Without channel→awareness→reasoning→action, there is no imp. Every other behavior is an invariant on top of this flow.

**Independent Test**: Construct an imp with a single channel, an awareness function that always returns `Wake`, and a reasoning function that publishes a known payload to a single declared action subject. Run an embedded NATS server. Publish one message on the channel subject; subscribe to the action subject; assert the action arrives with the expected payload within a small bounded time.

**Acceptance Scenarios**:

1. **Given** a running imp with a channel subscribed to subject `S` and a reasoning function that publishes to declared subject `A`, **When** a message is published on `S` and awareness returns `Wake`, **Then** the harness publishes the reasoning function's output on `A` exactly once.
2. **Given** the channel decode step rejects a malformed payload, **When** a malformed message arrives on `S`, **Then** the harness records a decode failure, awareness is not invoked for that message, and subsequent well-formed messages still flow through normally.
3. **Given** the entity extractor returns an empty/zero entity for a message, **When** that message arrives, **Then** the harness records an extraction failure and the message is not dispatched into awareness.

---

### User Story 2 - Awareness verdicts produce the right downstream behavior (Priority: P1)

A developer's awareness function inspects an incoming message and returns one of three verdicts. `Ignore` does nothing further. `Note` produces a lightweight, locally observable record but does not invoke reasoning. `Wake` queues reasoning asynchronously with a wake reason and entity identifier. The harness honors each verdict exactly.

**Why this priority**: The energy gradient — cheap awareness, expensive reasoning — is delivered by these verdicts. If `Note` invokes reasoning, or `Ignore` leaks state, or `Wake` is synchronous with channel dispatch, the framework's identity collapses.

**Independent Test**: Construct three minimal imps (or one imp with three test channels), each whose awareness function returns a different verdict. Publish on each channel and observe: (a) `Ignore` produces no reasoning invocation and no recorded note; (b) `Note` produces a recorded note but no reasoning invocation; (c) `Wake` produces a reasoning invocation with the supplied reason and entity. Verify reasoning runs in a separate goroutine — channel dispatch returns before reasoning completes.

**Acceptance Scenarios**:

1. **Given** awareness returns `Ignore`, **When** the message has been dispatched, **Then** no reasoning invocation is queued and no note record is produced.
2. **Given** awareness returns `Note` with a payload, **When** the message has been dispatched, **Then** a note record carrying that payload is observable through the imp's note hook and reasoning is not invoked.
3. **Given** awareness returns `Wake` with a reason `R` and entity `E`, **When** the message has been dispatched, **Then** the harness invokes reasoning asynchronously with reason `R` and entity `E`, and channel dispatch returns before reasoning completes.

---

### User Story 3 - The awareness/reasoning boundary is structural (Priority: P1)

The awareness context type exposes only what awareness is allowed to do — local state access and the verdict return. The reasoning context type exposes local state access and action publishing on declared subjects. The compiler refuses code that calls a publish from awareness because the method does not exist on the awareness context. At runtime, an action publish to a subject that is not in the imp's declared whitelist is rejected before reaching the messaging substrate, with a clear error naming the offending subject.

**Why this priority**: This is the constitutional guarantee. The harness does not enforce the boundary by convention or runtime check alone — it enforces it by typed surface. A developer who tries to publish from awareness writes code that does not compile; a developer who tries to publish off-whitelist gets a clear rejection at the call site.

**Independent Test**: Attempt to call action-publish from inside the awareness context — verify the type system prevents it (a build-time check or compilation test). At runtime, call reasoning-context publish with a subject not in the spec's whitelist; assert the publish returns a typed whitelist-violation error, the violation does not appear on the messaging substrate, and the imp continues running.

**Acceptance Scenarios**:

1. **Given** the awareness context type, **When** code attempts to invoke action-publish from awareness, **Then** the code fails to compile (the publish method is not present on the awareness context).
2. **Given** an imp whose action whitelist is `{A1, A2}`, **When** reasoning calls publish on subject `A3`, **Then** the call returns a whitelist-violation error naming `A3`, no message reaches NATS, and the reasoning function continues to control flow.
3. **Given** an imp whose action whitelist is `{A1, A2}`, **When** reasoning calls publish on subject `A1`, **Then** the message is published on `A1` and the publish call returns success.

---

### User Story 4 - Stream channel ingestion with declared durability (Priority: P1)

A developer declares a stream channel that consumes from a JetStream stream rather than a raw subject. The channel declaration names the stream, a filter subject (or pattern within the stream), and a consumer config — including an optional durable consumer name. Every other harness behavior (decode, entity extraction, awareness dispatch, reasoning invocation, action publishing) is identical to a subject channel; the imp's awareness and reasoning code is unchanged. Stream channels with a durable name survive process restart and bind the existing consumer; non-durable stream channels create an ephemeral consumer that is torn down on shutdown.

**Why this priority**: Stream channels are first-class in v1 alongside subject channels. Without them, any imp that needs replay across restarts or transient disconnects would have to bypass the harness — defeating the "harness is the minimal substrate" framing.

**Independent Test**: Construct an imp with a single stream channel referencing an existing JetStream stream and a durable consumer name. Run an embedded NATS server with JetStream. Publish a message to the stream's source subject; subscribe to the imp's declared action subject; assert the action arrives. Stop and restart the harness; publish another message; assert the imp resumes consumption from the durable position without redelivering already-acked messages.

**Acceptance Scenarios**:

1. **Given** a stream channel referencing stream `S` with filter subject `F` and durable name `D`, **When** the harness starts and the stream and consumer either exist or can be created, **Then** the harness binds (or creates) consumer `D` on stream `S` and begins dispatching messages on `F` through the same awareness/reasoning flow as a subject channel.
2. **Given** a stream channel with no durable name declared, **When** the harness starts, **Then** an ephemeral consumer is created; **When** the harness shuts down cleanly, **Then** the ephemeral consumer is torn down (no orphaned consumer remains).
3. **Given** a stream channel referencing a stream that does not exist, **When** the harness attempts to start, **Then** startup fails with a clear error naming the missing stream; no subscriptions are established.
4. **Given** a stream channel referencing an existing durable consumer whose configuration is incompatible with the channel's declared config (e.g., different filter subject), **When** the harness attempts to start, **Then** startup fails with a clear error naming the incompatibility.
5. **Given** a stream channel and an awareness function that returns any verdict without panic, **When** awareness returns, **Then** the underlying substrate message is acknowledged before reasoning completes; reasoning success or failure does not change the ack state.
6. **Given** a stream channel and either a decode/extraction failure or an awareness panic on a delivered message, **When** the failure is recorded, **Then** the underlying substrate message is negative-acknowledged and the substrate's consumer max-deliveries policy governs redelivery.

---

### User Story 5 - Per-entity local memory with bounded capacity (Priority: P2)

A developer declares one or more local-state shapes at imp construction (each with a name, a per-entity factory/zero-value, and a per-entity-count cap). On the first reference to `(state-name, entity)` from awareness or reasoning, the harness allocates an instance and indexes it. Subsequent references return the same instance. When the number of distinct entities for a given state name reaches the cap, attempts to allocate a new entity slot return a clear, typed cap-exceeded error rather than silently evicting an existing entity. Reads and writes on existing slots continue to work after the cap is reached.

**Why this priority**: Local memory is what awareness and reasoning operate on. Without per-entity scoping, awareness cannot interpret an entity over time. Without a bounded cap, the harness cannot stay small. Eviction policies belong to the sleep/wake feature, not here — so the cap fails loudly.

**Independent Test**: Construct an imp with a single state shape capped at `N`. Drive `N` distinct entities through awareness and verify each gets its own state instance retained across calls. Drive a `(N+1)`-th entity and verify the awareness/reasoning context returns a typed cap-exceeded error. Verify reads and writes against the original `N` entities still succeed after the rejection.

**Acceptance Scenarios**:

1. **Given** an imp with state shape `S` capped at `N`, **When** awareness or reasoning references `(S, entity_i)` for `i` in `1..N`, **Then** each reference returns a stable per-entity instance and subsequent reads return prior writes.
2. **Given** the cap of `N` distinct entities for state `S` has been reached, **When** awareness or reasoning attempts to allocate `(S, entity_{N+1})`, **Then** the call returns a typed cap-exceeded error naming the state shape and current count.
3. **Given** the cap has been reached and a cap-exceeded error has been raised, **When** awareness or reasoning later reads or writes one of the existing `N` entities, **Then** that read/write succeeds.

---

### User Story 6 - Reasoning runs concurrently and never blocks awareness (Priority: P2)

Two distinct entities receiving messages in close succession produce two reasoning invocations that run concurrently — neither waits for the other. A slow reasoning function for one entity does not delay the awareness layer for new messages on the same imp; awareness for new messages continues to dispatch while earlier reasoning is still in flight.

**Why this priority**: The energy gradient depends on awareness staying responsive. If reasoning can backpressure awareness, the imp gradually stops noticing the world. This invariant has to be wired in from day one.

**Independent Test**: Construct an imp whose reasoning function blocks on a controllable signal. Publish messages for two distinct entities and verify both reasoning invocations are running before either is released. While one reasoning invocation is held blocked, publish a third message (any entity) and verify awareness is dispatched and (if it returns `Wake`) the third reasoning invocation begins — without waiting for the held one to release.

**Acceptance Scenarios**:

1. **Given** reasoning is currently running for entity `E1` with a deliberate hold, **When** a new message arrives that maps to entity `E2` and awareness returns `Wake`, **Then** reasoning begins for `E2` immediately, observably overlapping with `E1`'s reasoning.
2. **Given** reasoning is currently running for any entity with a deliberate hold, **When** a new message arrives on the same channel, **Then** awareness is invoked for that message within a small bounded delay regardless of the held reasoning.
3. **Given** several reasoning invocations are in flight, **When** the harness is asked to shut down, **Then** new messages stop being dispatched, in-flight reasoning is given a configured drain window to complete, and shutdown returns once all in-flight reasoning has finished or the drain window has elapsed.

---

### User Story 7 - Same code, both deployment modes (Priority: P2)

The same imp definition runs in non-platform mode (subjects prefixed with a configured prefix; the imp lives in the same account as its consumers and producers) and in platform mode (subjects include the importing account's public key as a path segment; the imp lives in a platform account whose endpoints are exported to importer accounts). Endpoints, dispatch behavior, contracts, and developer-facing API are identical across modes — only the resolved subject path and per-request account attribution differ.

**Why this priority**: The framework's commitment is one codebase, two deployment shapes. If a developer has to write or test mode-specific code, the abstraction has failed.

**Independent Test**: Run the same imp definition twice against an embedded NATS server: once with `platform_mode = false` and a configured prefix, once with `platform_mode = true` and a configured importer account public key. Publish a message on the resolved subject for each mode; assert the action is published on the correctly-resolved action subject for each mode. The imp's source code is identical between runs.

**Acceptance Scenarios**:

1. **Given** the imp's spec declares channel subject `messages.in` and action subject `actions.out` and `platform_mode = false` with prefix `tenantA.imps.demo`, **When** the harness starts, **Then** the channel subscribes to `tenantA.imps.demo.messages.in` and reasoning publishes to `tenantA.imps.demo.actions.out`.
2. **Given** the same imp spec with `platform_mode = true` and importer account public key `ACCOUNT_PK`, **When** the harness starts, **Then** the channel subscribes and the action publishes to subjects that include `ACCOUNT_PK` as a path segment in the documented position, and the developer-facing channel and action declarations are unchanged from non-platform mode.
3. **Given** the harness is configured with `platform_mode = true` but no importer account public key is supplied, **When** the harness starts, **Then** startup fails with a clear configuration error naming the missing field; no subscriptions are established.

---

### User Story 8 - Clean lifecycle (Priority: P3)

The harness starts by establishing every declared subscription and registering the imp's identity (a name, a version, the resolved subject prefix). If any required setup step fails, the harness aborts startup and reports the cause; partial setup leaves no dangling subscriptions. On shutdown, the harness stops accepting new messages, drains in-flight reasoning within a configured window, unsubscribes cleanly, and returns.

**Why this priority**: A framework that leaks goroutines, leaves subscriptions open, or silently drops in-flight work on shutdown is operationally broken. Lifecycle is foundational but the surface is small enough to layer onto stories 1–7.

**Independent Test**: Start the harness against an embedded NATS server, observe the subscriptions on the expected resolved subjects, then call shutdown while two reasoning invocations are in flight. Assert: subscriptions are removed, the in-flight reasoning is allowed to complete (or times out at the configured drain window), shutdown returns within a bounded time, and no goroutines remain.

**Acceptance Scenarios**:

1. **Given** an imp spec, **When** the harness starts, **Then** every declared channel has a corresponding subscription on the resolved subject and the imp's identity (name, version, resolved subject prefix) is queryable.
2. **Given** any required startup step fails (e.g., an action subject is malformed, the NATS connection is unavailable), **When** the harness attempts to start, **Then** startup returns a clear error, no subscriptions remain registered, and no dispatch goroutines are running.
3. **Given** in-flight reasoning at the moment of shutdown, **When** shutdown is invoked with drain window `D`, **Then** new dispatches stop immediately, in-flight reasoning continues until completion or `D` elapses, and shutdown returns no later than `D` after the call.

---

### Edge Cases

- **Decode failure** on an arriving message — the harness records the failure (counter or hook) and continues dispatching subsequent messages; awareness is not invoked for the failed message.
- **Entity extraction failure** (extractor returns empty/zero or an error) — same handling as decode failure; awareness is not invoked.
- **Awareness panics** during dispatch — the panic is recovered, recorded against the imp, and dispatch for subsequent messages continues. The current message is treated as not having been dispatched.
- **Reasoning panics or returns an error** — the panic/error is recovered and recorded; the in-flight reasoning is considered finished (no retry by the harness — retry policy belongs to the imp). Other entities and channels are unaffected.
- **Reasoning runs longer than the shutdown drain window** — shutdown returns at the deadline; the harness does not block indefinitely.
- **Awareness or reasoning attempts to write to a state shape that was not declared in the spec** — the call returns a typed "unknown state shape" error.
- **Two awareness calls for the same entity arrive concurrently from two channels** — both observe a consistent snapshot of state for that entity (read-modify-write on the same entity is serialized within a state shape; cross-shape ordering is not guaranteed).
- **Channel pattern matches but the imp's connection lacks subject permission** — the subscription either is rejected at startup (preferred) or surfaces a runtime publish/subscribe authorization error captured by the harness.
- **`platform_mode = true` but no importer account public key configured** — startup fails with a clear configuration error (acceptance scenario 7.3).
- **Stream channel: stream does not exist at startup** — startup fails with a clear error naming the missing stream; no subscriptions are established (acceptance scenario 4.3).
- **Stream channel: durable consumer exists but its server-side config is incompatible with the channel's declared config** — startup fails with a clear error naming the incompatibility (acceptance scenario 4.4).
- **Stream channel: ack failure on the substrate** (e.g., transient JetStream error during ack) — the harness records the failure and continues; the substrate's redelivery semantics govern whether the message is redelivered.
- **Stream channel: max-deliveries exceeded on the consumer** — substrate-handled (the consumer's max-deliveries config governs); the harness exposes the message-stuck signal through its observability surface but does not introduce its own retry or dead-letter logic.
- **Multiple state shapes share the same name in the spec** — startup fails with a clear duplicate-shape error.
- **The same action subject appears on the whitelist more than once** — duplicates are accepted and de-duplicated; whitelist semantics are set membership.

## Requirements *(mandatory)*

### Functional Requirements

#### Imp specification

- **FR-001**: Developers MUST be able to construct an imp by declaring: a name, a version, zero or more channels, an awareness function, a reasoning function, zero or more local-memory state shapes (each with a name, a per-entity factory/zero-value, and a per-entity-count cap), and an action subject whitelist.
- **FR-002**: The harness MUST reject construction at the call site if the spec is incomplete (missing name, missing awareness function, missing reasoning function), if state shapes have duplicate names, or if any state shape's per-entity cap is non-positive. The error MUST name the offending field.
- **FR-003**: An imp's identity (name, version, resolved subject prefix) MUST be queryable from the running harness.

#### Channels

- **FR-004**: Each channel declaration MUST specify a **source kind** of either `subject` or `stream`, a kind-appropriate source descriptor (FR-004a / FR-004b), a decode step that turns a delivered message into a typed value, and an entity-extraction step that returns the entity identifier for that message.
  - **FR-004a**: For source kind `subject`, the source descriptor MUST be a NATS subject or subject pattern.
  - **FR-004b**: For source kind `stream`, the source descriptor MUST include (i) a JetStream stream name, (ii) a filter subject (or pattern within the stream), and (iii) a consumer config that MAY declare a durable consumer name and other consumer-config fields the harness passes through to the substrate.
- **FR-005**: At startup, for each declared channel the harness MUST establish the appropriate inbound subscription on the resolved subject for the active deployment mode (see FR-030/FR-031): a NATS core subscription for subject channels, and a JetStream consumer bind or create for stream channels.
  - **FR-005a**: For a stream channel with a declared durable consumer name `D` on stream `S`, the harness MUST bind to `D` if it already exists with a configuration compatible with the channel's declared config; if `D` does not exist, the harness MUST create it with the declared config; if `D` exists with an incompatible config, startup MUST fail with a clear error naming the incompatibility.
  - **FR-005b**: For a stream channel with no declared durable consumer name, the harness MUST create an ephemeral consumer at startup and MUST tear it down on clean shutdown.
  - **FR-005c**: For a stream channel referencing a stream that does not exist, startup MUST fail with a clear error naming the missing stream.
- **FR-006**: When a message arrives on a subscribed channel (subject or stream), the harness MUST invoke the channel's decode step. If decode returns an error, the harness MUST record the failure, MUST NOT invoke awareness for that message, and MUST continue dispatching subsequent messages.
- **FR-007**: When decode succeeds, the harness MUST invoke the channel's entity extractor on the decoded value. If extraction fails or returns an empty/zero entity, the harness MUST record the failure and MUST NOT invoke awareness for that message.
- **FR-008**: When decode and extraction succeed, the harness MUST dispatch the decoded value, the entity, and an awareness context into the imp's awareness function. The dispatch contract is identical for subject and stream channels — awareness sees no difference in source kind.
- **FR-008a**: For stream channels, the harness MUST acknowledge the delivered substrate message after the awareness function returns a verdict (`Ignore`, `Note`, or `Wake`) without panic. If decode or entity extraction fails for the message, or if awareness panics, the harness MUST negative-acknowledge the message; the substrate's consumer max-deliveries policy then governs redelivery. Reasoning success or failure MUST NOT affect the ack/NAK of the originating message — the ack decision is made at awareness completion.

#### Awareness

- **FR-009**: An awareness function MUST be invoked synchronously inside channel dispatch with the decoded value, the entity, and an awareness context.
- **FR-010**: An awareness function MUST return one of three verdicts: `Ignore`, `Note(payload)`, or `Wake(reason, entity)`. The verdict type MUST be defined by the harness; the payload and reason types are user-defined.
- **FR-011**: When awareness returns `Ignore`, the harness MUST take no further action (no reasoning, no recorded note).
- **FR-012**: When awareness returns `Note(payload)`, the harness MUST produce a locally observable note record carrying the payload and MUST NOT invoke reasoning.
- **FR-013**: When awareness returns `Wake(reason, entity)`, the harness MUST queue an asynchronous reasoning invocation with the supplied reason and entity, and dispatch MUST return before reasoning runs.
- **FR-014**: The awareness context type MUST expose only local-state access and the verdict return path. The action-publish method MUST NOT exist on the awareness context (compile-time absence, not runtime check).
- **FR-015**: If awareness panics, the harness MUST recover, record the failure, and continue dispatching subsequent messages. The current message is treated as not dispatched (no reasoning is queued). Note: awareness has no error return in v1 (see `research.md` R-6 and `contracts/public-api.md` `AwarenessFn`); panic recovery is the sole failure-mode path.

#### Reasoning

- **FR-016**: A reasoning function MUST be invoked asynchronously, in its own goroutine, with the wake reason, the entity, and a reasoning context.
- **FR-017**: The reasoning context type MUST expose local-state access and action publishing on the imp's declared whitelist.
- **FR-018**: Reasoning invocations for distinct entities MUST be allowed to run concurrently — the harness MUST NOT serialize them.
- **FR-019**: Reasoning invocations for the same entity MAY run concurrently in this feature; per-entity serialization is out of scope and ships in a later feature.
- **FR-020**: A running reasoning invocation MUST NOT block awareness dispatch for new messages on any channel.
- **FR-021**: If a reasoning function panics or returns an error, the harness MUST recover and record the failure; other reasoning invocations and other entities MUST be unaffected.
- **FR-021a**: The harness MUST NOT impose a built-in bound on the number of concurrent in-flight reasoning invocations per imp in this feature. Keeping wake rate × reasoning latency within the imp's footprint is the developer's responsibility; bounded-concurrency policies and overflow handling are deferred to a follow-up feature.
- **FR-021b**: The harness MUST expose the current count of in-flight reasoning invocations (per imp instance) through its observability surface so developers and operators can monitor and react.

#### Local memory

- **FR-022**: The harness MUST allocate per-entity state instances on first reference, indexed by `(state name, entity)`, using the factory declared in the spec.
- **FR-023**: Subsequent references to the same `(state name, entity)` from awareness or reasoning MUST return the same instance.
- **FR-024**: When the number of distinct entities for a given state name reaches the declared cap, attempts to allocate a new `(state name, entity)` slot MUST return a typed cap-exceeded error naming the state shape and the current count. The harness MUST NOT silently evict any existing entity.
- **FR-025**: Reads and writes against existing slots MUST continue to succeed after the cap has been reached.
- **FR-026**: References to a state shape that was not declared in the spec MUST return a typed unknown-shape error.

#### Actions and the whitelist

- **FR-027**: The reasoning context's publish call MUST check the requested subject against the spec's whitelist before publishing. If the subject is not on the whitelist, the call MUST return a typed whitelist-violation error naming the offending subject and the message MUST NOT reach NATS.
- **FR-028**: When the requested subject is on the whitelist, the harness MUST publish the message on the resolved subject for the active deployment mode (see FR-030/FR-031).
- **FR-029**: Awareness MUST have no action-publish surface; this is enforced by the awareness context type (FR-014).

#### Deployment modes

- **FR-030**: When the harness is configured with `platform_mode = false` and a subject prefix `P`, channel and action subjects MUST be resolved as `P.<declared-subject>`.
- **FR-031**: When the harness is configured with `platform_mode = true` and an importer account public key `K`, channel and action subjects MUST be resolved with `K` included as a path segment in the position defined by the platform-mode subject convention; the resolved-subject convention MUST be identical for channels and actions within a single run.
- **FR-032**: The imp's developer-facing API (channel and action declarations, awareness/reasoning signatures, context surfaces) MUST be identical across modes. Switching modes MUST NOT require source changes to the imp.
- **FR-033**: When `platform_mode = true` and no importer account public key is supplied, startup MUST fail with a clear configuration error.

#### Lifecycle

- **FR-034**: At startup, the harness MUST establish every declared channel subscription on the resolved subjects, register the imp's identity, and only then begin dispatching messages.
- **FR-035**: If any required startup step fails, the harness MUST abort startup, return a clear error naming the failed step, and leave no subscriptions or dispatch goroutines registered.
- **FR-036**: The harness MUST expose a shutdown call that (a) stops dispatching new messages, (b) waits up to a configured drain window for in-flight reasoning invocations to complete, (c) unsubscribes all channels, and (d) returns no later than the drain deadline.

#### Out of scope (explicit non-requirements)

- **FR-NS-1**: This feature does NOT provide a bounded-capability surface in the awareness context. The awareness context exposes only local-state access and the verdict return.
- **FR-NS-2**: This feature does NOT provide capability clients (inference, knowledge, tools), soulstream channels and operations, schedule channels via NATS scheduling, KV channels (NATS Key-Value bucket watchers as inbound message sources), sleep/wake snapshot integration, persistence/rehydration of memory, or audit-record emission. Each is a separate feature.
- **FR-NS-3**: This feature does NOT provide retry, circuit-breaker, or dead-lettering for any of: decode failures, awareness errors, reasoning errors, action publish failures. Recovery and continuation are sufficient.
- **FR-NS-4**: This feature does NOT provide cross-imp shared memory. Local memory is per-imp-instance.

### Key Entities

- **Imp Spec** — The declarative description of an imp: name, version, channels, awareness function, reasoning function, local-memory state shapes, action subject whitelist. Provided to the harness at construction.
- **Channel** — An inbound message-source declaration. Each channel has a *source kind* (`subject` or `stream`), a kind-appropriate descriptor, a decode step, and an entity extractor. Subject channels are core NATS subscriptions on a subject (or pattern). Stream channels are JetStream consumers (durable or ephemeral) bound to a stream and filter subject. KV channels are deferred to a follow-up feature.
- **Awareness Context** — A typed surface available to the awareness function. Exposes per-entity local-state access and the verdict return path. Does not expose action publishing or any unbounded operations.
- **Awareness Verdict** — One of `Ignore`, `Note(payload)`, `Wake(reason, entity)`. The contract between awareness and the harness.
- **Reasoning Context** — A typed surface available to the reasoning function. Exposes per-entity local-state access and action publishing on the declared whitelist.
- **Wake Reason** — A user-defined value carried from awareness's `Wake` verdict to the reasoning function. Opaque to the harness.
- **Local State Shape** — A named, capped, factory-backed per-entity state declaration. Indexed at runtime by `(state name, entity)`.
- **Action Whitelist** — The set of NATS subjects (declared in the spec) on which reasoning may publish. Subjects not in the set are rejected by the harness before reaching NATS.
- **Imp Identity** — The triple (name, version, resolved subject prefix) that identifies a running imp instance.
- **Note Record** — A locally observable record produced by the `Note` verdict, carrying a user-defined payload.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer can author and run a working imp (channel → awareness → reasoning → action) using only the harness's declarative surface and two functions, without writing any subscription, dispatch, or queueing code.
- **SC-002**: An end-to-end test (publisher publishes on a channel subject; assertion subscribes on the action subject) completes within a small bounded time on a local embedded messaging server (target: under 1 second per round-trip in a clean environment).
- **SC-003**: With reasoning held under a deliberate block for one entity, awareness for new messages on the same imp continues to be dispatched within a small bounded delay (target: under 50 ms additional latency vs. the unblocked baseline).
- **SC-004**: With reasoning invocations running concurrently for `K` distinct entities (target `K ≥ 100`), no entity's reasoning is delayed by another's progress in a way attributable to the harness.
- **SC-005**: An attempt to publish to a non-whitelisted subject is rejected at the publish call site with a typed error that names the offending subject; no message reaches the messaging substrate.
- **SC-006**: A compile-time check confirms the awareness context type does not expose an action-publish method.
- **SC-007**: An attempt to allocate a `(state-shape, entity)` slot beyond the declared cap returns a typed cap-exceeded error naming the state shape and the current count; no existing entity slot is evicted.
- **SC-008**: The same imp source builds and runs against both deployment modes (non-platform and platform) with no developer code changes; the only configuration delta is `platform_mode` and the mode-specific subject parameters.
- **SC-009**: Shutdown returns within `drain_window + small ε` regardless of in-flight reasoning, and leaves no subscriptions or dispatch goroutines.
- **SC-010**: The harness's per-message dispatch overhead (excluding user-supplied awareness/decode/extractor work) is bounded and independent of the number of in-flight reasoning invocations and the number of tracked entities (target: dispatch overhead growth less than linear in either dimension under typical workloads).
- **SC-011**: A single imp instance sustains continuous awareness dispatch on a channel while reasoning invocations remain in flight for thousands of entities, without observable awareness backpressure.

## Assumptions

- **NATS is the messaging substrate.** This is given by the framework's architecture (see `docs/00-vision.md` and `docs/01-harness-anatomy.md`); it is part of the contract, not an implementation choice within this feature.
- **The harness runs in-process with the imp's awareness and reasoning code.** This is consistent with the framework's "imps stay small" commitment and the harness anatomy document. Out-of-process harness deployment is not in scope.
- **Local memory is in-process for this feature.** Persistence, rehydration, retention, and eviction beyond the explicit cap are deferred to the sleep/wake feature.
- **Sharing across replicas is substrate-governed, not harness-governed in this feature.** For subject channels, subscriptions do not use queue groups by default — multiple instances of the same imp class will each receive every matching message. For stream channels, sharing across replicas follows JetStream's consumer semantics — replicas binding the same durable consumer name share the work; replicas with different (or no) durable names each consume independently. Queue-group semantics for subject-channel subscriptions and any harness-imposed sharding policy are deferred to a later feature when the deployment story for multiple replicas is specified.
- **Note records are locally observable** (e.g., through a hook or counter exposed by the harness) but are not emitted to the soulstream or audit stream. Soulstream integration is a separate feature.
- **Reasoning concurrency for the same entity is not serialized in this feature.** Awareness and reasoning code is responsible for being safe under concurrent same-entity wakes for now. Per-entity serialization (an opt-in offered by the harness anatomy doc) ships in a later feature.
- **In-flight reasoning is unbounded in v1.** The harness imposes no internal cap on concurrent reasoning goroutines per imp; the developer owns the discipline of keeping wake rate × reasoning latency within the imp's footprint. The harness exposes the in-flight count via its observability surface so the discipline is diagnosable. A bounding policy (cap + overflow handling) is deferred to a follow-up feature once real workload data informs the choice.
- **The decode and entity-extractor steps are pure, non-blocking, and free of side effects.** The harness invokes them inside the dispatch hot path; long-running work in either is the developer's responsibility to avoid.
- **Action publishes use the imp's existing NATS connection.** Per-request account attribution in platform mode is handled by the substrate (e.g., NATS account-export configuration); the harness does not maintain its own connection pool.
- **Awareness errors and reasoning errors are recovered (panic → recovered + recorded).** This is the simplest "keep running" policy and is consistent with the framework's "imps continue to notice the world" commitment. Richer policies (retry, dead-letter, escalation) are not in scope.
- **The legacy codebase at `../imps-legacy` may be consulted for prior-art on specific implementation questions** (NATS subscription lifecycle, queue group semantics, projection registry concurrency) but is NOT the source of architecture for this feature. The new awareness/reasoning split does not map onto the legacy projection/derivation/reactor split.

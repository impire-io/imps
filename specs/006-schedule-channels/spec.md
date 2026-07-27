# Feature Specification: Schedule Channels

**Feature Branch**: `006-schedule-channels`
**Created**: 2026-07-27
**Status**: Shipped
**Input**: User description: "Schedule channels per hq/02-DESIGN/0005-schedule-channels.md: the thin imps/schedule package — Channel sugar over the existing StreamSource with a header-only Tick decoder, plus typed Register/Deregister for the thinking/operator tier; the server owns the clock."

**Design source**: [`hq/02-DESIGN/0005-schedule-channels.md`](../../hq/02-DESIGN/0005-schedule-channels.md)
(graduated from research, [episode 0008](../../hq/04-JOURNEY/0008-schedule-channels.md)).
The scheduling primitive is the substrate's (JetStream message scheduling,
verified in the pinned `nats-server v2.14.0`); this feature consumes it and
defines none of it.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Periodic work arrives as an ordinary channel (Priority: P1)

An imp developer declares a schedule channel among the imp's channels. The
server produces the ticks; the imp receives them through the same dispatch
path as any other message — live while the imp runs, and as durable catch-up
when it starts after a gap. The developer writes no timers, no polling, and
no header parsing: each decoded tick names the schedule that produced it.

**Why this priority**: consuming ticks is the whole point of M3; an imp that
can attend periodic work without staying warm is the roadmap's promise.

**Independent Test**: against a provisioned stream with a registered
schedule, a one-channel imp receives live ticks with correct provenance;
after being stopped through several firings, a restarted imp catches up
through the same channel.

**Acceptance Scenarios**:

1. **Given** a registered schedule and a running imp declaring its tick
   channel, **When** the schedule fires, **Then** awareness receives the
   tick through the ordinary dispatch path with the producing schedule's
   identity decoded from it.
2. **Given** the imp is stopped while the schedule keeps firing, **When** it
   restarts with a durable cursor, **Then** it receives the backlog the
   retention policy left, in order, through the same channel.
3. **Given** a tick with headers or payload this package does not know,
   **When** it arrives, **Then** it is delivered undamaged (overridable
   decoder, never filtered).

---

### User Story 2 - Registering and removing schedules, typed (Priority: P2)

From thinking (or an operator script), the developer registers a schedule by
name: a pattern, a target subject, and options — tick TTL, time zone, body,
source, rollup — with the six substrate headers built for them. Registering
the same schedule again replaces it; deregistering stops future firings
without disturbing ticks already emitted.

**Why this priority**: the substrate primitive is stringly-typed; the typed
surface is the reason this package exists rather than documentation alone.

**Independent Test**: register with every option and read the stored
schedule back verifying each header; re-register and verify replacement
takes effect on the next firing; deregister and verify firing stops.

**Acceptance Scenarios**:

1. **Given** a call to register with a pattern, target, TTL, and body,
   **When** the stored schedule is read back, **Then** every header the
   options imply is present and correctly formatted, and no other
   scheduling header is.
2. **Given** an existing schedule, **When** it is registered again with a
   new pattern, **Then** the next firing follows the new pattern (one
   schedule per subject — replaced, not duplicated).
3. **Given** an existing schedule, **When** it is deregistered, **Then** no
   further ticks are produced, and previously emitted ticks are untouched.
4. **Given** a register call with an empty pattern or target, **When** it
   runs, **Then** it fails fast with a clear error before touching the
   substrate.

---

### User Story 3 - Stale ticks are governed by an explicit TTL (Priority: P3)

A schedule registered with a tick TTL produces ticks that expire on the
server if the imp sleeps past them: on wake, only the unexpired tail
arrives. A schedule without a TTL accumulates its full backlog — a
deliberate, visible choice at the registration site.

**Why this priority**: this is the roadmap's stale-tick exit criterion; the
research proved the server enforces it without imp-side filtering.

**Independent Test**: two schedules, one with a short TTL and one without;
stop the imp through several firings; on restart the no-TTL channel delivers
the full backlog while the TTL channel delivers strictly fewer — only the
unexpired tail — with exact counts.

**Acceptance Scenarios**:

1. **Given** a schedule with tick TTL T and an imp cold for longer than T,
   **When** the imp wakes, **Then** ticks older than T were expired by the
   server and never delivered.
2. **Given** a schedule without a TTL, **When** the imp wakes after the same
   gap, **Then** the full backlog arrives.
3. **Given** any TTL configuration, **When** ticks are delivered, **Then**
   no imp-side filtering occurred (the counts match what the server
   retained).

---

### Edge Cases

- **Registration tier**: registering is a write and belongs to thinking or
  operator tooling; awareness has no publish surface and the package
  documentation forbids handing it a substrate handle (the M1/M2a
  discipline).
- **Provisioning**: the stream (scheduling and TTL flags, subject capture)
  is the operator's; the package never creates or reconfigures streams, and
  a missing stream fails through the harness's existing named startup error.
- **One schedule per subject**: replacement is the substrate's semantics;
  the package adds no registry, reconciler, or janitor on top.
- **Warm-and-cold equivalence**: dispatch is identical either way; the only
  difference is whether the consumer was attached when the tick was
  appended.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The feature MUST ship as a new package inside the core module
  (`imps/schedule`) with the root harness package untouched, the dependency
  manifest byte-identical, and no build/CI wiring changes (the existing gate
  covers it).
- **FR-002**: The channel builder MUST return a standard harness
  `ChannelSpec` on the existing stream source — no new channel kind, no
  subject rewriting, deliver-all default — with durable naming and
  start-position passthrough, and decoder/extractor/name overrides.
- **FR-003**: The default decoder MUST be header-only, yielding the tick's
  subject, the producing schedule's identity, and the next-firing marker;
  payload decoding is an override.
- **FR-004**: Registration MUST build the substrate's scheduling headers
  from typed options (pattern, target, tick TTL, time zone, body, source,
  rollup), validate only what can be validated without the substrate
  (non-empty pattern and target; well-formed TTL), and publish exactly one
  message; the substrate remains the authority on pattern semantics.
- **FR-005**: Registering an already-registered schedule subject MUST
  replace it (substrate semantics surfaced, not reimplemented);
  deregistration MUST remove the schedule without disturbing emitted ticks.
- **FR-006**: The package MUST NOT run timers, produce ticks, poll, filter
  ticks, provision streams, or maintain any schedule registry.
- **FR-007**: Stale-tick policy MUST be explicit at the registration site:
  a TTL option whose absence is legal but documented as
  full-accumulation.
- **FR-008**: Registration MUST be usable from thinking and from operator
  tooling; nothing in the package may be reachable from awareness's bounded
  surface.

### Key Entities

- **Schedule**: a named, replaceable declaration on the substrate — pattern,
  target, options — stored as one message; never mirrored in the package.
- **Tick**: one firing, delivered as an ordinary message on the target
  subject; carries its producer's identity and the next-firing marker.
- **Tick TTL**: the explicit stale-tick governor; server-enforced expiry of
  ticks that outlive it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A registered schedule's ticks reach awareness through the
  unmodified dispatch path both live and as durable catch-up after a cold
  gap, with 100% of delivered ticks carrying correct producer identity.
- **SC-002**: With a TTL configured, 100% of ticks older than the TTL at
  wake are absent (server-expired) and the unexpired tail is delivered;
  without a TTL, 100% of the backlog is delivered — both with exact counts
  and zero imp-side filtering.
- **SC-003**: Register-with-all-options round-trips: every implied header
  present and well-formed on the stored schedule; re-registration observably
  switches the firing pattern; deregistration stops firing.
- **SC-004**: The harness core is provably untouched: root package
  unmodified, dependency manifest byte-identical, no Makefile/CI diff,
  boundary invariants (compile-deny) green.
- **SC-005**: The full repository gate passes with the new package covered
  and no test skipped.

## Assumptions

- The stream holding schedules and ticks is operator-provisioned with the
  scheduling and per-message-TTL capabilities enabled; subject-space
  discipline (schedule subjects vs. tick subjects) is the operator's.
- Tick cadences at imp scale are seconds-or-slower; the package imposes no
  cadence floor beyond the substrate's.
- The substrate's scheduling header contract is stable at the pinned server
  version; the integration suite reads any breakage out directly (reversal
  condition registered in episode 0008).
- Whole-imp snapshot sleep (M2b) is orthogonal: ticks fire whether the imp
  is warm or cold by construction, and TTL-pruned catch-up is exactly what
  a woken imp replays.

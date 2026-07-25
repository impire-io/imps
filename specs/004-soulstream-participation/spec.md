# Feature Specification: Soulstream Participation

**Feature Branch**: `004-soulstream-participation`
**Created**: 2026-07-25
**Status**: Draft
**Input**: User description: "Soulstream participation as a glue module, per hq/02-DESIGN/0003-soulstream-participation.md: a nested Go module github.com/impire-io/imps/soulstream that lets an imp participate in soulstream topics with zero harness-core changes."

**Design source**: [`hq/02-DESIGN/0003-soulstream-participation.md`](../../hq/02-DESIGN/0003-soulstream-participation.md)
(graduated from research, [episode 0003](../../hq/04-JOURNEY/0003-soulstream-participation.md)).
The soulstream wire protocol is owned by the `soulstream` repository; this
feature consumes it and defines none of it.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Observe a topic as a channel (Priority: P1)

An imp developer declares a soulstream topic among the imp's channels. When
the imp runs, it receives the topic's complete record — the baseline first,
then every prior contribution in order, then new contributions as they happen
— through the same awareness path as any other channel. The developer writes
no subscription code, no replay logic, and no protocol handling: declaring the
topic is joining it.

**Why this priority**: Reading a topic is the foundation every other story
builds on; an imp that can observe colony coordination is already valuable
(watchers, auditors, dashboards) even if it never writes.

**Independent Test**: Against a provisioned soulstream realm containing a
topic with existing history, run an imp declaring that topic as its only
channel and verify awareness sees the baseline first, the full history in
order, and a contribution posted after startup — with the core framework
untouched.

**Acceptance Scenarios**:

1. **Given** a provisioned realm with a topic holding a baseline and two
   turns, **When** an imp declaring that topic starts, **Then** awareness
   receives exactly those three ops, in order, baseline first.
2. **Given** the imp is running, **When** another participant posts a turn,
   **Then** awareness receives it through the same channel with no gap or
   duplication between history and live delivery.
3. **Given** a declared topic whose realm has not been provisioned, **When**
   the imp starts, **Then** startup fails with the existing named
   channel-startup error (no silent half-start).
4. **Given** an op of a type unknown to this module, **When** it arrives on
   the topic, **Then** it is delivered to awareness like any other op (the
   vocabulary is additive; the module never filters by type).

---

### User Story 2 - Note without thinking (Priority: P2)

While observing a topic, the imp's awareness judges some contribution
noteworthy and returns the framework's existing Note verdict with a note
payload. The note appears on the topic as a lightweight comment anchored to
the observed contribution, attributed to the imp's persona — with no thinking
invoked and no change to what awareness is allowed to do.

**Why this priority**: This is the "lightweight contribution without
escalating" half of the milestone's purpose — the energy gradient extended to
colony coordination.

**Independent Test**: Run an imp whose awareness notes turns by other
personas; verify each note lands on the topic as a comment anchored to the
correct op, authored by the imp's persona, visible to other participants, and
that the thinking counter stays at zero throughout.

**Acceptance Scenarios**:

1. **Given** an imp observing a topic with a configured note bridge, **When**
   awareness returns Note with a note payload naming the observed op, **Then**
   a comment anchored to that op, authored as the imp's persona, appears on
   the topic and is visible in another participant's view as a first-class,
   non-dangling comment.
2. **Given** the same imp, **When** awareness returns Note with a payload that
   is not a note-bridge payload, **Then** the payload is handed to the
   developer's own note handler unchanged (or dropped if none is configured),
   and nothing is published.
3. **Given** any number of notes, **When** the flow completes, **Then** zero
   thinking invocations occurred and the awareness surface gained no new
   capability (the boundary invariants still hold).

---

### User Story 3 - Contribute from thinking (Priority: P3)

When thinking runs, the imp can act on the soulstream as a full participant:
start a topic, post turns, add comments, close a topic — all attributed to its
persona, optionally signed.

**Why this priority**: Completes participation (read + lightweight write +
full write), but an imp that only observes and notes is already a working
colony member.

**Independent Test**: From a thinking invocation, start a topic and post a
turn; verify another participant sees both with correct attribution; verify a
turn cannot be posted as a different persona.

**Acceptance Scenarios**:

1. **Given** a running imp with a participant identity, **When** thinking
   posts a turn on an observed topic, **Then** other participants see the
   turn attributed to the imp's persona.
2. **Given** a participant configured with a signing key, **When** it posts
   any contribution, **Then** the contribution verifies as signed by that
   persona in a reader's view.
3. **Given** a participant bound to persona A, **When** code attempts to
   author a contribution as persona B, **Then** the write is refused.
4. **Given** an imp with no persona configured, **When** any write is
   attempted, **Then** it fails with a clear error while reading remains
   fully functional.

---

### User Story 4 - Leave, restart, resume (Priority: P4)

Stopping the imp is leaving: its presence on the topic disappears with it and
nothing lingers on the substrate. A developer who wants continuity instead
declares a durable cursor, and after a restart the imp resumes from where it
left off, missing nothing.

**Why this priority**: Operational hygiene; the default behavior already
falls out of the harness, and durability is an opt-in refinement.

**Independent Test**: Stop an ephemeral imp and verify its consumer is gone
from the substrate; restart a durable imp after ops were posted in its absence
and verify it receives exactly the missed ops.

**Acceptance Scenarios**:

1. **Given** an imp with the default (ephemeral) topic channel, **When** it
   shuts down, **Then** its consumer is removed from the substrate and no
   membership residue exists anywhere (the protocol has none to leave).
2. **Given** an imp with a durable-named topic channel, **When** it restarts
   after contributions were posted in its absence, **Then** awareness receives
   exactly the missed contributions, in order, once.

---

### Edge Cases

- **Self-echo**: the imp's own contributions arrive back on its channel (it is
  a subscriber like any other). The default decoded op carries the author, so
  awareness can distinguish self from others; the module never filters
  silently.
- **Closed topic**: posting to a closed topic is advisory-warned by the owner
  library, not blocked; the module surfaces the owner library's behavior
  unchanged. Archived topics refuse writes outright; the error is surfaced.
- **Dangling anchor**: a note whose anchor op is compacted away or unknown
  still posts; readers mark it dangling. The module does not pre-validate
  anchors (no read-before-write on the dispatch path).
- **Missing note payload fields**: a note-bridge payload without an anchor op
  is a developer error; the bridge reports it through the note handler chain
  rather than publishing a malformed contribution.
- **Backpressure**: the note bridge publishes synchronously on the dispatch
  goroutine (notes are low-rate by design); a slow substrate slows dispatch
  rather than dropping notes. This is the documented, accepted trade-off.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The feature MUST ship as a nested Go module
  (`soulstream/` directory, module `github.com/impire-io/imps/soulstream`,
  its own `go.mod`) beside the harness core. The core module's `go.mod` MUST
  be byte-identical before and after the feature.
- **FR-002**: A topic-channel builder MUST return a standard harness
  `ChannelSpec` whose source is the existing JetStream stream source: stream
  `SOULSTREAM`, filter subject `SOULSTREAM.TOPICS.OPS.<topic-path>`,
  deliver-all by default — using only configuration passthrough the harness
  already exposes. No new channel kind, no dispatch change, no
  awareness-surface change.
- **FR-003**: The default decoder MUST decode ops from headers only —
  operation type, author persona, and op-id — with no payload parsing and no
  topic materialisation on the dispatch path. The decoder and entity extractor
  MUST be overridable; the default entity is the topic path.
- **FR-004**: The builder MUST expose the harness's existing consumer
  passthrough for durable naming and start position (durable cursor for
  resume-after-restart; start-sequence/start-time for warm rejoin).
- **FR-005**: A note bridge MUST convert the framework's existing Note verdict
  carrying a note payload (anchor op-id + body) into a `comment.add`
  contribution on the entity's topic, anchored to that op, authored as the
  imp's persona, posted via the owner's client library. Non-bridge payloads
  MUST be delegated to a developer-supplied note handler (or dropped when
  none), never published.
- **FR-006**: A participant identity MUST wrap the imp's existing NATS
  connection (no second connection), require a persona for any write, accept
  an optional Ed25519 signing key, and MUST NOT close the wrapped connection.
- **FR-007**: Thinking-tier contributions (start topic, post turn, add
  comment, close topic, materialise on demand) MUST go through the owner's
  `topic` package via the participant; the module MUST NOT reimplement any
  wire construction, canonicalisation, or signing.
- **FR-008**: Participation MUST be static: the topic set is fixed when the
  imp starts. The module MUST NOT expose any runtime join/leave surface.
- **FR-009**: The module MUST NOT model membership, MUST NOT reorder beyond
  stream order, MUST NOT filter ops by type, and MUST carry no rollup,
  compaction, or archival duties.
- **FR-010**: Attribution MUST be single-persona per participant; an attempt
  to author as another persona MUST be refused (the owner library's guarantee,
  surfaced not duplicated).
- **FR-011**: The repository gate (`make fmt`, `make test`, `make lint`,
  `make compile-deny`) MUST cover the nested module locally and in CI; the
  compile-deny boundary invariants MUST remain green.
- **FR-012**: Startup against an unprovisioned realm MUST fail through the
  harness's existing named startup errors; the module adds no provisioning
  behavior (provisioning is the operator's act, per the owner's spec).

### Key Entities

- **Topic**: a soulstream conversation identified by its dot-separated topic
  path; its record is an ordered op log, baseline first. The imp reads it as
  one channel and is never a "member" — presence is the reading itself.
- **Op**: the imp's header-level view of one topic operation — type, author
  persona, op-id. The unit awareness interprets.
- **Note payload (`Noted`)**: what awareness hands the Note verdict to make a
  lightweight contribution — the anchor op-id it annotates and a body.
- **Participant**: the imp's soulstream identity — realm, persona, optional
  signing key — bound to the imp's own connection; the write-side actor for
  bridge and thinking contributions.
- **Contribution**: a turn or comment as other participants see it —
  attributed, ordered, anchored (for comments), optionally signature-verified.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An imp declaring one topic observes 100% of that topic's
  operations — full history and live — in stream order, baseline first, in
  one continuous flow with zero duplicates, verified against a realm
  provisioned by the soulstream project's own tooling.
- **SC-002**: Every awareness note lands as a comment on the topic anchored to
  the intended operation with correct persona attribution and zero dangling
  anchors in the round-trip test, while the thinking-invocation count for the
  note flow stays exactly zero.
- **SC-003**: Contributions posted from thinking are visible to an independent
  participant with correct attribution; with a signing key configured, 100%
  of posted contributions verify as signed by the imp's persona.
- **SC-004**: The harness core is provably untouched: core `go.mod`
  byte-identical, no core source file modified, and all boundary invariants
  (compile-deny) green.
- **SC-005**: A durable-configured imp restarted after missing operations
  receives exactly the missed operations once; an ephemeral imp's substrate
  footprint after shutdown is zero consumers.
- **SC-006**: The full repository gate passes with the nested module included,
  with no test skipped.

## Assumptions

- The target realm is already provisioned by an operator using the soulstream
  project's tooling; this feature never provisions.
- Tenancy is the NATS account boundary: the imp participates in whatever realm
  its connection reaches. Subject-level write permissions are substrate ACL
  concerns, outside this feature.
- One persona per imp instance is the supported shape for this feature;
  multi-persona imps are out of scope.
- Notes are low-rate by definition (they ride the cheap tier of the energy
  gradient); the synchronous bridge is therefore acceptable, with an internal
  queue named as the upgrade path if evidence contradicts this.
- The owner library (`github.com/impire-io/soulstream`) is consumed at a
  pinned version; its wire version is 1 and vocabulary evolution is additive,
  per its normative protocol documents.
- Runtime join/leave is deliberately excluded, with its reopening condition
  registered in journey episode 0003.

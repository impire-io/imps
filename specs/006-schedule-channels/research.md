# Phase 0 Research: Schedule Channels

All material unknowns were resolved by the `schedule-channels` research topic
([episode 0008](../../hq/04-JOURNEY/0008-schedule-channels.md)) and its
graduated design doc
([`hq/02-DESIGN/0005-schedule-channels.md`](../../hq/02-DESIGN/0005-schedule-channels.md)).
No NEEDS CLARIFICATION markers remain.

## D1. The primitive: JetStream message scheduling, consumed not wrapped

- **Decision**: consume the pinned substrate's message scheduling directly —
  schedules are headered messages in an `AllowMsgSchedules` stream; ticks
  are ordinary messages with `Nats-Scheduler` provenance; `Nats-Schedule-TTL`
  makes the server expire stale ticks (`AllowMsgTTL`).
- **Rationale**: pinned `[measured]` in `nats-server v2.14.0` source and
  spiked end-to-end (warm, cold, TTL both directions) with zero harness
  changes.
- **Alternatives considered**: framework-side timers (violates
  fire-while-cold and adds clock ownership — refuted); an external cron
  capability service (unnecessary — the substrate ships the primitive).

## D2. Shape: a thin core-module package, not documentation-only

- **Decision**: `imps/schedule` — `Channel` sugar over the existing
  `StreamSource` + typed `Register`/`Deregister`.
- **Rationale**: documentation-only leaves six stringly-typed headers and a
  provenance decode to every author — the error class `TopicChannel`
  removes. Zero new deps → no module boundary (the `persist` precedent).
- **Alternatives considered**: documentation-only (rejected, episode 0008);
  nested module (no deps to fence).

## D3. Header authority: local typed constants, server as semantic authority

- **Decision**: the package defines the header names once (`Nats-Schedule`,
  `-Target`, `-TTL`, `-Time-Zone`, `-Rollup`, `-Source`, and read-side
  `Nats-Scheduler`, `Nats-Schedule-Next`) and validates only what needs no
  substrate (non-empty pattern/target, well-formed TTL via
  `time.Duration`); pattern semantics stay the server's.
- **Rationale**: the pinned client library exports no scheduling header
  constants; one typed definition site is the package's purpose. Client-side
  cron parsing would fork the server's grammar.
- **Alternatives considered**: importing server constants (would add the
  server as a non-test dependency of the package — refuted); full client
  validation (grammar fork — refuted).

## D4. Deregistration: subject purge, nothing more

- **Decision**: `Deregister(ctx, js, stream, scheduleSubject)` purges the
  schedule subject in the stream; emitted ticks untouched.
- **Rationale**: the schedule IS the stored message; removing it is the
  substrate's removal semantics. Requires the stream name (purge is a
  stream op) — the one asymmetry with `Register`, accepted.
- **Alternatives considered**: delete-by-sequence (requires tracking a
  sequence the package refuses to own).

## D5. Tick.Next is the raw marker

- **Decision**: the decoder passes `Nats-Schedule-Next` through verbatim
  (RFC3339 of the next firing on repeating schedules; the server's purge
  marker on final firings), documented as informational.
- **Rationale**: parsing it would make the package opinionated about a
  server detail no imp decision depends on.

## D6. Test strategy: the spike productized plus header round-trips

- **Decision**: `schedule_test.go` reproduces the research spike (two
  schedules, warm ≥2 ticks each with provenance, 5 s cold, no-TTL full
  backlog vs TTL unexpired tail, exact-count comparisons, metrics clean);
  `register_test.go` reads stored schedules back asserting every
  option→header mapping, replacement, deregistration, and fail-fast
  validation; `channel_test.go` covers spec construction and decode without
  a server.
- **Rationale**: the spike is measured ground truth; header round-trips are
  the package's actual contract.
- **Alternatives considered**: asserting firing-pattern replacement by
  waiting out cadence changes (flake-prone; replacement is asserted at the
  storage level plus one fast-direction switch — register `@every 1h` then
  replace with `@every 1s` and observe a tick, which only ever waits on the
  fast pattern).

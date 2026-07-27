# Journey — schedule-channels (started 2026-07-27)

## 2026-07-27 — Bar 1: the primitive, pinned in the pinned server's source

Read `nats-server v2.14.0` (the exact version in the core `go.mod`) from the
module cache. The server-side scheduling primitive **exists and is
JetStream message scheduling** `[measured]` (readings in the pinned source):

- **A schedule is a message.** Published into a stream configured with
  `AllowMsgSchedules: true` (`server/stream.go:119-120`), carrying headers
  (`server/stream.go:640-664`): `Nats-Schedule` (the pattern — `@every
  <dur>`, cron expressions, predefined forms; optional
  `Nats-Schedule-Time-Zone`; `server/scheduler.go:327-362`),
  `Nats-Schedule-Target` (the subject ticks are emitted to — required, fire
  path removes schedules without it, `scheduler.go:197-201`), optional
  `Nats-Schedule-TTL` (absent is valid — `stream.go:5420-5429`), optional
  `Nats-Schedule-Rollup` and `Nats-Schedule-Source` (emit the *last message
  on a source subject* instead of the schedule's own body —
  `scheduler.go:203-209`).
- **A tick is an ordinary message on the target subject**, appended into the
  stream by the server with scheduling headers stripped and provenance
  added: `Nats-Scheduler: <schedule-subject>` and `Nats-Schedule-Next`
  (`scheduler.go:215-231`). One schedule per subject (keyed by subject,
  re-publishing replaces — `scheduler.go:66-79`).
- **The stale-tick governor is exactly the vision's TTL:** when
  `Nats-Schedule-TTL` is set, every emitted tick carries `Nats-TTL: <ttl>`
  (`scheduler.go:231-233`), so the server itself expires ticks that outlive
  the TTL (stream needs `AllowMsgTTL: true`, per-message TTLs,
  `stream.go:197-199` client-side / `Nats-TTL` `stream.go:640`). No imp-side
  filtering needed.
- **The pinned client is ready:** `nats.go v1.52.0`'s
  `jetstream.StreamConfig` exposes both `AllowMsgSchedules` and
  `AllowMsgTTL` (`jetstream/stream_config.go:197-212`) — no raw API calls
  required.

**The shape this implies:** a "schedule channel" is the **existing
`StreamSource`** on the tick target subject — the same seam as every other
channel. Registering a schedule is publishing one headered message (an
operator act or a thinking-tier act); warm firing is live consumer
delivery; cold catch-up is durable-consumer replay with the server having
already expired what the TTL says is stale. The Bar 2/3 spike tests exactly
this with the harness byte-identical.

## 2026-07-27 — Bars 2+3: the spike — warm, cold, and TTL-governed, zero harness changes

Built the spike as a scratchpad module (`schedspike`, `replace` to the imps
working tree). One stream (`AllowMsgSchedules` + `AllowMsgTTL`) holding
schedules and ticks; two schedules at 1 s cadence — `hb` with
`Nats-Schedule-TTL: 2s`, `audit` with no TTL; an imp consuming both tick
subjects through the **existing `StreamSource`** with durable consumers.

**Results `[measured]`** (3 consecutive `-race` runs, ~8 s each; imps
working tree byte-identical):

- **Warm (Bar 2):** the running imp received live ticks on both channels
  through the unmodified dispatch seam; every emitted tick carried the
  `Nats-Scheduler` provenance header naming its schedule; zero decode or
  extraction failures.
- **Cold catch-up (Bar 2):** after the imp was stopped for 5 s of firing,
  the restarted imp's durable consumers caught up through the same seam.
- **TTL governs accumulation (Bar 3), both directions:** the no-TTL `audit`
  schedule delivered its full cold backlog (≥3 ticks at 1 s cadence); the
  2 s-TTL `hb` schedule delivered strictly fewer — only the unexpired tail
  (1–4 ticks) — because the server itself stamped each tick `Nats-TTL: 2s`
  and expired the stale ones. No imp-side filtering anywhere.

## 2026-07-27 — Bar 4: sibling scopes (the episode-0007 corrective)

Grepped both siblings' hq for scheduling claims `[measured]`:

- **soulrealm**: every "scheduling" mention is workload *placement* — fleet
  scheduling across nodes (`roadmap.md:61`, `vision.md:57`,
  `0001-soulrealm-runtime.md:121,166`) — plus the `job` lifecycle shape
  ("runs to completion once (batch/scheduled)", `0001:75`), which names no
  trigger source. No cron, timer, or periodic-tick service is claimed.
  Complementary note for the design doc: a JetStream schedule tick could
  someday *be* the trigger that launches a soulrealm `job` — the same
  primitive serving both projects, neither owning the other's half.
- **soulstream**: "periodic" appears only as library routines any persona's
  process may run (periodic re-baselining, `core/03-topics.md:115`,
  `extensions/library-and-adapters.md:19`) and "a scheduled job" as an
  example of what may stand behind a persona (`core/02-identity.md:7`). No
  scheduling service claimed.

No boundary conflict exists: the *substrate* (nats-server) owns tick
production, and neither sibling claims it.

## 2026-07-27 — The design shape (for the graduated doc)

The spike needed zero imps code — the open call is whether M3 ships as
documentation-only or with a thin sugar package. Judgment `[judgment]`,
argued briefly both ways: documentation-only leaves six stringly-typed magic
headers (`Nats-Schedule`, `-Target`, `-TTL`, `-Time-Zone`, `-Rollup`,
`-Source`) and a scheduler-provenance decode for every imp author to
hand-roll — the same error class M1's `TopicChannel` existed to remove;
a full framework surface would over-claim a substrate feature the server
already owns. Resolution: a **minimal `imps/schedule` package** (core
module, zero new deps, like `persist`): `Channel(stream, target, opts…)`
returning the existing `StreamSource` spec with a header-only `Tick`
decoder, plus `Register`/`Deregister` header-builders for the
thinking/operator tier. Registration is never awareness-tier. Provisioning
(the stream with its two flags) stays the operator's act.

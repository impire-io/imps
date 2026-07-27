# Episode 0008 — Schedule channels: the server already owns the clock (2026-07-27)

M3's gate had an external half that had never been verified: "the
server-side scheduling primitive available on the target substrate." The
`schedule-channels` research topic pre-registered four bars — including the
episode-0007 corrective as a standing bar (sibling scopes inventoried
before any surface is designed) — and passed all four the same day.

**Bar 1 (the primitive pinned) — PASS `[measured]`.** The pinned
`nats-server v2.14.0` ships **JetStream message scheduling**, read from the
pinned module source, not docs folklore: a schedule is a headered message
(`Nats-Schedule` pattern — `@every`/cron —, `-Target`, optional `-TTL`,
`-Time-Zone`, `-Rollup`, `-Source`) in an `AllowMsgSchedules` stream; ticks
are ordinary messages on the target subject carrying `Nats-Scheduler`
provenance; a schedule's TTL stamps every tick `Nats-TTL` so **the server
itself expires stale ticks**. The pinned `nats.go v1.52.0` already exposes
both stream flags. The vision's "periodic work uses NATS server-side
scheduling" turned out not merely satisfiable but *literal* — the reversal
condition (a missing primitive escalating to a vision-level decision) was
never approached.

**Bars 2+3 (the spike) — PASS `[measured]`,** 3 consecutive `-race` runs,
imps tree byte-identical: two schedules at 1 s cadence fired into subjects
an imp consumed through the **existing `StreamSource`** — live while warm;
after a 5 s cold gap, durable-consumer catch-up through the same seam. The
TTL governed accumulation in both directions: the no-TTL schedule delivered
its full cold backlog (≥3 ticks), the 2 s-TTL schedule only the unexpired
tail (1–4), pruned server-side with no imp-side filtering. Every tick
carried its producer's identity in `Nats-Scheduler`.

**Bar 4 (sibling scopes) — PASS `[measured]`.** soulrealm's "scheduling" is
workload *placement* (fleet) plus an untriggered `job` lifecycle shape;
soulstream's "periodic" is persona-run library routines. Neither claims
periodic-tick production — the substrate owns it — and one complementary
future was recorded: a schedule tick could be the trigger that launches a
soulrealm `job`.

**What it opened:** the design —
[`../02-DESIGN/0005-schedule-channels.md`](../02-DESIGN/0005-schedule-channels.md)
— specifies M3 as a minimal `imps/schedule` package (core module, zero new
dependencies, the `persist` shape): `Channel` sugar over the existing
`StreamSource` with a header-only `Tick` decoder, and
`Register`/`Deregister` typed header-builders for the thinking/operator
tier. Documentation-only was argued and rejected `[judgment]` — it leaves
six stringly-typed magic headers to every imp author, the error class
`TopicChannel` exists to remove. The framework still produces no ticks,
runs no timers, and owns no schedule registry: the server owns the clock.
M3 is ready for `/speckit-specify`.

Reversal condition: the package's existence reverses if the substrate's
scheduling contract breaks compatibility in a way that makes typed headers
a maintenance liability rather than a safety (evidence: a nats-server
release renaming or re-shaping the `Nats-Schedule*` headers, which the
integration suite would read out directly); the TTL-optional-by-convention
default reverses on the first real incident of an unbounded tick backlog
harming an imp, to TTL-required-with-explicit-opt-out.

Trail: [`../02-DESIGN/0005-schedule-channels.md`](../02-DESIGN/0005-schedule-channels.md)
(the graduated design); the topic's pre-registration, primitive pinning,
spike record, and sibling inventory live in git history under
`hq/01-RESEARCH/schedule-channels/` (removed at graduation); commits
`5794178`, `2a25c7c`.

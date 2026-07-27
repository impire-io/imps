# Episode 0009 — M3 ships: an imp on the clock it doesn't own (2026-07-28)

Feature `006-schedule-channels` landed within a day of its research topic
opening — the third consecutive research-to-shipped cycle, and the third
consecutive milestone measured at **zero harness changes**. M3, schedule
channels, is done.

**What shipped `[measured]`:** the `imps/schedule` package — core module,
zero new dependencies, zero root-package changes, zero Makefile/CI edits.
`Channel(stream, target, opts…)` is sugar over the **existing**
`StreamSource` with a header-only `Tick{Subject, Scheduler, Next}` decoder
(provenance on every tick) and the target subject as entity;
`Register`/`Deregister` are typed builders over the substrate's six
scheduling headers — tick TTL as the explicit stale-tick governor, time
zone, body, source, rollup — with fail-fast validation proven to make zero
substrate contact on invalid input (nil-handle tests), and deregistration
as subject purge. The framework runs no timers, produces no ticks, and
owns no schedule registry: the server owns the clock.

**The suite, all `[measured]`:** header round-trips read stored schedules
back and assert every option→header mapping (and that minimal registration
writes nothing extra); re-registration replaces (same subject, new
pattern) and a fast-direction switch proves replacement governs the next
firing (register `@every 1h`, observe silence, replace with `@every 1s`,
observe a tick — never waiting out a slow cadence); the research spike is
the permanent warm/cold/TTL test — live ticks with provenance, a 5 s cold
gap, then durable catch-up where the no-TTL schedule delivered its full
backlog (≥3) and the 2 s-TTL schedule strictly fewer (the unexpired tail,
1–4), pruned server-side with zero imp-side filtering. Full gate `-race`
green across both modules; `compile-deny` green.

**Refuted / propagated:** one grammar assumption — the server's cron is
**six-field with a seconds field** (`0 0 12 * * *`), not five-field; a
five-field pattern is rejected with err_code 10189. Found by the test
suite on first contact, fixed, and propagated into the design doc and the
`Register` documentation alongside the full measured grammar (`@every`
min 1 s, `@at` one-shots, predefined `@hourly`…`@yearly`). `Tick.Next`
was also corrected from "empty on final firings" to the verbatim header
(the server writes a purge marker, not an absence).

**What it taught:** the fourth package to ride the existing seams
(soulstream, persist, schedule beside the untouched core) confirms the
pattern the constitution predicted — the harness's smallness is not a
limitation being worked around but the reason each milestone reduces to a
thin typed surface over something the substrate already does. **What it
opened:** the roadmap's remaining `[D]` fronts are **M4** (audit emission
— needs its design doc) and **M2b** (whole-imp snapshot sleep — gated on
soulrealm declaring suspend/resume). The vision's channel taxonomy is now
fully shipped: external, soulstream, and schedule channels all `[V]`.

Reversal condition: inherited from episode 0008 and unchanged — a
substrate release re-shaping the `Nats-Schedule*` headers flips typed
headers from safety to liability (the integration suite reads it out
directly), and the first real unbounded-backlog incident converts
TTL-optional into TTL-required-with-explicit-opt-out.

Trail: [`../02-DESIGN/0005-schedule-channels.md`](../02-DESIGN/0005-schedule-channels.md)
(now `[V]`, grammar drift propagated); anatomy schedule channels
`[D]`→`[V]`; `specs/006-schedule-channels/` (spec, plan, research, data
model, contracts, quickstart, tasks — all 13 closed); commits `5cfeacc`
(spec), `e9966f1` (plan), `07f2270` (tasks), `655b008` (implementation),
plus this landing change.

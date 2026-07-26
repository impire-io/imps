# Episode 0006 — M2 ships: the durable tier, transcribed from measurement (2026-07-26)

Feature `005-sleep-wake-persistence` landed the same day its research topic
opened — the second consecutive research-to-shipped single-day cycle. M2,
"sleep/wake and snapshot persistence," is done, and the pattern from M1
held: because the spike had already measured the whole boundary,
implementation was transcription, not discovery.

**What shipped `[measured]`:** the `imps/persist` package — in the **core
module**, with **zero new dependencies** (the reference backend uses
`jetstream`, already required), zero root-package changes, and zero Makefile
or CI edits (the existing gate covers the package by construction).
Surfaces exactly as designed, no contract drift: `Store[T]` — bounded LRU
residency (exported `DefaultBound = 256`), **write-through** persistence
(an `Update` return IS durability; the snapshot is continuous; stopping the
imp IS sleeping), lazy rehydration on access, and a wake hook fired exactly
once per rehydration with elapsed from the persisted last-active stamp,
before the state is observable, never writing back; `Delete` as the only
backend removal (eviction is a pure drop — asserted via a
counting-backend: 10 entities through a bound of 4 produced exactly 10
puts, 0 deletes, 0 loss); `Backend` as the minimal boundary with
`KVBackend` as reference; `Beacon` with first-start absence distinct from
zero sleep. The research spike is the permanent restart test: a real imp,
stop, fresh instance, codec-equal state, wake elapsed ≥ the true stop and
wall-clock-bounded, beacon agreeing at imp level. Deterministic elapsed
assertions ride an injected clock (exact 5-minute and 2-minute readings in
units), concurrency exactly-once holds under `-race`, and backend failures
surface as errors — never a silent zero. Full gate green across both
modules; `compile-deny` green.

**Refuted / propagated:** the anatomy's pre-M2 wording promised things the
measured design deliberately does differently, and the docs now say what
is true `[measured]`: eviction does not "send cold entities to the
backend" (write-through means they are already there — eviction drops);
persistence is not "on a configurable schedule" (it is continuous); the
wake-hook is not a framework call on restore (the store fires per-entity
on rehydration; the `Beacon` gates `main()` before `Run` — satisfying the
"single call before dispatch resumes" contract without a harness hook).
The two-tier memory rule is now explicit in the anatomy: the registry is
the cap-rejecting ephemeral tier, `persist` the durable tier, one tier per
concern.

**What it taught:** the byte-identity discipline is compounding — three
declared anatomy parts (soulstream channels, durable memory, the
wake-hook) have now shipped against a harness core that has not changed
since feature 002. **What it opened:** M3 (schedule channels) is the last
`[D]` channel kind, its M2-wake-semantics dependency now settled; its
remaining gate is a design doc plus the server-side scheduling primitive
on the target substrate. Audit emission (M4) remains behind it.

Reversal condition: inherited from episode 0005 and unchanged — real
two-tier inconsistency bugs or measured dispatch-latency damage from
bounded store IO in awareness reverses the beside-the-registry placement
into a harness-native memory redesign; update-rate evidence converts
write-through to write-back batching inside the package. This episode adds
one: if `DefaultBound = 256` proves wrong in either direction in real
deployments, the default changes with a journey note, not silently.

Trail: [`../02-DESIGN/0004-sleep-wake-persistence.md`](../02-DESIGN/0004-sleep-wake-persistence.md)
(now `[V]`, no drift); anatomy Memory / Persistence-and-sleep / wake-hook
sections propagated to `[V]`; `specs/005-sleep-wake-persistence/` (spec,
plan, research, data model, contracts, quickstart, tasks — all 16 closed);
commits `c15c26d` (spec), `46b8f32` (plan), `ad53620` (tasks), `d352002`
(implementation), plus this landing change.

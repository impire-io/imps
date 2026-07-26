# Episode 0005 — Sleep, wake, persistence: beside the registry, not inside it (2026-07-26)

M2's gate demands boundaries before mechanisms: the eviction/rehydration
boundary and the snapshot/restore contract specified before any backend is
chosen. The `sleep-wake-persistence` research topic pre-registered four bars
and passed all four the same day — with the same headline as M1: **zero
harness changes needed.**

**Bar 1 (seam inventory) — PASS `[measured]`.** Every hook persistence could
want was pinned `file:line` in the shipped state machinery. The
registry-riding route is blocked four ways at once: cap overflow *rejects*
with "No silent eviction occurs" as documented contract (`state.go:26-33`),
`StateRef.Get() any` has no error channel for a failed rehydration,
`Factory func() any` cannot know its entity, and no enumeration exists. But
the boundary needs none of it — `Entity` + user code + a backend are
already-sufficient.

**Bars 2–4 (the spike) — PASS `[measured]`,** 3 consecutive `-race` runs,
imps tree byte-identical: a write-through store (bounded LRU, NATS KV as
reference backend) fed by a real imp's awareness survived a stop-and-replace
restart with codec-equal state (`Counter == 6`); the per-entity wake hook
fired **exactly once per rehydration** with elapsed ≥ the 400 ms actually
slept and bounded by wall clock, advancing state as a pure function of the
delivered interval, with no re-fire on resident hits; and 10 entities
through a bound of 4 held residency at ≤ 4 with zero loss — write-through
makes eviction a lossless drop by construction, and makes the snapshot
continuous (nothing to flush at shutdown, no schedule to tune).

**Refuted:** the assumption that M2's "framework enforces eviction by
default" requires the framework's *registry* to evict. The adversarial pass
took the harness-native route at full strength and it lost on measured
grounds — it breaks four documented guarantees and moves backend IO into the
dispatch path. Placement resolved to glue `[judgment]`, refined at design
time from "nested module" to **a plain `imps/persist` package in the core
module**: M1's module boundary existed to fence a new dependency, and
persist adds none (`jetstream` is already a core dependency).

**What it opened:** the design —
[`../02-DESIGN/0004-sleep-wake-persistence.md`](../02-DESIGN/0004-sleep-wake-persistence.md)
— specifies the two-tier memory boundary (registry = ephemeral hot tier,
store = durable tier), the envelope contract (`{state, last_active}`,
write-through), per-entity wake-on-rehydration, the imp-level `Beacon`
pre-`Run` gate for isolation-snapshot sleep, and a minimal `Backend`
interface with NATS KV as reference only — no backend committed. Cold-start
message replay needed nothing new: feature 004's durable consumers already
carry consumer position. M2 is ready for `/speckit-specify`; M3 (schedule
channels) was gated on M2's wake semantics being settled, and they now are.

Reversal condition: two readings, registered at decision time — real imps
showing two-tier inconsistency bugs (state split across registry and store
going wrong in practice), or measured dispatch-latency damage from bounded
store IO in awareness, reverses the beside-the-registry placement into a
harness-native memory redesign. The write-through choice reverses on
update-rate evidence, into write-back batching inside the package.

Trail: [`../02-DESIGN/0004-sleep-wake-persistence.md`](../02-DESIGN/0004-sleep-wake-persistence.md)
(the graduated design); the topic's pre-registration, seam inventory, spike
record, and adversarial pass live in git history under
`hq/01-RESEARCH/sleep-wake-persistence/` (removed at graduation); commits
`dfa22dd`, `8d830f9`.

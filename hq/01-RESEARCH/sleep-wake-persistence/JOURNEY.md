# Journey — sleep-wake-persistence (started 2026-07-26)

## 2026-07-26 — Bar 1: the seam, inventoried

Read the shipped state machinery in full (`registry.go`, `state.go`, the two
context files, `spec.go`). The per-entity seam is: `State(name, entity)` →
`registry.ref` → `StateRef{Get, Set, Update}` over a `slot{mu, val any}`.
The inventory, `[measured]` (readings in the shipped source):

| Hook persistence would need | Present? | Evidence | Classification |
|---|---|---|---|
| Eviction / slot removal | **No** — slots are never deleted; cap overflow *rejects* new entities, and "No silent eviction occurs" is documented contract | `registry.go:58-66`, `state.go:26-33`; no `slots.Delete` anywhere | **blocking** *for the registry-riding route* (eviction would contradict a documented guarantee) |
| Enumeration (walk entities for snapshot) | **No** — `registry`/`shape` unexported, `sync.Map` never iterated outward | `registry.go:9-15,28-30` | blocking for registry-riding |
| Entity-aware creation (rehydrate at allocation) | **No** — `Factory func() any` takes no entity | `spec.go:35-39`, `registry.go:67` | blocking for registry-riding (signature change on the public spec) |
| IO-error surface on read (rehydration can fail) | **No** — `StateRef.Get() any` returns no error | `state.go:9-13` | blocking for registry-riding (context-adjacent surface change) |
| Entity identity + user code at both tiers | **Yes** — awareness and thinking both receive `Entity` and run arbitrary user code | `spec.go:14-30` | already-sufficient |
| Bounded outbound IO from user hooks | **Yes** — precedent set by M1's note bridge (hook-side publish); awareness's own `Request` is the bounded-round-trip archetype | episode 0004; `context_awareness.go:25-32` | already-sufficient |

**Reading of the inventory:** persistence cannot ride *inside* the registry
without breaking four documented guarantees at once — which is exactly the
reversal condition's shape. But the boundary does not need the registry: the
seam persistence actually requires is `Entity` + user code + a backend, all
already-sufficient. The M1-shaped hypothesis for the spike: a **persistence
store beside the registry** (glue), with its own bound, eviction, write-through,
and per-entity wake-on-rehydration — the harness registry remains the
ephemeral hot tier, byte-identical. Bar 1 verdict deferred until the spike
confirms zero unlisted harness changes (Bar 2).

## 2026-07-26 — Bars 2–4: the spike — beside the registry, everything passes

Built the spike as a scratchpad Go module (`sleepspike`, `replace` to the
imps working tree): a prototype persistence store **beside** the registry —
bounded LRU residency, **write-through** to a reference backend (NATS KV on
embedded server), rehydration on access, and a per-entity wake hook fired on
rehydration with elapsed time sourced from a persisted last-active stamp.
The snapshot/restore contract under test is one envelope per entity:
`{state: <codec bytes>, last_active: <wall clock>}`.

The scenario: a real imp whose awareness does `store.Update` per dispatched
message (a bounded KV round-trip — the same discipline as awareness's own
`Request`, and the M1 note-bridge precedent); three events mutate
`cust-1`'s counter; the imp stops (**that is sleep** — write-through means
there is nothing to flush); 400 ms pass; a fresh imp instance with a fresh
store rehydrates on first access.

**Results `[measured]`** (3 consecutive `-race` runs, ~1 s each; imps
working tree byte-identical throughout):

- **Bar 2 PASS** — rehydrated `Counter == 6`, equal under the codec to the
  pre-stop value; harness-core changes: **zero** (not even the minimal
  additions Bar 1 held in reserve).
- **Bar 3 PASS** — the wake hook fired **exactly once**, elapsed ≥ 400 ms
  and bounded by wall clock; state advanced deterministically from the
  delivered elapsed (`IdleMs` set from it); a resident re-access did not
  re-fire.
- **Bar 4 PASS** — 10 entities through a bound of 4: residency never
  exceeded 4 during writes or readback, and all 10 states read back intact
  after eviction (write-through makes eviction a lossless drop by
  construction).

## 2026-07-26 — Adversarial pass: where does per-entity persistence live?

**Position A — inside the registry (harness-native).** For: the anatomy says
"the framework enforces retention and eviction by default"; one memory
surface; no IO in user code. Against, at full strength `[measured]` (Bar 1):
riding the registry breaks four documented guarantees at once — cap
overflow *rejects*, "No silent eviction occurs" (`state.go:26-33`);
`StateRef.Get() any` has no error channel, so a failed rehydration could
only panic or silently zero (a stubs-are-never-silent violation);
`Factory func() any` cannot rehydrate without a public signature change;
no enumeration exists for snapshots. And it would put backend IO inside the
registry's slot lock on the dispatch path. Refuted.

**Position B — a store beside the registry (the spike).** For `[measured]`:
every bar passed with zero harness changes; write-through makes eviction
lossless by construction and *is* the snapshot (continuous, no schedule to
tune); the wake contract is a pure function of the persisted stamp. Against,
at full strength: two memory surfaces (registry = ephemeral hot tier, store
= durable tier) that developers must choose between; the anatomy's "by
default" now means *documented default*, not *only path*; awareness's
bounded-IO discipline becomes convention rather than structure
`[mechanism-argument]` — one KV round-trip is `Request`-equivalent, and the
compile-enforced boundary never governed what user closures capture (M1
precedent).

**Position C — minimal harness hooks (loader/evictor on StateShape).**
Against: inherits A's error-channel problem on `Get`, still moves backend
IO into the registry path, and grows the core spec surface for a capability
one module can carry. Refuted.

**Resolution `[judgment]`: B**, as an `imps/persist` glue module shaped like
M1's. The anatomy's "default" is satisfied by making the store the blessed,
documented path for durable per-entity state with a small default bound,
while `ImpSpec.States` remains the ephemeral tier. Registered reversal (for
the design doc): evidence of the two-tier split producing real
inconsistency bugs, or measured dispatch-latency damage from bounded IO in
awareness, moves the boundary into the harness as a redesign.

**Imp-level wake-hook (isolation-snapshot sleep):** expressible in glue as a
gate before `Run` — the store keeps an imp-scoped last-active stamp; user
code asks the store for `SleptFor()` and calls its wake hook before starting
the imp, satisfying the anatomy's "single call, before any channel dispatch
resumes" `[mechanism-argument]` (the isolation mechanism itself stays an
infrastructure choice, per the constitution). Cold-start message replay is
already shipped: feature 004's durable consumers.

**Anatomy alignment noted for the design doc:** the anatomy's
Persistence-and-sleep section distinguishes isolation-snapshot sleep (whole
memory image preserved by the infrastructure; **imp-level** wake-hook, "a
single call, before any channel dispatch resumes") from hard-restart
persistence (per-entity serialize + replay). Feature 004's `WithDurable`
already covers consumer position across restarts (anatomy line 28). The spike
measures the per-entity path (evict/rehydrate/wake-on-rehydration); the
imp-level wake-hook is a contract to specify, its elapsed sourced from the
same persisted last-active stamps.

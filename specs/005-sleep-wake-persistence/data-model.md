# Data Model: Sleep, Wake, and Per-Entity Persistence

The package holds no state of its own beyond the bounded resident cache;
everything durable lives behind the `Backend` boundary as envelopes.

## Envelope — the unit of persistence

| Field | Type | Rules |
|---|---|---|
| `state` | bytes (codec output; base64 in the JSON wire form) | Encoded by the store's codec; opaque to the envelope. |
| `last_active` | RFC3339Nano wall-clock stamp | Set on every write-through; the elapsed source for the wake hook. |

- Key: `<store-name>.<entity>`. Store names are the collision boundary;
  uniqueness per deployment is the operator's concern.
- Written atomically as one value on every `Update` (no torn state/stamp).
- A malformed envelope (undecodable JSON, codec failure) surfaces as an
  error from the access — never a silent zero.

## Store[T] — the durable tier

| Aspect | Rule |
|---|---|
| Residency | LRU-bounded map, default 256, `WithBound` overrides; hottest at front. |
| Miss | Backend get → decode → wake (if due) → insert (evicting coldest first). |
| Never-seen | Zero `T`, no wake, no error; inserted resident to avoid repeated misses. |
| Update | Rehydrating `Get` first (so wake runs before `fn`), then `fn`, then write-through, then residency refresh. Success ⇒ durable. |
| Eviction | Pure drop. MUST NOT write, flush, or delete backend state. |
| Delete | The only backend removal; clears residency too. |
| Concurrency | Operations serialize (one mutex, held across IO); wake-exactly-once holds under `-race`. |

State transitions for one entity:

```text
        ┌────────────┐  access   ┌──────────┐  evict (drop)  ┌──────────┐
        │ never-seen │──────────▶│ resident │───────────────▶│ backend- │
        └────────────┘ zero, no  └──────────┘                │   only   │
                        wake       ▲    │ Update: write-through └────┬───┘
                                   │    ▼                            │
                                   └── access: rehydrate + wake ◀────┘
                     Delete (explicit): removes resident AND backend state
```

## WakeFn[T] — the per-entity wake hook

- Signature: `(entity, elapsed, state) → state`.
- Fires exactly once per rehydration, before the state is observable; never
  on resident hits or never-seen entities; never writes back.
- Contract on the developer: pure in `elapsed` ("advance to now"), so a
  re-fire after eviction — computed from the same `last_active` — is
  harmless by construction.

## Beacon — the imp-level sleep clock

| Operation | Rule |
|---|---|
| `Stamp` | Writes now under the beacon's own key (`<name>`). Call on a heartbeat and at shutdown. |
| `SleptFor` | Reads the stamp: `(now − stamp, true, nil)`; never-stamped → `(0, false, nil)`; backend failure → error. |

- Usage pattern (documented, not enforced): `main()` asks `SleptFor`, runs
  the imp-level wake step, then `imp.Run` — satisfying "a single call,
  before any channel dispatch resumes".

## Backend — the persistence boundary

- `Get(ctx, key) ([]byte, error)` — `ErrNotFound` on miss (sentinel,
  `errors.Is`-able).
- `Put(ctx, key, value []byte) error`.
- `Delete(ctx, key) error` — deleting an absent key is not an error.
- `KVBackend(kv jetstream.KeyValue)` is the reference implementation,
  mapping the substrate's not-found to `ErrNotFound`.
- The framework never requires this backend; anything satisfying the
  interface serves.

## Relationship to the ephemeral tier

`ImpSpec.States` (registry) and `persist.Store` never interact. The
documented rule of thumb: state whose loss on restart is a bug goes in the
store; state rebuildable from the stream goes in the registry. One tier per
concern; the package performs no synchronization between them.

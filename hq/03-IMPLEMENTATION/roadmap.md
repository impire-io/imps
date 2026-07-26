# imps Roadmap

What is still to be built, in what order, behind which gates. **No dates** —
milestones are gated on exit criteria, not calendars. Shipped work lives in the
ledger at the bottom, one line each, with its full story in the journey
episode. The operating principles this plan obeys are the
[constitution](../00-GENESIS/constitution.md); the identity it serves is in
[vision.md](../00-GENESIS/vision.md).

Every milestone below is **declared in the vision or the anatomy** as part of
what an imp is, and was deliberately left out of the core so the substrate could
ship small. Each one ships as its own numbered feature through the spec-kit flow,
gated on a design doc in [`../02-DESIGN/`](../02-DESIGN/README.md) explicit
enough for `/speckit-specify`. The order below is the order the constitution's
"sleep is the common case" and "coordination happens through the soulstream"
commitments suggest; it is not fixed, and a real use case can re-order it.

---

## Now — the front

**M2 is ready to open.** The `sleep-wake-persistence` research topic
concluded ([episode 0005](../04-JOURNEY/0005-sleep-wake-persistence.md),
2026-07-26): the seam is inventoried, the spike measured a restart
round-trip, exactly-once wake with true elapsed time, and lossless bounded
eviction — all with zero harness changes — and the design doc,
[`../02-DESIGN/0004-sleep-wake-persistence.md`](../02-DESIGN/0004-sleep-wake-persistence.md),
is written for `/speckit-specify`. The next act is opening the numbered
feature from it.

## Next — the declared, unbuilt parts of an imp

### M2. Sleep/wake and snapshot persistence

The wake-hook (elapsed-sleep signal), per-entity eviction to a persistence
backend, rehydration on access, and cold-start replay — the "sleep is the
common case" commitment, made real (vision "Imps sleep when they're not
working"; anatomy, Memory / Persistence and sleep / The wake-hook `[D]`).
The research reshaped the milestone: persistence lives **beside** the
registry (riding it is blocked by four documented guarantees), shipping as
the `imps/persist` package — no new dependencies, harness core untouched,
write-through as the continuous snapshot, wake-on-rehydration per entity
plus a `Beacon` pre-`Run` gate at imp level, backend uncommitted behind a
minimal interface. Cold-start message replay needs nothing new (feature
004's durable consumers).

- *Gate:* **met (2026-07-26)** — design doc
  [`0004-sleep-wake-persistence.md`](../02-DESIGN/0004-sleep-wake-persistence.md)
  naming the snapshot/restore contract (envelope: codec bytes +
  last-active), the wake-hook semantics at both levels, and the
  eviction/rehydration boundary, with the backend left open (episode 0005).
- *Exit:* an imp survives a snapshot/restore cycle with time-dependent state
  advanced by the wake-hook; bounded local memory evicts cold entities and
  rehydrates them on access; the default stays small; the harness core and
  its `go.mod` byte-identical; gate green including `compile-deny`.

### M3. Schedule channels

Periodic work as a channel kind fed by NATS server-side scheduling; TTLs govern
whether stale ticks accumulate across long sleeps (vision "Periodic work uses
NATS server-side scheduling"; anatomy, Channels — Schedule channels `[D]`).

- *Gate:* a schedule-channel design doc, plus the server-side scheduling
  primitive available on the target substrate.
- *Exit:* a registered schedule fires whether the imp is warm or cold, dispatch
  is identical to other channel kinds, and stale-tick accumulation is governed
  by an explicit TTL. Depends on M2's wake semantics being settled.

### M4. Audit emission

Every action emission writes a tenant-scoped audit record — what was emitted,
by which imp, against which entity, in response to which escalation — so the
colony's behavior is reconstructable (anatomy, Action `[D]`).

- *Gate:* an audit design doc defining the record shape and the audit stream.
- *Exit:* action emissions are reconstructable from the audit stream; records
  carry no request/response content beyond attribution and lifecycle.

## Not a framework milestone (stays external, by constitution)

These are deliberately **not** on the roadmap — building them into the harness
would violate "capabilities are external; the harness is small."

- **Capability implementations** (inference, knowledge, tool execution) — each
  is a separate NATS micro service following
  [`../02-DESIGN/0002-capability-service-pattern.md`](../02-DESIGN/0002-capability-service-pattern.md).
  The framework never grows these.
- **Capability-specific client libraries** — live with their capability, not in
  the harness; an imp reaches them over a subject or via `Conn()`.
- **A central registry / discovery service** — the framework does no discovery;
  `$SRV.INFO` against a live deployment is the operator's tool (constitution,
  Non-Negotiables).

## Conventions

- A milestone becomes a numbered feature only when its `02-DESIGN/` doc is
  explicit enough for `/speckit-specify`. Feature numbers come from git
  branches; episode numbers from the journey sequence.
- **Landing a feature means, in the same merge:** gate green
  (`make fmt && make test && make lint` plus `make compile-deny`), this roadmap
  updated with the outcome and the episode link, the `04-JOURNEY/` episode
  written, and behavioral changes propagated into the `02-DESIGN/` docs.
- **Exit criteria are written before the work** and amended only openly. This
  file is load-bearing: changes to it are decisions and get a journey episode
  like any other.

---

## Ledger — shipped

| Feature | What landed | Episode |
|---|---|---|
| `001-harness-core` | The in-process Go substrate: channels (core-subject + JetStream), awareness dispatch, thinking invocation, per-entity local memory, action publishing; the awareness/thinking boundary compile-enforced. | [0001](../04-JOURNEY/0001-founding-the-harness.md) |
| `002-capability-client` | The outbound NATS surface: `Request` / `RequestMany` / `Publish` / `Conn`, literal subjects, byte-shaped, no framework codec or retry. | [0001](../04-JOURNEY/0001-founding-the-harness.md) |
| `004-soulstream-participation` | M1: soulstream topics as channels via the `imps/soulstream` nested glue module — `TopicChannel` on the existing `StreamSource`, the `Note`→`comment.add` bridge, `Participant` on the imp's own connection; harness core byte-identical. | [0004](../04-JOURNEY/0004-soulstream-participation-shipped.md) |

The post-001/002 refactor sweep (package flattened to the module root, `Wake`→`Think`,
`Reasoning`→`Thinking`, the action whitelist removed) is recorded in the same
founding episode; the moves into `hq/` and the MIT licensing decision are
[episode 0002](../04-JOURNEY/0002-hq-adoption-and-mit.md).

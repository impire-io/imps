# Journey — sleep-boundary-with-soulrealm (started 2026-07-27)

## 2026-07-27 — Bar 1: both scopes, pinned from the owners' documents

Swept soulrealm's entire hq (plus the soulstream work extension it builds
on) with a survey agent and read the imps GENESIS documents directly. The
evidence, `[measured]` (readings in the owning repos):

**Soulrealm declares** (its verbs, everywhere: *launch, supervise, observe,
retire*):

- **Durable state: explicitly disclaimed, non-negotiably.** Constitution
  Article I (`soulrealm/hq/00-GENESIS/constitution.md:13-20`): "Soulrealm is
  a runtime, never a store of record… the authoritative home of any artefact
  — its bytes, its history, its current state — is the soulstream topic… A
  workload that dies loses scratch state, never history. **No feature may
  make soulrealm the place a piece of durable truth lives.**" Echoed in the
  vision's refusals (`vision.md:62-65`) and FR-007. Its only local state is
  per-workload scratch, reaped on exit.
- **Suspend/resume, sleep, snapshot, scale-to-zero, idle: silent.** Zero
  hits across every hq doc and all Go code; the lifecycle model has terminal
  states only (`work.open → claim → done|abandon`), and even bounded
  auto-restart is "a named later feature, not built here"
  (`specs/001-launch-an-agent/spec.md:110-112`). Firecracker appears only as
  a pluggable backend option `[O]` — never with snapshot capability
  mentioned. This is the natural documentary home for suspend/resume, and it
  is conspicuously empty.
- **Slept-for / wake signal: silent** (no down/resume concept exists to
  signal).
- **Workload-internal memory management: silent** ("bounded" only ever means
  credential expiry; "resource limits" named once as an abstract backend
  seam).
- **imps: never mentioned.** No soulrealm doc references the imp framework.

**imps declares:**

- Vision (`imps/hq/00-GENESIS/vision.md:66-68`): "The framework uses
  snapshot-based suspension to preserve an imp's full memory image; NATS
  holds messages… on arrival, the imp wakes… **The imp doesn't know it was
  asleep.** The specific isolation mechanism — microVMs, containers,
  processes, or in-process simulation — is an infrastructure choice the
  framework does not dictate; **what the framework specifies is the contract
  that any isolation mechanism satisfies.**"
- Constitution (`constitution.md:150-152`): "designed around snapshot-based
  sleep first. Hard restart is the exception path… the framework specifies
  the contract, not the implementation."
- Anatomy (pre-005, on main): isolation-snapshot sleep preserves the full
  image `[D]`; hard restarts serialize per-entity state to a persistence
  backend `[D]`; local memory bounded with eviction `[D]`; wake-hook "a
  single call, before any channel dispatch resumes" `[D]`.

**The triangle nobody drew before:** the Impire family has *three* parties —
soulstream (the record), soulrealm (the room), imps (the inhabitant). The M2
research treated the boundary as imps-vs-substrate; the owner's challenge
treated it as imps-vs-soulrealm; the documents say durable truth belongs to
*soulstream*, execution to *soulrealm*, and the contract to *imps*.

## 2026-07-27 — Bar 2: five assignments, five discriminators

Each argued both ways at full strength; the discriminator is what the losing
side *structurally cannot do*.

1. **Whole-imp suspend/resume (the mechanism) → soulrealm** (a future
   capability of its isolation-backend seam). *Discriminator:* a process
   cannot snapshot its own memory image from inside — only the supervisor
   holding the isolation boundary can freeze and resume it, and the imps
   vision explicitly places the mechanism outside the framework
   `[measured]`. Soulrealm is silent today, but its backend seam (native /
   Docker / Firecracker `[O]` / K8s `[O]`) is the only Impire home the
   capability *can* land in. imps keeps only the contract statement.
2. **The authoritative slept-for signal → soulrealm supplies, imps
   contracts.** *Discriminator:* "the imp doesn't know it was asleep" is
   literal — under snapshot-suspension no imp code runs at suspend time, so
   no self-stamp can be authoritative; only the suspender knows the
   interval. Feature 005's `Beacon` is a **self-report** that covers
   graceful stops and hard restarts (and heartbeat-stamping bounds the error
   for crashes) — it structurally cannot cover the vision's primary sleep
   path. The imps-side contract is the wake *gate*; the runtime is the
   authoritative *source* when it exists.
3. **Per-entity durable state across restarts and redeploys → imps** (state
   lives on the substrate behind the backend boundary). *Discriminator
   one:* soulrealm's own constitution forbids it owning this — Article I is
   non-negotiable and names the disqualification itself `[measured]`.
   *Discriminator two:* even a perfect snapshot-sleep story cannot cover
   redeploys — a new imp binary cannot resume an old memory image, so
   per-entity serialization through codecs only the imp author defines is
   irreducibly imp-side. (Considered and rejected: persisting imp state
   into soulstream *topics* per the "everything worth keeping" doctrine —
   that doctrine governs collaboration artefacts; an imp's private
   interpretive state is exactly what the imps anatomy excludes from
   shared media, and flooding op-logs with private cache envelopes would
   break topic semantics `[mechanism-argument]`.)
4. **Bounded residency / eviction → imps.** *Discriminator:* eviction
   requires knowing per-entity access patterns and state shapes inside the
   process; a supervisor sees a black box and can only bound coarse RAM
   (cgroup-level resource limits — soulrealm's abstract seam, complementary
   not competing).
5. **Advance-by-elapsed wake code → imps.** *Discriminator:* only imp code
   knows which state is time-dependent and how it advances (EMA, idle-since,
   debounce); no runtime can advance application state.

**Where feature 005 actually overstepped `[judgment]`, now precise:** not in
building the durable tier (assignments 3–5 hold), but in *framing* — the
landing's anatomy rewrite recast "stopping is sleeping" as the sleep story,
demoting the isolation-snapshot path that the vision calls the common case
and that belongs to the runtime. The `Beacon` was presented as "the
imp-level wake gate" when it is structurally the *restart* clock — the
interim source, not the contract's authoritative supplier. And one honest
gap the reframing exposes: **nothing in imps today can receive a wake
mid-process** (a snapshot-resume continues inside `Run`; the pre-`Run` gate
never re-executes). That surface is correctly unbuilt — it must be
co-designed with the runtime's suspend capability — and must be labeled
`[D]`, not claimed by 005.

## 2026-07-27 — Bar 3: the disposition of feature 005 (proposed, pending teach-back)

| Surface | Disposition | Why |
|---|---|---|
| `persist.Store` (write-through, LRU, rehydration) | **keep-as-is** | Assignments 3–4; soulrealm constitutionally cannot own it; redeploys need it regardless of sleep |
| Per-entity wake-on-rehydration | **keep-as-is** | Assignment 5; pure imp code, measured contract |
| `persist.Beacon` | **keep-reframed** | Rename its story: the **restart clock** — self-reported elapsed for graceful stops / hard restarts / (heartbeat-bounded) crashes; explicitly *not* the snapshot-sleep signal; documented as the interim imp-level source until the runtime supplies the authoritative one |
| Imp-level wake for snapshot sleep | **stays `[D]`** | Unbuilt and correctly so — needs mid-process delivery co-designed with soulrealm's future suspend capability; the seam is named below |
| 005's anatomy rewrite (Persistence-and-sleep, wake-hook `[V]` claims) | **rewrite before merge** | Restore isolation-snapshot sleep as the runtime-owned common case (soulrealm named as the Impire-family implementation); scope the `[V]` claims to the durable tier + restart clock only |
| Design doc 0004 + episode 0006 + roadmap M2 | **re-scope** | M2 splits: **M2a** — durable memory tier + restart persistence (what 005 built; ships) and **M2b** — snapshot sleep/wake, `[D]`, gated on soulrealm growing suspend/resume and a co-designed wake-delivery contract; episode rewritten to record this challenge and the split |

**The integration seam (sketch, for M2b's future gate):** soulrealm's one
control plane is the op-log — a resume would be accompanied by a
runtime-emitted wake signal carrying the suspension interval, delivered so
the imp processes it before its message backlog (ordering contract to be
designed; candidates: a designated wake subject consumed first, or a
first-message guarantee on the control stream). The imps side then routes
that elapsed into the same advance-by-elapsed user code the per-entity hook
already established. Until then, the `Beacon` self-report is the documented
fallback, and the constitution's "sleep is the common case" stays aspirational
at the whole-imp level — truthfully labeled.

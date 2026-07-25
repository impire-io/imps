# Journey — soulstream-participation (started 2026-07-25)

## 2026-07-25 — Bar 1: the contract, pinned from the owning repo

Swept the `soulstream` repo (local checkout, `main`, clean) with a read-only
survey agent, then independently spot-checked the load-bearing citations by
reading the cited lines directly (`topic/subjects.go`, `topic/follow.go`,
`docs/topic.md`, `realm/spec.go`, `hq/02-DESIGN/core/03-topics.md`). Every
spot-check matched the survey verbatim. Result: [`contract.md`](contract.md),
one row per consumed subject/operation, each with `file:line` evidence
`[measured]` (readings in the owning repo's source and normative docs).

**The finding that reshapes M1:** soulstream has **no join/leave operation and
no membership state**. "Joining" a topic is subscribing (a JetStream ordered
consumer filtered to `SOULSTREAM.TOPICS.OPS.<path>` — history then live,
baseline first); "leaving" is stopping the consumer; posting rights are subject
permissions, nothing else `[measured]` (`docs/topic.md:20-21`,
`03-topics.md:29`, zero join/leave types in `topic/vocab.go`). This is exactly
the channel-add/channel-remove shape M1 hypothesised — the harness would not be
modelling membership at all.

**Natural `Note` mapping found:** `comment.add` anchored to an op-id — a
lightweight contribution that annotates an existing op without opening
anything. Awareness observes a dispatched op and can note *on that op*; the
anchor is the op it just saw. `turn.post` remains the thinking-tier
contribution. `[mechanism-argument]` — the mapping is argued from the
vocabulary's shape, not yet exercised.

**No protocol state machine found:** no session, handshake, heartbeat, ack, or
cursor anywhere in topic participation. The consumer-side state is a per-topic
frontier (merge-safe if stale) and the baseline-first replay rule on read
`[measured]` (contract.md, "Protocol state" section). On today's evidence the
reversal condition is not triggered.

**Open questions carried forward to Bar 2** *(all three answered below, same day)*:

1. **Library vs. wire:** consume soulstream's own Go client
   (`github.com/impire-io/soulstream/topic` — `Handle`, `Follow`, `PostTurn`,
   `AddComment` already exist) or speak the wire from imps directly? The
   library route imports the capability owner's code into the harness; the
   wire route re-implements headers, canonical signing binding, and frontier
   logic that the owner already maintains. Constitution lens: "capabilities
   are external" cuts both ways here — soulstream is substrate coordination
   (vision: "coordination happens through the soulstream"), not a capability
   service. Needs the adversarial pass before the design doc.
2. **Where frontier state lives** — in the channel, or in per-entity local
   memory keyed by topic?
3. **Whether an imp's topic read rides the existing JetStream channel kind
   unchanged** — the existing channel kind vs. the ordered-consumer,
   baseline-first shape soulstream reads need. This is precisely Bar 2's
   spike.

## 2026-07-25 — Bars 2 and 3: the spike, and the loop closing with zero harness changes

Built the spike as a standalone Go test module in the session scratchpad
(`soulspike`), with `replace` directives pointing at both local working trees —
so the test compiles against the imps harness **exactly as committed**, and any
harness edit would show in `git status`. The scenario, end to end:

1. Embedded NATS with JetStream (`imps/testutil/natstest`), provisioned to the
   soulstream shape by the owner's own code (`realm.ProvisionOn` accepts any
   JetStream handle — no NATS context file needed).
2. A `scribe` persona starts a topic (`topic.StartTopic`) and posts two turns
   of history *before* the imp exists.
3. The imp declares **one existing `StreamSource` channel** — stream
   `SOULSTREAM`, `FilterSubject: SOULSTREAM.TOPICS.OPS.<path>`,
   `DeliverPolicy: DeliverAllPolicy` — nothing else. Its `Decode` reads three
   headers (`Soulstream-Type`, `Soulstream-Author`, `Nats-Msg-Id`); no payload
   parsing, no soulstream import needed for the read path.
4. Awareness returns the **existing `Note` verdict** on other personas' turns;
   the `OnNote` hook is the soulstream bridge: it posts `comment.add` anchored
   to the noted op via the owner's library (`topic.Open(...).AddComment`),
   over the imp's **own** NATS connection (`realm.NewClient` wraps an existing
   conn — no second connection).
5. A third turn is posted live after the imp joined; the imp's comments
   round-trip back through its own channel; the scribe materialises the topic
   with the owner's library and sees the imp's comments as first-class,
   properly anchored contributions.
6. The imp shuts down; its ephemeral consumer is deleted — that *is* leaving,
   the protocol has nothing else to require.

**Results `[measured]`** (3 consecutive `-race` runs, ~0.5 s each):
7/7 ops observed in stream order, baseline first, history→live over one
continuous consumer with no seam; `NotesDelivered=3`, `ThinksDispatched=0`,
`DecodeFailures=0`, `AwarenessPanics=0`; owner's materialised view shows all 3
imp comments anchored (`Dangling=false`); imp consumer deleted on shutdown;
`git status --porcelain` empty in **both** working trees throughout — the
dispatch seam is not just unchanged in contract, it is byte-identical in fact.
`make compile-deny` green on the same tree. Awareness's surface gained
*nothing* — stronger than Bar 3's "at most Note", because `Note` already
existed as a verdict and the emission lives in the `OnNote` hook, outside
awareness's hands.

**The three carried questions, answered:**

1. *Library vs. wire* — see the adversarial pass below.
2. *Where frontier state lives* — for notes: nowhere. The anchor is the op-id
   awareness just observed, and posting with an empty frontier is legal and
   merge-safe `[measured]` (comments landed cleanly and anchored correctly with
   no `Materialise` call). For turns from thinking: in the owner library's
   `Handle`, per use — thinking materialises, posts, drops the handle. No
   harness state anywhere.
3. *Does the existing JetStream channel kind read a topic unchanged* — yes
   `[measured]`, see above.

## 2026-07-25 — Adversarial pass: how does an imp speak soulstream?

The framework-identity call flagged at Bar 1, argued at full strength per the
working agreement.

**Position A — reimplement the wire in imps.** For: no dependency on a pre-1.0
module; wire v1 is frozen ("rare to never" changes); the read path needs only
three header reads. Against, at full strength: the read path needs no
implementation *at all* — so "wire route" really means reimplementing the
**write** path: id generation, attribution, mention parsing plus notify
fan-out, RFC-8785 canonicalisation, Ed25519 signing, frontier handling. All of
it duplicates code the contract owner ships and maintains, and signature drift
fails *silently* (verification is a read-side annotation, so a subtly wrong
canonicalisation just produces ops nobody can verify). Refuted as the default.

**Position B — import the soulstream library into the harness core.** For: one
obvious way; owner-maintained writes. Against, at full strength: every imp —
including ones that never touch soulstream — inherits `uuid`, `jcs`,
`natscontext`, and a pre-1.0 soulstream version coupling; the harness go.mod
today has exactly two direct dependencies and "the harness is small" is a
constitutional commitment; soulstream's release cadence would drive harness
releases. Refuted.

**Position C — a glue package outside the harness core** (own module path, own
go.mod; consumes the owner's `realm`+`topic` packages; provides the
`ChannelSpec` builder, the `OnNote` bridge, and turn-posting for thinking).
This is literally what the spike ran. For `[measured]`: harness byte-identical;
the imp-side integration is ~40 lines; `realm.NewClient` wraps the imp's
existing connection; dependency cost measured by `go mod tidy` on the spike:
`uuid`, `jcs`, `natscontext` (the MCP SDK is *not* pulled — module pruning
keeps it out of any consumer that imports only `realm`/`topic`). Against, at
full strength: M1 then ships zero harness code — the anatomy declares
soulstream channels part of what an imp *is*, and pushing them to a glue
module makes the coordination story a convention rather than a guarantee; and
**dynamic** join/leave (thinking deciding to join a topic at runtime) cannot
be expressed, because `ImpSpec.Channels` is bound once at `Run`.

**Resolution `[judgment]`, with the mechanism-argument spelled out:** C wins
for M1. The protocol itself has no join/leave — presence *is* the consumer —
so "joining" reduces to declaring the channel, which the existing spec surface
already expresses. The one capability C cannot deliver, runtime channel
add/remove, is exactly the one thing no current use case demands; per the
constitution's "the simpler shape by default", dynamic channel lifecycle is
deferred to its own milestone gated on a named use case, not smuggled into M1.
This resolution's reversal condition: **the first real colony scenario in
which the set of topics an imp participates in must change without a restart**
— that evidence reopens the harness-change question (a runtime channel
lifecycle on the existing bind/teardown seam, which `start.go`/`stream.go`
already structure per-channel).

**Design note carried to the doc:** `OnNote` runs synchronously on the
dispatch goroutine, so a bridge that publishes JetStream ops blocks dispatch
for the publish round-trip `[mechanism-argument]`. Harmless at note rates in
the spike; the design doc must either declare it acceptable (notes are
low-rate by definition) or specify an async bridge. Not a harness change
either way.

# Episode 0004 — M1 ships: soulstream participation as a glue module (2026-07-25)

Feature `004-soulstream-participation` landed the same day the research topic
that gated it was pre-registered — research (episode 0003) → design doc →
spec-kit flow (spec, plan, tasks, implementation) → shipped module, with the
gate green at every commit. M1, "soulstream coordination channels," is done.

**What shipped `[measured]`:** the nested module
`github.com/impire-io/imps/soulstream` (own `go.mod`, go 1.26.2, owner
library pinned at the public tag `v0.4.0` — verified code-identical to the
checkout the research spike measured, one docs-only commit apart). Three
surfaces, exactly as designed: `TopicChannel` building a plain `ChannelSpec`
on the existing `StreamSource` (subject verbatim, deliver-all default,
header-only `Op` decode, topic path as entity, durable/start-seq/start-time/
decoder/extractor/name options); `NoteBridge` turning the shipped `Note`
verdict carrying `Noted{AnchorOp, Body}` into anchored `comment.add`
contributions, delegating other payloads and reporting malformed ones without
publishing; `Participant` wrapping the imp's own connection with persona
attribution and optional Ed25519 signing. The research spike became the
permanent integration suite — 7 tests covering all 12 functional
requirements: baseline-first history into live on one consumer, unknown-type
delivery, unprovisioned-realm startup failure with the harness's named error,
the note round-trip verified non-dangling in the owner's materialised view
with `ThinksDispatched == 0`, thinking contributions with attribution,
`SigVerified` via keyring beside `SigUnsigned` history, read-only write
refusal, ephemeral-consumer deletion on leave, and exact durable resume (2/2
missed ops, no duplicates). Full gate `-race` green across both modules;
`compile-deny` green.

**The load-bearing number `[measured]`:** the harness core's diff for the
entire feature is **zero** — `go.mod`/`go.sum` byte-identical to `main`, no
core source file touched. The energy-gradient boundary needed no new
enforcement because awareness's surface never moved.

**Refuted / adjusted along the way:** the feature number — `003` was already
taken by the historical `003-flatten-package` branch, so M1 is feature `004`
(feature numbers come from git branches). Two design-doc signatures drifted
during implementation and were propagated back the same day: `NoteBridge`
gained an explicit `onErr` callback, and `NewParticipant` takes a `context`
and pre-checks JetStream reachability itself — because the owner's client
closes a connection it fails to construct around, and the imp's connection
must survive a failed construction `[measured]` (regression test in
`participant_test.go`).

**What it taught:** the research-first pipeline earned its keep — because the
spike had already measured the whole loop, implementation was transcription,
not discovery; the only surprises were packaging-level (feature numbering, an
owner-library error path closing the conn). **What it opened:** M2
(sleep/wake + snapshot persistence) is now the front, gated on its design
doc; the deferred runtime join/leave keeps its episode-0003 reversal
condition; inbox/mention channels (`SOULSTREAM_NOTIFY`) are a named,
unclaimed follow-on.

Reversal condition: the module shape (nested glue module, static topic set,
synchronous note bridge) reverses on the readings already registered — a real
scenario requiring an imp's topic set to change without restart reopens
runtime channel lifecycle; measured note-rate pressure on dispatch converts
the bridge to an internal queue (glue-module change only). The `v0.4.0` pin
loosens only when a soulstream release breaks a consumed behavior, which the
integration suite would read out directly.

Trail: [`../02-DESIGN/0003-soulstream-participation.md`](../02-DESIGN/0003-soulstream-participation.md)
(now `[V]`, drift propagated); `specs/004-soulstream-participation/` (spec,
plan, research, data model, contracts, quickstart, tasks — all 22 tasks
closed); commits `f4ac7c4` (spec), `b0deeec` (plan), `d01bd04` (tasks),
`4038f92` (implementation), plus this landing change.

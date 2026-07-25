# Tasks: Soulstream Participation

**Input**: Design documents from `/specs/004-soulstream-participation/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/go-api.md, contracts/repo-gate.md, quickstart.md

**Tests**: included — the spec's success criteria are measured readings and the research method requires the spike to become the permanent suite (research.md D8).

**Organization**: tasks are grouped by user story so each story is independently implementable and testable. All module code lives in the NEW nested module `soulstream/`; the harness core is untouched by every task (FR-001).

## Phase 1: Setup

- [x] T001 Create the nested module skeleton: `soulstream/go.mod` (module `github.com/impire-io/imps/soulstream`, `go 1.26.2`, `require github.com/impire-io/imps v0.0.0` + `replace github.com/impire-io/imps => ../`, `require github.com/impire-io/soulstream v0.4.0`, `require github.com/nats-io/nats.go v1.52.0`) and a placeholder `soulstream/doc.go`; run `go mod tidy` inside `soulstream/` and commit the generated `soulstream/go.sum`
- [x] T002 Verify the v0.4.0 pin against the research spike's ground truth: confirm `git -C /Users/calmera/Impire/soulstream describe --tags` vs `v0.4.0`, and that `realm.ProvisionOn`, `realm.NewClient`, `topic.StartTopic/Open/PostTurn/AddComment`, `topic.OpsSubject`, and `realm.StreamName` exist unchanged at the pinned tag (compile a scratch import against the pin). If the local checkout is ahead in a way the module depends on, STOP and surface it — do not depend on unreleased behavior (research.md D3)
- [x] T003 [P] Extend `Makefile` per `contracts/repo-gate.md`: `tidy`, `build`, `test`, `lint` each also run in `soulstream/` (e.g. `cd soulstream && go test -race -count=1 ./...`); `fmt` and `compile-deny` unchanged
- [x] T004 [P] Extend `.github/workflows/ci.yml` per `contracts/repo-gate.md`: Build and Test steps cover `soulstream/`; add a second golangci-lint invocation with `working-directory: soulstream`; setup-go stays on `go-version-file: go.mod` (GOTOOLCHAIN=auto covers the 1.26.2 floor)

## Phase 2: Foundational (blocking prerequisites for all user stories)

- [x] T005 Implement `soulstream/op.go`: the `Op{Type, Author, ID}` type, the header-only default decoder (`Soulstream-Type`, `Soulstream-Author`, `Nats-Msg-Id` — no payload parse, no library call), the default entity extractor (topic path), and the default channel-name derivation (`"soulstream:"+path`) per `contracts/go-api.md`

**Checkpoint**: the module compiles; every user story can now build on `Op`.

## Phase 3: User Story 1 — Observe a topic as a channel (Priority: P1) 🎯 MVP

**Goal**: declaring a topic in `ImpSpec.Channels` delivers the topic's baseline-first history then live ops through the unmodified dispatch seam.

**Independent Test**: against an embedded realm provisioned by the owner's tooling with pre-existing history, a one-channel imp sees baseline → history → live, in order, with the harness core untouched.

- [x] T006 [US1] Implement `soulstream/channel.go`: `TopicChannel(path, ...TopicChannelOption) imps.ChannelSpec` on the existing `imps.StreamSource` (stream `SOULSTREAM`, filter `SOULSTREAM.TOPICS.OPS.<path>` verbatim, `DeliverAllPolicy` default) with options `WithDurable`, `WithStartSeq`, `WithStartTime`, `WithDecoder`, `WithEntityExtractor`, `WithName` per `contracts/go-api.md`
- [x] T007 [P] [US1] Unit tests in `soulstream/channel_test.go`: spec construction (stream/filter/deliver-all defaults), each option's effect, default decode from headers only, default entity = topic path, name default and override
- [x] T008 [US1] Integration scaffolding + happy path in `soulstream/participation_test.go`: helper starting `natstest` + `realm.ProvisionOn` + a scribe persona posting history via the owner library (the research spike's shape); test that a `TopicChannel` imp observes baseline first, the full history in order, then a live turn on the same continuous consumer, and that an op of an unknown type is delivered like any other (spec US1 scenarios 1, 2, 4; SC-001)
- [x] T009 [US1] Startup-failure test in `soulstream/participation_test.go`: a `TopicChannel` imp against a JetStream server without the `SOULSTREAM` stream fails `Run` with the harness's `ErrStreamNotFound` and leaves no partial subscriptions (spec US1 scenario 3; FR-012)

**Checkpoint**: US1 is a shippable MVP — an observing imp works end-to-end.

## Phase 4: User Story 2 — Note without thinking (Priority: P2)

**Goal**: awareness's existing `Note` verdict with a `Noted` payload becomes an anchored `comment.add` on the topic, with zero thinking.

**Independent Test**: an imp noting other personas' turns produces comments anchored to the right ops, attributed to its persona, visible in the owner library's materialised view, with `ThinksDispatched == 0`.

- [x] T010 [US2] Implement `soulstream/participant.go`: `Participant`, `NewParticipant(ctx, nc, realm, persona, ...ParticipantOption)` (slug validation + JetStream reachability via the owner's `realm.NewClient`; empty persona = read-only), `WithSigner`, `Topic(path) *topic.Handle`, `StartTopic`; the wrapped connection is NEVER closed by the Participant per `contracts/go-api.md`
- [x] T011 [P] [US2] Unit tests in `soulstream/participant_test.go`: invalid realm/persona errors, read-only construction succeeds, the wrapped connection remains open and usable after Participant use
- [x] T012 [US2] Implement `soulstream/notebridge.go`: `Noted{AnchorOp, Body}` and `NoteBridge(p, next, onErr)` — `Noted` → `comment.add` anchored via `p.Topic(entity).AddComment` (best-effort frontier, synchronous); non-`Noted` payloads → `next` (nil drops); empty `AnchorOp`/`Body`, non-topic entity, or publish failure → `onErr` (nil drops), never published, per `contracts/go-api.md`
- [x] T013 [P] [US2] Unit tests in `soulstream/notebridge_test.go`: payload routing (Noted vs other), malformed-Noted cases, nil `next`/`onErr` behavior
- [x] T014 [US2] Integration note round-trip in `soulstream/participation_test.go`: awareness notes other personas' turns; each note lands as a first-class non-dangling comment anchored to the intended op with the imp's persona in the owner's materialised view; the harness counters show `NotesDelivered == N`, `ThinksDispatched == 0`; a non-`Noted` payload reaches the wrapped `next` handler and publishes nothing (spec US2 scenarios 1–3; SC-002)

**Checkpoint**: US1 + US2 = an observing, noting colony member.

## Phase 5: User Story 3 — Contribute from thinking (Priority: P3)

**Goal**: thinking acts as a full participant — turns, comments, topic lifecycle — attributed and optionally signed.

**Independent Test**: a turn posted from a thinking invocation is visible to an independent participant with correct attribution; signing and attribution guards hold.

- [x] T015 [US3] Integration in `soulstream/participation_test.go`: a `Think` verdict's thinking posts a turn via `participant.Topic(entity)`; a second, independent participant materialises the topic and sees the turn attributed to the imp's persona; a read-only (no-persona) participant's write attempt fails with the owner's "persona required" error while its reads keep working (spec US3 scenarios 1, 4)
- [x] T016 [US3] Integration in `soulstream/participation_test.go`: a signer-configured participant's contributions verify as signed by the imp's persona in a reader's view (owner materialised view signature status), and authoring as a different persona is refused by the attribution guard (spec US3 scenarios 2, 3; SC-003)

**Checkpoint**: full participation loop complete.

## Phase 6: User Story 4 — Leave, restart, resume (Priority: P4)

**Goal**: shutdown is leaving (zero substrate residue); durable channels resume exactly.

**Independent Test**: consumer gone after ephemeral shutdown; durable imp receives exactly the ops it missed.

- [x] T017 [US4] Integration in `soulstream/participation_test.go`: (a) ephemeral — after imp shutdown the topic channel's consumer is deleted from the `SOULSTREAM` stream (track its name; owner-library reads create transient ordered consumers, so assert on the specific name); (b) durable — an imp with `WithDurable` stopped, missing ops posted, restarted: awareness receives exactly the missed ops once, in order (spec US4 scenarios 1, 2; SC-005)

## Phase 7: Polish & Cross-Cutting Concerns

- [x] T018 [P] Write `soulstream/doc.go` package documentation per the `contracts/go-api.md` documentation contract: participation model (declaring is joining, stopping is leaving), energy-gradient placement, the static-participation boundary with its episode-0003 reversal condition, and the prohibition on handing a `Participant` to awareness code
- [x] T019 [P] Compile-check the quickstart: keep `specs/004-soulstream-participation/quickstart.md` in sync with the implemented API (adjust the doc if any signature drifted during implementation; the contract file governs — drift beyond the contract must be surfaced, not silently adopted)
- [x] T020 Core byte-identity verification per `contracts/repo-gate.md`: `git diff main -- go.mod go.sum` is empty; the branch's changed-file list stays within `soulstream/`, `Makefile`, `.github/workflows/ci.yml`, `specs/004-soulstream-participation/`, `hq/`, `CLAUDE.md` (SC-004)
- [x] T021 Full gate: `make fmt && make test && make lint` plus `make compile-deny` — green across BOTH modules, zero skipped tests (SC-006, FR-011)
- [ ] T022 Landing duties in the same change (hq/00-GENESIS/how-we-work.md): move M1 to the roadmap ledger with the outcome, write the journey episode via `/journey-log`, propagate any behavioral drift back into `hq/02-DESIGN/0003-soulstream-participation.md`, refresh `hq/04-JOURNEY/README.md` "Where things stand"

## Dependencies & Execution Order

- **Phase 1 → Phase 2 → user stories**: T001 blocks everything; T002 blocks T008+ (any task touching the owner library); T003/T004 can run any time after T001 but must be done before T021.
- **US1 (T006–T009)**: needs T005. Delivers the MVP alone.
- **US2 (T010–T014)**: needs T005; T010 → T012 → T014; independent of US1's tests but T014 reuses T008's scaffolding — implement T008 first or extract the helper.
- **US3 (T015–T016)**: needs T010 (Participant); reuses the integration scaffolding.
- **US4 (T017)**: needs T006; reuses the scaffolding.
- **Polish (T018–T022)**: T018/T019 parallel any time after the APIs settle; T020/T021 last; T022 is the landing act.

**Parallel opportunities**: T003 ∥ T004 (different files); T007 ∥ T008 after T006; T011 ∥ T012 after T010; T013 ∥ T014 after T012; T018 ∥ T019 ∥ (T015–T017) once APIs are stable.

## Implementation Strategy

MVP first: T001–T009 ships an observing imp (US1) alone. Then US2 (the note
bridge — the milestone's heart), US3, US4 as independent increments, each
leaving the suite green. The integration file grows around one shared
scaffold (embedded NATS + `ProvisionOn` + scribe persona) mirroring the
research spike, so every story's test is a variation on measured ground
truth. Land only at T022 with the gate green and the hq duties done in the
same change.

package soulstream

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	imps "github.com/impire-io/imps"
	"github.com/impire-io/imps/testutil/natstest"

	"github.com/impire-io/soulstream/identity"
	"github.com/impire-io/soulstream/realm"
	"github.com/impire-io/soulstream/topic"
)

// scaffold is the shared integration fixture, mirroring the research spike
// (journey episode 0003): embedded NATS with JetStream, a realm provisioned
// by the owner's own tooling, and a scribe persona that started a topic and
// posted two turns of history before any imp exists.
type scaffold struct {
	s            *natstest.Server
	js           jetstream.JetStream
	scribe       *realm.Client
	handle       *topic.Handle
	path         string
	turn1, turn2 string
}

func newScaffold(ctx context.Context, t *testing.T) *scaffold {
	t.Helper()
	s := natstest.New(t)
	js := s.JetStream(t)
	if _, err := realm.ProvisionOn(ctx, js); err != nil {
		t.Fatalf("provision: %v", err)
	}
	nc, err := nats.Connect(s.URL())
	if err != nil {
		t.Fatalf("connect scribe: %v", err)
	}
	t.Cleanup(nc.Close)
	scribe, err := realm.NewClient(ctx, nc, realm.Config{Realm: "testrealm", Persona: "scribe"})
	if err != nil {
		t.Fatalf("scribe client: %v", err)
	}
	h, err := topic.StartTopic(ctx, scribe, topic.StartTopicInput{
		Name:  "Participation Suite",
		State: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("start topic: %v", err)
	}
	sc := &scaffold{s: s, js: js, scribe: scribe, handle: h, path: h.Path()}
	if sc.turn1, err = h.PostTurn(ctx, "first turn"); err != nil {
		t.Fatalf("turn1: %v", err)
	}
	if sc.turn2, err = h.PostTurn(ctx, "second turn"); err != nil {
		t.Fatalf("turn2: %v", err)
	}
	return sc
}

// connect opens a dedicated connection for an imp under test.
func (sc *scaffold) connect(t *testing.T) *nats.Conn {
	t.Helper()
	nc, err := nats.Connect(sc.s.URL())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(nc.Close)
	return nc
}

// runImp starts the imp and waits for readiness; cleanup shuts it down.
func runImp(ctx context.Context, t *testing.T, imp *imps.Imp) chan error {
	t.Helper()
	runErr := make(chan error, 1)
	go func() { runErr <- imp.Run(ctx) }()
	waitFor(t, "imp ready", func() bool { return imp.Ready() })
	return runErr
}

func shutdownImp(t *testing.T, imp *imps.Imp, runErr chan error) {
	t.Helper()
	if err := imp.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

func consumerNames(ctx context.Context, t *testing.T, js jetstream.JetStream) []string {
	t.Helper()
	st, err := js.Stream(ctx, realm.StreamName)
	if err != nil {
		t.Fatalf("stream lookup: %v", err)
	}
	var names []string
	for name := range st.ConsumerNames(ctx).Name() {
		names = append(names, name)
	}
	return names
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// recorder collects ops observed by awareness, race-safely.
type recorder struct {
	mu   sync.Mutex
	seen []Op
}

func (r *recorder) add(o Op) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, o)
}

func (r *recorder) snapshot() []Op {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Op(nil), r.seen...)
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.seen)
}

// TestObserve_HistoryThenLive covers spec US1 scenarios 1, 2, 4 (SC-001):
// baseline-first history, live continuation on the same consumer, and
// unknown op types delivered like any other.
func TestObserve_HistoryThenLive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sc := newScaffold(ctx, t)

	rec := &recorder{}
	spec := imps.ImpSpec{
		Name:     "observer",
		Version:  "0.0.1",
		Channels: []imps.ChannelSpec{TopicChannel(sc.path)},
		Awareness: func(_ context.Context, decoded any, entity imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			if entity != imps.Entity(sc.path) {
				t.Errorf("entity = %q, want topic path %q", entity, sc.path)
			}
			rec.add(decoded.(Op))
			return imps.Ignore()
		},
		Thinking: func(context.Context, any, imps.Entity, imps.ThinkingContext) error { return nil },
	}
	imp, err := imps.NewImp(spec, sc.connect(t))
	if err != nil {
		t.Fatalf("NewImp: %v", err)
	}
	runErr := runImp(ctx, t, imp)
	defer shutdownImp(t, imp, runErr)

	// History: baseline first, then the two turns, in stream order.
	waitFor(t, "history replay", func() bool { return rec.count() >= 3 })
	got := rec.snapshot()
	if got[0].Type != topic.TypeBaseline {
		t.Errorf("first op = %q, want baseline first", got[0].Type)
	}
	if got[1].ID != sc.turn1 || got[2].ID != sc.turn2 {
		t.Errorf("history order = %+v, want turn1 then turn2", got[1:3])
	}

	// Live: a new turn and an unknown-type op arrive on the same consumer.
	turn3, err := sc.handle.PostTurn(ctx, "third turn, live")
	if err != nil {
		t.Fatalf("turn3: %v", err)
	}
	unknownID, err := sc.handle.Post(ctx, "suite.unknown-kind", map[string]string{"future": "vocab"})
	if err != nil {
		t.Fatalf("unknown op: %v", err)
	}
	waitFor(t, "live ops", func() bool { return rec.count() >= 5 })
	got = rec.snapshot()
	if got[3].ID != turn3 {
		t.Errorf("live turn: got %+v", got[3])
	}
	if got[4].ID != unknownID || got[4].Type != "suite.unknown-kind" {
		t.Errorf("unknown-type op must be delivered verbatim: %+v", got[4])
	}

	m := imp.Metrics()
	if m.DecodeFailures != 0 || m.ExtractionFailures != 0 || m.AwarenessPanics != 0 {
		t.Errorf("unexpected failures: %+v", m)
	}
}

// TestObserve_UnprovisionedRealm covers spec US1 scenario 3 (FR-012): a
// missing op-log stream fails startup with the harness's named error.
func TestObserve_UnprovisionedRealm(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s := natstest.New(t)
	s.JetStream(t) // JetStream on, but no ProvisionOn: no SOULSTREAM stream
	nc, err := nats.Connect(s.URL())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()

	spec := imps.ImpSpec{
		Name:     "orphan",
		Version:  "0.0.1",
		Channels: []imps.ChannelSpec{TopicChannel("nowhere-topic")},
		Awareness: func(context.Context, any, imps.Entity, imps.AwarenessContext) imps.Verdict {
			return imps.Ignore()
		},
		Thinking: func(context.Context, any, imps.Entity, imps.ThinkingContext) error { return nil },
	}
	imp, err := imps.NewImp(spec, nc)
	if err != nil {
		t.Fatalf("NewImp: %v", err)
	}
	runResult := make(chan error, 1)
	go func() { runResult <- imp.Run(ctx) }()
	select {
	case err := <-runResult:
		var notFound *imps.ErrStreamNotFound
		if !errors.As(err, &notFound) {
			t.Fatalf("err = %v, want *imps.ErrStreamNotFound", err)
		}
		if imp.Ready() {
			t.Fatal("imp reports ready after failed startup")
		}
	case <-ctx.Done():
		t.Fatal("Run did not return on startup failure")
	}
}

// TestNoteBridge_RoundTrip covers spec US2 (SC-002): Note verdicts with a
// Noted payload become anchored comments with zero thinking; other payloads
// fall through to the wrapped handler and publish nothing.
func TestNoteBridge_RoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sc := newScaffold(ctx, t)

	ncImp := sc.connect(t)
	participant, err := NewParticipant(ctx, ncImp, "testrealm", "imp")
	if err != nil {
		t.Fatalf("participant: %v", err)
	}

	var nextMu sync.Mutex
	var nextGot []any
	rec := &recorder{}
	spec := imps.ImpSpec{
		Name:     "noter",
		Version:  "0.0.1",
		Channels: []imps.ChannelSpec{TopicChannel(sc.path)},
		Awareness: func(_ context.Context, decoded any, _ imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			op := decoded.(Op)
			rec.add(op)
			switch {
			case op.Type == topic.TypeTurnPost && op.Author != "imp":
				return imps.Note(Noted{AnchorOp: op.ID, Body: "noted"})
			case op.Type == topic.TypeBaseline:
				// A non-bridge payload must reach the wrapped handler and
				// publish nothing.
				return imps.Note("local only")
			default:
				return imps.Ignore()
			}
		},
		Thinking: func(context.Context, any, imps.Entity, imps.ThinkingContext) error {
			t.Error("thinking must not run in the note flow")
			return nil
		},
		OnNote: NoteBridge(participant,
			func(_ imps.Entity, payload any) {
				nextMu.Lock()
				nextGot = append(nextGot, payload)
				nextMu.Unlock()
			},
			func(_ imps.Entity, n Noted, err error) {
				t.Errorf("bridge error for %+v: %v", n, err)
			},
		),
	}
	imp, err := imps.NewImp(spec, ncImp)
	if err != nil {
		t.Fatalf("NewImp: %v", err)
	}
	runErr := runImp(ctx, t, imp)
	defer shutdownImp(t, imp, runErr)

	turn3, err := sc.handle.PostTurn(ctx, "third turn, live")
	if err != nil {
		t.Fatalf("turn3: %v", err)
	}

	// 1 baseline + 3 scribe turns + 3 imp comments = 7 ops round-trip
	// through the imp's own channel.
	waitFor(t, "note round-trip", func() bool { return rec.count() >= 7 })

	// Owner's-eye view: the comments are first-class, anchored, attributed.
	view, err := topic.Open(sc.scribe, sc.path).Materialise(ctx)
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}
	if view.Malformed != "" {
		t.Fatalf("owner view malformed: %s", view.Malformed)
	}
	wantAnchors := map[string]bool{sc.turn1: false, sc.turn2: false, turn3: false}
	for _, c := range view.Contributions {
		if c.Type != topic.TypeCommentAdd || c.Author != "imp" {
			continue
		}
		if c.Dangling {
			t.Errorf("comment %s is dangling", c.OpID)
		}
		if _, ok := wantAnchors[c.Anchor]; !ok {
			t.Errorf("comment anchored to unexpected op %q", c.Anchor)
			continue
		}
		wantAnchors[c.Anchor] = true
	}
	for anchor, found := range wantAnchors {
		if !found {
			t.Errorf("no imp comment anchored to %s", anchor)
		}
	}

	// The non-bridge payload reached the wrapped handler; nothing extra was
	// published (exactly 3 comments counted above), and no thinking ran.
	nextMu.Lock()
	if len(nextGot) != 1 || nextGot[0] != "local only" {
		t.Errorf("next handler got %v, want [\"local only\"]", nextGot)
	}
	nextMu.Unlock()
	m := imp.Metrics()
	if m.ThinksDispatched != 0 {
		t.Errorf("ThinksDispatched = %d, want 0", m.ThinksDispatched)
	}
	if m.NotesDelivered != 4 { // 3 Noted + 1 local
		t.Errorf("NotesDelivered = %d, want 4", m.NotesDelivered)
	}
}

// TestThinking_Contributions covers spec US3 scenarios 1 and 4 (SC-003):
// thinking posts as a full participant; a read-only participant can read
// but not write.
func TestThinking_Contributions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sc := newScaffold(ctx, t)

	ncImp := sc.connect(t)
	participant, err := NewParticipant(ctx, ncImp, "testrealm", "imp")
	if err != nil {
		t.Fatalf("participant: %v", err)
	}

	spec := imps.ImpSpec{
		Name:     "contributor",
		Version:  "0.0.1",
		Channels: []imps.ChannelSpec{TopicChannel(sc.path)},
		Awareness: func(_ context.Context, decoded any, entity imps.Entity, _ imps.AwarenessContext) imps.Verdict {
			op := decoded.(Op)
			if op.Type == topic.TypeTurnPost && op.Author == "scribe" {
				return imps.Think(op, entity)
			}
			return imps.Ignore()
		},
		Thinking: func(ctx context.Context, reason any, entity imps.Entity, _ imps.ThinkingContext) error {
			op := reason.(Op)
			_, err := participant.Topic(string(entity)).PostTurn(ctx, "ack "+op.ID)
			return err
		},
	}
	imp, err := imps.NewImp(spec, ncImp)
	if err != nil {
		t.Fatalf("NewImp: %v", err)
	}
	runErr := runImp(ctx, t, imp)
	defer shutdownImp(t, imp, runErr)

	// Two scribe turns in history → two thinking runs → two imp turns,
	// visible to an independent participant with correct attribution.
	waitFor(t, "imp turns visible to the scribe", func() bool {
		view, err := topic.Open(sc.scribe, sc.path).Materialise(ctx)
		if err != nil || view.Malformed != "" {
			return false
		}
		var impTurns int
		for _, c := range view.Contributions {
			if c.Type == topic.TypeTurnPost && c.Author == "imp" {
				impTurns++
			}
		}
		return impTurns == 2
	})

	// A read-only participant reads fine and is refused on write.
	readOnly, err := NewParticipant(ctx, sc.connect(t), "testrealm", "")
	if err != nil {
		t.Fatalf("read-only participant: %v", err)
	}
	if _, err := readOnly.Topic(sc.path).Materialise(ctx); err != nil {
		t.Errorf("read-only read failed: %v", err)
	}
	if _, err := readOnly.Topic(sc.path).PostTurn(ctx, "should fail"); err == nil || !strings.Contains(err.Error(), "persona") {
		t.Errorf("read-only write: err = %v, want persona-required error", err)
	}
}

// TestSigning_AndAttributionGuard covers spec US3 scenarios 2 and 3
// (SC-003): signed contributions verify as the imp's persona; cross-persona
// authorship is refused.
func TestSigning_AndAttributionGuard(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sc := newScaffold(ctx, t)

	key, err := identity.GenerateSigningKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := NewParticipant(ctx, sc.connect(t), "testrealm", "signer-imp", WithSigner(key))
	if err != nil {
		t.Fatalf("signer participant: %v", err)
	}
	signedTurn, err := signer.Topic(sc.path).PostTurn(ctx, "signed contribution")
	if err != nil {
		t.Fatalf("signed post: %v", err)
	}

	// A reader who knows the persona's public key sees the op as verified;
	// the scribe's own unsigned history stays unsigned (not failed).
	reader := topic.Open(sc.scribe, sc.path)
	reader.UseKeyring(&identity.Keyring{Keys: map[string][]string{
		"signer-imp": {key.PublicKey()},
	}})
	view, err := reader.Materialise(ctx)
	if err != nil {
		t.Fatalf("materialise: %v", err)
	}
	var checkedSigned, checkedUnsigned bool
	for _, c := range view.Contributions {
		switch c.OpID {
		case signedTurn:
			checkedSigned = true
			if c.Sig != topic.SigVerified {
				t.Errorf("signed turn Sig = %q, want %q", c.Sig, topic.SigVerified)
			}
		case sc.turn1:
			checkedUnsigned = true
			if c.Sig != topic.SigUnsigned {
				t.Errorf("scribe turn Sig = %q, want %q", c.Sig, topic.SigUnsigned)
			}
		}
	}
	if !checkedSigned || !checkedUnsigned {
		t.Fatalf("expected both the signed turn and an unsigned scribe turn in the view")
	}

	// The attribution guard (owner library, surfaced unchanged): a
	// persona-bound client refuses to author as anyone else.
	if err := sc.scribe.EnforceAuthor("someone-else"); err == nil {
		t.Fatal("cross-persona authorship must be refused")
	}
}

// TestLeave_And_DurableResume covers spec US4 (SC-005): ephemeral shutdown
// leaves zero substrate footprint; a durable channel resumes exactly.
func TestLeave_And_DurableResume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	sc := newScaffold(ctx, t)

	newObserver := func(rec *recorder, opts ...TopicChannelOption) *imps.Imp {
		spec := imps.ImpSpec{
			Name:     "leaver",
			Version:  "0.0.1",
			Channels: []imps.ChannelSpec{TopicChannel(sc.path, opts...)},
			Awareness: func(_ context.Context, decoded any, _ imps.Entity, _ imps.AwarenessContext) imps.Verdict {
				rec.add(decoded.(Op))
				return imps.Ignore()
			},
			Thinking: func(context.Context, any, imps.Entity, imps.ThinkingContext) error { return nil },
		}
		imp, err := imps.NewImp(spec, sc.connect(t))
		if err != nil {
			t.Fatalf("NewImp: %v", err)
		}
		return imp
	}

	// (a) Ephemeral: the imp's consumer is the only one on the stream at
	// start (owner-library reads create transient ordered consumers later),
	// and it is deleted on shutdown — leaving needs nothing else.
	rec := &recorder{}
	imp := newObserver(rec)
	runErr := runImp(ctx, t, imp)
	waitFor(t, "one consumer", func() bool { return len(consumerNames(ctx, t, sc.js)) == 1 })
	ephemeralName := consumerNames(ctx, t, sc.js)[0]
	waitFor(t, "history", func() bool { return rec.count() >= 3 })
	shutdownImp(t, imp, runErr)
	waitFor(t, "ephemeral consumer deleted on leave", func() bool {
		for _, n := range consumerNames(ctx, t, sc.js) {
			if n == ephemeralName {
				return false
			}
		}
		return true
	})

	// (b) Durable: run, see history, stop; miss two ops; resume and receive
	// exactly the missed ops, once, in order.
	recA := &recorder{}
	impA := newObserver(recA, WithDurable("resume-watcher"))
	runErrA := runImp(ctx, t, impA)
	waitFor(t, "durable history", func() bool { return recA.count() >= 3 })
	shutdownImp(t, impA, runErrA)

	turn3, err := sc.handle.PostTurn(ctx, "missed one")
	if err != nil {
		t.Fatalf("turn3: %v", err)
	}
	turn4, err := sc.handle.PostTurn(ctx, "missed two")
	if err != nil {
		t.Fatalf("turn4: %v", err)
	}

	recB := &recorder{}
	impB := newObserver(recB, WithDurable("resume-watcher"))
	runErrB := runImp(ctx, t, impB)
	defer shutdownImp(t, impB, runErrB)
	waitFor(t, "resume delivery", func() bool { return recB.count() >= 2 })
	time.Sleep(300 * time.Millisecond) // settle: catch duplicates/replays
	got := recB.snapshot()
	if len(got) != 2 || got[0].ID != turn3 || got[1].ID != turn4 {
		t.Fatalf("resumed ops = %+v, want exactly [%s %s]", got, turn3, turn4)
	}
}

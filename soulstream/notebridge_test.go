package soulstream

import (
	"testing"

	imps "github.com/impire-io/imps"
)

func TestNoteBridge_NilParticipantPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on nil Participant")
		}
	}()
	NoteBridge(nil, nil, nil)
}

func TestNoteBridge_RoutesNonNotedToNext(t *testing.T) {
	var got any
	var gotEntity imps.Entity
	bridge := NoteBridge(&Participant{},
		func(e imps.Entity, p any) { gotEntity, got = e, p },
		func(imps.Entity, Noted, error) { t.Error("onErr must not fire for non-Noted payloads") },
	)
	bridge("some-topic", "just a local note")
	if got != "just a local note" || gotEntity != "some-topic" {
		t.Errorf("next got (%q, %v)", gotEntity, got)
	}
}

func TestNoteBridge_NilNextDropsNonNoted(t *testing.T) {
	bridge := NoteBridge(&Participant{}, nil, nil)
	bridge("some-topic", 42) // must not panic, must not publish
}

func TestNoteBridge_MalformedNotedGoesToOnErr(t *testing.T) {
	// The zero-client Participant would panic if the bridge ever reached a
	// publish — reaching onErr instead proves nothing is published.
	cases := []struct {
		name   string
		entity imps.Entity
		noted  Noted
	}{
		{"empty anchor", "some-topic", Noted{Body: "b"}},
		{"empty body", "some-topic", Noted{AnchorOp: "op-1"}},
		{"empty entity", "", Noted{AnchorOp: "op-1", Body: "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotErr error
			bridge := NoteBridge(&Participant{},
				func(imps.Entity, any) { t.Error("next must not fire for Noted payloads") },
				func(_ imps.Entity, _ Noted, err error) { gotErr = err },
			)
			bridge(tc.entity, tc.noted)
			if gotErr == nil {
				t.Fatal("expected an error through onErr")
			}
		})
	}
}

func TestNoteBridge_NilOnErrDropsMalformed(t *testing.T) {
	bridge := NoteBridge(&Participant{}, nil, nil)
	bridge("some-topic", Noted{}) // must not panic, must not publish
}

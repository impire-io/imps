package harness

import (
	"errors"
	"testing"
)

func TestResolver_NonPlatformMode(t *testing.T) {
	r, err := newResolver("tenantA.imps.demo", false, "")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got, want := r.resolve("messages.in"), "tenantA.imps.demo.messages.in"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if r.resolvedPrefix() != "tenantA.imps.demo" {
		t.Fatalf("prefix mismatch: got %q", r.resolvedPrefix())
	}
}

func TestResolver_PlatformMode(t *testing.T) {
	r, err := newResolver("platform", true, "ABCD1234")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got, want := r.resolve("actions.out"), "platform.ABCD1234.actions.out"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if r.resolvedPrefix() != "platform.ABCD1234" {
		t.Fatalf("prefix mismatch: got %q", r.resolvedPrefix())
	}
}

func TestResolver_MissingPrefix(t *testing.T) {
	_, err := newResolver("", false, "")
	var cfg *ErrConfigInvalid
	if !errors.As(err, &cfg) || cfg.Field != "prefix" {
		t.Fatalf("expected ErrConfigInvalid{Field:prefix}, got %v", err)
	}
}

func TestResolver_PlatformMissingImporterPK(t *testing.T) {
	_, err := newResolver("platform", true, "")
	var cfg *ErrConfigInvalid
	if !errors.As(err, &cfg) || cfg.Field != "importer_account_pk" {
		t.Fatalf("expected ErrConfigInvalid{Field:importer_account_pk}, got %v", err)
	}
}

func TestResolver_WildcardPassthrough(t *testing.T) {
	r, _ := newResolver("tenantA", false, "")
	if got := r.resolve("events.*.created"); got != "tenantA.events.*.created" {
		t.Fatalf("wildcard mangled: %q", got)
	}
	if got := r.resolve("events.>"); got != "tenantA.events.>" {
		t.Fatalf("> mangled: %q", got)
	}
	rp, _ := newResolver("platform", true, "PK")
	if got := rp.resolve("events.*.created"); got != "platform.PK.events.*.created" {
		t.Fatalf("platform wildcard mangled: %q", got)
	}
}

//go:build awareness_conn_must_fail
// +build awareness_conn_must_fail

// This file is intentionally NOT included in normal builds. The build tag
// `awareness_conn_must_fail` activates it; building under that tag MUST
// fail because imps.AwarenessContext does not expose a Conn method
// (the raw-*nats.Conn escape hatch is reasoning-only — SC-104, FR-103b).
//
// To run the assertion:
//
//	go vet -tags=awareness_conn_must_fail ./integration/compiletest/...
//
// A successful (non-error) build under that tag is the failure.
package compiletest

import (
	"github.com/impire-io/imps"
)

func mustNotCompileConn(a imps.AwarenessContext) {
	// AwarenessContext has no Conn method. This line MUST fail to compile.
	_ = a.Conn()
}

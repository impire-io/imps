//go:build awareness_requestmany_must_fail
// +build awareness_requestmany_must_fail

// This file is intentionally NOT included in normal builds. The build tag
// `awareness_requestmany_must_fail` activates it; building under that tag
// MUST fail because imps.AwarenessContext does not expose a RequestMany
// method (the structural enforcement of the energy gradient — SC-104).
//
// To run the assertion:
//
//	go vet -tags=awareness_requestmany_must_fail ./integration/compiletest/...
//
// A successful (non-error) build under that tag is the failure.
package compiletest

import (
	"context"

	"github.com/impire-io/imps"
)

func mustNotCompileRequestMany(a imps.AwarenessContext) {
	// AwarenessContext has no RequestMany method. This line MUST fail to
	// compile.
	_, _ = a.RequestMany(context.Background(), "anything", nil)
}

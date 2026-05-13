//go:build awareness_publish_must_fail
// +build awareness_publish_must_fail

// This file is intentionally NOT included in normal builds. The build tag
// `awareness_publish_must_fail` activates it; building under that tag MUST
// fail because imps.AwarenessContext does not expose a Publish method
// (the structural enforcement of the energy gradient — SC-006).
//
// To run the assertion:
//
//	go vet -tags=awareness_publish_must_fail ./integration/compiletest/...
//
// A successful (non-error) build under that tag is the failure.
package compiletest

import (
	"context"

	"github.com/impire-io/imps"
)

func mustNotCompile(a imps.AwarenessContext) {
	// AwarenessContext has no Publish method. This line MUST fail to
	// compile.
	_ = a.Publish(context.Background(), "anything", nil)
}

# Compile-time assertion: awareness has no `Publish`

This package contains a single build-tagged file
(`awareness_no_publish.go`) whose presence-of-build-failure is the
assertion that satisfies SC-006.

## How to verify

```sh
# Expected: a compile error — that is the test passing.
go vet -tags=awareness_publish_must_fail ./integration/compiletest/...
```

If the command succeeds with no error, the energy-gradient guarantee
documented in `specs/001-harness-core/contracts/public-api.md`
("Compile-time guarantees" #1) has been broken — `AwarenessContext` has
gained a `Publish` method that lets awareness call into the substrate.

## Why a build tag

The default build excludes the file (the build tag is opaque to normal
`go build` / `go test`). Only the verification command above activates the
file, which is the right behavior — the rest of the test suite stays
green because the assertion is "this *should not* compile".

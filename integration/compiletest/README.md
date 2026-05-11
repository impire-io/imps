# Compile-time assertions: the awareness energy gradient

This package contains build-tagged files whose **build failure** is the
assertion. Each file targets one method that exists on
`ReasoningContext` but is intentionally absent on `AwarenessContext` —
the compile error proves the structural energy gradient holds (SC-006,
SC-104, FR-103b).

| File | Build tag | Forbidden method |
|---|---|---|
| `awareness_no_publish.go` | `awareness_publish_must_fail` | `AwarenessContext.Publish` |
| `awareness_no_requestmany.go` | `awareness_requestmany_must_fail` | `AwarenessContext.RequestMany` |
| `awareness_no_conn.go` | `awareness_conn_must_fail` | `AwarenessContext.Conn` |

## How to verify

```sh
# Each command MUST produce a compile error — that is the test passing.
go vet -tags=awareness_publish_must_fail     ./integration/compiletest/...
go vet -tags=awareness_requestmany_must_fail ./integration/compiletest/...
go vet -tags=awareness_conn_must_fail        ./integration/compiletest/...
```

A successful (non-error) build under any of those tags means
`AwarenessContext` has grown the corresponding method — a regression
against `specs/002-capability-client/contracts/request-reply.md`
"Compile-time guarantees" (or, for `Publish`,
`specs/001-harness-core/contracts/public-api.md`).

The Makefile target `compile-deny` wraps the three commands and asserts
non-zero exit for each.

## Why build tags

The default build excludes these files (the build tags are opaque to
normal `go build` / `go test`). Only the verification commands above
activate the files, which is the right behavior — the rest of the test
suite stays green because the assertion is "this *should not* compile".

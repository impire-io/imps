# Contract: Repository Gate Coverage for the Nested Module

The gate promise — `make fmt && make test && make lint` plus
`make compile-deny`, all green, none skipped — extends to the nested module in
the same invocations. No second command sequence, no separate CI workflow.

## Makefile

| Target | Contract after this feature |
|---|---|
| `fmt` | Formats the whole tree (already recursive: `gofmt -s -w .`, `goimports -w .`) — covers `soulstream/` with no edit. |
| `tidy` | Runs `go mod tidy` in the core **and** in `soulstream/`. |
| `build` | Builds core packages **and** `soulstream/...`. |
| `test` | Depends on `compile-deny` (unchanged), runs `go test -race -count=1 ./...` in the core **and** in `soulstream/`. No test skipped in either module. |
| `lint` | Runs `golangci-lint run ./...` in the core **and** in `soulstream/`. |
| `compile-deny` | Unchanged — the energy-gradient build-tag assertions are a core-module invariant. It MUST remain green with the nested module present. |

## CI (`.github/workflows/ci.yml`)

| Step | Contract after this feature |
|---|---|
| setup-go | Unchanged (`go-version-file: go.mod`); the nested module's `go 1.26.2` floor is satisfied by Go's default `GOTOOLCHAIN=auto` switching. |
| gofmt check | Unchanged — already covers the whole tree. |
| Build | Covers both modules. |
| Compile-deny | Unchanged. |
| Test | Covers both modules (`-race`), including the hq structural lint. |
| Lint | golangci-lint runs in both modules (second invocation or `working-directory` step for `soulstream/`). |

## Core byte-identity (FR-001 / SC-004)

- The core module's `go.mod` and `go.sum` are **not modified** by this
  feature. Verification: `git diff --stat main -- go.mod go.sum` is empty on
  the feature branch at landing.
- No file outside `soulstream/`, `Makefile`, `.github/workflows/ci.yml`,
  `specs/004-soulstream-participation/`, `hq/`, and `CLAUDE.md` (speckit
  pointer) is touched. Verification: the landing diff's file list.

## Dependency pin

- `soulstream/go.mod` requires `github.com/impire-io/soulstream v0.4.0`
  (public tag) and `replace`s `github.com/impire-io/imps => ../`.
- CI MUST fetch the pinned tag from the public proxy — no repo-local sibling
  checkout, no vendoring, no auth requirement.

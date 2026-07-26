# Contract: Gate Coverage and Byte-Identity for `persist`

## Gate coverage — no wiring needed

`persist` is a package in the core module, so the existing gate covers it
with **zero** Makefile or CI changes:

| Invocation | Covers persist because |
|---|---|
| `make fmt` | recursive over the tree |
| `make test` (`go test -race -count=1 ./...`) | `./...` includes `./persist` |
| `make lint` (`golangci-lint run ./...`) | same |
| `make compile-deny` | unchanged — core invariant, must stay green |
| CI Build / Test / Lint | same invocations |

Any diff to `Makefile` or `.github/workflows/ci.yml` in this feature is a
contract violation.

## Byte-identity (FR-001 / SC-005)

- The **root package** (every `*.go` at the repository root) is not
  modified.
- `go.mod` and `go.sum` are byte-identical to `main` — the feature adds no
  dependencies (`jetstream` and the embedded test server are already
  required).
- Verification at landing: `git diff main -- go.mod go.sum '*.go'`
  restricted to the repo root is empty; the branch's changed-file list
  stays within `persist/`, `specs/005-sleep-wake-persistence/`, `hq/`, and
  `CLAUDE.md` (speckit pointer).

## Test-shape contract

- No test skips. The restart integration test runs a real imp against the
  embedded server (the research spike productized).
- Unit determinism: elapsed-time assertions in unit tests use the injected
  internal clock, not sleeps; the restart test uses the real clock with
  wall-clock bounds.
- A failing-backend stub exercises SC-006's error-never-silent-zero on both
  `Get` and `Update`.

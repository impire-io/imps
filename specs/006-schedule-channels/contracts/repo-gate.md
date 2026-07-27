# Contract: Gate Coverage and Byte-Identity for `schedule`

## Gate coverage — no wiring needed

`schedule` is a core-module package: `make fmt/test/lint` and CI cover it
with **zero** Makefile or workflow changes (the `persist` precedent). Any
diff to `Makefile` or `.github/workflows/ci.yml` in this feature is a
contract violation.

## Byte-identity (FR-001 / SC-004)

- No root-package `*.go` modified; `go.mod`/`go.sum` byte-identical to
  `main`.
- The branch's changed-file list stays within `schedule/`,
  `specs/006-schedule-channels/`, `hq/`, and `CLAUDE.md` (speckit pointer).
- Verification at landing: `git diff main -- go.mod go.sum` empty; changed
  files enumerated.

## Test-shape contract

- No skips. Integration runs against the embedded server with a stream
  configured `AllowMsgSchedules` + `AllowMsgTTL` (the operator-provisioning
  stand-in).
- The warm/cold/TTL test is the research spike productized with exact-count
  comparisons (no-TTL backlog strictly greater than TTL-governed tail).
- Header round-trips read the stored schedule back
  (`GetLastMsgForSubject`) and assert every option→header mapping,
  replacement on re-register, and purge on deregister.
- Timing-sensitive assertions use generous bounds and fast-direction
  switches only (never wait out a slow cadence).

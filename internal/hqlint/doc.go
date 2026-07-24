// Package hqlint holds the structural lint for the hq/ headquarters layout.
//
// It is a test-only package: the checks live in hqlint_test.go and ride the
// standard quality gate (make test / go test ./...), locally and in CI. They
// enforce the invariants promised in hq/00-GENESIS/how-we-work.md — the five
// areas exist with their READMEs, research topics carry legal non-terminal
// states, journey episodes are contiguously numbered / indexed / carry a
// reversal condition, the spec-kit constitution symlink resolves into GENESIS,
// and relative markdown links inside hq/ resolve.
//
// This file exists only so the directory is a buildable Go package under
// `go build ./...`; it has no runtime behavior.
package hqlint

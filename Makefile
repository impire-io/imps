.PHONY: fmt tidy test test-race lint build check compile-deny

GOFLAGS ?=

# Build tags whose presence MUST fail to compile. Each one asserts that a
# specific method (Publish / RequestMany / Conn) is absent from
# AwarenessContext — the structural enforcement of the energy gradient.
COMPILE_DENY_TAGS := \
	awareness_publish_must_fail \
	awareness_requestmany_must_fail \
	awareness_conn_must_fail

fmt:
	gofmt -s -w .
	@if command -v goimports >/dev/null 2>&1; then goimports -w .; fi

tidy:
	go mod tidy

test: compile-deny
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

build:
	go build ./...

# compile-deny asserts that each build tag under ./integration/compiletest/
# produces a compile error. A successful (non-error) build is a regression.
compile-deny:
	@set -e; for tag in $(COMPILE_DENY_TAGS); do \
		echo "compile-deny: $$tag"; \
		if go vet -tags=$$tag ./integration/compiletest/... >/dev/null 2>&1; then \
			echo "compile-deny FAIL: build succeeded under tag $$tag"; \
			exit 1; \
		fi; \
	done

check: fmt tidy test lint

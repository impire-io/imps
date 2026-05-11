.PHONY: fmt tidy test test-race lint build check

GOFLAGS ?=

fmt:
	gofmt -s -w .
	@if command -v goimports >/dev/null 2>&1; then goimports -w .; fi

tidy:
	go mod tidy

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

build:
	go build ./...

check: fmt tidy test lint

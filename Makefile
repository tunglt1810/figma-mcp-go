.PHONY: test test-go test-ts coverage coverage-go coverage-go-html coverage-ts build build-go build-ts

build: build-go build-ts

build-go:
	go build -o bin/figma-mcp-go ./cmd/figma-mcp-go

build-ts:
	cd plugin && pnpm run build

test: test-go test-ts

test-go:
	go test ./...

test-ts:
	cd plugin && pnpm test

coverage: coverage-go coverage-ts

coverage-go:
	go test -coverprofile=bin/coverage.out ./... && go tool cover -func=bin/coverage.out

coverage-ts:
	cd plugin && pnpm test:coverage

.PHONY: fmt fmt-check vet test test-go test-ts coverage coverage-go coverage-go-html coverage-ts build build-go build-ts

fmt:
	gofmt -w .

fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "These files are not gofmt-formatted; run 'make fmt':"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	go vet ./...

build: build-go build-ts

build-go:
	go build -o bin/figma-mcp-go ./cmd/figma-mcp-go

build-ts:
	cd plugin && bun run build

test: test-go test-ts

test-go:
	go test ./...

test-ts:
	cd plugin && bun test

coverage: coverage-go coverage-ts

coverage-go:
	go test -coverprofile=bin/coverage.out ./... && go tool cover -func=bin/coverage.out

coverage-ts:
	cd plugin && bun test --coverage

.PHONY: fmt fmt-check vet deps-check test test-go test-ts coverage coverage-go coverage-go-html coverage-ts build build-go build-ts

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

# The compiler rejects import cycles, not import directions. Without this the
# layering is back to being a convention.
#
# Every edge is checked twice, once over the production import graph and once
# with -test, because `go list -deps` alone cannot see a test-only import and a
# test is a plausible way for the coupling to arrive. Two edges are production
# only: internal/tools/leader_rpc_test.go drives the leader's /rpc with the real
# Check and a real cluster.Leader, which is the only way to pin "every call is
# checked exactly once before reaching the plugin" at that entry point from
# outside it. Those two are the sole test-side crossings in the tree.
#
# The self-test up front is not decoration. The body reports a violation when a
# grep succeeds, so anything that makes every grep fail — a wrong module path, a
# renamed package, a `go list` that errors — would otherwise print "layering
# holds" while checking nothing.
deps-check:
	@module=github.com/tunglt1810/figma-mcp-go; \
	if ! go list -deps ./internal/cluster | grep -Fqx "$$module/internal/bridge"; then \
	  echo "deps-check: broken — it cannot see the known cluster -> bridge edge"; \
	  exit 1; \
	fi; \
	fail=0; \
	check() { \
	  deps=$$(go list -deps $$3 ./internal/$$1) || exit 1; \
	  if printf '%s\n' "$$deps" | grep -Fqx "$$module/internal/$$2"; then \
	    echo "forbidden import: internal/$$1 -> internal/$$2$$4"; \
	    fail=1; \
	  fi; \
	}; \
	prod() { check "$$1" "$$2" "" ""; }; \
	both() { prod "$$1" "$$2"; check "$$1" "$$2" -test " (through a test import)"; }; \
	prod tools cluster; prod tools bridge; \
	both cluster tools; both cluster figma; \
	both bridge cluster; both bridge tools; both bridge figma; \
	both figma bridge; both figma cluster; both figma tools; \
	if [ $$fail -eq 0 ]; then echo "deps-check: layering holds"; fi; \
	exit $$fail

build: build-go build-ts

build-go:
	go build -o bin/figma-mcp-go ./cmd/figma-mcp-go

build-ts:
	cd plugin && bun run build

test: deps-check test-go test-ts

test-go:
	go test ./...

test-ts:
	cd plugin && bun test

coverage: coverage-go coverage-ts

coverage-go:
	go test -coverprofile=bin/coverage.out ./... && go tool cover -func=bin/coverage.out

coverage-ts:
	cd plugin && bun test --coverage

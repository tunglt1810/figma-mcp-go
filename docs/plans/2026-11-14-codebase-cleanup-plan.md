# Codebase cleanup plan

**Date:** 2026-11-14
**Source:** codebase audit of 2026-11-14 (no spec; each phase that changes behavior writes its decision to `docs/specs` in the last step).
**Order:** lowest impact first. Each phase is one pull request. A later phase does not start before the earlier one merges.

Baseline before phase 1: `make test && make build` passes on `main`.

## Phase 1 — Delete stale documents (no code)

1. Delete `docs/diagrams/` (four generated files). The README mermaid diagrams stay.
2. Delete `docs/PULL_REQUEST.md` and `docs/plans/2026-10-14-runtime-defects-and-enhancements-plan.md`. Both describe merged work.
3. Add one line to `README.md` that says which code comes from the three projects in `licenses/`. Do not delete `licenses/`.

Check: `rg -n 'diagrams/|PULL_REQUEST|2026-10-14-runtime-defects-and-enhancements-plan' .` returns no link to a deleted file.

## Phase 2 — Delete dead code (no behavior change)

1. Delete `makeHandler` in `internal/tools/handlers.go` and its test in `tools_handler_test.go`.
2. Delete `Bridge.MarshalJSON` in `internal/bridge/bridge.go` and its two tests.
3. Delete the `port` field of `Election` in `internal/cluster/election.go`.
4. Delete `handleReadRequest` in `plugin/src/read-handlers.ts`.
5. Delete the unused `handle*Request` wrappers in `read-document.ts`, `read-export.ts`, `read-styles.ts`, `write-components.ts`, `write-prototype.ts`. If a test calls a wrapper, change the test to call the handler map.
6. Delete the duplicate imports in `plugin/src/main.ts` (one name for each handler map).

Check: `make test && make build && golangci-lint run --enable-only unused ./...`

## Phase 3 — Local Go simplifications (no behavior change)

1. Replace `itoa` (`cluster/election.go`) and `containsString` (`tools/toolspec.go`) with the standard library calls.
2. In `cluster.Exposed`, delete the `""` and `"localhost"` branches and the `loopbackAddresses` map. `main.go` accepts only an IP.
3. Keep one exposure warning. Delete the one in `cmd/figma-mcp-go/main.go`; the one in `leader.go` stays, because only a leader listens.
4. Make `Node` and `Election` share one `Follower`. `NewElection` takes it from the node.
5. Change `FollowerTimeoutFor(tool)` in `bridge/timeout.go` to a constant. Correct the comment in `cluster/follower.go`.
6. Rename `plugin/src/ui/i18n.ts` to `strings.ts` and add parameter types.

Check: `make fmt-check vet test`

## Phase 4 — Comments

1. Delete the section-divider comments (`// ── … ──`) in `internal/` and `plugin/src/`.
2. Rewrite or delete the comments that tell change history ("used to", "replaced seven", "as in the TypeScript version"). Keep the reason, drop the history.

Check: `rg -c '^\s*// ── ' internal cmd plugin/src` returns nothing. `make test` passes.

## Phase 5 — Build and release tooling

1. Correct `plugin/tsconfig.json` (`lib`, `types` for `bun:test`, separate DOM and plugin typings) until `tsc --noEmit` has zero errors. Correct the real type errors that it shows.
2. Add a `typecheck` script to `plugin/package.json`, a `typecheck-ts` target to `Makefile`, and one step to the `verify-plugin` job in `.github/workflows/ci.yml`.
3. In `.github/workflows/release.yml`, add `.packages[0].version` of `server.json` to the version gate. Then delete the `npm version` call and the "Set version in server.json" step.

Check: `cd plugin && bun run typecheck`. The release change is checked on the next tag; a dry run is `jq -r '.packages[0].version' server.json`.

## Phase 6 — Close `/rpc` to browsers (small behavior change, security)

1. In `handleRPC` (`internal/cluster/leader.go`), refuse a request that has an `Origin` header, and refuse a `Content-Type` other than `application/json`. Status 403 and 415.
2. Add tests to `internal/cluster/leader_test.go`: a cross-origin POST is refused, a `text/plain` POST is refused, a follower call still passes.

Check: `go test ./internal/cluster/...`. Manual: `curl -X POST -H 'Origin: https://example.com' 127.0.0.1:1994/rpc` returns 403.

Note: this phase has the highest value. It is safe to move it before phase 1.

## Phase 7 — Structure changes inside one side

1. Move `internal/figma` into `internal/tools`. Delete the `figma` edges from `deps-check` in `Makefile`. Update the package rule line in `README.md`.
2. Version skew in one place. The server adds its skew message to the `server-info` frame (`bridge/bridge.go`). The panel shows that text. Delete `plugin/src/ui/version-check.ts` and its test. A panel that connects to an older server shows no banner.
3. Tool list from one source. Add a `go generate` step that writes the `tools` block of `glama.json` and the README tool table from the spec table. Change the sync test in `tools_schema_test.go` to "generated output is current". First read the Glama schema: if `tools` is optional, delete the block and the test.

Check: `make deps-check test`, `go generate ./... && git diff --exit-code`.

## Phase 8 — Plugin handler envelope (wide, mechanical)

1. Change the dispatcher (`plugin/src/main.ts`, `write-handlers.ts`) so that a handler returns only its data. The dispatcher adds `type` and `requestId`, and calls `figma.commitUndo()` after a mutating handler.
2. Convert the handler files one module per commit: 94 return sites, 61 `commitUndo` calls. Update the tests of each module in the same commit.
3. Delete the "answer with the name the caller used" re-wrap in `create_node`, `set_paint`, `manage_page`, `manage_variable`, `create_style`.

Check after each commit: `make test-ts`. At the end: `rg -c 'requestId: request.requestId' plugin/src -g '!*test*'` shows only the dispatcher. Manual smoke test in Figma: one create, one modify, one pipeline, one undo.

## Phase 9 — Pipeline contract (breaking change)

1. Go: the `Validate` of `batch_execute_pipeline` (`internal/tools/tools_write.go`) refuses an action that is not in `specRegistry`, and runs `validateSpec` on each step that has no `$variable` in a checked field.
2. Go: `Check` refuses an unknown tool name. `ValidateRPC` moves into `Check`; `validate.go` is deleted.
3. Plugin: the pre-merge handlers (`create_frame`, `set_fills`, `add_page`, …) become plain functions that the merged handlers call. They leave the handler maps. Delete `INTERNAL_DISPATCH_TARGETS` from `tool-contract.test.ts`. Reduce `CREATE_ACTIONS` and `DESTRUCTIVE_TOOLS` to public names.
4. Update the prompts in `internal/prompts/` that name an old action. Add the break to `CHANGELOG.md`. Bump the minor version.

Check: `make test`. New tests: a step with a bad hex color is refused; a step with action `create_frame` is refused; `/rpc` with an unknown tool is refused.

## Phase 10 — Pipeline rollback through Figma undo (needs an experiment first)

1. Experiment in Figma: commit, run a pipeline that deletes, groups and reparents, call `figma.triggerUndo()`. Record whether the document returns to the committed state, and whether it does so in Dev Mode and FigJam.
2. If yes: replace the write-ahead log in `plugin/src/batch-pipeline.ts` (`SNAPSHOT_PROPS`, `snapshotNode`, `restoreNodeProperties`, `executeRollback`, `isCreateStep`, `CREATE_ACTIONS`) with commit-before and undo-on-failure. Update the tool description in `tools_write.go` and regenerate the golden schema.
3. If no: stop this phase and write the result in the spec.

Check: `make test`, then the experiment of step 1 as a manual regression.

## Phase 11 — Product decisions (need the owner's answer before work)

1. One read/write classification. Decide whether `set_selection`, page navigation and `save_version_checkpoint` are allowed in read-only mode. Then make the server flag and the panel guard use the same list, generated from the Go spec table.
2. Auto-copy. Decide between: default off, or delete it (`copy-helper.ts`, `bridge/clipboard.go`, the `copy_to_clipboard` frame, the `autoCopyBroken` state) and keep pin plus manual copy.

Check: `make test`; manual check of the panel in Figma.

## Last step

Move each lasting decision to `docs/specs` (the `/rpc` rule, the pipeline contract, the rollback method, the read-only list, the auto-copy decision). Then delete this plan.

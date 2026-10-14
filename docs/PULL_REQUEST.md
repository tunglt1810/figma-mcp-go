# Pull Request: Fix Pipeline Rollback, Read-Only Guard, IPv6 Bind, and Serialization Defects

**Branch:** `fix/pipeline-rollback-and-runtime-defects` -> `main`
**Standard:** ASD-STE100 Issue 9 (Strict Mode)

## Summary of Changes

This pull request fixes nine runtime and data-safety defects across the Figma plugin and the Go MCP server. All changes preserve the existing MCP tool schemas in `internal/tools/testdata/tools_schema.json` and maintain the package dependency rules in `make deps-check`.

## 1. Pipeline Rollback Safety (`plugin/src/batch-pipeline.ts`, `plugin/src/write-create.ts`)

- **Fix `import_image` rollback on existing nodes (BUG-01):** When a caller provides `nodeId`, `import_image` updates `fills` on an existing node instead of creating a rectangle. `isCreateStep` now returns `!params?.nodeId` for `import_image`. A pipeline rollback now restores the original `fills` instead of calling `node.remove()` on the user's node.
- **Prevent destructive rollback on `create_component` (BUG-02):** `create_component` moves all children from an existing frame into a new component and deletes the source frame. Deleting `create_component` from `CREATE_ACTIONS` prevents `executeRollback` from deleting the converted component and its children.
- **Delete orphaned SVG nodes from `create_vector` on rollback (BUG-03):** Added `create_vector` to `CREATE_ACTIONS` so that pipeline rollback deletes newly created vector nodes.
- **Support `import_image` target node resolution inside pipelines:** `import_image` now reads `p.nodeId ?? request.nodeIds?.[0]` so that pipeline steps can target existing nodes when `handleBatchPipelineRequest` moves `nodeId` into `request.nodeIds`.

## 2. Document Read Performance and Token Export (`plugin/src/read-document.ts`, `plugin/src/read-styles.ts`)

- **Eliminate exponential re-serialization in `get_document` (BUG-05):** Deleted the `p.depth == null` exclusion from `plain` so depth-limited full walks use `serializeNode(node, budget, 0)` directly. In `serializeWithDepth`, passed `makeBudget(undefined, 0)` to `serializeNode` and walked `node.children` in memory instead of re-serializing the full subtree at each level.
- **Resolve `VARIABLE_ALIAS` in CSS token exports (BUG-07):** Updated `export_tokens` (`format: "css"`) to resolve alias values to `var(--target-name)` instead of dropping `COLOR` aliases or writing `[object Object]` for `FLOAT`, `STRING`, and `BOOLEAN` aliases.

## 3. Server Security, Networking, and Concurrency (`cmd/`, `internal/`)

- **Enforce `--read-only` on Leader `/rpc` (BUG-08):** Added `tools.CheckReadOnly` and passed it to `cluster.NewNode` when `--read-only` is active. A read-only leader now rejects mutating `/rpc` requests from followers or local HTTP clients.
- **Support IPv6 addresses in `Leader`, `Node`, and `Election` (BUG-10):** Replaced `fmt.Sprintf("%s:%d", ip, port)` with `net.JoinHostPort` and updated `cluster.Exposed` to check `net.ParseIP(ip).IsLoopback()`.
- **Prevent WebSocket reader stalls on clipboard writes (BUG-11):** Added a 2-second context timeout (`exec.CommandContext`) to `WriteOSClipboard`. The bridge now runs clipboard writes on a separate goroutine outside `readLoop`.
- **Clear stale plugin metadata on disconnect and upgrade (BUG-12):** Reset `b.pluginVersion` and `b.pluginHandlers` under `b.mu.Lock()` in `HandleUpgrade` and `readLoop` cleanup.
- **Align built-in MCP prompt parameters with tool specs (BUG-13):** Replaced `modeName` with `initialModeName` for `create_collection` in `generate_color_palette.go`. Updated `generate_component_variants.go` to reference `set_text_ranges`, and updated the reaction schema example in `reaction_to_connector_strategy.go`.

## Test Plan

1. Run `make deps-check` to check package layering.
2. Run `go test ./...` to check Go unit tests and golden schema snapshots.
3. Run `cd plugin && bun test` to check TypeScript unit tests, including the new `isCreateStep` and `CREATE_ACTIONS` tests in `plugin/src/batch-pipeline.test.ts`.

# Plan: Runtime Defect Fixes and Feature Enhancements

**Date:** 2026-10-14
**Spec:** `docs/specs/2026-10-14-runtime-defects-and-enhancements-design.md`
**Walkthrough:** `docs/walkthroughs/2026-08-28-go-backend-architecture.md`

This plan lists the implementation tasks, file locations, and test commands for the runtime defect fixes and feature enhancements. Each task keeps `internal/tools/testdata/tools_schema.json` and `make deps-check` green.

## Phase 1: Pipeline Rollback and Write Handler Fixes (`plugin/src/`)

### Task 1.1: Fix `import_image`, `create_component`, and `create_vector` Rollback Classification

**Files:**

* `plugin/src/batch-pipeline.ts`

* `plugin/src/write-create.ts`

* `plugin/src/batch-pipeline.test.ts`

**Steps:**

1. In `plugin/src/batch-pipeline.ts`, delete `'create_component'` from `CREATE_ACTIONS` and add `'create_vector'`.

2. In `isCreateStep(action, params)`, add:

   ```
   if (action === 'import_image') {
     return !params?.nodeId;
   }
   
   ```

3. In `plugin/src/write-create.ts`, update `import_image` to read `const targetId = p.nodeId ?? (request.nodeIds && request.nodeIds[0])` so that `import_image` finds existing target nodes when called inside `batch_execute_pipeline`.

4. In `plugin/src/batch-pipeline.test.ts`, move `'create_vector'` out of `KEEPS_EXISTING_NODES`, add `'create_component'` to `KEEPS_EXISTING_NODES`, and add unit tests for `isCreateStep('import_image', ...)` with and without `nodeId`.

### Task 1.2: Preserve Range Precedence in `set_text_ranges`

**Files:**

* `plugin/src/write-text.ts`

* `plugin/src/write-text.test.ts`

**Steps:**

1. Update `plugin/src/write-text.ts` so overlapping ranges apply in caller array order (`Later ranges win on overlap`).

2. Track per-character font changes across ranges in `rangeFonts` before calling `loadFonts`.

## Phase 2: Read Performance and Token Export Fixes (`plugin/src/`)

### Task 2.1: Eliminate Redundant Subtree Serialization in `get_document`

**Files:**

* `plugin/src/read-document.ts`

* `plugin/src/read-search.test.ts`

**Steps:**

1. In `plugin/src/read-document.ts`, change `plain` from `detail === "full" && !dedupeComponents && p.depth == null` to `detail === "full" && !dedupeComponents`.

2. In `serializeWithDepth`, replace `await serializeNode(node)` with `await serializeNode(node, makeBudget(undefined, 0), 0)`.

3. Walk `node.children` directly instead of calling `figma.getNodeByIdAsync(child.id)` on each child.

### Task 2.2: Resolve `VARIABLE_ALIAS` in `export_tokens`

**Files:**

* `plugin/src/read-styles.ts`

* `plugin/src/read-styles.test.ts`

**Steps:**

1. In `plugin/src/read-styles.ts` (`format === "css"`), check whether `val` is `{ type: "VARIABLE_ALIAS", id: ... }`.

2. Resolve the target variable via `figma.variables.getVariableByIdAsync(val.id)` and format the value as `var(--<target-css-name>)`.

3. Add a test in `plugin/src/read-styles.test.ts` for `COLOR` and `FLOAT` alias variables in CSS output.

## Phase 3: Go Server Guards, Networking, Clipboard, and Prompts (`cmd/`, `internal/`)

### Task 3.1: Enforce `--read-only` on Leader `/rpc`

**Files:**

* `internal/tools/toolspec.go`

* `cmd/figma-mcp-go/main.go`

* `internal/tools/leader_rpc_test.go`

**Steps:**

1. Add `CheckReadOnly` in `internal/tools/toolspec.go` to reject any tool that is not in `readOnlyModeSpecs()`.

2. In `cmd/figma-mcp-go/main.go`, pass `tools.CheckReadOnly` to `cluster.NewNode` when `*readOnly` is `true`.

3. Add a test in `internal/tools/leader_rpc_test.go` that posts a write tool to `/rpc` on a leader configured with `CheckReadOnly` and checks that `/rpc` returns HTTP 400.

### Task 3.2: Fix IPv6 Host-Port Formatting and Loopback Check

**Files:**

* `internal/cluster/leader.go`

* `internal/cluster/node.go`

* `internal/cluster/election.go`

* `internal/cluster/leader_test.go`

**Steps:**

1. Replace `fmt.Sprintf("%s:%d", ip, port)` with `net.JoinHostPort` in `Leader.Start`, `NewNode`, and `NewElection`.

2. In `cluster.Exposed(ip)`, parse `ip` with `net.ParseIP(ip)` and return `false` when `parsed.IsLoopback()`.

3. Add unit tests for `Exposed("::1")` and `Exposed("127.0.0.2")` in `internal/cluster/leader_test.go`.

### Task 3.3: Isolate Clipboard Subprocess from WebSocket `readLoop`

**Files:**

* `internal/bridge/clipboard.go`

* `internal/bridge/bridge.go`

**Steps:**

1. In `internal/bridge/clipboard.go`, wrap the OS clipboard command in `exec.CommandContext` with a 2-second timeout.

2. In `internal/bridge/bridge.go`, run `WriteOSClipboard` on a separate goroutine when `readLoop` receives `copy_to_clipboard`.

3. Reset `b.pluginVersion = ""` and `b.pluginHandlers = nil` under `b.mu.Lock()` in `HandleUpgrade` and `readLoop` cleanup.

### Task 3.4: Synchronize Built-In Prompt Parameters

**Files:**

* `internal/prompts/generate_color_palette.go`

* `internal/prompts/generate_component_variants.go`

* `internal/prompts/reaction_to_connector_strategy.go`

**Steps:**

1. Replace `modeName=` with `initialModeName=` on lines 79 and 83 of `generate_color_palette.go`.

2. Update `generate_component_variants.go` to use `set_text_ranges` for text scaling.

3. Update `reaction_to_connector_strategy.go` to show the `actions` array structure.

## Phase 4: Feature Enhancements Roadmap

| Order | Feature ID | Target Packages | Summary | 
| ----- | ----- | ----- | ----- | 
| 1 | `FEAT-02` | `internal/tools`, `plugin/src` | Add `aliasTo`, `scopes`, and `codeSyntax` to `manage_variable`. | 
| 2 | `FEAT-03` | `internal/tools`, `plugin/src` | Add `offset` and `nextOffset` pagination to `get_document` and `get_nodes_info`. | 
| 3 | `FEAT-05` | `internal/tools`, `plugin/src` | Add pre-flight static checks and `dry_run` mode to `batch_execute_pipeline`. | 
| 4 | `FEAT-06` | `internal/tools`, `plugin/src` | Add `computedCss` option via `node.getCSSAsync()` in `get_nodes_info`. | 
| 5 | `FEAT-04` | `internal/bridge`, `internal/cluster` | Forward MCP `notifications/progress` and stream `/rpc` heartbeats. | 
| 6 | `FEAT-01` | `internal/bridge`, `internal/tools` | Multiplex multiple Figma file sessions on one WebSocket port. | 

## Verification Commands

Run these commands from the repository root after completing Phases 1 through 3:

```
make deps-check
make fmt-check
go vet ./...
go test ./...
cd plugin && bun test

```
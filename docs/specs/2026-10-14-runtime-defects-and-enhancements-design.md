# Specs: Runtime Defect Fixes and Feature Enhancements for the Figma MCP Plugin

**Date:** 2026-10-14
**Spec:** `docs/specs/2026-10-14-runtime-defects-and-enhancements-design.md`
**Plan:** `docs/plans/2026-10-14-runtime-defects-and-enhancements-plan.md`

## 1. Introduction

This document specifies the design for two sets of changes in `figma-mcp-go`:

1. **Runtime and Data-Safety Defect Fixes**: Fixes for 13 hidden defects in pipeline rollback, tree serialization, token export, `/rpc` guards, IPv6 networking, clipboard writes, and prompts.
2. **Feature Enhancements**: Specifications for 6 new capabilities covering multi-file WebSocket routing, variable aliasing, tree pagination, MCP progress notifications, pipeline dry-run mode, and computed CSS extraction.

All changes keep the existing package dependency graph (`cmd -> tools -> figma` and `cmd -> cluster -> bridge`) enforced by `make deps-check`.

## 2. Pipeline Rollback and Schema Guards (`batch_execute_pipeline`)

### 2.1. In-Place Image Fill Rollback (`import_image` — BUG-01)

#### Input Payload

```json
{
  "steps": [
    {
      "id": "paint_avatar",
      "action": "import_image",
      "params": {
        "nodeId": "10:5",
        "imageUrl": "https://example.com/avatar.png"
      }
    }
  ]
}
```

#### Logic

- When the payload does not contain `nodeId`, `import_image` creates a new `RECTANGLE` node. `isCreateStep` returns `true`, and rollback calls `node.remove()` on the new rectangle.
- When the payload contains `nodeId`, `import_image` changes the `fills` array on the existing node and returns `{ id: target.id }`.
- `isCreateStep("import_image", params)` must return `!params?.nodeId`:
  ```typescript
  if (action === "import_image") {
    return !params?.nodeId;
  }
  ```
- Because `isCreateStep` returns `false` when `nodeId` is present, `executeBatchPipeline` captures a `MODIFY` snapshot of `target.fills` before the step runs. On failure, `executeRollback` restores the original `fills` and does not delete the user's node.
- Inside `write-create.ts`, `import_image` reads `const targetId = p.nodeId ?? request.nodeIds?.[0]` so that it finds the target node after `handleBatchPipelineRequest` moves `nodeId` to `request.nodeIds`.

### 2.2. Component Conversion and Vector Creation (`create_component`, `create_vector` — BUG-02, BUG-03)

#### Logic

- `create_component` moves all children from an existing `FRAME` into a new `COMPONENT` and calls `node.remove()` on the source frame.
- If `CREATE_ACTIONS` contains `create_component`, a later rollback calls `component.remove()` and deletes both the component and all children moved from the source frame.
- Delete `create_component` from `CREATE_ACTIONS` and classify it in `KEEPS_EXISTING_NODES`.
- In `write-create.ts`, use `figma.createComponentFromNode(node)` when available so that conversion preserves effects, layout grids, per-corner radii, opacity, and constraints.
- `create_vector` creates a new `VECTOR` or `FRAME` node from SVG markup. Add `create_vector` to `CREATE_ACTIONS` so that rollback deletes the created node when a later step fails.

### 2.3. Step Parameter Validation in Go (`batchPipelineSpec` — BUG-04)

#### Logic

- In `internal/tools/tools_write.go`, attach a `Validate` function to `batchPipelineSpec`.
- For each entry in `steps`:
  1. Check that `action` is a non-empty string.
  2. Look up `action` in `specRegistry`.
  3. Extract literal parameters (any parameter value that is not a `$variable` string) and run `validateSpec` for that `action`.

## 3. Document Serialization, Text Ranges, and Token Export

### 3.1. Single-Pass Depth Serialization (`get_document` — BUG-05)

#### Problem

In `plugin/src/read-document.ts`, `plain` is `detail === "full" && !dedupeComponents && p.depth == null`. When a caller passes `depth` or `dedupe_components: true` with `detail: "full"`, `get_document` calls `serializeWithDepth`. Line 207 calls `await serializeNode(node)` without a budget, which serializes the entire subtree to the leaves at every recursion level.

#### Logic

1. Set `const plain = detail === "full" && !dedupeComponents` because `budget` already enforces `p.depth`.
2. Inside `serializeWithDepth`, pass `makeBudget(undefined, 0)` to `serializeNode(node, makeBudget(undefined, 0), 0)`. That call serializes only the current node without walking descendants.
3. Walk `node.children` directly in memory instead of calling `figma.getNodeByIdAsync(child.id)` on each child.

### 3.2. Range Order and Font Inheritance (`set_text_ranges` — BUG-06)

#### Logic

- The tool contract states that later ranges win on overlap.
- In `plugin/src/write-text.ts`, keep ranges in caller array order instead of sorting by `start` index when ranges overlap.
- For partial font overrides (`fontFamily` or `fontStyle` alone), track the active font per character offset across earlier ranges before calling `loadFonts`.

### 3.3. Variable Alias Resolution in Token Exports (`export_tokens` — BUG-07)

#### Input Payload

```json
{
  "format": "css"
}
```

#### Logic

- In `plugin/src/read-styles.ts`, check whether `val` is a variable alias (`val && typeof val === "object" && "type" in val && val.type === "VARIABLE_ALIAS"`).
- When `val` is an alias in `css` mode, look up the target variable with `figma.variables.getVariableByIdAsync(val.id)` and output `var(--<target-css-name>)`:
  ```typescript
  const target = await figma.variables.getVariableByIdAsync((val as any).id);
  if (target) {
    const targetCssName = "--" + target.name.toLowerCase().replace(/[/\s]+/g, "-").replace(/[^a-z0-9-]/g, "");
    cssValue = `var(${targetCssName})`;
  }
  ```
- This check keeps `COLOR` aliases in the CSS output. It also prevents `FLOAT`, `STRING`, and `BOOLEAN` aliases from formatting as `[object Object]`.

## 4. Cluster, Bridge, and Prompt Synchronization

### 4.1. Read-Only Guard on Leader `/rpc` (`--read-only` — BUG-08)

#### Logic

- In `internal/tools/toolspec.go`, define `CheckReadOnly(tool string, nodeIDs []string, params map[string]any) ([]string, map[string]any, error)`:
  ```go
  func CheckReadOnly(tool string, nodeIDs []string, params map[string]any) ([]string, map[string]any, error) {
  	if spec, ok := specRegistry[tool]; ok {
  		if !spec.ReadOnly && !slices.Contains(readOnlyModeExports, tool) {
  			return nil, nil, fmt.Errorf("%s is disabled because the server is running in --read-only mode", tool)
  		}
  	}
  	return Check(tool, nodeIDs, params)
  }
  ```
- In `cmd/figma-mcp-go/main.go`, pass `tools.CheckReadOnly` to `cluster.NewNode` when `*readOnly` is `true`.

### 4.2. Follower Timeout Alignment (`Follower.Send` — BUG-09)

#### Logic

- The leader extends tool timeouts up to `MaxToolTimeout` (10 minutes) when the plugin sends `progress_update` frames.
- Update `FollowerTimeoutFor` or stream progress heartbeats over `/rpc`. This prevents a follower from closing the HTTP request at 35 seconds while the plugin still reports progress.

### 4.3. IPv6 Host-Port Formatting and Loopback Detection (BUG-10)

#### Logic

- In `internal/cluster/leader.go`, `internal/cluster/node.go`, and `internal/cluster/election.go`, format TCP and HTTP addresses with `net.JoinHostPort(ip, portStr)` instead of `fmt.Sprintf("%s:%d", ip, port)`.
- In `cluster.Exposed(ip)`, parse `ip` with `net.ParseIP(ip)` and return `false` when `parsed.IsLoopback()`.

### 4.4. Non-Blocking Clipboard Writes and Connection State Reset (BUG-11, BUG-12)

#### Logic

- In `internal/bridge/clipboard.go`, use `exec.CommandContext` with a 2-second timeout (`clipboardTimeout = 2 * time.Second`).
- In `internal/bridge/bridge.go`, run `WriteOSClipboard` in a background goroutine so that `readLoop` continues to read WebSocket frames and pong responses.
- In `HandleUpgrade` and `readLoop` teardown, clear `b.pluginVersion = ""` and `b.pluginHandlers = nil` under `b.mu.Lock()`.

### 4.5. Prompt Parameter Synchronization (BUG-13)

#### Logic

- In `internal/prompts/generate_color_palette.go`, replace `modeName=` with `initialModeName=` for `manage_variable(action="create_collection")`.
- In `internal/prompts/generate_component_variants.go`, instruct the model to scale `fontSize` with `set_text_ranges`.
- In `internal/prompts/reaction_to_connector_strategy.go`, use the `actions` array with `type: "NODE"` and `navigation: "NAVIGATE"`.

## 5. New Feature Specifications

### 5.1. Multi-File WebSocket Multiplexing (`FEAT-01`)

#### Input Payload

```json
{
  "file": "Design System v2",
  "scope": "page"
}
```

#### Logic

- Store active plugin connections in `Bridge` as `sessions map[string]*pluginSession` keyed by `fileKey` or `fileName`.
- When only one Figma file connects to the bridge, route all tool calls to that session automatically.
- When multiple Figma files connect, route by the optional `file` argument. If the caller omits `file`, return an error that lists the active file names.

### 5.2. Variable Aliasing, Scopes, and Code Syntax (`manage_variable` — `FEAT-02`)

#### Input Payload

```json
{
  "action": "set_value",
  "variableId": "VariableID:20:1",
  "modeId": "20:0",
  "aliasTo": "VariableID:10:1",
  "scopes": ["ALL_FILLS"],
  "codeSyntax": {
    "WEB": "var(--color-primary)"
  }
}
```

#### Logic

- Add `aliasTo` (`kindString`), `scopes` (`kindStringArray`), and `codeSyntax` (`kindObject`) to `manage_variable` in `internal/tools/tools_write_variables.go`.
- Reject payloads that provide both `value` and `aliasTo`.
- In `plugin/src/write-variables.ts`, resolve `aliasTo` with `figma.variables.getVariableByIdAsync(p.aliasTo)` and assign `figma.variables.createVariableAlias(target)` to the mode.

### 5.3. Cursor Pagination for Large Trees (`get_document`, `get_nodes_info` — `FEAT-03`)

#### Input Payload

```json
{
  "scope": "page",
  "maxNodes": 200,
  "offset": 200
}
```

#### Logic

- Add `offset` (`kindNumber`, minimum 0) to `get_document` and `get_nodes_info`.
- Track `skip: number` inside `SerializeBudget`. Decrement `skip` for each node visited in pre-order traversal before decrementing `remaining`.
- When `budget.truncated` is `true`, return `nextOffset: offset + maxNodes` in the response object.

### 5.4. MCP Progress Notifications and Streaming `/rpc` (`FEAT-04`)

#### Logic

- When an incoming MCP `tools/call` request contains `_meta.progressToken`, attach an `onProgress` callback to the bridge's `pendingEntry`.
- Forward each plugin `progress_update` frame to the MCP client as a `notifications/progress` message over stdio.
- Stream newline-delimited JSON progress frames from the leader's `/rpc` handler when the follower sends `Accept: application/x-ndjson`.

### 5.5. Pipeline Pre-Flight Check and Dry-Run Mode (`batch_execute_pipeline` — `FEAT-05`)

#### Input Payload

```json
{
  "dry_run": true,
  "stop_on_error": true,
  "steps": [
    {
      "id": "s1",
      "action": "create_node",
      "params": { "type": "FRAME", "name": "Card" },
      "export_vars": { "id": "$card_id" }
    }
  ]
}
```

#### Logic

- Add `dry_run` (`kindBool`) to `batchPipelineSpec`.
- Before step 0 executes in `executeBatchPipeline`:
  1. Check that some step $j < k$ exports every `$variable` used in step $k$.
  2. Check that all static `nodeId`, `parentId`, `componentId`, and `styleId` targets exist in the document.
  3. Collect all fonts from text steps and call `loadFonts` once up front.
- When `dry_run` is `true`, return the check results and variable dependency map without changing the document.

### 5.6. Computed CSS Extraction (`get_nodes_info` — `FEAT-06`)

#### Input Payload

```json
{
  "nodeIds": ["1:1", "1:2"],
  "computedCss": true
}
```

#### Logic

- Add `computedCss` (`kindBool`) to `get_nodes_info`.
- In `plugin/src/read-document.ts`, when `computedCss` is `true`, call `await node.getCSSAsync()` on each resolved node and attach the returned map to `serialized.css`.

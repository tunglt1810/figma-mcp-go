# Specification: Batch Execute Pipeline for Figma MCP

- **Status:** Draft (Approved Design)
- **Author:** Antigravity & anh guộc
- **Date:** 2026-08-01
- **Target Components:** `internal/` (Go Bridge) & `plugin/src/` (Figma Plugin)

---

## 1. Overview & Problem Statement

Currently, each creation or modification command in Figma (e.g., `create_frame`, `create_text`, `set_fills`) requires an isolated WebSocket round-trip between the Go MCP Server and the Figma Plugin (TypeScript / Main Thread).

When an AI generates an entire web page or complex user interface consisting of hundreds of UI nodes, the accumulated network latency becomes significant.

### Proposed Solution
Add a new MCP tool `batch_execute_pipeline` that enables sending an array of mutation steps in a single WebSocket payload. Commands execute sequentially within the Figma main thread with the following capabilities:
1. **Stateful Variable Binding:** Downstream commands can reference IDs or attributes created by upstream commands (e.g., `$header_frame.id`).
2. **Compensation Rollback Engine (WAL):** Automatically unwinds changes (deleting created nodes and restoring modified properties) if any step in the batch fails.
3. **Pipeline-level Retry Feedback:** Returns detailed failure information (failed step index, action, error message) to the Go Bridge / AI Client so the AI can adjust the payload and retry the pipeline.

---

## 2. Protocol & Data Schemas

### 2.1 MCP Tool Definition (`internal/tools_write.go`)
- **Tool Name:** `batch_execute_pipeline`
- **Description:** Execute a batch array of Figma mutation steps in a single transactional pipeline with variable binding and rollback support.

#### Request Payload:
```json
{
  "stop_on_error": true,
  "steps": [
    {
      "id": "step_1",
      "action": "create_frame",
      "params": {
        "name": "Main Canvas",
        "width": 1440,
        "height": 900,
        "fills": [{"type": "SOLID", "color": {"r": 1, "g": 1, "b": 1}}]
      },
      "export_vars": {
        "id": "$main_frame"
      }
    },
    {
      "id": "step_2",
      "action": "create_text",
      "params": {
        "parent_id": "$main_frame",
        "text": "Hello Figma MCP",
        "font_size": 32,
        "x": 40,
        "y": 40
      },
      "export_vars": {
        "id": "$title_text"
      }
    }
  ]
}
```

#### Response Payload:
- **Success Case (`200 OK`):**
```json
{
  "success": true,
  "completed_steps": 2,
  "exports": {
    "$main_frame": "123:456",
    "$title_text": "123:457"
  },
  "results": [
    { "step_id": "step_1", "status": "ok", "node_id": "123:456" },
    { "step_id": "step_2", "status": "ok", "node_id": "123:457" }
  ]
}
```

- **Error Case with Rollback (`500 / Tool Execution Error`):**
```json
{
  "success": false,
  "completed_steps": 1,
  "failed_step": {
    "index": 1,
    "step_id": "step_2",
    "action": "create_text",
    "error": "Font Inter-Bold is not loaded in Figma context"
  },
  "rollback_executed": true,
  "rolled_back_steps": 1
}
```

---

## 3. Architecture & Execution Engine

```
[ AI Client / Go MCP Server ]
             │
             │ Single WS Message: batch_execute_pipeline
             ▼
   [ plugin/src/main.ts ]
             │
             ▼
[ batch-pipeline.ts (Executor) ]
   ├── 1. Symbol Table (Map<string, any>) ── [ Interpolate $vars ]
   ├── 2. WAL Log Stack (LogEntry[])      ── [ Record CREATE/MODIFY ]
   └── 3. Step Runner (Existing Handlers)
             │
      ┌──────┴──────┐
   SUCCESS        ERROR
      │             │
      ▼             ▼
   Commit       Trigger WAL Rollback (LIFO)
   Return       Remove created nodes & Restore properties
   Exports      Return failed_step details
```

### 3.1 Symbol Table & Variable Interpolator
- Plugin maintains a `symbolTable = new Map<string, any>()`.
- Before executing each step, `resolveParams(params, symbolTable)` recursively scans the parameter object:
  - Strings matching pattern `^\$[a-zA-Z0-9_]+(\.[a-zA-Z0-9_]+)?$` are resolved against `symbolTable`.
  - If a referenced variable is missing from `symbolTable`, an exception is thrown: `Undefined pipeline variable: ${varName}`.

### 3.2 WAL (Write-Ahead Log) Rollback Engine
A LIFO rollback stack records all mutations on the canvas:

```typescript
type LogEntry =
  | { type: 'CREATE'; nodeId: string }
  | { type: 'MODIFY'; nodeId: string; previousState: Record<string, any> };
```

- **Logging Rules:**
  - Immediately after a `CREATE` action succeeds: Push `{ type: 'CREATE', nodeId: node.id }`.
  - Before a `MODIFY` action executes: Snapshot original properties (e.g. `fills`, `strokes`, `x`, `y`, `width`, `height`, `characters`), then push `{ type: 'MODIFY', nodeId: node.id, previousState }`.

- **Rollback Process:**
  - Upon catching an exception at step `k`:
  - Iteratively `pop()` each entry from `walStack`:
    - For `CREATE`: Execute `(await figma.getNodeByIdAsync(nodeId))?.remove()`.
    - For `MODIFY`: Restore property values using `restoreNodeProperties(node, previousState)`.

---

## 4. Impacted Components & Files

### 4.1 Go Backend (`internal/`)
- [MODIFY] `internal/schema.go`: Define structs `BatchPipelineRequest`, `BatchPipelineStep`, and `BatchPipelineResponse`.
- [MODIFY] `internal/tools_write.go`: Register MCP tool handler for `batch_execute_pipeline`.
- [MODIFY] `internal/bridge.go`: Extend timeout for batch execution requests (default 120s).

### 4.2 Figma Plugin (`plugin/src/`)
- [NEW] `plugin/src/batch-pipeline.ts`: Implement Symbol Table, Interpolator, WAL Rollback Stack, and `executeBatchPipeline()`.
- [MODIFY] `plugin/src/main.ts`: Handle ws event `batch_execute_pipeline` and delegate to `executeBatchPipeline()`.
- [NEW] `plugin/src/batch-pipeline.test.ts`: Add unit tests for variable interpolation and WAL rollback logic.

---

## 5. Verification Plan

### Automated Tests
- **Go Unit Tests:** Test schema serialization/deserialization for `batch_execute_pipeline` payload in `internal/schema_test.go`.
- **TypeScript Unit Tests:**
  - `bun test plugin/src/batch-pipeline.test.ts`: Test variable resolution (`$var1`), error throwing on missing vars, and WAL stack unwind logic.

### Manual Verification
1. Build and run Figma Plugin dev environment.
2. Send a valid batch request creating Header + Title + Subtitle. Verify Figma canvas creates a single frame containing all sub-nodes.
3. Send an invalid batch request failing intentionally at step 3 (invalid parameters). Verify the returned payload shows `rollback_executed: true` and no leftover artifact nodes remain on the canvas.

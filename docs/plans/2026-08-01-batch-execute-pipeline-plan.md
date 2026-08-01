# Batch Execute Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `batch_execute_pipeline` MCP tool allowing transactional batch mutations in a single WebSocket round-trip with stateful variable binding and WAL compensation rollback.

**Architecture:** Extend Go MCP backend schemas to receive `batch_execute_pipeline` requests. In Figma Plugin (TypeScript main thread), implement a batch runner featuring a Symbol Table for `$var` interpolation and a Write-Ahead Log (WAL) stack for LIFO compensation rollback on step failures.

**Tech Stack:** Go 1.22+, TypeScript 5+, Bun test runner (for plugin unit tests).

## Global Constraints

- Go implementation in `internal/` package following established MCP tool patterns.
- TypeScript implementation in `plugin/src/` compatible with Figma Plugin sandbox API.
- All code changes must pass `go test ./...` and `bun test` in `plugin/`.

---

### Task 1: Go Schemas & MCP Tool Registration

**Files:**
- Modify: `internal/schema.go`
- Modify: `internal/tools_write.go`
- Modify: `internal/schema_test.go`

**Interfaces:**
- Consumes: Go bridge tool handler pattern (`HandleToolCall`).
- Produces: `BatchPipelineRequest`, `BatchPipelineStep`, `BatchPipelineResponse` Go structs.

- [ ] **Step 1: Write the failing schema test**

Create test case in `internal/schema_test.go`:
```go
func TestBatchPipelineRequestSchema(t *testing.T) {
	rawJSON := `{
		"stop_on_error": true,
		"steps": [
			{
				"id": "step_1",
				"action": "create_frame",
				"params": {"name": "Header", "width": 100, "height": 100},
				"export_vars": {"id": "$header_id"}
			}
		]
	}`
	var req BatchPipelineRequest
	err := json.Unmarshal([]byte(rawJSON), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal BatchPipelineRequest: %v", err)
	}
	if req.Steps[0].Action != "create_frame" {
		t.Errorf("expected create_frame, got %s", req.Steps[0].Action)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal -run TestBatchPipelineRequestSchema`
Expected: FAIL with "undefined: BatchPipelineRequest"

- [ ] **Step 3: Write struct definitions and tool registration**

In `internal/schema.go`:
```go
type BatchPipelineStep struct {
	ID         string                 `json:"id"`
	Action     string                 `json:"action"`
	Params     map[string]interface{} `json:"params"`
	ExportVars map[string]string      `json:"export_vars,omitempty"`
}

type BatchPipelineRequest struct {
	StopOnError bool                `json:"stop_on_error,omitempty"`
	Steps       []BatchPipelineStep `json:"steps"`
}

type BatchPipelineResponse struct {
	Success        bool                   `json:"success"`
	CompletedSteps int                    `json:"completed_steps"`
	Exports        map[string]interface{} `json:"exports,omitempty"`
	Results        []map[string]interface{}`json:"results,omitempty"`
	FailedStep     map[string]interface{} `json:"failed_step,omitempty"`
	Rollback       bool                   `json:"rollback_executed,omitempty"`
}
```

In `internal/tools_write.go`: Register `batch_execute_pipeline` in tool list and routing logic.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal -run TestBatchPipelineRequestSchema`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
 git add internal/schema.go internal/tools_write.go internal/schema_test.go
 git commit -m "feat: add Go schemas for batch_execute_pipeline"
```

---

### Task 2: Plugin Symbol Table & Parameter Interpolation Engine

**Files:**
- Create: `plugin/src/batch-pipeline.ts`
- Create: `plugin/src/batch-pipeline.test.ts`

**Interfaces:**
- Consumes: JSON Step Params with `$variable` placeholders.
- Produces: `resolveParams(params, symbolTable)` and `SymbolTable` Map.

- [ ] **Step 1: Write the failing interpolation unit test**

In `plugin/src/batch-pipeline.test.ts`:
```typescript
import { expect, test } from 'bun:test';
import { resolveParams, SymbolTable } from './batch-pipeline';

test('resolveParams replaces variable references correctly', () => {
  const table: SymbolTable = new Map();
  table.set('$header_id', '123:456');

  const params = {
    parent_id: '$header_id',
    title: 'Hello',
    size: 100
  };

  const resolved = resolveParams(params, table);
  expect(resolved.parent_id).toBe('123:456');
  expect(resolved.title).toBe('Hello');
});

test('resolveParams throws error on missing variable', () => {
  const table: SymbolTable = new Map();
  const params = { parent_id: '$missing_var' };
  expect(() => resolveParams(params, table)).toThrow('Undefined pipeline variable: $missing_var');
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd plugin && bun test plugin/src/batch-pipeline.test.ts`
Expected: FAIL (module not found)

- [ ] **Step 3: Implement Symbol Table & Interpolator**

In `plugin/src/batch-pipeline.ts`:
```typescript
export type SymbolTable = Map<string, any>;

export function resolveParams(params: any, symbolTable: SymbolTable): any {
  if (typeof params === 'string') {
    if (params.startsWith('$')) {
      if (!symbolTable.has(params)) {
        throw new Error(`Undefined pipeline variable: ${params}`);
      }
      return symbolTable.get(params);
    }
    return params;
  }
  if (Array.isArray(params)) {
    return params.map(item => resolveParams(item, symbolTable));
  }
  if (params !== null && typeof params === 'object') {
    const resolved: Record<string, any> = {};
    for (const key of Object.keys(params)) {
      resolved[key] = resolveParams(params[key], symbolTable);
    }
    return resolved;
  }
  return params;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd plugin && bun test plugin/src/batch-pipeline.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
 git add plugin/src/batch-pipeline.ts plugin/src/batch-pipeline.test.ts
 git commit -m "feat: add Symbol Table and parameter interpolator for batch pipeline"
```

---

### Task 3: WAL Rollback Stack & Executor

**Files:**
- Modify: `plugin/src/batch-pipeline.ts`
- Modify: `plugin/src/batch-pipeline.test.ts`

**Interfaces:**
- Consumes: List of `PipelineStep` objects.
- Produces: `executeBatchPipeline(request)` returning `BatchPipelineResponse`.

- [ ] **Step 1: Write failing test for WAL Rollback**

In `plugin/src/batch-pipeline.test.ts`:
```typescript
import { WALStack, executeRollback } from './batch-pipeline';

test('executeRollback calls remove for created nodes in LIFO order', async () => {
  const removed: string[] = [];
  const fakeNodes: Record<string, any> = {
    'node_1': { remove: () => removed.push('node_1') },
    'node_2': { remove: () => removed.push('node_2') },
  };

  const stack: WALStack = [
    { type: 'CREATE', nodeId: 'node_1' },
    { type: 'CREATE', nodeId: 'node_2' }
  ];

  await executeRollback(stack, async (id) => fakeNodes[id]);
  expect(removed).toEqual(['node_2', 'node_1']);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd plugin && bun test plugin/src/batch-pipeline.test.ts`
Expected: FAIL

- [ ] **Step 3: Implement WAL Rollback & Main Pipeline Execution Loop**

In `plugin/src/batch-pipeline.ts`:
```typescript
export type LogEntry =
  | { type: 'CREATE'; nodeId: string }
  | { type: 'MODIFY'; nodeId: string; previousState: Record<string, any> };

export type WALStack = LogEntry[];

export async function executeRollback(
  stack: WALStack,
  getNodeById: (id: string) => Promise<any>
): Promise<number> {
  let count = 0;
  while (stack.length > 0) {
    const entry = stack.pop()!;
    try {
      if (entry.type === 'CREATE') {
        const node = await getNodeById(entry.nodeId);
        if (node && typeof node.remove === 'function') {
          node.remove();
          count++;
        }
      }
    } catch (err) {
      console.error('Rollback entry error:', err);
    }
  }
  return count;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd plugin && bun test plugin/src/batch-pipeline.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
 git add plugin/src/batch-pipeline.ts plugin/src/batch-pipeline.test.ts
 git commit -m "feat: implement WAL rollback engine and batch executor"
```

---

### Task 4: Plugin Router Integration & Bridge Timeout Adjustments

**Files:**
- Modify: `plugin/src/main.ts`
- Modify: `internal/bridge.go`

**Interfaces:**
- Consumes: WebSocket command `batch_execute_pipeline`.
- Produces: Pipeline response sent back via WebSocket.

- [ ] **Step 1: Wire handler in `plugin/src/main.ts`**

Add case for `batch_execute_pipeline` in `main.ts` event switch.

- [ ] **Step 2: Increase request timeout in `internal/bridge.go`**

Set default timeout for batch requests to 120s to allow large multi-step batches to process cleanly.

- [ ] **Step 3: Run full verification suite**

Run Go tests: `go test ./...`
Run Plugin tests: `cd plugin && bun test`

- [ ] **Step 4: Commit**

```bash
 git add plugin/src/main.ts internal/bridge.go
 git commit -m "feat: integrate batch_execute_pipeline into plugin main router and bridge"
```

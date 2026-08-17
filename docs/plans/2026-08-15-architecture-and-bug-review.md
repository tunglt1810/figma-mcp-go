# Architecture & Bug Review — figma-mcp-go

**Date:** 2026-08-15
**Scope:** the whole Go server (`internal/`, `cmd/`) plus the Figma plugin (`plugin/src/`)
**Status:** research only — this document exists to choose work packages from. The body is kept as written on the day of the survey; see the table below for what has since been done.

### Shipped from this report

| Package | Result |
|---|---|
| **G1** — P0-1, P0-2, P0-3 | Pipeline rollback snapshots the right target node, restores properties, and returns `results` on failure (P2-11 goes with it) |
| **G2** — P1-5, P1-6, P1-7 | Validation moved into `Node.Send`; one timeout table for bridge/follower/progress; hex colors validated in both Go and the plugin |
| **G3** — P2-8 → P2-16 | `gofmt`/`go vet` in CI; mixed fills/strokes; `$variable` matches identifiers only; node IDs normalized through the whole param tree; exports can overwrite; WebSocket Origin checked; `BatchPipeline*` dead code deleted |
| **G4** | Tool surface Tier 1: eight node-property tools merged into `set_node_properties` (breaking — the plugin must be reinstalled) |
| **G5** | Tool surface Tier 2: shapes 7→1, paint 3→1, styles 4→1, pages 4→1. 77 → 63 tools |
| **G6** | Declarative tool table — every tool declares itself in a `toolSpec`; `ValidateRPC` is now a table lookup. P1-4 (`steps` declared with the wrong type) goes with it |
| **G7** — B3 | **Declined** — leader/follower stays; see below |
| **G8** — B4 + B5 | Bridge pings every 20s and drops a connection that stops answering; plugin dispatch is a map, so a duplicate tool name is an error at load time |

No bug on this list is still open.

**B5 was done, but the reason given in this report is wrong.** Performance was never the
problem: ten `switch` statements over a string are nanoseconds, and the round-trip is
milliseconds. What the map actually buys is (a) a duplicate tool name across two modules
throwing at load time instead of "whichever module comes first wins", and (b) one place to
look a name up. The Go-table-vs-plugin-handler test (`tools_plugin_test.go`) is kept
alongside it.

**On the warning in the Tier 2 section** ("merge too hard and the LLM picks the wrong
parameter more often than it picks the wrong tool"): the warning is right, so each merged
tool has a `requireVariant` — a parameter belonging to a different variant is **rejected by
name, along with the variant's name**, rather than dropped silently. A hard-to-see failure
traded for one that says what it is.

**G7 — decision: not done, leader/follower stays.** The reasoning in B3 has weakened
considerably: both bug sources it cites (P1-5 divergent validation, P1-6 divergent
timeouts) are fixed, so what remains is ~500 lines that work.

Two topologies exist side by side and do different things:

- **Several clients → one Figma file**, no configuration: the first process to take 1994
  holds the WebSocket, the rest proxy through `/rpc`. This is leader/follower.
- **Several clients → several Figma files**: one `--port` per client, each plugin instance
  pointed at its own port. No follower runs at all.

The second was always possible but had never been written down anywhere; it is now in the
README. Removing leader/follower would take away the first — the zero-config case of two
terminals acting on one file — for a package that is already published on npm. Keep it.

---

## 0. Baseline numbers

| Metric | Value | Measured with |
|---|---|---|
| Go LOC (excluding tests) | ~4,900 | `wc -l` |
| Plugin TS LOC (excluding tests) | ~2,400 | `wc -l` |
| MCP tools | 84 | `tools_schema_test.go` |
| `tools/list` payload | **58,931 bytes ≈ 14.7k tokens** | `HandleMessage` + `json.Marshal` |
| Validation lines (`schema.go`) | 940 | runs on only one of the two code paths (see P1-5) |
| `go test ./...` | PASS | — |
| `gofmt -l .` | **5 files misformatted** | election.go, schema.go, schema_test.go, tools_write.go, tools_write_components.go |

14.7k tokens of schema is ~7% of a 200k context window, paid in **every** session, before
any work happens.

---

## PART A — BUGS

### 🔴 P0-1 — `batch_execute_pipeline` rollback deletes the user's existing nodes (data loss)

**Location:** `plugin/src/batch-pipeline.ts:99-101`

```ts
if (res && res.id) {
  walStack.push({ type: 'CREATE', nodeId: res.id });
}
```

The WAL treats **every** result carrying an `.id` as a node that was just created. But
plenty of handlers return the `id` of a node that **already existed**.

**Reachable repro today:**

```json
{"steps":[
  {"id":"s1","action":"rename_page","params":{"pageName":"Home","newName":"Landing"}},
  {"id":"s2","action":"create_frame","params":{"parentId":"$does_not_exist"}}
]}
```

- `rename_page` returns `data: { id: page.id, ... }` (`plugin/src/write-page.ts:71`) — the id of the **user's real page**
- s2 fails → `executeRollback` calls `page.remove()`
- → **the whole page and everything on it is gone for good.**

Once P0-2 (wiring `nodeIds`) is fixed, this hole widens to another ~20 handlers:
`set_fills`, `set_text`, `set_strokes`, `rename_node`, `set_auto_layout`… all of which
return the `id` of a node that already existed.

**Fix:** push `CREATE` only when the action is on an allow-list of `create_*` /
`clone_node` / `add_page` / `import_image`, or let the handler declare
`data.__created = true` itself. The latter is safer because it does not depend on a naming
convention.

---

### 🔴 P0-2 — The pipeline does not pass `nodeIds` → ~34 of 57 write tools are unusable

**Location:** `plugin/src/batch-pipeline.ts:155`

```ts
const subReq = { type: action, requestId: `${request.requestId}_${action}`, params };
```

There is no `nodeIds` field. But handlers read it like this
(`plugin/src/write-modify.ts:8`):

```ts
const nodeId = request.nodeIds && request.nodeIds[0];
if (!nodeId) throw new Error("nodeId is required");
```

**Uses of `request.nodeIds` in the plugin's write handlers:**

| File | Uses | State inside a pipeline |
|---|---|---|
| `write-modify.ts` | 20 | ❌ all broken |
| `write-components.ts` | 8 | ❌ mostly broken |
| `write-styles.ts` | 3 | ❌ `apply_style_to_node`, `set_effects`, `bind_variable_to_node` |
| `write-prototype.ts` | 2 | ❌ broken |
| `write-create.ts` | 1 | ❌ `create_component` |
| `write-page.ts`, `write-variables.ts` | 0 | ✅ work |

So the pipeline **can only create nodes, not modify any**. The flagship "transactional
mutation" feature works at half strength.

**Why the tests miss it:** `batch-pipeline.test.ts` only exercises `executeBatchPipeline`
with a hand-written `mockDispatcher` — it never runs through
`handleBatchPipelineRequest` → `dispatchSingle`, which is exactly where `nodeIds` is lost.

**Fix:**
```ts
const dispatcher = async (action: string, params: any) => {
  const { nodeId, nodeIds, ...rest } = params ?? {};
  const ids = nodeIds ?? (nodeId ? [nodeId] : undefined);
  const subReq = { type: action, requestId: `${request.requestId}_${action}`, nodeIds: ids, params: rest };
  ...
};
```
plus an integration test that goes through the real `handleWriteRequest`.

---

### 🔴 P0-3 — WAL `MODIFY` is unimplemented, contradicting its own design doc

**Location:** `plugin/src/batch-pipeline.ts:32-52`

`LogEntry` declares `MODIFY` with a `previousState`, but:
- nothing ever pushes a `MODIFY` entry
- `executeRollback` only has the `if (entry.type === 'CREATE')` branch

The design doc `docs/specs/2026-08-01-batch-execute-pipeline-design.md:142,148` specifies
it clearly: snapshot `fills/strokes/x/y/width/height/characters` before a MODIFY, roll back
with `restoreNodeProperties()`. That part was never written.

**Consequence:** the tool description says *"transactional … with rollback support"* while
in reality every change to an existing node is **not undoable**. The LLM believes the
description and uses the pipeline for dangerous operations.

**Fix:** either implement the MODIFY snapshot as specified, or change the description to
*"rollback only removes newly created nodes; changes to existing nodes are not undone"*.
The latter is cheap and honest, so do it immediately alongside P0-1.

---

### 🟠 P1-4 — The `steps` schema declares the wrong type, and is not `required`

**Location:** `internal/tools_write.go:25`

The schema the server actually emits:
```json
"steps": { "type": "object", "properties": {}, "description": "Array of pipeline steps to execute in sequence" }
```

Three problems at once:
1. `type: "object"`, but the plugin iterates `req.steps[i]` → it needs an **array**. Any MCP client that validates strictly will reject it.
2. `properties: {}` — the LLM gets no information at all about the shape of a step (`id`/`action`/`params`/`export_vars`).
3. `required: []` — `steps` is optional.

**Fix:** `mcp.WithArray("steps", mcp.Required(), mcp.WithObjectItems(...))` describing the
full step shape. Add a `batch_execute_pipeline` case to `ValidateRPC` (there is none today).

---

### 🟠 P1-5 — 940 lines of validation run only on the secondary code path

**Location:** `internal/leader.go:127` — the **only** call site of `ValidateRPC`.

The real flow (`internal/node.go:75-78`):

```
MCP client → tool handler → node.Send()
                              ├── role == LEADER   → bridge.Send()        ← NO validation
                              └── role == FOLLOWER → follower.Send()
                                                      → leader /rpc → ValidateRPC ← validated
```

The first process to start is always the **leader**. Which means that for the vast majority
of users (one MCP client), **all of `schema.go` is dead code**.

**Concrete consequence:** the same `set_opacity(opacity=5)` call
- as leader → sent straight to Figma → an incomprehensible error from the plugin API, or a silent wrong result
- as follower → `"opacity must be between 0 and 1"`

Behavior differs depending on which process started first — extremely hard to debug.

**Fix:** move `ValidateRPC` to the top of `Node.Send()`. Leave the leader's `/rpc` check in
place (defense in depth for input off the network). One line of code reclaims 940 lines of
logic that is currently idle.

---

### 🟠 P1-6 — Inconsistent timeout scale → `get_document` / pipelines always fail on a follower

| Where | Timeout |
|---|---|
| `bridge.go:193` default | 30s |
| `bridge.go:194` `get_document` | 60s |
| `bridge.go:196` `batch_execute_pipeline` | 120s |
| `bridge.go:110` reset on progress | **hardcoded 60s** |
| `follower.go:29` HTTP client | **hardcoded 35s** |

Two bugs:

1. **A follower can never wait out a long tool.** A large file needs 45s → the leader is
   still waiting while the follower gave up at 35s. `batch_execute_pipeline` (120s) has an
   effective ceiling of 35s through a follower.
2. **A progress update shortens the pipeline's timeout.**
   `entry.timer.Reset(60 * time.Second)` — one progress update at second 10 of a 120s
   pipeline lowers the ceiling to 70s. The mechanism meant to extend the timeout does the
   opposite.

The comment at `follower.go:28` (`35s > 30s bridge timeout`) was true when written, but went
stale once the two special timeouts were added.

**Fix:** one shared timeout table, with `follower.client.Timeout` derived from it
(`timeoutFor(tool) + 5s`), and the progress reset using `timeoutFor(tool)` rather than a
hardcoded 60s. Add an overall ceiling so a plugin sending progress forever cannot keep a
request alive indefinitely.

---

### 🟠 P1-7 — No layer validates hex colors

**Location:** `plugin/src/write-helpers.ts:5-13`

```ts
export const hexToRgb = (hex: string) => {
  const clean = hex.replace("#", "");
  return { r: parseInt(clean.slice(0,2),16)/255, ... };
};
```

| Input | Result |
|---|---|
| `#f00` (3-character shorthand) | `b = parseInt("", 16)/255 = NaN` → broken fill, **no error** |
| `red` | `{r: NaN, g: NaN, b: NaN}` |
| `rgb(255,0,0)` | NaN |

`schema.go:363` only checks `color != ""`. LLMs very often emit `#f00` or a color name.

**Fix:** validate and expand the shorthand in Go
(`^#?([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`), returning a clear error instead of
a silent NaN. Apply it to `set_fills`, `set_strokes`, `create_paint_style`, and
`set_gradient_fills.stops[].color`.

---

### 🟡 P2-8 — `set_fills` with mode=`append` breaks when `fills` is `figma.mixed`

`plugin/src/write-modify.ts:56-58` spreads `(node as any).fills` directly. If the node has
mixed fills, `fills` is a `symbol` → `TypeError: fills is not iterable`.

`set_gradient_fills:34` already has an `Array.isArray(...)` guard. Two handlers of the same
kind, inconsistent with each other.

---

### 🟡 P2-9 — `resolveParams` swallows every string starting with `$`

`plugin/src/batch-pipeline.ts:11-16`: `if (params.startsWith('$'))` → throws if the string
is not in the symbol table.

`create_text(text: "$100")` inside a pipeline → `Error: Undefined pipeline variable: $100`.
Prices, CSS variables, and template strings all hit it.

**Fix:** accept only the pattern `^\$[A-Za-z_][A-Za-z0-9_]*$`, and support `$$` → `$` as an
escape.

---

### 🟡 P2-10 — The WebSocket disables the Origin check entirely

`internal/bridge.go:52`: `InsecureSkipVerify: true`.

Combined with `bridge.go:64-70` (a new connection **replaces** the old one), any web page
the user happens to have open can:
1. `new WebSocket("ws://127.0.0.1:1994/ws")` — WS is not blocked by CORS
2. kick the real plugin off
3. receive every subsequent tool request and return fabricated data

Low severity (it needs the user to be running the server and to open a malicious page), but
the fix costs almost nothing.

**Fix:** an `OriginPatterns` allow-list instead of skipping the check; the Figma plugin
iframe sends `Origin: null` or `https://www.figma.com`.

---

### 🟡 P2-11..16 — Small items

| # | Problem | Location |
|---|---|---|
| 11 | A failed pipeline loses `results`, so the caller cannot tell which steps ran | `batch-pipeline.ts:120-131` |
| 12 | `Node.Send` mutates the caller's slice/map (side effect) | `node.go:63-71` |
| 13 | `NormalizeNodeID` only applies to top-level `nodeId`/`parentId` — missing `componentId`, `startNodeId`, `endNodeId`, and **every param nested inside a pipeline step** | `node.go:66-71` |
| 14 | 5 files fail `gofmt`; CI runs only `test` + `build`, with no `gofmt -l` / `go vet` | `.github/workflows/ci.yml` |
| 15 | `save_screenshots` uses `O_EXCL` → it can never overwrite, so the user has to delete the file by hand before every re-capture | `tools.go:223` |
| 16 | Dead code: `BatchPipelineRequest/Step/Response` are declared in Go but used by nobody (the plugin has its own TS version) | `schema.go:1091-1110` |

---

## PART B — ARCHITECTURAL SIMPLIFICATION

### B1. 84 boilerplate handlers → one declarative table

Every tool today is hand-written to the same ~25-40 line pattern:

```go
s.AddTool(mcp.NewTool("set_strokes",
    mcp.WithDescription(...), mcp.WithString("nodeId", ...), mcp.WithString("color", ...), ...
), func(ctx, req) (*mcp.CallToolResult, error) {
    nodeID, _ := req.GetArguments()["nodeId"].(string)
    params := map[string]interface{}{"color": req.GetArguments()["color"]}
    if sw, ok := req.GetArguments()["strokeWeight"].(float64); ok { params["strokeWeight"] = sw }
    ...
    resp, err := node.Send(ctx, "set_strokes", []string{nodeID}, params)
    return renderResponse(resp, err)
})
```

That is ~1,400 lines across eight `tools_*.go` files with **not one line of distinct logic**
in them — just copying arguments into a map.

**Proposal:**
```go
type toolSpec struct {
    Name, Desc  string
    NodeIDsFrom string          // "nodeId" | "nodeIds" | ""
    Params      []paramSpec     // name, type, required, desc, enum
}
```
One generic handler reads `Params`, one loop registers them. Estimated **~70% less Go
code**, and more importantly: it opens the way to B2.

### B2. One source of truth for a tool's contract

A tool is currently described in **four independent places**:

| Place | File |
|---|---|
| MCP JSON Schema | `internal/tools_*.go` |
| Runtime validation | `internal/schema.go` |
| Implementation | `plugin/src/write-*.ts` (switch-case) |
| Documentation | `README.md`, `docs/specs/` |

**Bugs P1-4, P1-5 and P1-7 are all direct consequences of these four drifting apart.**
Nothing detects the drift — `tools_schema_test.go` only counts tools and checks
`items.type`.

**Proposal:** a single `tools.yaml` that generates (a) the Go registration, (b) the Go
validator, (c) the TS dispatch type. Schema drift becomes a compile error instead of a
runtime error on the user's machine.

### B3. Leader/Follower — consider removing or inverting

Current cost: `election.go` + `leader.go` + `follower.go` + `node.go` + the RPC types ≈
**500 lines**, and more importantly **two parallel code paths** — precisely the source of
P1-5 (divergent validation) and P1-6 (divergent timeouts).

The question to answer before touching it: **how many users actually run several MCP
clients at once?** If few:

- **Option A (remove):** one process, one bridge, `EADDRINUSE` → a clear "an instance is
  already running" error. Deletes ~500 lines and one code path.
- **Option B (invert, recommended if multi-client is needed):** split out a
  `figma-mcp-go serve` daemon that holds the bridge; every stdio process becomes a pure
  follower. **Only one code path** for a tool call → P1-5 and P1-6 disappear structurally,
  with no manual fix.

### B4. The bridge needs a keepalive and a request ceiling

No ping/pong → a connection dies silently and is only noticed when the first request times
out (30s). Add `conn.Ping()` every 20s. And put an overall ceiling on a request's lifetime
so progress updates cannot hold it open forever (see P1-6).

### B5. Plugin: a 7-layer dispatch chain → a map

`plugin/src/main.ts:24`:
```ts
(await handleReadRequest(request)) ?? (await handleWriteRequest(request))
```
Every **write** request has to pass through three read handlers first. Then
`write-handlers.ts:12-18` chains seven more, each one a large `switch`.

Replace with `Record<string, Handler>` — O(1), and a duplicate tool name becomes a build
error instead of "whichever handler comes first wins".

---

## PART C — SHRINKING THE TOOL SURFACE

**Current cost: 58,931 bytes ≈ 14.7k tokens per session.**

Beyond the context cost, a large tool count also degrades the model's tool-selection
accuracy — 84 choices with many near-synonymous pairs.

### Tier 1 — Safe merges (same shape, low risk): **84 → 72**

| Merge | Current tools | Becomes |
|---|---|---|
| Node properties (8→1) | `set_visible`, `lock_nodes`, `unlock_nodes`, `rotate_nodes`, `reorder_nodes`, `set_blend_mode`, `set_constraints`, `set_opacity` | `set_node_properties(nodeIds, {visible?, locked?, rotation?, order?, blendMode?, constraints?, opacity?})` |
| Geometry (2→1) | `move_nodes`, `resize_nodes` | `transform_nodes(nodeIds, {x?, y?, width?, height?})` |
| Scan (2→1) | `scan_text_nodes` | drop — exactly `scan_nodes_by_types(['TEXT'])`, and **its own description already admits it is a "shorthand"** |
| Node read (2→1) | `get_node` | drop — exactly `get_nodes_info([id])`, and the description already recommends the other one |
| Reactions (2→1) | `remove_reactions` | drop — exactly `set_reactions(mode='replace', reactions=[])` |
| Annotations (2→1) | `clear_annotations` | drop — exactly `set_annotations([])` |

All eight tools in the first group share the `nodeIds[] + one property` shape, so merging
does not blur any meaning. The other four groups are **purely redundant tools being
deleted**, with no capability lost.

Estimate: **-12 tools, ~3.5k tokens saved.**

### Tier 2 — Aggressive merges (needs thought): **72 → 58**

| Merge | Tools | Becomes | Risk |
|---|---|---|---|
| Shapes (7→1) | `create_rectangle/ellipse/star/polygon/line/frame/section` | `create_node(type, ...)` | Medium — the params genuinely differ (`radius` vs `width/height` vs `pointCount`), so the schema becomes a loose union |
| Paint (3→1) | `set_fills`, `set_gradient_fills`, `set_strokes` | `set_paint(nodeId, target, paint)` | Medium |
| Styles (4→1) | `create_paint/text/effect/grid_style` | `create_style(type, ...)` | Low — already the same pattern |
| Pages (4→1) | `add/delete/rename/navigate_to_page` | `manage_page(action, ...)` | Low |

Further estimate: **-14 tools, ~4k tokens saved.** That leaves ~7k tokens of schema (a 52%
reduction).

**The trade-off to be aware of:** merge too hard and the LLM picks the wrong parameter more
often than it picks the wrong tool. The shape group (7→1) is the riskiest, because the
params really are different. Recommendation: do Tier 1 first, re-measure quality, then
decide on Tier 2.

**Breaking change:** both tiers break existing user prompts and workflows. Bundle them into
one major version and keep deprecated aliases for a release or two.

---

## PART D — WORK PACKAGE MENU

| Package | Contents | Estimate | Breaking | Priority |
|---|---|---|---|---|
| **G1** | P0-1, P0-2, P0-3 — fix the data loss, wire `nodeIds`, add a real integration test for the pipeline | ~1 day | No | 🔴 Do now |
| **G2** | P1-4, P1-5, P1-6, P1-7 — the `steps` schema, validation into `Node.Send`, unified timeouts, hex validation | ~1 day | No | 🟠 High |
| **G3** | P2-8 → P2-16 — clear the small bugs, add `gofmt`/`go vet` to CI, delete dead code | ~0.5 day | No | 🟡 Medium |
| **G4** | Tool surface Tier 1 (84→72) | ~1-2 days | **Yes** | 🟡 Medium |
| **G5** | Tool surface Tier 2 (72→58) | ~2-3 days | **Yes** | ⚪ Low |
| **G6** | B1 + B2 — declarative tool table plus a single source of truth | ~3-4 days | No (internal) | 🟠 High (stops the bugs recurring) |
| **G7** | B3 — simplify leader/follower | ~2 days | Possibly (CLI flag) | 🟡 Medium |
| **G8** | B4 + B5 — bridge keepalive plus plugin dispatch map | ~0.5 day | No | 🟡 Medium |

### Suggested order

**For the best value per unit of effort:** G1 → G2 → G3 (2.5 days, nothing breaking, every
verified bug handled).

**To fix the root cause:** G1 → G6 → G4. Doing G6 before G4 turns tool merging into a
config-file edit rather than a rewrite of eight Go files — and stops the "four sources
drifting apart" class of bug from recurring.

**If context cost is the priority:** G4 first (an immediate -3.5k tokens/session), but most
of it will have to be redone once G6 lands.

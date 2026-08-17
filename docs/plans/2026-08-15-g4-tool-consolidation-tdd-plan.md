# G4 — Tool Consolidation Tier 1 (TDD Plan)

**Date:** 2026-08-15
**Decision taken:** a new plugin handler plus a fanout fallback in Go; the old tool names are removed from the MCP schema entirely (their wire handlers stay in the plugin).
**Prerequisite:** [Architecture & Bug Review](./2026-08-15-architecture-and-bug-review.md)

---

## 0. CORRECTIONS to the earlier report

While verifying the code to write this plan, **all four tools I filed under "purely
redundant, nothing lost" turned out to be wrong.** In detail:

| Tool | What the old report said | The truth (verified) |
|---|---|---|
| `scan_text_nodes` | = `scan_nodes_by_types(['TEXT'])` | ❌ **Flatly wrong.** `scan_nodes_by_types` returns `{id,name,type,bbox}` — **no `characters`**. It also has `if (!n.visible) return`, skipping hidden nodes, which `scan_text_nodes` does not. The two return completely different data. |
| `get_node` | = `get_nodes_info([id])` | ⚠️ Same `serializeNode`, but `get_node` **throws** when the node is missing while `get_nodes_info` **silently filters it out** (`read-document.ts:76`). Deleting it loses a diagnostic. |
| `remove_reactions` | = `set_reactions(replace, [])` | ⚠️ Only true when removing everything. With `indices: [1,3]` (removing specific reactions) there is **no replacement** short of a get→filter→set round trip. |
| `clear_annotations` | = `set_annotations([])` | ⚠️ `clear_annotations` takes **`nodeIds[]`, many nodes**; `set_annotations` takes only **one `nodeId`**. Clearing 10 nodes becomes 10 calls. |

Root cause of the mistake: I trusted the tools' own descriptions (`scan_text_nodes`
describes itself as *"Shorthand for scan_nodes_by_types with ['TEXT']"*) instead of reading
the implementation. That description is **wrong** and gets fixed as part of this plan.

### The token numbers were wrong too

Measured per tool (`tools/list` = 58,802 bytes):

| Group | Bytes today | After merging (estimate) | Saved |
|---|---|---|---|
| 8 node-property tools | 4,070 | ~1,100 (`set_node_properties`) | **2,970** |
| `move_nodes` + `resize_nodes` | 1,176 | ~700 (`transform_nodes`) | **476** |
| `remove_reactions` | 636 | 0 (+150 into `set_reactions`) | **486** |
| `clear_annotations` | 376 | 0 (+120 into `set_annotations`) | **256** |
| `get_node` | 495 | 0 (+80 into `get_nodes_info`) | **415** |
| `scan_text_nodes` | 487 | **kept as-is** | 0 |
| | | **Total** | **~4,600 bytes ≈ 1,150 tokens** |

**The old report claimed "-12 tools, ~3.5k tokens" — the reality is -11 tools, ~1.15k
tokens** (7.8% of the schema, ~0.6% of a 200k window). Why: this group of tools is smaller
than average (~500 vs 700 bytes/tool), and the three survivors have to grow to keep their
capabilities.

### Worth reconsidering before starting

If your main motivation is **cutting tokens**, G4 is not worth 2-3 days for 1.15k tokens.
Most of the saving sits in exactly **one** item: merging the eight node-property tools
(2,970 bytes = 65% of the total benefit, and technically the cleanest part).

Three routes:

- **G4-mini** — Phase 1+2 only (node properties + transform). -8 tools, ~3.4k bytes, ~1 day, no regressions at all. **The best benefit-to-effort ratio.**
- **Full G4** — the plan below. -11 tools, ~4.6k bytes, 2-3 days.
- **Switch to Tier 2** — those tools are much larger (`create_*`, 7 tools; `create_*_style`, 4 tools). The saving is considerably bigger, but the risk of the LLM picking the wrong parameter is higher.

The plan below is written for **full G4**; phases 1-2 stand alone, so cutting down to
G4-mini just means stopping after Phase 2.

---

## 1. Scope, settled

| Phase | Change | Δ tools |
|---|---|---|
| 1 | `set_visible`, `lock_nodes`, `unlock_nodes`, `rotate_nodes`, `reorder_nodes`, `set_blend_mode`, `set_constraints`, `set_opacity` → **`set_node_properties`** | −7 |
| 2 | `move_nodes`, `resize_nodes` → **`transform_nodes`** | −1 |
| 3 | `remove_reactions` → folded into `set_reactions` via `removeIndices` | −1 |
| 4 | `clear_annotations` → `set_annotations` accepts `nodeIds[]` | −1 |
| 5 | `get_node` → `get_nodes_info` reports missing ids explicitly | −1 |
| 6 | `scan_text_nodes` **kept**, only its incorrect description fixed | 0 |
| | **84 → 73** | **−11** |

**Principle running through all of it:** no phase may lose a capability. Everything that
was possible before must remain possible after, in exactly **one** tool call.

---

## 2. Technical prerequisite — a test seam for the fanout

The fanout fallback cannot be tested without a separable send layer. Today `Node.Send`
calls `*Bridge` / `*Follower` directly (`node.go:75-78`).

**Phase 0 has to come first:**

```go
type sender interface {
    Send(ctx context.Context, tool string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error)
}
```

`Node` holds a `sender`, and tests inject a fake. `*Bridge` and `*Follower` already have
exactly this signature → nothing on either side needs changing.

Bonus: this seam is also what **P1-5** needs (moving `ValidateRPC` into `Node.Send`) in the
later G2 package.

---

## 3. The fallback mechanism

An old plugin does not know `set_node_properties` → `main.ts:26` throws
`Unknown request type: set_node_properties` → which arrives back in Go as `resp.Error`.

```go
const unknownTypePrefix = "Unknown request type"

func sendWithFanout(ctx, s sender, modern string, nodeIDs []string,
                    params map[string]any, legacy []legacyCall) (BridgeResponse, error) {
    resp, err := s.Send(ctx, modern, nodeIDs, params)
    if err != nil || !strings.HasPrefix(resp.Error, unknownTypePrefix) {
        return resp, err          // new plugin, or a real error → pass it straight back
    }
    return fanout(ctx, s, nodeIDs, legacy)  // old plugin
}
```

**A trade-off that has to be documented for users:** the fanout path issues N separate
commands → **N undo entries** (Ctrl+Z has to be pressed N times) and it is not atomic (if
the second command fails, the first has already been applied). A new plugin does not have
this problem. → add a line to the README recommending a plugin update.

Matching on a string prefix is fragile. It is acceptable here because that string is
produced by **this repository itself** (`main.ts:26`), not by the Figma API. Add a test that
pins the string on both sides so nobody changes one without the other.

---

## 4. TDD — Phase 0: a golden tool set

Replace `TestToolSchemas_AllToolsRegistered` (which asserts `const want = 84`) with an
assertion on the **list of names**, so each phase gets a precise RED→GREEN signal instead of
just a count.

**RED** — write the test with the current 84 names → it must go GREEN immediately (this is
the baseline, nothing has changed yet):

```go
// internal/tools_schema_test.go
var expectedTools = []string{ /* 84 names, pre-sorted */ }

func TestToolSchemas_ExpectedToolSet(t *testing.T) {
    got := toolNames(listTools(t))   // sorted
    if diff := cmp.Diff(expectedTools, got); diff != "" {
        t.Errorf("tool set changed unexpectedly (-want +got):\n%s", diff)
    }
}
```

Every subsequent phase: edit `expectedTools` **first** → RED → implement → GREEN.

Keep `TestToolSchemas_ArrayItemsHaveType` as well (it guards against the Copilot validation
bug) — the new tool has a `nodeIds` array, so this test has to cover it.

---

## 5. Phase 1 — `set_node_properties` (8 → 1)

This is the largest phase and 65% of the benefit. The eight existing plugin handlers have
**identical skeletons** (verified at `write-modify.ts:157-320`):

```
for nid of nodeIds:
  n = await getNodeByIdAsync(nid)
  if !n              → results.push({nodeId, error: "Node not found"});     continue
  if !(prop in n)    → results.push({nodeId, error: "does not support X"}); continue
  apply prop
  results.push({nodeId, <prop>: value})
commitUndo()
```

→ merging them into one loop that applies several properties is a pure transformation, with
no logic lost.

### Settled shape

```jsonc
// request
{ "nodeIds": ["1:1","2:2"],
  "visible": true, "locked": false, "opacity": 0.5, "rotation": 45,
  "order": "bringToFront", "blendMode": "MULTIPLY",
  "constraints": { "horizontal": "STRETCH", "vertical": "MIN" } }

// response — errors are per-property, not per-node
{ "results": [
    { "nodeId": "1:1", "applied": { "opacity": 0.5, "visible": true } },
    { "nodeId": "2:2", "applied": { "opacity": 0.5 },
      "errors": { "rotation": "Node does not support rotation" } },
    { "nodeId": "9:9", "error": "Node not found" }
] }
```

Why errors are per-property: a node may support `opacity` but not `rotation`. Merging and
then reporting the error at node level would **lose information** compared with the eight
old tools.

### RED — Go

```
internal/tools_schema_test.go
  ✎ expectedTools: drop 8 names, add "set_node_properties"

internal/tools_handler_test.go
  + TestSetNodeProperties_Schema
      - nodeIds required, type array, items.type == "string"
      - all 7 optionals present: visible/locked/opacity/rotation/order/blendMode/constraints
      - constraints is an object with horizontal + vertical

internal/schema_test.go
  + TestValidateRPC_SetNodeProperties  (table-driven)
      - empty nodeIds                         → "nodeIds is required"
      - no property passed at all             → "at least one property is required"
      - opacity = 5                           → "opacity must be between 0 and 1"
      - opacity = 0 and 1                     → valid (boundaries)
      - blendMode = "NEON"                    → invalid
      - order = "bringToMiddle"               → invalid
      - constraints.horizontal = "MIDDLE"     → invalid
      - malformed nodeId                      → invalid
      - all 7 properties, valid               → ""

internal/fanout_test.go            (new — needs the Phase 0 seam)
  + TestFanout_ModernPluginSingleCall
      fake sender returns OK → exactly 1 call, tool == "set_node_properties"
  + TestFanout_LegacyPluginFansOut
      fake returns Error "Unknown request type: set_node_properties"
      → 3 follow-up calls: set_visible / set_opacity / set_blend_mode
      → results merged into the same shape as the modern path
  + TestFanout_RealErrorNotRetried
      fake returns Error "Node not found" → NO fanout, returned directly (no retry storm)
  + TestFanout_PreservesNodeIDs
```

### RED — Plugin

```
plugin/src/write-modify.test.ts
  + describe("set_node_properties")
      - several properties in one call, exactly one commitUndo
      - node without rotation support → errors.rotation, opacity still applied
      - missing node                  → results[].error == "Node not found"
      - empty nodeIds                 → throws "nodeIds is required"
      - order = "bringToFront" moves to the right index (mock parent.children)
      - constraints merge with the previous value, leaving the untouched axis alone
      - no property passed → throws
```

The `constraints` merge test matters: the old handler (`write-modify.ts:310-313`) does
`{...n.constraints}` and then overwrites only the axis that was passed. That behavior has to
be preserved exactly.

### GREEN
- `plugin/src/write-modify.ts`: add `case "set_node_properties"`, **keeping all 8 old cases** (the fanout needs them).
- `internal/tools_write_nodeprops.go` (new): tool registration plus the `legacyCall` table.
- `internal/fanout.go` (new): `sendWithFanout` and `fanout`.
- `internal/schema.go`: add the `set_node_properties` case.

### REFACTOR
- Delete the 8 registrations in `tools_write_modify.go` and the 8 cases in `schema.go`.
- Do **not** delete the 8 plugin cases.
- Run `gofmt -w` (5 files are already misformatted — see P2-14).

---

## 6. Phase 2 — `transform_nodes` (2 → 1)

Same mould as Phase 1, smaller. Preserve the **exact semantics** of `move_nodes` (its
current description says explicitly *"not a relative offset"*) — do not add a relative mode,
that is a different scope.

```jsonc
{ "nodeIds": ["1:1"], "x": 100, "y": 200, "width": 300, "height": 400 }
```

### RED
```
Go   ✎ expectedTools: −move_nodes −resize_nodes +transform_nodes
Go   + TestValidateRPC_TransformNodes
        - none of x/y/width/height given → "at least one of x, y, width, or height is required"
        - width <= 0                     → invalid
        - height <= 0                    → invalid
        - x only                         → valid
Go   + TestFanout_TransformNodes → legacy fanout produces move_nodes + resize_nodes
TS   + describe("transform_nodes")
        - x/y only    → resize() is not called
        - width only  → resize(w, n.height), height preserved
        - node without "resize" → errors.resize, x/y still applied
        - exactly one commitUndo for move + resize together   ← the old path cost two
```

That last test is the visible, real benefit: repositioning and resizing is now **one** undo
entry instead of two.

---

## 7. Phase 3 — `set_reactions` absorbs `remove_reactions`

Add `removeIndices` to keep the ability to remove by index — something
`set_reactions(replace, [])` **cannot** do.

```jsonc
{ "nodeId": "1:1", "removeIndices": [1, 3] }   // remove reactions #1 and #3
{ "nodeId": "1:1", "reactions": [...], "mode": "append" }
```

### RED
```
Go  ✎ expectedTools: −remove_reactions
Go  + TestValidateRPC_SetReactions_RemoveIndices
       - both reactions and removeIndices    → "reactions and removeIndices are mutually exclusive"
       - neither one                         → "reactions or removeIndices is required"
       - removeIndices with a non-number     → invalid
       - negative removeIndices              → invalid   (new constraint; the old code did not check)
       - removeIndices: []                   → valid, meaning remove everything  ← preserves old behavior
TS  + set_reactions with removeIndices [1,3] → indices 0 and 2 remain
    + removeIndices: [] → removes everything (matches write-prototype.ts:69-73)
    + out-of-range removeIndices → ignored, no throw
    + reactions still behaves as before (regression)
```

Note the old quirk that has to be preserved: an **empty** `indices` means *remove
everything*, not *remove nothing* (`write-prototype.ts:70-73`). Easy to break in a rewrite.

---

## 8. Phase 4 — `set_annotations` takes several nodes

`nodeId` (one node) → `nodeIds[]` (many). `annotations: []` replaces `clear_annotations`.

**Breaking response shape:** `set_annotations` currently returns `{id, success}`; it becomes
`{results:[...]}` like every other multi-node tool. This must go in the migration note.

### RED
```
Go  ✎ expectedTools: −clear_annotations
Go  + TestValidateRPC_SetAnnotations
       - empty nodeIds            → invalid
       - annotations missing      → "annotations array is required"
       - annotations: []          → valid (the clear path)
       - 1 of 3 ids malformed     → invalid, naming which id
TS  + set_annotations across several nodes → one results[] entry per node
    + annotations: [] clears across several nodes
    + node without annotation support → results[].error, the others still run
```

Note: `tools_schema_test.go:67` currently **excludes** `set_annotations` from the
`items.type` check. Once the schema is fixed, drop that exclusion — treat it as paying off a
small debt.

---

## 9. Phase 5 — `get_nodes_info` reports missing ids

Delete `get_node`, and at the same time **fix a silent bug**: `read-document.ts:76` filters
out nonexistent nodes without saying anything.

```jsonc
// before: [ {...}, {...} ]          ← a wrong id vanishes without trace
// after:  { "nodes": [...], "missing": ["9:9"] }
```

**Breaking response shape** — this is G4's biggest change for existing users. State it
clearly in the migration note.

### RED
```
Go  ✎ expectedTools: −get_node
Go  + TestValidateRPC_GetNodesInfo: one valid id still passes (replacing get_node)
TS  + get_nodes_info returns { nodes, missing }
    + a nonexistent id → lands in missing, does NOT vanish
    + a DOCUMENT node  → filtered out (preserving old behavior)
    + one id           → nodes has exactly one element
    + all ids wrong    → { nodes: [], missing: [all of them] }
```

---

## 10. Phase 6 — Docs & release

1. `README.md` — the tool tables (14 rows at L124-212), plus a **Migration** section mapping old names to new.
2. `README.md` — add a warning: an old plugin still works via the fanout but costs several undo entries, so re-download the zip.
3. `glama.json` — 85 entries, must match the real schema.
4. **Fix `scan_text_nodes`'s incorrect description** — drop *"Shorthand for scan_nodes_by_types with ['TEXT']"* and replace it with the truth: it returns `characters`/`fontSize`/`fontName` and **scans hidden nodes too**, whereas `scan_nodes_by_types` returns no text and skips hidden nodes. This is the exact sentence that misled me in the earlier report.
5. Bump the **major** version (`version.go`, `server.json`, `plugin/package.json`).
6. CI: add `gofmt -l` and `go vet` (P2-14) so this phase does not produce more misformatted files.

---

## 11. Order & estimates

| Phase | Contents | Est. | Cuttable? |
|---|---|---|---|
| 0 | Golden tool set + the `sender` seam | 0.5d | No (foundation) |
| 1 | `set_node_properties` + fanout | 1.0d | No (65% of the benefit) |
| 2 | `transform_nodes` | 0.5d | No |
| 3 | `set_reactions` + `removeIndices` | 0.25d | ✓ |
| 4 | multi-node `set_annotations` | 0.25d | ✓ |
| 5 | `get_nodes_info` + missing | 0.25d | ✓ |
| 6 | Docs, glama, version, CI | 0.5d | No |
| | **Total** | **~3.25d** | **G4-mini = Phase 0+1+2+6 ≈ 2.5d** |

Up from the 1-2 day estimate in the earlier report, because (a) the three surviving tools
have to be modified to keep their capabilities, and (b) the fanout fallback needs a test
seam.

---

## 12. Definition of done

- [ ] `go test ./...` green; `make test-ts` green
- [ ] `gofmt -l .` lists nothing; `go vet ./...` clean
- [ ] `TestToolSchemas_ExpectedToolSet` matches exactly 73 names
- [ ] `TestToolSchemas_ArrayItemsHaveType` green **with no remaining exclusion** for `set_annotations`
- [ ] Fanout tests cover all three branches: new plugin / old plugin / real error
- [ ] `tools/list` re-measured, confirmed at ~54,200 bytes (down from 58,802)
- [ ] Manual smoke test in real Figma: new plugin gives 1 undo entry; the old plugin (previous release zip) still works via the fanout
- [ ] No tool lost a capability — check the §1 table row by row

## 13. Risks

| Risk | Mitigation |
|---|---|
| The `"Unknown request type"` prefix match drifts when someone edits `main.ts` | Pin the string in tests on both sides; declare a constant with a cross-referencing comment |
| N undo entries from the fanout are annoying | State it in the README and prompt a plugin update; the modern path is unaffected |
| Changing `get_nodes_info`'s shape breaks user workflows | Major version bump plus a migration note; this phase is cuttable if it should wait |
| The LLM picks the wrong parameter in a merged tool | Say clearly in the description that each property is optional and independent; keep enums in the schema so the client can validate |
| Merging loses per-property errors | The `applied` + `errors` shape in §5 preserves the granularity of the eight old tools |

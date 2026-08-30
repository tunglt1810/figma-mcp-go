# Tool Consolidation Implementation Plan

**Goal:** Remove 13 tools from the MCP surface by folding each into a tool that
already covers it, without losing a single capability. 78 tools → 65.

**Architecture:** Every merge follows one shape — widen the surviving tool's
spec in `internal/tools/`, teach the plugin handler the capability the removed
tool had, drop the removed tool from `expectedTools`, and regenerate the golden
schema. Handlers that a surviving tool delegates to stay in the plugin and are
listed in `INTERNAL_DISPATCH_TARGETS`; handlers nothing routes to any more are
deleted, because `tool-contract.test.ts` fails on an unreachable handler.

**Tech Stack:** Go 1.27 (`encoding/json/v2`), mcp-go, Bun 1.4 test runner,
TypeScript, Svelte 5.

**Spec:** none — this plan is the record of the analysis. The overlap for each
task was verified against the code, not against the tools' own descriptions.
That distinction matters: `docs/plans/2026-08-15-g4-tool-consolidation-tdd-plan.md`
§0 documents four merges that looked safe from the descriptions and were not.

## Global Constraints

- **No version change.** `plugin/package.json`, `server.json` and `version.go`
  stay at `0.3.1`. Deliberate: the user chose it.
- **No capability may be lost.** Everything possible before must remain
  possible after, in exactly one tool call. Each task states what would have
  been lost and how it is preserved.
- **The golden schema is regenerated, never hand-edited.**
  `go test ./internal/tools/ -run TestToolsSchemaGolden -update` (check the
  flag name in `tools_golden_test.go` before relying on it).
- **`expectedTools` in `tools_schema_test.go` is edited first**, so every task
  starts RED with a precise diff of what moved.
- Test both sides. A Go-only change passes `go test` while the plugin still
  answers the old shape.
- One commit per task.

---

## Task 1: `set_layout_sizing` → `set_auto_layout` over many nodes

**Overlap:** all 10 of `set_layout_sizing`'s params are among
`set_auto_layout`'s 25. The only real difference is arity — `nodeIds` vs
`nodeId`.

**Files:**
- Modify: `internal/tools/tools_write_modify.go` (both specs)
- Modify: `plugin/src/write-modify.ts:380` (`set_auto_layout`)
- Modify: `internal/tools/tools_schema_test.go` (`expectedTools`)
- Test: `plugin/src/write-modify.test.ts`, `internal/tools/validate_test.go`

**Steps:**
- [ ] Drop `set_layout_sizing` from `expectedTools`; run `go test ./internal/tools/` → RED.
- [ ] `set_auto_layout`: `NodeIDs: nodeIDsMulti`, `NodeIDsReq: true`, plural
      `NodeIDDesc`. Fold the "a row of siblings that should all FILL" sentence
      from `set_layout_sizing`'s description into it.
- [ ] Delete the `set_layout_sizing` spec.
- [ ] Plugin: `set_auto_layout` loops `request.nodeIds`, answering
      `{results: [{nodeId, name} | {nodeId, error}]}` — the shape every other
      multi-node tool uses. A node that throws (no auto layout on its parent)
      is reported against itself, leaving the others applied, exactly as
      `set_layout_sizing` did.
- [ ] Keep the `set_layout_sizing` handler and add it to
      `INTERNAL_DISPATCH_TARGETS`: a pipeline step still names it.
- [ ] Tests: several nodes in one call → one `commitUndo`, one entry per node;
      a node whose parent has no auto layout → its own `error`, siblings still
      applied; a missing node → `Node not found`.

**Preserved:** the multi-node sizing pass, now with the frame's own layout
available in the same call.

---

## Task 2: `get_node` → `get_nodes_info` reports what it could not find

**Overlap:** identical — both call `serializeNode`. **What would be lost:**
`get_node` throws on a missing id; `get_nodes_info` filters it out silently
(`read-document.ts:137`). Deleting `get_node` without fixing that turns a typo
into an empty answer with no explanation.

**Files:**
- Modify: `internal/tools/tools_read_document.go`, `tools_schema_test.go`
- Modify: `plugin/src/read-document.ts:86,129`
- Test: `plugin/src/read-search.test.ts`

**Steps:**
- [ ] Drop `get_node` from `expectedTools` → RED.
- [ ] Plugin `get_nodes_info`: collect the ids that resolved to nothing and
      answer `{nodes, missing: [...]}`, omitting `missing` when it is empty.
      A `DOCUMENT` node keeps being filtered without landing in `missing` —
      it was found, it is just not serializable.
- [ ] Delete the `get_node` spec and its plugin handler.
- [ ] Rewrite `get_nodes_info`'s description: it no longer says "prefer this
      over get_node", it says a missing id is reported under `missing`.
- [ ] Tests: one wrong id lands in `missing` and does not vanish; all ids wrong
      → `{nodes: [], missing: [...]}`; nothing missing → no `missing` key.

---

## Task 3: `get_pages` → `get_metadata`

**Overlap:** total. `get_pages` returns `{currentPageId, pages}`;
`get_metadata` returns those plus `fileName`, `currentPageName`, `pageCount`
(`read-document.ts:330` vs `:348`).

**Steps:**
- [ ] Drop `get_pages` from `expectedTools` → RED.
- [ ] Delete the spec and the plugin handler.
- [ ] Extend `get_metadata`'s description to say it lists every page with its
      id and name, so the tool that replaced `get_pages` is findable by the
      same words.
- [ ] Remove `get_pages` from `READ_TOOLS` in `tool-classes.ts`.

---

## Task 4: `scan_nodes_by_types` and `scan_text_nodes` → `search_nodes`

**Overlap:** `scan_nodes_by_types`' params are a subset of `search_nodes`', and
both answer id/name/type/bounds. **What would be lost:** two things, and the
2026-08-15 plan got this wrong by trusting the descriptions —
1. `scan_nodes_by_types` skips hidden nodes (`read-document.ts:544`);
   `search_nodes` does not.
2. `scan_text_nodes` returns `characters`, `fontSize` and `fontName`, which
   `search_nodes` does not return at all.

**Files:**
- Modify: `internal/tools/tools_read_document.go`, `tools_schema_test.go`
- Modify: `plugin/src/read-document.ts:405` (`search_nodes`)
- Modify: `plugin/src/tool-classes.ts` (`READ_TOOLS`)
- Test: `plugin/src/read-search.test.ts`

**Steps:**
- [ ] Drop both names from `expectedTools` → RED.
- [ ] `search_nodes`: `query` becomes optional (the plugin already treats an
      absent query as "match everything" — `read-document.ts:406`). Add:
      - `includeHidden` (bool, **default true**, preserving today's behaviour).
        `false` reproduces `scan_nodes_by_types`.
      - `includeText` (bool, default false). When true, a TEXT hit carries
        `characters`, `fontSize` and `fontName`, reproducing `scan_text_nodes`.
- [ ] Say in the description that `limit` defaults to 50, so a caller
      reproducing an unbounded scan raises it — the old scans had no limit.
      The answer already reports `truncated`.
- [ ] Delete both specs and both plugin handlers.
- [ ] Tests: `includeHidden: false` skips a hidden node and its subtree;
      default includes it; `includeText` puts `characters` on a TEXT hit and
      nothing on a FRAME hit; no `query` matches everything of the given types.

---

## Task 5: `clear_annotations` → `set_annotations` over many nodes

**Overlap:** clearing is `annotations: []`. **What would be lost:** arity —
`clear_annotations` takes `nodeIds[]`, `set_annotations` one `nodeId`. Ten
nodes would become ten calls.

**Files:**
- Modify: `internal/tools/tools_write_components.go:83,94`
- Modify: `plugin/src/write-components.ts:220,242`
- Modify: `tools_schema_test.go`, `plugin/src/tool-classes.ts`

**Steps:**
- [ ] Drop `clear_annotations` from `expectedTools` → RED.
- [ ] `set_annotations`: `nodeIDsMulti`, answering `{results: [...]}` — a
      breaking response-shape change, noted in the README.
- [ ] Plugin: loop, reporting "does not support annotations" per node instead
      of throwing for the whole call.
- [ ] Delete `clear_annotations` — spec and handler.
- [ ] The `set_annotations` exclusion in `TestToolSchemas_ArrayItemsHaveType`
      stays. It is pre-existing debt about `items.type`, unrelated to arity.
- [ ] Tests: `annotations: []` clears across several nodes; one unsupported
      node does not stop the others; a missing node is reported per node.

---

## Task 6: `rename_node` → `batch_rename_nodes` takes a literal name

**Overlap:** both write `node.name`. `batch_rename_nodes` only computes the new
name from find/replace/prefix/suffix, so a literal rename has no expression
there today.

**Decision:** the tool keeps the name `batch_rename_nodes`. Renaming it to
`rename_nodes` would read better but costs a second breaking rename in the same
release for no capability.

**Steps:**
- [ ] Drop `rename_node` from `expectedTools` → RED.
- [ ] Add a `name` param: sets the name outright. Reject it together with
      `find`/`prefix`/`suffix` — a literal name and a substitution in one call
      have no defined order, and silently picking one is the failure mode
      `requireVariant` exists to prevent.
- [ ] Loosen the existing "at least one of find/replace, prefix, or suffix"
      rule to include `name`.
- [ ] Plugin: `name` wins outright when present.
- [ ] Delete `rename_node` — spec and handler.
- [ ] Mention Figma's slash path notation (`Icons/Arrow/Left`) in the `name`
      description; that sentence lived on `rename_node` and is worth keeping.
- [ ] Tests: `name` across two nodes sets both; `name` with `find` → the
      rejection names both; find/replace still behaves as before.

---

## Task 7: `move_nodes`, `resize_nodes`, `set_corner_radius` → `set_node_properties`

**Overlap:** all three are `nodeIds` plus a couple of numbers, over the same
per-node loop `set_node_properties` already runs. This is the same merge that
folded eight tools into `set_node_properties` in the first place.

**Files:**
- Modify: `internal/tools/tools_write_modify.go` (4 specs, `nodePropertyKeys`)
- Modify: `plugin/src/write-modify.ts:48` (`applyNodeProperties`), `:267,284,337`
- Test: `plugin/src/write-modify.test.ts`, `internal/tools/validate_test.go`

**Steps:**
- [ ] Drop all three from `expectedTools` → RED.
- [ ] Add to `set_node_properties`: `x`, `y`, `width`, `height`,
      `cornerRadius`, `topLeftRadius`, `topRightRadius`, `bottomLeftRadius`,
      `bottomRightRadius`. Add them to `nodePropertyKeys` so "at least one
      property" counts them.
- [ ] Plugin `applyNodeProperties`: three new blocks, each reporting into
      `applied`/`errors` like the existing ones —
      - position: `x`/`y`, guarded by `"x" in n`
      - size: one `n.resize(w, h)` call, defaulting the omitted axis to the
        node's current value, guarded by `"resize" in n`
      - radii: the five keys, guarded by `"cornerRadius" in n`
- [ ] Delete the three specs and the three plugin handlers.
- [ ] Tests: move and resize in one call → **one** `commitUndo` (was two);
      `width` alone preserves the height; a node without `resize` reports
      `errors.width`/`errors.height` while `opacity` in the same call still
      applies; per-corner radii still land individually.

**Preserved:** everything, and repositioning plus resizing is now one undo
entry instead of two.

---

## Task 8: `remove_reactions` → `set_reactions` takes `removeIndices`

**Overlap:** removing everything is `set_reactions(replace, [])`. **What would
be lost:** removing reactions #1 and #3 by index, which has no expression short
of a get→filter→set round trip.

**Steps:**
- [ ] Drop `remove_reactions` from `expectedTools` → RED.
- [ ] `set_reactions`: add `removeIndices` (number array, min 0), mutually
      exclusive with `reactions`; one of the two is required.
- [ ] Preserve the quirk in `write-prototype.ts:70`: an **empty**
      `removeIndices` means remove everything, not remove nothing.
- [ ] Plugin: fold the removal branch into `set_reactions`; delete
      `remove_reactions`.
- [ ] Update `get_reactions`' description — it currently points at
      `remove_reactions` by name.
- [ ] Tests: `removeIndices: [1, 3]` leaves 0 and 2; `[]` removes everything;
      an out-of-range index is ignored rather than throwing; both arguments
      together → rejected; neither → rejected.

---

## Task 9: `get_design_context` → `get_document` grows `scope: "selection"`

The heaviest task, and the one whose response shape changes most. Do it last
among the merges.

**Overlap:** both answer a depth-limited tree, and each description has to
explain when to use the other — the surest sign of a pair that should be one
tool. **What differs, and must survive:** `get_design_context` walks from the
**selection**, defaults `depth` to 2, has three `detail` levels, and dedupes
component instances. `get_document` walks a page or the whole file, is
unbounded, and honours `maxNodes`.

**Files:**
- Modify: `internal/tools/tools_read_document.go`, `tools_schema_test.go`
- Modify: `plugin/src/read-document.ts:8,152`
- Modify: `plugin/src/tool-classes.ts`
- Test: `plugin/src/read-search.test.ts`

**Steps:**
- [ ] Drop `get_design_context` from `expectedTools` → RED.
- [ ] `get_document` gains `detail` (minimal/compact/full) and
      `dedupe_components`, and `scope` gains `"selection"`.
- [ ] Plugin: one handler, three roots — selection, current page, every page —
      one serializer chosen by `detail`, and one shared `makeBudget`, so
      `maxNodes` and `depth` now bound every scope including the selection.
- [ ] Response shape by scope, so no existing `get_document` caller breaks:
      `page` and `document` keep the tree they answer today; `selection`
      answers `{fileName, currentPage, selectionCount, context, componentDefs?,
      globalVars?}` — what `get_design_context` answers now.
- [ ] `dedupe_components` becomes available on `page` and `document` too. That
      is new reach, not new code: `deduplicateStyles` already runs on both
      paths; only the instance-level dedupe was selection-only.
- [ ] Delete `get_design_context` — spec and handler.
- [ ] Fix every description that names `get_design_context`: `get_selection`,
      `get_document`, `get_nodes_info`.
- [ ] Tests: `scope: "selection"` with `detail: "minimal"` matches what
      `get_design_context` answered; the default scope still answers the page
      tree unchanged (regression); `maxNodes` truncates a selection walk;
      `dedupe_components` on a page collapses repeated instances.

---

## Task 10: `get_screenshot` + `save_screenshots` → `export_screenshots`

The user chose the merged form: one tool, `items`, `outputPath` optional.

**Overlap:** both export the same nodes through the same plugin call; only the
destination differs. **What would be lost without care:** `save_screenshots`
names a file per node (`items[].outputPath`), and `get_screenshot` with no
node ids exports the current selection.

**Files:**
- Modify: `internal/tools/tools_read_export.go` (both specs → one)
- Modify: `plugin/src/read-export.ts`, `plugin/src/tool-classes.ts`
- Test: `internal/tools/tools_read_export_test.go`, `plugin/src/read-export.test.ts`

**Steps:**
- [ ] Drop both names from `expectedTools`, add `export_screenshots` → RED.
- [ ] One spec: `items` (optional array of `{nodeId, outputPath?, format?,
      scale?}`), plus top-level `format` and `scale` defaults. Omitting `items`
      exports the current selection as base64, which is what `get_screenshot`
      with no ids does today.
- [ ] Validate: every `items[].nodeId` is colon format; an item with
      `outputPath` set to `""` is rejected rather than silently returning
      base64.
- [ ] The Custom handler splits by item: those with an `outputPath` are
      written to disk and reported as file metadata, those without come back
      as base64, in one answer.
- [ ] Keep the plugin's `get_screenshot` handler — it is what the Go side calls
      per item — and add it to `INTERNAL_DISPATCH_TARGETS`.
- [ ] Update `SERVER_SIDE_TOOLS` in `tool-contract.test.ts`:
      `save_screenshots` → `export_screenshots`.
- [ ] Update the three descriptions that point at these tools by name:
      `get_image_bytes`, `set_export_settings`, `export_frames_to_pdf`.
- [ ] Tests: items with and without `outputPath` in one call → both kinds in
      one answer; no `items` → selection as base64; a bad `nodeId` is rejected
      before anything is written.

---

## Task 11: Documentation and the golden artifacts

**Files:** `README.md`, `glama.json`, `internal/tools/testdata/tools_schema.json`

**Steps:**
- [ ] Regenerate the golden schema.
- [ ] README: update the tool tables (the removed rows, the widened ones), and
      add a Migration section mapping each removed name to its replacement and
      the argument that stands in for it.
- [ ] README: note the three breaking response shapes —
      `set_auto_layout`, `set_annotations` and `export_screenshots` now answer
      `{results: [...]}`.
- [ ] `glama.json`: its description still claims 84 tools and its list still
      names internal handlers like `add_page`. Correct the count to 65 and
      reconcile the list against the regenerated golden schema.
- [ ] `internal/prompts/*.go`: eight strategy prompts name removed tools.
      Rewrite those references.
- [ ] Final gate: `go build ./...`, `go vet ./...`, `gofmt -l .` silent,
      `go test ./...`, `bun test`, `bun run build`.

---

## Definition of done

- [ ] `expectedTools` lists exactly 65 names
- [ ] `tool-contract.test.ts` green — no unreachable handler, nothing declared
      without an implementation
- [ ] No tool lost a capability — check each task's "what would be lost"
- [ ] `plugin/package.json`, `server.json`, `version.go` still say `0.3.1`
- [ ] No reference to a removed tool name survives in Go, TS, or docs
      (`grep` for each of the 13)

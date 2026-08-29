# Plugin Upgrade Implementation Plan

**Goal:** Close the four gaps in `docs/specs/2026-08-29-plugin-upgrade-design.md` — tool coverage, silent wrong answers, an unobservable panel, and unbounded work — without changing how an existing setup connects.

**Architecture:** Each task is a vertical slice: plugin handler, its tests, the Go `toolSpec`, the golden snapshot, and the README row, in that order. A slice is complete when both test suites and both builds pass, so no task depends on a later one to be correct. Tasks 1–6 are the cheap high-impact fixes; 7–10 are the panel and the protocol; 11–15 are API coverage; 16–18 are the closing items.

**Tech Stack:** Go 1.27, `encoding/json/v2`, `log/slog`, `github.com/mark3labs/mcp-go` v0.46.0, `github.com/coder/websocket` v1.8.14. Plugin: TypeScript 5.9, Svelte 5, Vite 6, Bun 1.4 test runner, `@figma/plugin-typings` 1.124.

**Spec:** `docs/specs/2026-08-29-plugin-upgrade-design.md`

**Status:** Tasks 1–18 shipped in `acc1ceb` (PR #4, version 0.3.1). The remaining work is listed at the end and is not part of that PR.

## Global Constraints

- Every new tool needs, in the same task: a plugin handler, a Go `toolSpec` in the matching `internal/tools/tools_*.go`, an entry in `expectedTools` in `tools_schema_test.go`, a regenerated golden snapshot, and a README table row. Missing any one of them is a tool that half-exists.
- `internal/tools/testdata/tools_schema.json` **does** change in this plan, unlike the 2026-08-28 plan. Regenerate with `go test ./internal/tools/ -run Golden -update`, then read the diff tool-by-tool before accepting it. Delete the `.actual` file afterwards.
- A new tool must be classified in `plugin/src/tool-classes.ts`. `tool-classes.test.ts` fails if it is not.
- `gofmt -l .` clean and `go test ./...` passing before every commit; `bun test` and `bun run build` from `plugin/` likewise.
- Defaults never change for an existing user. New guards start `off`; new limits start unbounded.
- Commit after every task. Branch: `claude/plugin-upgrade-features-avneku`.

---

## File Structure

New plugin modules, each with a sibling `.test.ts`:

| Module | Holds |
|---|---|
| `plugin/src/write-viewport.ts` | `set_selection`, `pageOf` |
| `plugin/src/write-document.ts` | `save_version_checkpoint`, `set_codegen_result`, `manage_plugin_data` |
| `plugin/src/write-text.ts` | `set_text_ranges`, range resolution and font inheritance |
| `plugin/src/write-vector.ts` | boolean ops, flatten, outline stroke, `create_vector`, `commonParent` |
| `plugin/src/write-component-properties.ts` | `combine_as_variants`, `manage_component_properties`, property-id resolution |
| `plugin/src/cancellation.ts` | the cancelled-id set and `throwIfCancelled` |
| `plugin/src/write-queue.ts` | `enqueueWrite` |
| `plugin/src/tool-classes.ts` | read/harmless/destructive classification, no `figma` dependency |
| `plugin/src/codegen.ts` | Dev Mode provider, block storage format, lookup walk |
| `plugin/src/ui/version-check.ts` | major.minor skew comparison |
| `plugin/src/ui/prefs.ts` | stored preferences and their defaults |
| `plugin/src/ui/activity.ts` | the activity-log ring buffer |

New Go files: `internal/tools/tools_write_viewport.go`, `tools_write_document.go`, `tools_write_vector.go`, `tools_write_component_properties.go`, `internal/bridge/version_skew.go`.

---

## Tasks

### Phase A — cheap, high impact

- [x] **Task 1: Version skew warning.** `version-check.ts` and `internal/bridge/version_skew.go` compare `major.minor`; the plugin sends a `plugin-info` frame on connect; the bridge logs the skew and the panel shows a one-line banner with the full remedy in its tooltip. Unreadable versions stay silent.
- [x] **Task 2: Auto-copy node id on by default.** `prefs.ts` normalizes stored preferences; absent means "never chose", so upgrading users get the new default while an explicit opt-out is honoured. Preferences ride in the same stored object as the address, so the panel's first connect is not delayed by a second round trip.
- [x] **Task 3: Auto-layout sizing.** `layoutSizingHorizontal/Vertical`, min/max (`Nullable` so an explicit `null` clears one), `layoutPositioning`, `layoutAlign`, `layoutGrow`, `itemReverseZIndex`, `strokesIncludedInLayout`, `clipsContent`. Drop the `type !== "FRAME"` gate for an `"layoutMode" in node` check. Added `paramSpec.Nullable` and threaded it through `specArgs` and `ValidateRPC`.
- [x] **Task 4: `set_selection`.** One tool with `select` and `zoom` flags rather than two tools. Switches pages when needed; refuses a selection spanning pages. No `commitUndo` — a camera move on the undo stack would make Ctrl+Z scroll the canvas.
- [x] **Task 5: Document-wide search.** `scope: "page" | "document"`, loading one page at a time via `page.loadAsync()` and emitting a `progress_update` per page, which extends the bridge's timeout budget so no per-tool timeout entry is needed. Result carries `truncated`.
- [x] **Task 6: One undo checkpoint per pipeline, plus `save_version_checkpoint`.** `withSingleUndoCheckpoint` swallows the handlers' `commitUndo` and makes one at the end, restoring the exact original reference.

### Phase B — panel and protocol

- [x] **Task 7: Activity log.** Ring buffer of 20 in `ui/activity.ts`; running state derived from the log rather than a parallel `Set`, so the banner and the log cannot disagree. Panel resizes when the log opens; the clock ticks only while something runs.
- [x] **Task 8: Guard modes.** `off`/`confirm`/`read-only` with `tool-classes.ts` classification and a confirmation dialog. Changing mode releases anything held under the old one rather than leaving the server waiting on a dialog that is gone.
- [x] **Task 9: Undo button.** `figma.triggerUndo()` behind a `typeof` guard, plus `resize_ui` in the plugin core with clamped bounds.
- [x] **Task 10: Cancellation.** `cancel_request` from both the caller-cancelled and timed-out paths, sharing `writeControlFrame` with the server-info reply. Checks between pages, inside the two recursive scans, and between pipeline steps. A cancelled pipeline rolls back regardless of `stop_on_error`.

### Phase C — API coverage

- [x] **Task 11: Rich text.** `set_text_ranges` for per-range styling; `serializeStyledSegments` on the read side, returning nothing for a uniformly styled node. `set_text` gains the paragraph-level properties and `text` becomes optional — a call that only changes the wrap mode should not blank the node.
- [x] **Task 12: Vector and boolean geometry.** `boolean_operation` (caller's node order preserved, because `SUBTRACT` and `EXCLUDE` read it), `flatten_nodes`, `outline_stroke` (a node with no visible stroke is `skipped`, not a failure), `create_vector` (a single-path SVG is unwrapped from its frame).
- [x] **Task 13: Component sets and properties.** `combine_as_variants`; `manage_component_properties` with `add`/`edit`/`delete`/`bind`. Properties are addressed by name and the current id resolved on every call, because Figma mints a new id on rename. `bind` merges into `componentPropertyReferences` rather than replacing it.
- [x] **Task 14: Mask, layout grids, images.** `isMask`/`maskType` join `set_node_properties`; `set_layout_grids` with the grid builder shared with `create_grid_style`; `import_image` accepts a URL and an existing node; `get_image_bytes` returns the original asset, deduplicated by image hash.
- [x] **Task 15: Response ceiling and editor types.** `get_document` gains `depth` and `maxNodes` over a shared budget; `manifest.json` adds `figjam` and `slides`, which is what makes `create_connector` reachable at all.

### Phase D — closing

- [x] **Task 16: Dev Mode codegen.** `set_codegen_result` writes blocks to shared plugin data and sets relaunch data; `codegen.ts` serves them. Codegen mode skips the panel and the WebSocket bootstrap entirely.
- [x] **Task 17: Handler-list handshake and the cross-language contract test.** The plugin announces its handler list; `checkPluginSupports` refuses an unsupported tool with a remedy before anything is written. `tool-contract.test.ts` runs on the plugin side and reads the server's own golden snapshot, checking both directions.
- [x] **Task 18: Write queue.** `enqueueWrite` serialises mutating requests; reads bypass it. This closes the window Task 6 opened.

---

## Verification

Run from the repository root:

```bash
gofmt -l . && go build ./... && go test ./... -count=1
cd plugin && bun test && bun run build
```

Expected: `gofmt` silent, 7 Go packages `ok`, 599 plugin tests passing across 27 files, both Vite builds clean.

The golden snapshot is `cf8b3518b7caa920932b19be341b1b4ec75fece9eaf69e73262461f4afcfb844` at 76 tools.

---

## Remaining

None of this blocks anything. Grouped so a later session need not re-read the spec.

### API coverage

- [ ] `set_layout_sizing` applying to several nodes at once (spec §"Auto-layout").
- [ ] `get_document` still serializes the current page only — no `scope: "document"` as `search_nodes` has.
- [ ] Write-side `strokeCap`, `strokeJoin`, `dashPattern`, `strokeMiterLimit`. The serializer already reads `dashPattern`.
- [ ] `exportSettings` preset on a node.
- [ ] `imageTransform` (crop) and image `filters`.
- [ ] `figma.codegen.on("preferenceschange")` for picking a language or framework.

### Performance

- [ ] `deduplicateStyles`/`globalVars` applies to `get_document` only; extend to `get_nodes_info` and `get_design_context`.
- [ ] Widen `progress_update` coverage: per pipeline step, multi-node export, `find_replace_text`.

### Panel

- [ ] Remember a panel size the user dragged.
- [ ] Follow Figma's light/dark theme instead of the hard-coded dark palette.
- [ ] A "send selection to AI" pin, so a stable context set replaces copying ids by hand.
- [ ] Vietnamese/English strings.

### Testing

- [ ] A `dynamic-page` fixture with several pages, so a missing `loadAsync` fails a test rather than returning an empty answer.
- [ ] A test that forces a new write handler to be considered for `CREATE_ACTIONS` in `batch-pipeline.ts`. The comment there already warns about the trap; nothing enforces it.
- [ ] Report missing fonts up front rather than letting `loadFontAsync` throw part-way through a run.

### Security

- [ ] The `--ip` flag and `allowedDomains: ["*"]` still expose an unauthenticated port. Pairing was considered and rejected (spec §"Rejected"); if revisited, gate destructive tools rather than the connection.

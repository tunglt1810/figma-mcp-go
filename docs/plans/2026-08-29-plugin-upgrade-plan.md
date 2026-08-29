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

Added by Phase 5:

| Module | Holds |
|---|---|
| `plugin/src/progress.ts` | `reportProgress`, the clamp, and the yield that lets a message leave |
| `plugin/src/fonts.ts` | `loadFonts` — every font at once, one error naming all the missing ones |
| `plugin/src/pinned.ts` | the pinned context set, in the plugin core's memory |
| `plugin/src/ui/i18n.ts` | the panel's strings, one table per locale |
| `plugin/src/dynamic-page.fixture.ts` | test support: pages that report no children until loaded |

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

Expected: `gofmt` silent, 7 Go packages `ok`, 680 plugin tests passing across 33 files, both Vite builds clean.

The golden snapshot is `909a1e5a4e8d74c59825658338f45a7eb4906a6610fb341e7a7cb7608798bb59` at 78 tools.

---

## Remaining

Everything under this heading in the first draft of this plan has since been
done — see the "Follow-up" phase below. What is left here is what nobody has
asked for and nothing depends on.

- [ ] The plugin connection is still unauthenticated. Pairing was considered and
      rejected (spec §"Rejected"), and the exposure is now reported rather than
      closed: the server warns when `--ip` moves the listener off loopback, and
      the panel raises its `confirm` guard when it hears about it. Closing it
      properly means a pairing handshake, and the cost to the default local
      setup is the reason it has not been paid.
- [ ] `get_design_context` has no `scope: "document"`, only `get_document` and
      `search_nodes` do. It is the exploration tool, so a whole-file mode would
      need a token budget of its own rather than the same one.
- [ ] The panel's Vietnamese is picked from the browser's language tags, with no
      way to override it. A third locale, or a user who wants English on a
      Vietnamese machine, needs a setting.
- [ ] `set_export_settings` writes presets but nothing reads them back: the
      serializer does not report `exportSettings`, so a caller cannot see what a
      node already has before replacing it.

---

## Phase 5 — Follow-up

The items the first draft of this plan listed as Remaining.

### API coverage

- [x] `set_layout_sizing`: a new tool taking `nodeIds`, applying the sizing half
      of `set_auto_layout` across several nodes. Its parameters are derived from
      `autoLayoutParams()` by name rather than retyped, so the two tools cannot
      document the same property differently.
- [x] `get_document` gains `scope`, matching `search_nodes`. One depth/maxNodes
      budget is shared across the pages and styles are deduped file-wide.
- [x] Stroke geometry on `set_node_properties`: `strokeWeight`, `strokeAlign`,
      `strokeCap`, `strokeJoin`, `strokeMiterLimit`, `dashPattern`. They belong
      to the node, not to a paint, which is why they are not on `set_paint`.
      Assignment there is now guarded per property.
- [x] `set_export_settings`: the presets under Export in the right-hand panel,
      across several nodes. It exports nothing itself.
- [x] `import_image` takes `crop` and `filters`. The crop is a rectangle in
      fractions of the image; `cropToTransform` is the one place that knows
      Figma's 2x3 matrix.
- [x] A Dev Mode **Language** preference, declared in the manifest and refreshed
      on `preferenceschange` — Figma does not re-render the panel by itself.

### Performance

- [x] `deduplicateStyles` now applies to `get_nodes_info` too, which moves its
      answer to `{nodes, globalVars?}`. `get_design_context` already had it.
- [x] `progress_update` moved into one module and covers the batch pipeline
      (per step), `find_replace_text`, `get_screenshot`, and
      `export_frames_to_pdf`.

### Panel

- [x] The panel draws its own resize grip — Figma gives a plugin window none —
      and stores the dragged size with the other preferences.
- [x] Colours became tokens with a light and a dark set, keyed off the
      `figma-dark` class Figma puts on the document element.
- [x] A pinned context set, read with `get_selection(source: "pinned")`. It
      lives in the plugin core's memory: a working set for one sitting, not a
      property of the document.
- [x] Vietnamese and English strings. Only the panel is translated; refusal text
      and the activity log stay English, because the MCP client and a bug report
      reader are their audience.

### Testing

- [x] A `dynamic-page` fixture whose pages report no children until `loadAsync`,
      verified by removing a load and watching the suite go red.
- [x] Every write handler is classified as creating or keeping, so a new one
      fails the test until `CREATE_ACTIONS` has been considered.
- [x] Missing fonts are collected and reported before any text is written, in
      one error naming all of them.

### Security

- [x] The exposure is reported rather than closed. `--ip` off loopback warns at
      startup, the server-info frame carries `exposed`, and the panel raises its
      `confirm` guard once — gating the destructive tools, not the connection.
      Pairing itself stays rejected; see the Remaining item above.

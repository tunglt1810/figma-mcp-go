# Proposed upgrades and new features for the Figma plugin

> **This is the original survey.** The settled design and the remaining work are
> written up in `docs/specs/2026-08-29-plugin-upgrade-design.md` and
> `docs/plans/2026-08-29-plugin-upgrade-plan.md` — those two follow the repo's
> conventions and are what to read first. This one is kept because it lists the
> missing APIs one by one, in more detail than the spec's summary.

Date: 2026-08-29 · Scope: `plugin/` (and the protocol surface it touches in
`internal/bridge`)

This document is a **proposal**. Each item states the problem as it stands, what
is proposed, the files involved, and an effort estimate (S/M/L).

> **Status, 2026-08-29** — all four waves shipped, marked ✅ in the table below
> and against each item. Two decisions departed from the original proposal and
> are recorded in 3.2 and 2: the pairing token was dropped because it costs the
> smooth-connect experience, and Dev Mode codegen was built along a path that
> does not need MCP sampling.
>
> **Follow-up, later the same day** — every item that section 7 listed as
> remaining has since been built. Section 7 now says what each of them became.

---

## 0. Priority summary

| # | Item | Why | Effort | Status |
|---|------|-----|--------|--------|
| 1 | Auto-layout sizing (`HUG`/`FILL`), absolute positioning, min/max | Blocks every responsive layout — the sorest point today | S | ✅ |
| 2 | Plugin ↔ server version-skew warning | An old plugin against a new server gives an unreadable "Unknown request type" | S | ✅ |
| 3 | `set_selection` | Human-in-the-loop: "show me what you just made" | S | ✅ |
| 4 | Activity log and an approval mode in the panel | Makes an AI write observable, and safe | M | ✅ |
| 5 | Range-level rich text | No way to make multi-style text or a link | M | ✅ |
| 6 | Vector and boolean ops | No way to build an icon | M | ✅ |
| 7 | Component sets, variants, component properties | No way to build a complete design system | M | ✅ |
| 8 | Document-wide search | Silently wrong under `documentAccess: dynamic-page` | S | ✅ |
| 9 | Dev Mode codegen provider | The big differentiator against other Figma MCP servers | L | ✅ (see 2) |
| 10 | Request cancellation and paging of large responses | Stability on large files | M | ✅ |
| 11 | One undo step per pipeline, plus a version-history checkpoint | Undoing a pipeline stops being twenty Ctrl+Z presses | S | ✅ |
| 12 | Auto-copy node id on by default | The thing every session starts with | S | ✅ |

---

## 1. Filling in the Figma Plugin API (new tools)

### 1.1 Auto layout is missing the part that matters most — ✅ **done**
`applyAutoLayout` (`plugin/src/write-helpers.ts`) sets only `layoutMode`,
padding, `itemSpacing`, alignment, sizing mode, and wrap. Missing:

- `layoutSizingHorizontal` / `layoutSizingVertical` (`FIXED` | `HUG` | `FILL`) —
  this is the API designers actually use; `primaryAxisSizingMode` is the older
  one and cannot express a child's "fill container".
- `layoutPositioning: "ABSOLUTE"` plus `constraints`, for an element floating
  inside an auto layout.
- `minWidth` / `maxWidth` / `minHeight` / `maxHeight` (responsive).
- `layoutGrow`, `layoutAlign` for each child.
- `itemReverseZIndex`, `strokesIncludedInLayout`, `clipsContent`.

`set_auto_layout` (`plugin/src/write-modify.ts`) also hard-refuses anything
where `node.type !== "FRAME"`, while `COMPONENT`, `COMPONENT_SET` and `INSTANCE`
all carry auto layout — it should ask for `"layoutMode" in node` instead.

**Done**: `applyAutoLayout` extended with `layoutSizingHorizontal/Vertical`,
`minWidth/maxWidth/minHeight/maxHeight` (null clears one), `layoutPositioning`,
`layoutAlign`, `layoutGrow`, `itemReverseZIndex`, `strokesIncludedInLayout`, and
`clipsContent`; the `type !== FRAME` gate is gone. A `Nullable` flag was added to
`paramSpec` on the Go side so an explicit null reaches the plugin instead of
being dropped like an absent argument.

**Follow-up**: `set_layout_sizing` now applies the sizing half of this across
several nodes at once.

### 1.2 Range-level rich text — ✅ **done**
`set_text` writes the whole of `characters` in one font and one colour. Missing:

- Reading: `getStyledTextSegments([...])` — the serializer returns text as one
  flat block, so "design → code" loses inline bold, links, and colour.
- Writing: `setRangeFontName`, `setRangeFills`, `setRangeFontSize`,
  `setRangeTextDecoration`, `setRangeHyperlink`, `setRangeListOptions`,
  `setRangeTextStyleId`.
- Paragraph properties: `paragraphSpacing`, `paragraphIndent`, `textAutoResize`,
  `textTruncation`, `maxLines`, `leadingTrim`.

**Proposed**: a `set_text_ranges` tool taking an array of `{start, end, style}`,
and `serializeNode` extended to return `styledSegments`. Effort: M.

### 1.3 Vector and boolean operations — **entirely absent** — ✅ **done**
No `booleanOperation`, `flatten`, `outlineStroke`, `vectorPaths`, or
`setVectorNetworkAsync`. So the AI cannot build an icon, simplify a shape, or
import an SVG path.

**Proposed**: `boolean_operation` (union/subtract/intersect/exclude),
`flatten_nodes`, `outline_stroke`, and `create_vector` taking SVG path data —
`figma.createNodeFromSvg` is the shortest route. Effort: M.

### 1.4 Component sets, variants, component properties — ✅ **done**
There is `create_component`, `swap_component`, `detach_instance`, and
`set_instance_overrides`. Missing:

- `figma.combineAsVariants`, to make a `COMPONENT_SET`.
- `componentPropertyDefinitions`: add, edit, and delete a property
  (`BOOLEAN`, `TEXT`, `INSTANCE_SWAP`, `VARIANT`).
- Binding a property to a node (`componentPropertyReferences`).
- Switching an instance's variant by `{Size: "Large", State: "Hover"}` rather
  than by knowing the child component's id.

**Proposed**: a `manage_component_properties` group plus `set_variant`. This is
the missing piece behind any claim of full design-system automation. Effort: M.

### 1.5 Selection and viewport, on the write side — ✅ **done**
The plugin only **reads** the selection. There is no way for the AI to say
"here, look at this".

**Done**: one `set_selection` tool with `select` and `zoom` flags rather than two
tools — `select: false, zoom: true` is "focus without touching the user's
selection". It calls `setCurrentPageAsync` when the node is on another page, and
refuses a list spanning pages, because a Figma selection belongs to exactly one.

### 1.6 Walking the document under `dynamic-page` — ✅ **done**
`manifest.json` declares `documentAccess: "dynamic-page"`, but nothing calls
`figma.loadAllPagesAsync()`. `search_nodes` walks only from `figma.currentPage`,
and `get_document` serializes only the current page. On a file with several
pages, "not found" is **silently wrong** rather than an error.

**Done**: `search_nodes` takes `scope: "page" | "document"`. It loads one page at
a time with `page.loadAsync()` rather than `loadAllPagesAsync()`, so a large file
pays per page; it emits a `progress_update` per page, so no per-tool timeout
entry is needed; and it returns `truncated`, so a complete answer is
distinguishable from a capped one.

**Follow-up**: `get_document` now takes the same `scope`.

### 1.7 Masks, layout grids, and the node properties still missing
- ✅ `isMask` / `maskType` — no way to make a mask. **Done** on
  `set_node_properties`.
- ✅ `layoutGrids` on a frame (there was a *grid style*, but no direct grid).
  **Done** as `set_layout_grids`.
- `effects` per node versus an effect style — coverage worth re-checking.
- ✅ `strokeCap`, `strokeJoin`, `dashPattern`, `strokeMiterLimit` — the
  serializer read `dashPattern` but nothing could write it. **Done** on
  `set_node_properties`, which also gained `strokeWeight` and `strokeAlign`.
- ✅ `exportSettings` on a node, so the export presets are set up for the
  designer. **Done** as `set_export_settings`.

### 1.8 Images
- ✅ `figma.createImageAsync(url)` — import by URL rather than pushing base64
  through the WebSocket. **Done**; `import_image` no longer requires base64.
- ✅ Reading back: `getImageByHash(hash).getBytesAsync()`, needed by
  "design → code" when assets have to be exported. **Done** as `get_image_bytes`.
- ✅ `imageTransform` (crop) and `filters` (exposure/contrast/saturation).
  **Done**; the crop is expressed as a rectangle in fractions of the image, and
  `cropToTransform` is the one place that knows Figma's 2x3 matrix.

### 1.9 Metadata attached to the file
- ✅ `setPluginData` / `setSharedPluginData`: store the link between a component
  and a code file, mark a node as AI-generated, record a design-token binding —
  so a later run has durable context instead of asking again from scratch.
  **Done** as `manage_plugin_data`.
- ✅ `setRelaunchData`: put an "Edit with AI" button on the node in Figma.
  **Done**, attached by `set_codegen_result`.
- ✅ `figma.saveVersionHistoryAsync(title)`: **done** as
  `save_version_checkpoint`. The write-ahead rollback in `batch-pipeline.ts` dies
  with the plugin, so a named version is the only way back that outlives the
  session.

### 1.10 Wider editor types — ✅ **done**
`manifest.json` declared `["figma", "dev"]`. `"figjam"` and `"slides"` can be
added — `create_connector` (a FigJam tool) already existed while the editor list
left FigJam out.

---

## 2. Dev Mode codegen provider — ✅ **done, along a different path**

The manifest already had `capabilities: ["inspect"]`, but the plugin never
registered `figma.codegen.on("generate", ...)`. Registering it makes Dev Mode's
Code panel show code the MCP server produced for the selected node — the
designer clicks a node and sees React, SwiftUI, or Compose written against their
real codebase.

**The flow first proposed** — `codegen.on("generate")` → WebSocket → the Go
server → ask the AI client → return the code — needs two things: a
plugin-initiated request direction through the bridge, which does not exist, and
**MCP sampling**, so the server can ask the client for a completion. Sampling is
optional in the MCP protocol and absent from the clients this server targets, so
building a whole second protocol direction for a feature no client can run is not
worth it.

**Done — the flow inverted.** The client generates the code with the whole
repository in front of it and attaches it to the node with `set_codegen_result`;
the provider serves what is stored. It is written to **shared plugin data**, so
it travels with the file: the whole team's Dev Mode shows it, not only the
machine that generated it. Lookup walks from the selected node to the component
its instance came from and then up its ancestors, so attaching code to a
COMPONENT covers every instance of it.

The cost is that the code does not update when the design changes — the tool has
to be called again. That is acceptable, and in exchange the code is better,
because the client can see the real codebase.

**Follow-up**: `figma.codegen.on("preferenceschange")` and a Language selector
are now in place.

---

## 3. The plugin panel (`plugin/src/ui/App.svelte`)

The 320×230 panel shows the file, page, and selection; the node list with copy
buttons; an "AI is working…" banner; the server address; and a connection badge.

### 3.1 Activity log — ✅ **done**
"AI is working…" does not say what the AI is doing. The payload the UI already
receives carries `payload.type` and `requestId` — it only needs displaying.

**Done**: the last 20 requests with tool name, duration, ✓/✗, the error, and a
copy button for pasting into a bug report. The running state is read off the log
rather than a separate Set, so the banner and the log cannot disagree. The panel
grows when the log opens.

### 3.2 Safe mode and approvals — the pairing token was dropped
`networkAccess.allowedDomains: ["*"]` with no authentication means **any process
on the machine that can open a WebSocket can drive the open Figma file**.

**Done**, (1) and (2), as one three-state Guard button — off / confirm /
read-only — defaulting to off so every existing user's behaviour is unchanged:
1. **read-only** — blocks every write tool at the panel.
2. **confirm** — a confirmation dialog for the destructive tools
   (`delete_nodes`, `delete_page`, `delete_style`, `delete_variable`,
   `detach_instance`, `find_replace_text`, `batch_rename_nodes`,
   `boolean_operation`, `flatten_nodes`, and a pipeline containing any of them).
3. ~~**Pairing token**~~ — **dropped from the roadmap**: it is paid for in the
   smooth-connect experience, which is the plugin's advantage over the Figma MCP
   servers built on the REST API. The risk is recorded here rather than closed;
   if it is revisited, the least disruptive shape is a code required for the
   destructive tools only, not for the connection.

Effort: S for (1), M for (2).

**Follow-up**: the risk is no longer silent. The server warns when `--ip` moves
the listener off loopback, the `server-info` frame carries an `exposed` flag, and
the panel raises its confirm guard once when it hears one — which is exactly the
shape named above. Pairing itself is still rejected.

### 3.3 Undo from the panel — ✅ **done**
**Done**: an Undo button on `figma.triggerUndo()`. With a pipeline collapsed into
one undo step, one press undoes a whole pipeline.

### 3.4 Small panel improvements worth having — ✅ **all done**
- ✅ `figma.ui.resize` and remembering the size (it was fixed at 320×230, which
  the activity log outgrows). The panel now draws its own resize grip, since
  Figma gives a plugin window none, and stores the dragged size.
- ✅ Follow Figma's light and dark themes rather than hard-coding a `#1e1e1e`
  background. The colours are now tokens with a light and a dark set, keyed off
  the `figma-dark` class Figma puts on the document element.
- ✅ A "send selection to the AI" button — pinning a stable context set instead
  of copying ids by hand. Read with `get_selection(source: "pinned")`.
- `figma.notify` on a write error, with a button that jumps to the node. Still
  open.
- ✅ Vietnamese and English strings.
- ✅ Show a percentage when a `progress_update` arrives (the bridge supported it
  and the panel ignored it).

---

## 4. Protocol and performance

### 4.1 A plugin → server handshake — ✅ **done**
The server sends `get_server_info` and the plugin answers with its version.
Nothing goes the other way: the server does not know which handlers the plugin
has. An old plugin meeting a new tool reports "Unknown request type: X", and the
user has no way to tell that the plugin is the old half.

**Proposed**: the plugin sends `{pluginVersion, protocolVersion, handlers: [...]}`
on connect. It already has `Object.keys(readHandlers)` and `writeHandlers`, so it
costs almost nothing. That buys:
- A clear error: "this tool needs plugin ≥ v1.4, you are running v1.2".
- Hiding tools the client cannot use (MCP's `tools/list_changed`).
- A contract test: every tool in `internal/tools/toolspec.go` must have a
  matching handler in the plugin. `toolspec_wire_test.go` already pins the wire
  shape; the handler-name comparison is what is missing.

**Done**: the plugin sends a `plugin-info` frame with its version on connect; the
bridge records it and logs a warning, and the panel shows a banner naming only
the half that is behind, with the remedy. The comparison is on `major.minor`,
because the plugin is installed by hand from a release zip while the server
updates itself through `npx @latest`, so patch drift is normal; an unreadable
version (a dev build) stays silent. The handler list is sent too, so a tool the
plugin does not have is reported as "update the plugin" rather than as "Unknown
request type" at call time, and a contract test compares the two sides.

### 4.2 Cancelling a request — ✅ **done**
There is no way to cancel. A `get_document` on a large file runs until it times
out with the panel stuck on "AI is working…".

**Done**: the bridge sends `cancel_request` when the caller cancels its context
or the request runs out of budget. Cancellation is advisory — a handler that does
not check simply runs to the end and has its answer discarded, so a new loop that
forgets to check is slow rather than broken. Checks are in `search_nodes`,
`get_local_components`, `scan_text_nodes`, `scan_nodes_by_types`, and between
pipeline steps.

### 4.3 Large payloads — ✅ **done**
- `get_document` serialized the whole page in one go, so a large file could break
  the `postMessage` between the panel and the core, or swamp the AI client's
  context.
- **Done**: `get_document` takes `depth` and `maxNodes`. One budget is shared
  across the walk and spent in tree order, so a capped result is reproducible; a
  node whose children were withheld reports `childCount`/`childrenOmitted`, and
  the tree carries `truncated`.
- **Follow-up**: `deduplicateStyles`/`globalVars` applied to `get_document` only.
  It now applies to `get_nodes_info` as well, which moved that tool's answer from
  a bare array to `{nodes, globalVars?}`; `get_design_context` already had it.

### 4.4 A write queue and one undo step — ✅ **done**
`figma.ui.onmessage` is async, so writes can interleave. Each write handler calls
`figma.commitUndo()` itself, so a 20-step pipeline made 20 undo steps.

**Done**: `withSingleUndoCheckpoint` swallows each handler's `commitUndo` inside a
pipeline and makes one at the end. Rollback runs inside the checkpoint, so a
pipeline that fails and reverses itself leaves the undo stack as it found it.

**Also done**: a serial queue for write requests — which fixes a bug the undo
collapsing itself introduced. A plain write landing while a pipeline was running
had its checkpoint swallowed too, and became part of the pipeline's undo step.
Reads do not queue.

### 4.5 Wider progress coverage — ✅ **done**
Only three handlers emitted `progress_update` (`read-document.ts` ×2,
`read-styles.ts` ×1). It should cover each batch-pipeline step, a multi-node
export, `find_replace_text`, and `scan_text_nodes`.

**Follow-up**: progress moved into one module, and now covers the pipeline per
step, `find_replace_text`, `get_screenshot`, and `export_frames_to_pdf`.

---

## 5. Quality and testing

- ✅ A contract test between handler names and the Go toolspec: it runs on the
  plugin side and reads the server's own golden schema. Every tool the server
  offers must have a handler, and every handler must be reachable — either as a
  tool, or through a documented delegation target.
- `mergeHandlers` already catches a duplicate name at load, which is good. There
  should also be a test asserting that every new write handler was considered for
  `CREATE_ACTIONS` in `batch-pipeline.ts` — the comment there warns about exactly
  this risk, but nothing watches it. **Follow-up**: done; every write handler is
  now listed as creating or keeping, and adding one fails the test until it is
  classified.
- A test for the `dynamic-page` path: a fixture with several pages, to catch a
  missing load. **Follow-up**: done, with a fixture whose pages report no children
  until `loadAsync` is called — verified by removing a load and watching the
  suite go red.
- Fonts: report the missing ones rather than letting `loadFontAsync` throw
  part-way through. **Follow-up**: done; `set_text_ranges` and `find_replace_text`
  collect every font they need, load them together, and raise one error naming
  all the missing ones before anything is written.

---

## 6. Proposed roadmap

**Wave 1 — cheap, high impact (S) — ✅ done**
Auto-layout sizing · version-skew warning · `set_selection` · document-wide
search · one undo step per pipeline · version-history checkpoint · auto-copy on
by default.

**Wave 2 — experience and safety (M) — ✅ done**
Activity log · read-only and destructive-confirm · request cancellation · an undo
button in the panel. (The pairing token was dropped — see 3.2.)

**Wave 3 — API coverage (M) — ✅ done**
Range-level rich text · vector and boolean ops · component sets and properties ·
masks and layout grids · images by URL · reading original image bytes back.

**Wave 4 — differentiation (L) — ✅ done**
Dev Mode codegen provider (see 2) · a size limit on `get_document` ·
FigJam and Slides · the handler-list handshake · the write queue.

---

## 7. What was left, and what it became

Nothing here blocked anything, which is why it was collected rather than done at
the time. All of it has since been built; each line says what it turned into.

**API coverage**
- `set_layout_sizing` across several nodes (1.1) → a new tool, with its
  parameters derived from `autoLayoutParams()` by name rather than retyped.
- `get_document` serialized only the current page, with no `scope: "document"`
  like `search_nodes` (1.6) → it takes the same `scope` now, sharing one budget
  across the pages and deduping styles file-wide.
- `strokeCap`, `strokeJoin`, `dashPattern`, `strokeMiterLimit` on the write side
  (1.7) → on `set_node_properties`, along with `strokeWeight` and `strokeAlign`.
- `exportSettings` presets on a node (1.7) → `set_export_settings`.
- `imageTransform` (crop) and `filters` (1.8) → on `import_image`, with the crop
  given as a rectangle in fractions of the image.
- `figma.codegen.on("preferenceschange")` for picking a language (2) → a Language
  selector in the manifest, refreshed on the event because Figma does not
  re-render the Code panel by itself.

**Performance**
- `deduplicateStyles` for `get_nodes_info` and `get_design_context` (4.3) →
  `get_design_context` already had it; `get_nodes_info` gained it, which moved
  its answer to `{nodes, globalVars?}`.
- Wider `progress_update` coverage (4.5) → one module, covering the pipeline per
  step, `find_replace_text`, `get_screenshot`, and `export_frames_to_pdf`.

**Panel**
- Remember a panel size the user dragged (3.4) → the panel draws its own resize
  grip and stores the size with the other preferences.
- Follow Figma's light/dark theme (3.4) → colour tokens with a light and a dark
  set, keyed off the `figma-dark` class.
- A "send selection to the AI" pin (3.4) → `get_selection(source: "pinned")`,
  with the set held in the plugin core's memory.
- Vietnamese and English strings (3.4) → one table per locale; only the panel is
  translated.

**Testing**
- A `dynamic-page` test with several pages (5) → a fixture whose pages report no
  children until loaded.
- A test forcing a new write handler to be considered for `CREATE_ACTIONS` (5) →
  every write handler is classified as creating or keeping.
- Report missing fonts rather than letting `loadFontAsync` throw part-way (5) →
  every font is loaded before anything is written, in one error naming all the
  missing ones.

**Security**
- The unauthenticated port (3.2) → reported rather than closed: a startup warning
  on `--ip` off loopback, an `exposed` flag on the `server-info` frame, and the
  panel raising its confirm guard once. Pairing itself stays rejected.

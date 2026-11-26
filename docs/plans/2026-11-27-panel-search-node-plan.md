# Panel Search Node Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A person can type node IDs or a Figma link into the plugin panel, or click a node in the selection list, to move the Figma view to those nodes.

**Architecture:** A pure parser in the UI turns the text into node IDs. The UI sends a new `go_to_nodes` message to the plugin core. The core reuses the lookup and page checks of `set_selection` through one shared function, and caps the zoom at 100% for panel calls only.

**Tech Stack:** TypeScript, Svelte 5, Figma Plugin API, `bun test`.

**Spec:** `docs/specs/2026-11-27-panel-search-node-design.md`

Work on branch `feat/panel-search-node`, not on `main`.

## Global Constraints

- Plugin only. No change to Go code or to the `set_selection` MCP tool behaviour (no zoom cap there).
- Button label: `Search node`. The Enter key in the box does the same as the button.
- Box position: top of the panel, above the **File** row.
- A change of the Figma selection never changes the text in the box.
- Zoom cap for panel calls: `1` (100%).
- A click on a node item never changes the selection.
- All panel strings go in `plugin/src/ui/strings.ts`, in English.
- Checks: `cd plugin && bun test && bun run typecheck && bun run build`.

## Review Focus

1. Instance sublayer IDs such as `I12:34;56:78` and their URL form `I12-34;56-78` must parse as one ID. `;` is not a separator. (Task 1 test)
2. A link with an encoded value `node-id=12%3A34` must give `12:34`. (Task 1 test)
3. Text with junk, such as `abc` or `12:`, must report the bad token, not send a partial list. (Task 1 test)
4. The same ID twice, or a trailing comma, must not produce a duplicate or an empty ID. (Task 1 test)
5. A node that is gone when the person clicks it in the list must show the error line, not fail silently. (Task 2 test for the core reply)

---

### Task 1: Node ID parser

**Files:**
- Create: `plugin/src/ui/node-ids.ts`
- Test: `plugin/src/ui/node-ids.test.ts`

**Interfaces:**
- Produces: `parseNodeIds(input: string): { ids: string[]; invalid: string[] }`

Rules:
- Split on commas and whitespace (including new lines). Drop empty tokens.
- A token that is a URL (starts with `http`) gives its `node-id` query value, URI-decoded. A URL with no `node-id` is invalid.
- Change every `-` to `:`.
- Valid ID: `/^I?\d+:\d+(;\d+:\d+)*$/`. Anything else goes to `invalid` as the original token.
- Keep the first occurrence of each ID, in input order.

- [ ] **Step 1: Write the failing tests** in `node-ids.test.ts`:
  - `parseNodeIds("12:34")` gives `{ ids: ["12:34"], invalid: [] }`
  - `parseNodeIds("12-34")` gives `ids: ["12:34"]`
  - `parseNodeIds("https://www.figma.com/design/AbC/Shop?node-id=12-34&t=x")` gives `ids: ["12:34"]`
  - `parseNodeIds("https://www.figma.com/design/AbC/Shop?node-id=12%3A34")` gives `ids: ["12:34"]`
  - `parseNodeIds("I12-34;56-78")` gives `ids: ["I12:34;56:78"]`
  - `parseNodeIds("12:34, 12-35\n12:34,")` gives `ids: ["12:34", "12:35"]`
  - `parseNodeIds("12:34 abc 12:")` gives `{ ids: ["12:34"], invalid: ["abc", "12:"] }`
  - `parseNodeIds("https://www.figma.com/design/AbC/Shop")` gives `invalid` with that URL
  - `parseNodeIds("   ")` gives `{ ids: [], invalid: [] }`
- [ ] **Step 2:** Run `cd plugin && bun test src/ui/node-ids.test.ts`. Expected: FAIL, module not found.
- [ ] **Step 3:** Implement `parseNodeIds` in `node-ids.ts`. Use `new URL(token).searchParams.get("node-id")`, which also decodes.
- [ ] **Step 4:** Run the same command. Expected: PASS.
- [ ] **Step 5:** Commit: `feat(plugin): parse node IDs and Figma links for panel search`.

### Task 2: Core go-to-nodes with zoom cap

**Files:**
- Modify: `plugin/src/write-viewport.ts`
- Modify: `plugin/src/main.ts` (`figma.ui.onmessage`)
- Test: `plugin/src/write-viewport.test.ts`, `plugin/src/main.test.ts`

**Interfaces:**
- Produces in `write-viewport.ts`:
  `goToNodes(nodeIds: string[], opts: { select: boolean; maxZoom?: number }): Promise<{ nodes: any[]; page: any }>`
  It holds the lookup, the PAGE/DOCUMENT check, the detached check, the same-page check, the page switch, the selection, and `scrollAndZoomIntoView`. After the fit, if `maxZoom` is set and `figma.viewport.zoom > maxZoom`, it sets `figma.viewport.zoom = maxZoom`. It throws the same error texts `set_selection` throws today. `nodeIds` is never empty here.
- The `set_selection` handler keeps its own empty-list (clear) branch and its `zoom: false` option. It calls `goToNodes` without `maxZoom`. Its result shape does not change.
- Produces in `main.ts`: UI-to-core message `{ type: "go_to_nodes", nodeIds: string[], select: boolean }`. Core reply: `{ type: "go_to_nodes_result", count: number, pageName: string }` or `{ type: "go_to_nodes_result", error: string }`. The core calls `goToNodes(nodeIds, { select, maxZoom: 1 })`.

- [ ] **Step 1: Write the failing tests.** In `write-viewport.test.ts`, add `zoom` to the mock viewport, then:
  - `goToNodes(["1:1"], { select: true, maxZoom: 1 })` with a mock fit that sets `zoom = 8` leaves `zoom === 1`
  - the same with a fit that sets `zoom = 0.25` leaves `zoom === 0.25`
  - `goToNodes(["1:1"], { select: false, maxZoom: 1 })` leaves `currentPage.selection` unchanged
  - `set_selection` with a fit that sets `zoom = 8` leaves `zoom === 8` (no cap for the AI tool)
  The existing `set_selection` tests must still pass unchanged.
  In `main.test.ts`:
  - message `go_to_nodes` for a known node posts `{ type: "go_to_nodes_result", count: 1, pageName }`
  - message `go_to_nodes` for `"9:9"` (missing) posts a reply whose `error` contains `Node not found: 9:9`
- [ ] **Step 2:** Run `cd plugin && bun test src/write-viewport.test.ts src/main.test.ts`. Expected: the new tests FAIL.
- [ ] **Step 3:** Move the shared logic from the `set_selection` handler into `goToNodes`, add the cap, and add the `go_to_nodes` branch to `figma.ui.onmessage` in `main.ts`. Catch the error and send it as `error`.
- [ ] **Step 4:** Run `cd plugin && bun test`. Expected: PASS, all files.
- [ ] **Step 5:** Commit: `feat(plugin): go to nodes from the panel with a 100% zoom cap`.

### Task 3: Panel search box and clickable node items

**Files:**
- Modify: `plugin/src/ui/App.svelte`
- Modify: `plugin/src/ui/strings.ts`

**Interfaces:**
- Consumes: `parseNodeIds` (Task 1), the `go_to_nodes` / `go_to_nodes_result` messages (Task 2).

Strings to add to `t`:
- `searchNode: "Search node"`
- `searchPlaceholder: "Node ID or Figma link"`
- `searchWent: (count: number, page: string) => \`Went to ${count} node(s) · page ${page}\``
- `searchInvalid: (tokens: string[]) => \`Not a node ID: ${tokens.join(", ")}\``
- `goToNodeTitle: "Show this node"`

Behaviour:
- The box and button are the first child of `.info-section`, above the File row. Styles follow the mockup: `.seek`, `.seek-row`, `.seek-input` (monospace, 11px, red border when there is an error), `.seek-btn` (accent colours), `.seek-msg` (10px, red for errors, muted for success).
- Button click or Enter: run `parseNodeIds`. No IDs and no invalid tokens: do nothing. Any invalid token: show `searchInvalid` and send nothing. Otherwise post `go_to_nodes` with `select: true`.
- `go_to_nodes_result` sets the line: `searchWent(count, pageName)` or the `error` text.
- Typing in the box clears the line and the red border.
- The `plugin-status` handler does not touch the box value.
- A click on `.node-name` in a node item posts `go_to_nodes` with `[node.id]` and `select: false`. Add `cursor: pointer` and `title={t.goToNodeTitle}`. The Copy button keeps its own behaviour.

- [ ] **Step 1:** Make the changes above.
- [ ] **Step 2:** Run `cd plugin && bun test && bun run typecheck && bun run build`. Expected: all pass. `strings.test.ts` covers the new strings.
- [ ] **Step 3:** Check in Figma Desktop with `plugin/manifest.json` loaded from `plugin/dist`:
  - type `1:2` (an ID on another page) and press Enter: the page switches, the node is selected, the zoom is at most 100%
  - paste a link copied with "Copy link to selection": same result
  - type `abc`: red line `Not a node ID: abc`
  - select 2 nodes, click the second in the list: the view moves, the list still shows 2 nodes
  - change the selection on the canvas: the box text stays
- [ ] **Step 4:** Commit: `feat(plugin): search node box and clickable selection list in the panel`.

### Task 4: Close out

- [ ] **Step 1:** Add an entry under the unreleased section of `CHANGELOG.md` if the file has one. Check its format first.
- [ ] **Step 2:** Move the lasting decisions (zoom cap 100% for panel calls only, click does not change the selection, accepted ID forms) into `docs/specs/2026-11-27-panel-search-node-design.md` if they are not there yet. Then delete this plan.
- [ ] **Step 3:** Commit: `docs: close the panel search node plan`.

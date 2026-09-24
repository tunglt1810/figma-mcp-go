# figma-mcp-go

Figma MCP — Local Plugin Integration [![tunglt1810/figma-mcp-go server](https://glama.ai/mcp/servers/tunglt1810/figma-mcp-go/badges/score.svg)](https://glama.ai/mcp/servers/tunglt1810/figma-mcp-go)
<p>
  <a href="https://www.npmjs.com/package/@tunglt1810/figma-mcp-go"><img src="https://img.shields.io/npm/v/@tunglt1810/figma-mcp-go?color=blue" alt="npm version" /></a>
  <a href="https://registry.modelcontextprotocol.io/?q=figma-mcp-go"><img src="https://img.shields.io/badge/MCP-Registry-purple" alt="MCP Registry" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT" /></a>
  <a href="https://github.com/tunglt1810/figma-mcp-go/stargazers"><img src="https://img.shields.io/github/stars/tunglt1810/figma-mcp-go?style=social" alt="GitHub stars" /></a>
</p>

Open-source Figma MCP server with full read and write access through a Figma plugin. Turn text into designs and designs into code. Works with Claude, Cursor, GitHub Copilot, and any MCP client.

**Highlights**
- Runs locally through the Figma Plugin API. No REST API token needed.
- Reads and edits the open Figma file live — 60 tools.
- Covers styles, variables, components, prototypes, text, and multi-step batches with undo.
- Built-in prompts such as `read_design_strategy` and `design_strategy`.
- Lean on tokens: short tool descriptions, capped node trees, and images returned as images.

**Styles, Variables, Components, Prototypes, and Content**

https://github.com/user-attachments/assets/eae41471-fc72-4574-8261-4f42c38b8c99

**Text to Design, Design to Code**

https://github.com/user-attachments/assets/17bda971-0e83-4f18-8758-8ac2b8dcba62

---

## Why this exists

Most Figma MCP servers use the cloud **Figma REST API**. AI tools make hundreds of quick calls per session, and every cloud round trip adds delay.

This server talks to a **Figma plugin** on your own machine instead. You get fast, live read and write access to the open file, with no API token.

---

## Setup

Install with `npx` or `bunx`. No build step. Watch the setup video or follow the steps below.

[![Watch the video](https://img.youtube.com/vi/DjqyU0GKv9k/sddefault.jpg)](https://youtu.be/DjqyU0GKv9k)

### 1. Add the server to your AI tool

**Claude Code CLI**
```bash
# via npx
claude mcp add -s project figma-mcp-go -- npx -y @tunglt1810/figma-mcp-go@latest

# or via bunx
claude mcp add -s project figma-mcp-go -- bunx @tunglt1810/figma-mcp-go@latest
```

**Codex CLI**
```bash
# via npx
codex mcp add figma-mcp-go -- npx -y @tunglt1810/figma-mcp-go@latest

# or via bunx
codex mcp add figma-mcp-go -- bunx @tunglt1810/figma-mcp-go@latest
```

**.mcp.json** (Claude and other MCP-compatible tools)
```json
{
  "mcpServers": {
    "figma-mcp-go": {
      "command": "npx",
      "args": ["-y", "@tunglt1810/figma-mcp-go"]
    }
  }
}
```

**.vscode/mcp.json** (Cursor / VS Code / GitHub Copilot)
```json
{
  "servers": {
    "figma-mcp-go": {
      "type": "stdio",
      "command": "npx",
      "args": [
        "-y",
        "@tunglt1810/figma-mcp-go"
      ]
    }
  }
}
```

### 2. Install the Figma plugin

1. In Figma Desktop: **Plugins → Development → Import plugin from manifest**
2. Pick `manifest.json` from [plugin.zip](https://github.com/tunglt1810/figma-mcp-go/releases)
3. Run the plugin in any Figma file

### 3. Use more than one AI tool (optional)

Each AI tool starts its own server, but only one can hold the plugin connection on a port.

**Same file:** change nothing. The first server takes port 1994; the others forward their calls to it. If it stops, another takes over in a few seconds.

**Different files:** give each tool its own port, and set the same port in that file's plugin (settings gear):

```json
{
  "mcpServers": {
    "figma-mcp-go": {
      "command": "npx",
      "args": ["-y", "@tunglt1810/figma-mcp-go", "--port", "1995"]
    }
  }
}
```

The plugin remembers the last port for all files, so check it when you open the plugin elsewhere.

`--ip 0.0.0.0` accepts connections from other machines.

> **Security:** the plugin connection has no login. On the default `127.0.0.1` only your machine can reach it. On any other address, anyone who can reach the port can read and edit your open file. The server warns you and the plugin turns on `confirm` mode. Prefer an SSH tunnel.

### Plugin panel

The panel shows the connected file, the selection, and what the AI is doing.

| Control | What it does |
| ------- | ------------ |
| **Guard** | `off` (default) runs everything. `confirm` asks before deletes and bulk changes. `read-only` blocks all changes. |
| **Undo** | Undoes the last change. A whole `batch_execute_pipeline` run is one undo step. |
| **Log** | Shows every request with tool, time, and error. `Copy` exports it for bug reports. |
| **Pin** | Saves the current selection. `get_selection(source: "pinned")` returns it even after the user clicks elsewhere. |

Settings and panel size are saved per machine. The panel follows Figma's light and dark theme.

### Dev Mode

`set_codegen_result` saves code on a node. It shows in Dev Mode's **Code** panel for everyone on the file. Code on a component shows for all its instances. The **Language** menu filters blocks; if a language has no block, all blocks show.

---

## Tools

### Read

| Tool | What it does |
| ---- | ------------ |
| `get_document` | Node tree of the selection, page, or whole file. Stops at 500 nodes by default; `depth`, `maxNodes`, `detail`, `dedupe_components` control size |
| `get_nodes_info` | Full details of nodes by ID. Unknown IDs go in `missing`; `depth` and `maxNodes` limit children |
| `get_selection` | Selected nodes, or pinned ones with `source: "pinned"` |
| `get_metadata` | File name, current page, and all pages |
| `search_nodes` | Find nodes by name and/or type on a page, in a node, or in the whole file |
| `get_viewport` | View center, zoom, and visible area |
| `get_styles` | Paint, text, effect, and grid styles |
| `get_variable_defs` | Variable collections, modes, and values |
| `get_local_components` | All components and component sets |
| `get_instance_overrides` | An instance's component properties and values |
| `get_annotations` | Dev Mode annotations |
| `get_fonts` | Fonts on the current page, most used first |
| `get_reactions` | A node's prototype reactions |

### Export

| Tool | What it does |
| ---- | ------------ |
| `export_screenshots` | Export nodes as images: saved to a file with `outputPath`, or returned (PNG/JPG as image, SVG as text). No items = the selection |
| `export_frames_to_pdf` | Save frames as one multi-page PDF |
| `get_image_bytes` | Original image files from image fills, as base64 |
| `export_tokens` | Variables and paint styles as JSON or CSS |
| `set_export_settings` | Set a node's Export presets |

### Create

| Tool | What it does |
| ---- | ------------ |
| `create_node` | FRAME, RECTANGLE, ELLIPSE, STAR, POLYGON, LINE, or SECTION |
| `create_text` | Text node (loads the font) |
| `create_vector` | Vector from SVG, e.g. an icon |
| `import_image` | Image from a URL or base64, as a new rectangle or into a node |
| `create_component` | Turn a frame into a component |
| `create_component_instance` | Instance of a local or library component |
| `combine_as_variants` | Combine components into a variant set |
| `manage_component_properties` | Add, edit, delete, or bind component properties |
| `create_connector` | Connector line (FigJam only) |
| `boolean_operation` | UNION, SUBTRACT, INTERSECT, or EXCLUDE shapes |
| `flatten_nodes` | Merge nodes into one vector |
| `outline_stroke` | Turn a stroke into a filled shape |

### Edit

| Tool | What it does |
| ---- | ------------ |
| `set_node_properties` | Position, size, radius, visibility, lock, opacity, rotation, blend, constraints, layer order, mask, stroke |
| `set_paint` | Solid or gradient fill, or solid stroke |
| `set_auto_layout` | Auto layout: direction, padding, gap, align, HUG/FILL sizing, min/max |
| `set_layout_grids` | Column, row, or square grids |
| `set_effects` | Shadows, blurs, noise, texture, glass |
| `set_text` | Text content and whole-node text settings |
| `set_text_ranges` | Style part of a text: bold, color, link, list |
| `find_replace_text` | Find and replace text, regex allowed |
| `set_instance_overrides` | Set an instance's component properties |
| `swap_component` | Swap an instance to another component |
| `detach_instance` | Turn instances into plain frames |
| `clone_node` | Copy a node |
| `reparent_nodes` | Move nodes into another parent |
| `group_nodes` / `ungroup_nodes` | Group or ungroup |
| `batch_rename_nodes` | Rename by name, find/replace, regex, prefix, or suffix |
| `set_annotations` | Dev Mode annotations (paid seat) |
| `set_reactions` | Prototype reactions: set, append, or remove |
| `delete_nodes` | Delete nodes |

### Styles, variables, pages

| Tool | What it does |
| ---- | ------------ |
| `create_style` | PAINT, TEXT, EFFECT, or GRID style |
| `update_paint_style` | Rename or recolor a paint style |
| `apply_style_to_node` | Apply a style to a node |
| `delete_style` | Delete a style |
| `manage_variable` | Create collections, modes, and variables; set values; delete; bind to nodes |
| `manage_page` | Add, delete, rename, or go to a page |

### Workflow

| Tool | What it does |
| ---- | ------------ |
| `batch_execute_pipeline` | Run many write steps in one call, pass results between steps, undo on error |
| `set_selection` | Select and zoom to nodes to show the user |
| `save_version_checkpoint` | Save a named version before big changes |
| `set_codegen_result` | Save code for Dev Mode's Code panel |
| `manage_plugin_data` | Store key/value notes on a node in the file |

### Prompts

| Prompt | What it does |
| ------ | ------------ |
| `read_design_strategy` | How to read designs well |
| `design_strategy` | How to create and edit designs |
| `text_replacement_strategy` | Replace text across a design in chunks |
| `annotation_conversion_strategy` | Turn manual notes into Figma annotations |
| `swap_overrides_instances` | Copy overrides between instances |
| `reaction_to_connector_strategy` | Turn prototype links into flow diagrams |

---

## Upgrading

**Update the plugin when you update the server.** `npx` updates the server, but the plugin is installed by hand. An old plugin rejects new commands with `Unknown request type`.

### 0.4.0 — token savings

- `export_screenshots` returns PNG/JPG as MCP image blocks and SVG as text, not base64 in JSON. Each result's `contentIndex` points to its block. PDF stays base64. Returned images default to scale 1; files stay at 2.
- `get_document` stops at 500 nodes by default. Scope `selection` now really stops at 2 levels.
- `get_nodes_info` accepts `depth` and `maxNodes` (default 500) and sets `truncated`.
- Text nodes no longer report default alignment (`LEFT`, `TOP`).
- `tools/list` is about 40% smaller: shorter descriptions, and annotations only where they differ from MCP defaults (read tools now have `readOnlyHint`).

### 0.3.0

No tool names or arguments changed.

- **Logs** are structured (`time=… level=INFO msg=… component=bridge`) instead of `[bridge] …`. They still go to stderr.
- **Log level:** `FIGMA_MCP_LOG=debug|info|warn|error` (default `info`). Tool parameters only show at `debug`.
- **Startup:** calls made before the server is ready say so, instead of `connection refused`. Calls during a plugin reconnect wait for it.
- **Large requests** no longer disconnect the plugin or block other calls.
- **`/ping`** also returns `role`, `connected`, `pending`, and `uptimeSeconds`.

Removed tools and their replacements:

| Removed | Use instead |
| ------- | ----------- |
| `set_layout_sizing` | `set_auto_layout({ nodeIds, … })` |
| `get_node` | `get_nodes_info({ nodeIds: [id] })` |
| `get_pages` | `get_metadata()` |
| `scan_nodes_by_types` | `search_nodes({ nodeId, types, includeHidden: false })` |
| `scan_text_nodes` | `search_nodes({ nodeId, types: ["TEXT"], includeText: true })` |
| `clear_annotations` | `set_annotations({ nodeIds, annotations: [] })` |
| `rename_node` | `batch_rename_nodes({ nodeIds, name })` |
| `move_nodes` | `set_node_properties({ nodeIds, x, y })` |
| `resize_nodes` | `set_node_properties({ nodeIds, width, height })` |
| `set_corner_radius` | `set_node_properties({ nodeIds, cornerRadius })` |
| `remove_reactions` | `set_reactions({ nodeId, removeIndices })` |
| `get_design_context` | `get_document({ scope: "selection", detail, dedupe_components })` |
| `get_screenshot` / `save_screenshots` | `export_screenshots({ items })` |
| `create_variable_collection` | `manage_variable({ action: "create_collection", name })` |
| `add_variable_mode` | `manage_variable({ action: "add_mode", collectionId, modeName })` |
| `create_variable` | `manage_variable({ action: "create", name, collectionId, type, value })` |
| `set_variable_value` | `manage_variable({ action: "set_value", variableId, modeId, value })` |
| `delete_variable` | `manage_variable({ action: "delete", variableId \| collectionId })` |
| `bind_variable_to_node` | `manage_variable({ action: "bind", nodeId, variableId, field })` |

Changed responses: `set_auto_layout` and `set_annotations` return `{results}` (one per node); `get_document` returns `{fileName, scope, currentPage, nodes}` for every scope; `export_screenshots` returns `{total, succeeded, failed, results}`.

### 0.1.0

| Removed | Use instead |
| ------- | ----------- |
| `set_visible` | `set_node_properties({ nodeIds, visible })` |
| `lock_nodes` / `unlock_nodes` | `set_node_properties({ nodeIds, locked })` |
| `set_opacity` | `set_node_properties({ nodeIds, opacity })` |
| `rotate_nodes` | `set_node_properties({ nodeIds, rotation })` |
| `set_blend_mode` | `set_node_properties({ nodeIds, blendMode })` |
| `set_constraints` | `set_node_properties({ nodeIds, constraints: { horizontal, vertical } })` |
| `reorder_nodes` | `set_node_properties({ nodeIds, order })` |
| `create_frame`, `create_rectangle`, `create_ellipse`, `create_star`, `create_polygon`, `create_line`, `create_section` | `create_node({ type, … })` |
| `set_fills` | `set_paint({ type: "SOLID", color })` |
| `set_strokes` | `set_paint({ type: "SOLID", target: "stroke", color, strokeWeight })` |
| `set_gradient_fills` | `set_paint({ type: "GRADIENT_LINEAR" \| "GRADIENT_RADIAL", stops, geometry })` |
| `create_paint_style`, `create_text_style`, `create_effect_style`, `create_grid_style` | `create_style({ type: "PAINT" \| "TEXT" \| "EFFECT" \| "GRID", name, … })` |
| `add_page`, `delete_page`, `rename_page`, `navigate_to_page` | `manage_page({ action: "add" \| "delete" \| "rename" \| "navigate", … })` |

Notes: `create_effect_style`'s `type` is now `effectType`. Gradients work on fills only. Ellipse arcs and rings (`startAngle`, `endAngle`, `innerRadiusRatio`) now work.

---

## Development

- **Server:** Go 1.27+ (`make test-go`, `make build-go`)
- **Plugin:** Bun 1.4+ (`cd plugin && bun install && bun run build`)
- **All tests:** `make test`
- **Logs:** stderr. `FIGMA_MCP_LOG=debug` shows tool parameters and traffic.
- **Package rules:** `make deps-check` fails on wrong-way imports. `internal/` has `bridge` (plugin WebSocket), `cluster` (leader election), `figma` (rules), and `tools` (tool table). Allowed: `tools → figma`, `cluster → bridge`.

macOS binaries need macOS 13 or later (Go 1.27).

## Contributing

Issues and PRs are welcome.

## Star History

<a href="https://www.star-history.com/?repos=tunglt1810%2Ffigma-mcp-go&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=tunglt1810/figma-mcp-go&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=tunglt1810/figma-mcp-go&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=tunglt1810/figma-mcp-go&type=date&legend=top-left" />
 </picture>
</a>

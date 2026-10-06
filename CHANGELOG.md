# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

**Update the plugin when you update the server.** `npx` updates the server, but
the plugin is installed by hand. An old plugin rejects new commands with
`Unknown request type`.

## [Unreleased]

## [0.5.0] - 2026-10-06

### Added

- `--read-only` flag. The server then offers only the 17 tools that cannot change the Figma file. `tools/list` is about 84% smaller.
- `set_paint` accepts `GRADIENT_ANGULAR` and `GRADIENT_DIAMOND`. A gradient can now go on a stroke.
- Read tools now report angular and diamond gradients. Before, these paints were left out.
- `swap_component` accepts `componentKey` for a library component. It also accepts a component set and uses its default variant.
- `create_component_instance` accepts the key of a library component set.

### Changed

- `swap_component` no longer requires `componentId`. It requires `componentId` or `componentKey`.
- `create_component_instance` rejects `x` without `y`, and `y` without `x`. Before, it ignored a single coordinate.
- Tool and parameter descriptions in `tools/list` now use full sentences and no semicolons. Tool names and arguments are unchanged.
- The `create_connector` error outside a FigJam file now reads `The create_connector tool operates only in a FigJam file`.
- The specs in `docs/specs` use simpler wording.

### Fixed

- `set_paint` now applies `opacity` to a solid stroke. Before, it ignored the value.
- The `maxNodes` description of `get_document` now says that the limit also does not apply with `depth`.
- The FigJam connectors spec said that a connector needs two endpoints. It now says one is enough, as the server requires.

## [0.4.1] - 2026-09-27

### Changed

- Shorter tool descriptions in `tools/list`. Tool names and arguments are unchanged.
- `glama.json` tool descriptions now match the server.
- Clearer wording in code comments and docs.

## [0.4.0] - 2026-09-25

### Added

- `get_nodes_info` accepts `depth` and `maxNodes` (default 500) and sets `truncated`.
- `export_screenshots` results have a `contentIndex` that points to their content block.
- Read tools now have `readOnlyHint`.

### Changed

- `export_screenshots` returns PNG/JPG as MCP image blocks and SVG as text, not base64 in JSON. PDF stays base64.
- Returned images default to scale 1. Files stay at 2.
- `get_document` stops at 500 nodes by default.
- Text nodes no longer report default alignment (`LEFT`, `TOP`).
- `tools/list` is about 40% smaller: shorter descriptions, and annotations only where they differ from MCP defaults.

### Fixed

- `get_document` with scope `selection` now really stops at 2 levels.

## [0.3.1] - 2026-08-30

### Added

- Log level setting: `FIGMA_MCP_LOG=debug|info|warn|error` (default `info`). Tool parameters only show at `debug`.
- `/ping` also returns `role`, `connected`, `pending`, and `uptimeSeconds`.

### Changed

- Logs are structured (`time=… level=INFO msg=… component=bridge`) instead of `[bridge] …`. They still go to stderr.
- Calls made before the server is ready say so, instead of `connection refused`. Calls during a plugin reconnect wait for it.
- `set_auto_layout` and `set_annotations` return `{results}`, one per node.
- `get_document` returns `{fileName, scope, currentPage, nodes}` for every scope.
- `export_screenshots` returns `{total, succeeded, failed, results}`.

### Removed

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

### Fixed

- Large requests no longer disconnect the plugin or block other calls.

## [0.1.0] - 2026-08-17

### Changed

- `create_effect_style`'s `type` argument is now `effectType` (in `create_style`).
- Gradients work on fills only.

### Removed

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

### Fixed

- Ellipse arcs and rings (`startAngle`, `endAngle`, `innerRadiusRatio`) now work.

[Unreleased]: https://github.com/tunglt1810/figma-mcp-go/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/tunglt1810/figma-mcp-go/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/tunglt1810/figma-mcp-go/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/tunglt1810/figma-mcp-go/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/tunglt1810/figma-mcp-go/compare/v0.2.0...v0.3.1
[0.1.0]: https://github.com/tunglt1810/figma-mcp-go/compare/v0.0.7...v0.1.0

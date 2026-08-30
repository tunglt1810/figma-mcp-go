import { describe, expect, it } from "bun:test";
import { readHandlers } from "./read-handlers";
import { writeHandlers } from "./write-handlers";

// The Go server declares the tools clients see; this plugin implements them.
// Nothing has ever checked that the two agree, and a mismatch is invisible from
// either side: the server happily offers a tool whose only symptom is "Unknown
// request type" at call time, and a handler nobody routes to is dead code that
// still gets maintained.
//
// The golden schema snapshot is the server's side of the contract, already
// regenerated whenever a tool changes, so it is read rather than duplicated.

const golden = await Bun.file(
  new URL("../../internal/tools/testdata/tools_schema.json", import.meta.url),
).json();

const goTools = new Set<string>(Object.keys(golden));
const pluginHandlers = new Set<string>([
  ...Object.keys(readHandlers),
  ...Object.keys(writeHandlers),
  // Intercepted before dispatch, so it is not in either map.
  "batch_execute_pipeline",
]);

/** Tools the Go server answers itself, without ever asking the plugin. */
const SERVER_SIDE_TOOLS = new Set([
  "export_screenshots",
]);

/**
 * Handlers the merged tools delegate to.
 *
 * The MCP surface consolidated seven shape tools into create_node, four page
 * tools into manage_page, and so on. The originals stayed as implementations —
 * a pipeline step still names them directly — so they are handlers without
 * being tools.
 */
const INTERNAL_DISPATCH_TARGETS = new Set([
  "create_frame", "create_rectangle", "create_ellipse", "create_star",
  "create_polygon", "create_line", "create_section",
  "add_page", "delete_page", "rename_page", "navigate_to_page",
  "create_paint_style", "create_text_style", "create_effect_style", "create_grid_style",
  "set_fills", "set_strokes", "set_gradient_fills",
  // export_screenshots calls this once per item, with params it builds itself.
  "get_screenshot",
]);

describe("the plugin implements what the server offers", () => {
  it("every tool the server declares has a handler", () => {
    const missing = [...goTools]
      .filter((tool) => !pluginHandlers.has(tool) && !SERVER_SIDE_TOOLS.has(tool))
      .sort();
    expect(missing).toEqual([]);
  });

  it("every handler is reachable — as a tool, or as a documented delegation target", () => {
    const unreachable = [...pluginHandlers]
      .filter((name) => !goTools.has(name) && !INTERNAL_DISPATCH_TARGETS.has(name))
      .sort();
    expect(unreachable).toEqual([]);
  });

  it("the server-side list names tools that really exist", () => {
    for (const tool of SERVER_SIDE_TOOLS) {
      expect(goTools.has(tool)).toBe(true);
    }
  });

  it("the delegation list names handlers that really exist", () => {
    for (const name of INTERNAL_DISPATCH_TARGETS) {
      expect(pluginHandlers.has(name)).toBe(true);
    }
  });
});

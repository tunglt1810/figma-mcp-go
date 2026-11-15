import { describe, expect, it } from "bun:test";
import { readHandlers } from "./read-handlers";
import { writeHandlers } from "./write-handlers";
import { isMutating } from "./tool-classes";

// The Go server declares the tools clients see, and this plugin implements
// them. Nothing ever checked that the two agree, and neither side can see a
// mismatch. The server offers a tool whose only symptom is "Unknown request
// type" at call time. A handler nothing routes to is dead code that still
// gets maintained.
//
// The golden schema snapshot is the server's side of the contract, and it is
// regenerated whenever a tool changes. So it is read here, not duplicated.

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

/** Tools the Go server answers by itself, without asking the plugin. */
const SERVER_SIDE_TOOLS = new Set([
  "export_screenshots",
]);

/** Handlers that a Go handler calls under a name no client sees. */
const INTERNAL_DISPATCH_TARGETS = new Set([
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

  it("every handler is reachable — as a tool, or as a documented internal call", () => {
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

// The server's --read-only mode and the panel's read-only guard answer the
// same question. The Go spec table is the one definition. This reads what it
// allows and checks that the panel allows the same calls, no more and no fewer.
const readOnlyCalls: Record<string, true | Record<string, boolean>> = await Bun.file(
  new URL("../../internal/tools/testdata/read_only_calls.json", import.meta.url),
).json();

describe("the panel's read-only guard matches the server's read-only mode", () => {

  const cases: [string, any, boolean][] = [];
  for (const tool of goTools) {
    if (SERVER_SIDE_TOOLS.has(tool)) continue;
    const allowed = readOnlyCalls[tool];
    if (allowed !== undefined && allowed !== true) {
      for (const [action, runs] of Object.entries(allowed)) cases.push([tool, { action }, runs]);
    } else {
      cases.push([tool, undefined, allowed === true]);
    }
  }

  it.each(cases)("%s %j: runs in read-only mode = %p", (tool, params, runs) => {
    expect(isMutating(tool, params)).toBe(!runs);
  });
});

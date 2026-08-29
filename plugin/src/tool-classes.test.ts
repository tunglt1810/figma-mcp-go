import { describe, expect, it } from "bun:test";
import { readHandlers } from "./read-handlers";
import { writeHandlers } from "./write-handlers";
import {
  DESTRUCTIVE_TOOLS,
  HARMLESS_WRITE_TOOLS,
  READ_TOOLS,
  destructiveReason,
  isDestructive,
  isMutating,
} from "./tool-classes";

// ── The lists must track the real handler maps ───────────────────────────────

describe("tool classification covers the real handlers", () => {
  it("READ_TOOLS is exactly the read handler map", () => {
    expect([...READ_TOOLS].sort()).toEqual(Object.keys(readHandlers).sort());
  });

  it("every write handler is accounted for", () => {
    for (const name of Object.keys(writeHandlers)) {
      expect(READ_TOOLS.has(name)).toBe(false);
    }
  });

  it("HARMLESS_WRITE_TOOLS name real write handlers", () => {
    for (const name of HARMLESS_WRITE_TOOLS) {
      expect(Object.keys(writeHandlers)).toContain(name);
    }
  });

  it("DESTRUCTIVE_TOOLS name real write handlers", () => {
    for (const name of DESTRUCTIVE_TOOLS) {
      expect(Object.keys(writeHandlers)).toContain(name);
    }
  });

  it("a tool cannot be both harmless and destructive", () => {
    for (const name of DESTRUCTIVE_TOOLS) {
      expect(HARMLESS_WRITE_TOOLS.has(name)).toBe(false);
    }
  });
});

// ── isMutating ───────────────────────────────────────────────────────────────

describe("isMutating", () => {
  it("reads never mutate", () => {
    expect(isMutating("get_document")).toBe(false);
    expect(isMutating("search_nodes")).toBe(false);
  });

  it("ordinary writes mutate", () => {
    expect(isMutating("create_frame")).toBe(true);
    expect(isMutating("set_text")).toBe(true);
  });

  it("pointing at a node and saving a version do not mutate", () => {
    expect(isMutating("set_selection")).toBe(false);
    expect(isMutating("save_version_checkpoint")).toBe(false);
  });

  it("navigating pages is allowed but the other page actions are not", () => {
    expect(isMutating("manage_page", { action: "navigate" })).toBe(false);
    expect(isMutating("manage_page", { action: "add" })).toBe(true);
    expect(isMutating("manage_page", { action: "delete" })).toBe(true);
    // No action at all is not a navigate, so it is treated as a change.
    expect(isMutating("manage_page", {})).toBe(true);
  });

  it("reading plugin data does not mutate, writing it does", () => {
    expect(isMutating("manage_plugin_data", { action: "get" })).toBe(false);
    expect(isMutating("manage_plugin_data", { action: "keys" })).toBe(false);
    expect(isMutating("manage_plugin_data", { action: "set" })).toBe(true);
    expect(isMutating("manage_plugin_data", { action: "delete" })).toBe(true);
  });

  it("a pipeline mutates when any step does", () => {
    expect(isMutating("batch_execute_pipeline", {
      steps: [{ action: "get_node" }, { action: "set_selection" }],
    })).toBe(false);
    expect(isMutating("batch_execute_pipeline", {
      steps: [{ action: "get_node" }, { action: "create_frame" }],
    })).toBe(true);
  });

  it("a pipeline with unreadable steps is assumed to mutate", () => {
    expect(isMutating("batch_execute_pipeline", {})).toBe(true);
  });

  it("an unknown tool is assumed to mutate", () => {
    expect(isMutating("some_future_tool")).toBe(true);
  });
});

// ── isDestructive ────────────────────────────────────────────────────────────

describe("isDestructive", () => {
  it("flags the deletes and the bulk rewrites", () => {
    expect(isDestructive("delete_nodes")).toBe(true);
    expect(isDestructive("find_replace_text")).toBe(true);
    expect(isDestructive("detach_instance")).toBe(true);
  });

  it("leaves ordinary edits alone", () => {
    expect(isDestructive("set_text")).toBe(false);
    expect(isDestructive("create_frame")).toBe(false);
    expect(isDestructive("ungroup_nodes")).toBe(false);
  });

  it("flags manage_page only when it deletes", () => {
    expect(isDestructive("manage_page", { action: "delete" })).toBe(true);
    expect(isDestructive("manage_page", { action: "add" })).toBe(false);
  });

  it("flags manage_component_properties only when it deletes", () => {
    expect(isDestructive("manage_component_properties", { action: "delete" })).toBe(true);
    expect(isDestructive("manage_component_properties", { action: "add" })).toBe(false);
  });

  // Undo puts these back, but the pipeline's rollback log cannot un-merge
  // geometry Figma has already combined.
  it("flags the geometry operations that consume their inputs", () => {
    expect(isDestructive("boolean_operation")).toBe(true);
    expect(isDestructive("flatten_nodes")).toBe(true);
    expect(isDestructive("outline_stroke")).toBe(false);
  });

  it("flags a pipeline carrying a destructive step", () => {
    expect(isDestructive("batch_execute_pipeline", {
      steps: [{ action: "create_frame" }, { action: "delete_nodes" }],
    })).toBe(true);
    expect(isDestructive("batch_execute_pipeline", {
      steps: [{ action: "create_frame" }],
    })).toBe(false);
  });

  it("flags a pipeline whose destructive step is a page delete", () => {
    expect(isDestructive("batch_execute_pipeline", {
      steps: [{ action: "manage_page", params: { action: "delete" } }],
    })).toBe(true);
  });
});

// ── destructiveReason ────────────────────────────────────────────────────────

describe("destructiveReason", () => {
  it("names the tool for a plain call", () => {
    expect(destructiveReason("delete_nodes")).toBe("delete_nodes");
  });

  it("names the offending step for a pipeline", () => {
    expect(destructiveReason("batch_execute_pipeline", {
      steps: [{ action: "create_frame" }, { action: "delete_nodes" }],
    })).toBe("pipeline containing delete_nodes");
  });

  it("spells out a page delete", () => {
    expect(destructiveReason("manage_page", { action: "delete" })).toBe("delete_page");
  });

  it("names the component property being deleted", () => {
    expect(destructiveReason("manage_component_properties", { action: "delete", property: "Size" }))
      .toBe("delete component property Size");
  });
});

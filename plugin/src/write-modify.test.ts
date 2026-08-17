import { describe, it, expect, beforeEach } from "vitest";
import { handleWriteModifyRequest } from "./write-modify";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

beforeEach(() => {
  commitUndoCalled = false;
  mockNodes = {};
  (globalThis as any).figma = {
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    commitUndo: () => { commitUndoCalled = true; },
  };
});

// ── set_opacity ───────────────────────────────────────────────────────────────

describe("set_corner_radius", () => {
  it("sets uniform cornerRadius", async () => {
    mockNodes["1:1"] = { id: "1:1", cornerRadius: 0 };
    const res = await handleWriteModifyRequest(makeRequest("set_corner_radius", ["1:1"], { cornerRadius: 8 }));
    expect(mockNodes["1:1"].cornerRadius).toBe(8);
    expect(res?.data.results[0].cornerRadius).toBe(8);
    expect(commitUndoCalled).toBe(true);
  });

  it("sets per-corner radii independently", async () => {
    mockNodes["1:1"] = {
      id: "1:1", cornerRadius: 0,
      topLeftRadius: 0, topRightRadius: 0, bottomLeftRadius: 0, bottomRightRadius: 0,
    };
    await handleWriteModifyRequest(makeRequest("set_corner_radius", ["1:1"], {
      topLeftRadius: 8, topRightRadius: 0, bottomLeftRadius: 8, bottomRightRadius: 0,
    }));
    expect(mockNodes["1:1"].topLeftRadius).toBe(8);
    expect(mockNodes["1:1"].topRightRadius).toBe(0);
    expect(mockNodes["1:1"].bottomLeftRadius).toBe(8);
    expect(mockNodes["1:1"].bottomRightRadius).toBe(0);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("set_corner_radius", ["9:9"], { cornerRadius: 4 }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without cornerRadius support", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Text" }; // no cornerRadius property
    const res = await handleWriteModifyRequest(makeRequest("set_corner_radius", ["1:1"], { cornerRadius: 4 }));
    expect(res?.data.results[0].error).toContain("does not support corner radius");
  });

  it("handles multiple nodeIds", async () => {
    mockNodes["1:1"] = { id: "1:1", cornerRadius: 0 };
    mockNodes["2:2"] = { id: "2:2", cornerRadius: 0 };
    const res = await handleWriteModifyRequest(makeRequest("set_corner_radius", ["1:1", "2:2"], { cornerRadius: 12 }));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].cornerRadius).toBe(12);
    expect(mockNodes["2:2"].cornerRadius).toBe(12);
  });

  it("returns null for unrecognised type", async () => {
    const res = await handleWriteModifyRequest(makeRequest("unknown_op"));
    expect(res).toBeNull();
  });
});

// ── set_visible ───────────────────────────────────────────────────────────────

describe("reparent_nodes", () => {
  it("moves a node to a new parent", async () => {
    const children: any[] = [];
    const newParent = { id: "2:2", appendChild: (n: any) => children.push(n) };
    mockNodes["1:1"] = { id: "1:1", name: "Node" };
    mockNodes["2:2"] = newParent;
    const res = await handleWriteModifyRequest(makeRequest("reparent_nodes", ["1:1"], { parentId: "2:2" }));
    expect(children).toHaveLength(1);
    expect(res?.data.results[0].newParentId).toBe("2:2");
    expect(commitUndoCalled).toBe(true);
  });

  it("throws if parentId is missing", async () => {
    await expect(handleWriteModifyRequest(makeRequest("reparent_nodes", ["1:1"], {}))).rejects.toThrow("parentId is required");
  });

  it("throws if parent node not found", async () => {
    mockNodes["1:1"] = { id: "1:1" };
    await expect(handleWriteModifyRequest(makeRequest("reparent_nodes", ["1:1"], { parentId: "9:9" }))).rejects.toThrow("Parent not found");
  });

  it("throws if parent cannot contain children", async () => {
    mockNodes["1:1"] = { id: "1:1" };
    mockNodes["2:2"] = { id: "2:2" }; // no appendChild
    await expect(handleWriteModifyRequest(makeRequest("reparent_nodes", ["1:1"], { parentId: "2:2" }))).rejects.toThrow("cannot contain children");
  });

  it("reports error for missing child node", async () => {
    const newParent = { id: "2:2", appendChild: () => {} };
    mockNodes["2:2"] = newParent;
    const res = await handleWriteModifyRequest(makeRequest("reparent_nodes", ["9:9"], { parentId: "2:2" }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });
});

// ── batch_rename_nodes ────────────────────────────────────────────────────────

describe("batch_rename_nodes", () => {
  it("renames with find/replace", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Button/Primary" };
    mockNodes["2:2"] = { id: "2:2", name: "Button/Secondary" };
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["1:1", "2:2"], {
      find: "Button", replace: "Btn",
    }));
    expect(mockNodes["1:1"].name).toBe("Btn/Primary");
    expect(mockNodes["2:2"].name).toBe("Btn/Secondary");
    expect(res?.data.results[0].oldName).toBe("Button/Primary");
    expect(res?.data.results[0].name).toBe("Btn/Primary");
    expect(commitUndoCalled).toBe(true);
  });

  it("adds prefix and suffix", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Card" };
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["1:1"], {
      prefix: "UI/", suffix: "_v2",
    }));
    expect(mockNodes["1:1"].name).toBe("UI/Card_v2");
    expect(res?.data.results[0].name).toBe("UI/Card_v2");
  });

  it("renames using regex", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame 123" };
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["1:1"], {
      find: "\\d+", replace: "X", useRegex: true,
    }));
    expect(mockNodes["1:1"].name).toBe("Frame X");
  });

  it("captures regex error per-node", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Card" };
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["1:1"], {
      find: "[invalid", replace: "X", useRegex: true,
    }));
    expect(res?.data.results[0].error).toContain("Invalid regex");
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["9:9"], { prefix: "x" }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("throws for empty nodeIds", async () => {
    await expect(handleWriteModifyRequest(makeRequest("batch_rename_nodes", [], { prefix: "x" }))).rejects.toThrow();
  });
});

// ── find_replace_text ─────────────────────────────────────────────────────────

describe("find_replace_text", () => {
  beforeEach(() => {
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      currentPage: {
        type: "PAGE",
        children: [],
      },
      loadFontAsync: async () => {},
    };
  });

  it("replaces text in matching TEXT nodes", async () => {
    const textNode = {
      id: "1:1", name: "Label", type: "TEXT", characters: "Hello World",
      fontName: { family: "Inter", style: "Regular" },
    };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [textNode] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "World", replace: "Figma" }));
    expect(textNode.characters).toBe("Hello Figma");
    expect(res?.data.replaced).toBe(1);
    expect(res?.data.results[0].newText).toBe("Hello Figma");
    expect(commitUndoCalled).toBe(true);
  });

  it("skips nodes where text does not match", async () => {
    const textNode = { id: "1:1", name: "Label", type: "TEXT", characters: "Goodbye", fontName: { family: "Inter", style: "Regular" } };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [textNode] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "Hello", replace: "Hi" }));
    expect(res?.data.replaced).toBe(0);
    expect(textNode.characters).toBe("Goodbye");
  });

  it("searches recursively through nested children", async () => {
    const textNode = { id: "2:2", name: "Nested", type: "TEXT", characters: "foo bar", fontName: { family: "Inter", style: "Regular" } };
    const frame = { id: "1:1", type: "FRAME", children: [textNode] };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [frame] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "foo", replace: "baz" }));
    expect(textNode.characters).toBe("baz bar");
    expect(res?.data.replaced).toBe(1);
  });

  it("supports scoped search within a subtree when nodeId provided", async () => {
    const textNode = { id: "2:2", name: "Inner", type: "TEXT", characters: "target", fontName: { family: "Inter", style: "Regular" } };
    const frame = { id: "1:1", type: "FRAME", children: [textNode] };
    mockNodes["1:1"] = frame;
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", ["1:1"], { find: "target", replace: "done" }));
    expect(textNode.characters).toBe("done");
    expect(res?.data.replaced).toBe(1);
  });

  it("uses regex when useRegex is true", async () => {
    const textNode = { id: "1:1", name: "Label", type: "TEXT", characters: "Price: $99", fontName: { family: "Inter", style: "Regular" } };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [textNode] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "\\$\\d+", replace: "$199", useRegex: true }));
    expect(textNode.characters).toBe("Price: $199");
    expect(res?.data.replaced).toBe(1);
  });

  it("captures regex error per-node", async () => {
    const textNode = { id: "1:1", type: "TEXT", characters: "hello", fontName: { family: "Inter", style: "Regular" } };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [textNode] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "[bad", replace: "x", useRegex: true }));
    expect(res?.data.replaced).toBe(0);
    expect(res?.data.results[0].error).toContain("Invalid regex");
  });

  it("throws if find is missing", async () => {
    await expect(handleWriteModifyRequest(makeRequest("find_replace_text", [], { replace: "x" }))).rejects.toThrow("find is required");
  });

  it("throws if replace is missing", async () => {
    await expect(handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "x" }))).rejects.toThrow("replace is required");
  });
});

// ── set_node_properties ───────────────────────────────────────────────────────
//
// Replaces set_visible, lock_nodes, unlock_nodes, set_opacity, rotate_nodes,
// reorder_nodes, set_blend_mode and set_constraints. Errors stay per-property:
// a node may support opacity but not rotation, and collapsing that into a
// single per-node error would lose detail the eight separate tools had.

describe("set_node_properties", () => {
  it("applies several properties in one call with a single undo entry", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame", visible: true, opacity: 1, rotation: 0 };

    const res = await handleWriteModifyRequest(
      makeRequest("set_node_properties", ["1:1"], { visible: false, opacity: 0.5, rotation: 45 }),
    );

    expect(res?.data.results[0].applied).toEqual({ visible: false, opacity: 0.5, rotation: 45 });
    expect(mockNodes["1:1"].visible).toBe(false);
    expect(mockNodes["1:1"].opacity).toBe(0.5);
    expect(mockNodes["1:1"].rotation).toBe(45);
    expect(commitUndoCalled).toBe(true);
  });

  it("applies falsy values rather than skipping them", async () => {
    mockNodes["1:1"] = { id: "1:1", visible: true, locked: true, opacity: 1 };

    const res = await handleWriteModifyRequest(
      makeRequest("set_node_properties", ["1:1"], { visible: false, locked: false, opacity: 0 }),
    );

    expect(res?.data.results[0].applied).toEqual({ visible: false, locked: false, opacity: 0 });
  });

  it("reports unsupported properties per property, still applying the rest", async () => {
    mockNodes["1:1"] = { id: "1:1", opacity: 1 }; // no rotation support

    const res = await handleWriteModifyRequest(
      makeRequest("set_node_properties", ["1:1"], { opacity: 0.5, rotation: 90 }),
    );

    expect(res?.data.results[0].applied).toEqual({ opacity: 0.5 });
    expect(res?.data.results[0].errors.rotation).toContain("does not support rotation");
    expect(mockNodes["1:1"].opacity).toBe(0.5);
  });

  it("reports a missing node once, not per property", async () => {
    const res = await handleWriteModifyRequest(
      makeRequest("set_node_properties", ["9:9"], { opacity: 0.5, visible: false }),
    );

    expect(res?.data.results[0].error).toBe("Node not found");
    expect(res?.data.results[0].applied).toBeUndefined();
  });

  it("merges constraints with the axes not supplied", async () => {
    mockNodes["1:1"] = { id: "1:1", constraints: { horizontal: "MIN", vertical: "MIN" } };

    const res = await handleWriteModifyRequest(
      makeRequest("set_node_properties", ["1:1"], { constraints: { horizontal: "STRETCH" } }),
    );

    expect(mockNodes["1:1"].constraints).toEqual({ horizontal: "STRETCH", vertical: "MIN" });
    expect(res?.data.results[0].applied.constraints).toEqual({ horizontal: "STRETCH", vertical: "MIN" });
  });

  it("reorders within the parent and reports the resulting index", async () => {
    const child = { id: "1:1" } as any;
    const parent: any = { id: "0:1", children: [child, { id: "2:2" }, { id: "3:3" }] };
    parent.insertChild = (index: number, node: any) => {
      parent.children = parent.children.filter((c: any) => c !== node);
      parent.children.splice(index, 0, node);
    };
    child.parent = parent;
    mockNodes["1:1"] = child;

    const res = await handleWriteModifyRequest(
      makeRequest("set_node_properties", ["1:1"], { order: "bringToFront" }),
    );

    expect(res?.data.results[0].applied.index).toBe(2);
    expect(parent.children[2]).toBe(child);
  });

  it("rejects an invalid order value", async () => {
    mockNodes["1:1"] = { id: "1:1" };
    await expect(
      handleWriteModifyRequest(makeRequest("set_node_properties", ["1:1"], { order: "sideways" })),
    ).rejects.toThrow(/order must be/);
  });

  it("requires at least one property", async () => {
    mockNodes["1:1"] = { id: "1:1", opacity: 1 };
    await expect(
      handleWriteModifyRequest(makeRequest("set_node_properties", ["1:1"], {})),
    ).rejects.toThrow(/at least one property/);
  });

  it("requires nodeIds", async () => {
    await expect(
      handleWriteModifyRequest(makeRequest("set_node_properties", [], { opacity: 0.5 })),
    ).rejects.toThrow("nodeIds is required");
  });

  it("handles multiple nodes independently", async () => {
    mockNodes["1:1"] = { id: "1:1", opacity: 1, rotation: 0 };
    mockNodes["2:2"] = { id: "2:2", opacity: 1 }; // no rotation

    const res = await handleWriteModifyRequest(
      makeRequest("set_node_properties", ["1:1", "2:2"], { opacity: 0.3, rotation: 10 }),
    );

    expect(res?.data.results[0].applied).toEqual({ opacity: 0.3, rotation: 10 });
    expect(res?.data.results[1].applied).toEqual({ opacity: 0.3 });
    expect(res?.data.results[1].errors.rotation).toContain("does not support rotation");
  });
});

describe("set_fills", () => {
  it("replaces the existing fills", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", fills: [{ type: "SOLID" }] };
    await handleWriteModifyRequest(
      makeRequest("set_fills", ["1:1"], { color: "#ff0000" }),
    );
    expect(mockNodes["1:1"].fills).toHaveLength(1);
    expect(mockNodes["1:1"].fills[0].color.r).toBeCloseTo(1);
  });

  it("appends to the existing fills", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", fills: [{ type: "SOLID" }] };
    await handleWriteModifyRequest(
      makeRequest("set_fills", ["1:1"], { color: "#00ff00", mode: "append" }),
    );
    expect(mockNodes["1:1"].fills).toHaveLength(2);
  });

  // A node whose children disagree reports fills as figma.mixed, a symbol.
  // Spreading that threw "fills is not iterable"; set_gradient_fills already
  // guarded against it, set_fills did not.
  it("appends onto mixed fills instead of throwing", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", fills: Symbol("figma.mixed") };
    await handleWriteModifyRequest(
      makeRequest("set_fills", ["1:1"], { color: "#00ff00", mode: "append" }),
    );
    expect(mockNodes["1:1"].fills).toHaveLength(1);
  });

  it("reports a bad color rather than painting NaN", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", fills: [] };
    await expect(
      handleWriteModifyRequest(makeRequest("set_fills", ["1:1"], { color: "red" })),
    ).rejects.toThrow(/hex color/i);
  });
});

describe("set_strokes", () => {
  it("appends onto mixed strokes instead of throwing", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", strokes: Symbol("figma.mixed") };
    await handleWriteModifyRequest(
      makeRequest("set_strokes", ["1:1"], { color: "#000000", mode: "append" }),
    );
    expect(mockNodes["1:1"].strokes).toHaveLength(1);
  });

  it("accepts shorthand hex", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", strokes: [] };
    await handleWriteModifyRequest(
      makeRequest("set_strokes", ["1:1"], { color: "#f00" }),
    );
    expect(mockNodes["1:1"].strokes[0].color.r).toBeCloseTo(1);
  });
});

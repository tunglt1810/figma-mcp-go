import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteModifyRequest } from "./write-modify";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;
let progressPosts: any[] = [];

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

// ── set_auto_layout ───────────────────────────────────────────────────────────

// It absorbed set_layout_sizing, so putting one sizing on a row of siblings has
// to stay a single call — that was the whole reason the plural tool existed.
describe("set_auto_layout", () => {
  const sizeableNode = (id: string) => ({
    id,
    name: `Node ${id}`,
    type: "FRAME",
    layoutMode: "NONE",
    layoutSizingHorizontal: "FIXED",
    layoutSizingVertical: "FIXED",
  });

  it("applies the same sizing to several nodes in one undo entry", async () => {
    mockNodes["1:1"] = sizeableNode("1:1");
    mockNodes["1:2"] = sizeableNode("1:2");
    const res = await handleWriteModifyRequest(
      makeRequest("set_auto_layout", ["1:1", "1:2"], { layoutSizingHorizontal: "FILL" }),
    );
    expect(res?.data.results.map((r: any) => r.nodeId)).toEqual(["1:1", "1:2"]);
    expect(mockNodes["1:1"].layoutSizingHorizontal).toBe("FILL");
    expect(mockNodes["1:2"].layoutSizingHorizontal).toBe("FILL");
    expect(commitUndoCalled).toBe(true);
  });

  it("still sets the frame's own layout", async () => {
    mockNodes["1:1"] = sizeableNode("1:1");
    await handleWriteModifyRequest(
      makeRequest("set_auto_layout", ["1:1"], { layoutMode: "HORIZONTAL", itemSpacing: 8 }),
    );
    expect(mockNodes["1:1"].layoutMode).toBe("HORIZONTAL");
    expect(mockNodes["1:1"].itemSpacing).toBe(8);
  });

  // One sibling that cannot FILL — its parent has no auto layout — must not
  // take the siblings that could down with it.
  it("reports a node that throws against itself, leaving the others applied", async () => {
    mockNodes["1:1"] = sizeableNode("1:1");
    mockNodes["1:2"] = {
      ...sizeableNode("1:2"),
      set layoutSizingHorizontal(_v: string) {
        throw new Error("Cannot set layoutSizingHorizontal on a node whose parent has no auto layout");
      },
      get layoutSizingHorizontal() { return "FIXED"; },
    };
    const res = await handleWriteModifyRequest(
      makeRequest("set_auto_layout", ["1:1", "1:2"], { layoutSizingHorizontal: "FILL" }),
    );
    expect(mockNodes["1:1"].layoutSizingHorizontal).toBe("FILL");
    expect(res?.data.results[1].error).toContain("no auto layout");
  });

  it("reports a node type that has no auto layout at all", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Slice", type: "SLICE" };
    const res = await handleWriteModifyRequest(
      makeRequest("set_auto_layout", ["1:1"], { layoutMode: "VERTICAL" }),
    );
    expect(res?.data.results[0].error).toContain("does not support auto layout");
  });

  it("reports a missing node per node rather than failing the call", async () => {
    mockNodes["1:1"] = sizeableNode("1:1");
    const res = await handleWriteModifyRequest(
      makeRequest("set_auto_layout", ["9:9", "1:1"], { layoutSizingVertical: "HUG" }),
    );
    expect(res?.data.results[0].error).toBe("Node not found");
    expect(mockNodes["1:1"].layoutSizingVertical).toBe("HUG");
  });

  it("throws when no node ids are given", async () => {
    await expect(
      handleWriteModifyRequest(makeRequest("set_auto_layout", [], { layoutMode: "VERTICAL" })),
    ).rejects.toThrow("nodeIds is required");
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

// set_paint replaced set_fills, set_gradient_fills and set_strokes on the MCP
// surface. These check the router reaches each implementation.
describe("set_paint", () => {
  const paint = (params: any) =>
    handleWriteModifyRequest({ type: "set_paint", requestId: "req-1", nodeIds: ["1:1"], params });

  it("routes SOLID to fills by default", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", fills: [], strokes: [] };
    const res = await paint({ type: "SOLID", color: "#ff0000" });
    expect(mockNodes["1:1"].fills).toHaveLength(1);
    expect(mockNodes["1:1"].strokes).toHaveLength(0);
    expect(res.type).toBe("set_paint");
  });

  it("routes SOLID to strokes and carries strokeWeight", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", fills: [], strokes: [] };
    await paint({ type: "SOLID", target: "stroke", color: "#000000", strokeWeight: 3 });
    expect(mockNodes["1:1"].strokes).toHaveLength(1);
    expect(mockNodes["1:1"].strokeWeight).toBe(3);
    expect(mockNodes["1:1"].fills).toHaveLength(0);
  });

  it("routes a gradient and passes the kind through", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", fills: [] };
    await paint({
      type: "GRADIENT_LINEAR",
      stops: [{ position: 0, color: "#ff0000" }, { position: 1, color: "#00ff00" }],
      geometry: { start: { percentX: 0, percentY: 0 }, end: { percentX: 100, percentY: 0 } },
    });
    expect(mockNodes["1:1"].fills[0].type).toBe("GRADIENT_LINEAR");
  });

  it("refuses a gradient on a stroke", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", fills: [], strokes: [] };
    await expect(
      paint({ type: "GRADIENT_LINEAR", target: "stroke", stops: [], geometry: {} }),
    ).rejects.toThrow(/gradients can only target fill/);
  });

  it("reports an unknown kind rather than silently doing nothing", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Box", fills: [] };
    await expect(paint({ type: "IMAGE", color: "#ff0000" })).rejects.toThrow(/SOLID, GRADIENT_LINEAR, or GRADIENT_RADIAL/);
  });
});

// ── stroke geometry ───────────────────────────────────────────────────────────

describe("set_node_properties stroke geometry", () => {
  it("sets caps, joins, dashes and the miter limit across nodes", async () => {
    for (const id of ["1:1", "1:2"]) {
      mockNodes[id] = {
        id,
        strokeWeight: 1, strokeAlign: "CENTER",
        strokeCap: "NONE", strokeJoin: "MITER", strokeMiterLimit: 4,
        dashPattern: [],
      };
    }
    const res = await handleWriteModifyRequest(
      makeRequest("set_node_properties", ["1:1", "1:2"], {
        strokeWeight: 3,
        strokeAlign: "OUTSIDE",
        strokeCap: "ARROW_LINES",
        strokeJoin: "ROUND",
        strokeMiterLimit: 8,
        dashPattern: [4, 2],
      }),
    );
    for (const id of ["1:1", "1:2"]) {
      expect(mockNodes[id].strokeCap).toBe("ARROW_LINES");
      expect(mockNodes[id].strokeJoin).toBe("ROUND");
      expect(mockNodes[id].strokeMiterLimit).toBe(8);
      expect(mockNodes[id].dashPattern).toEqual([4, 2]);
      expect(mockNodes[id].strokeAlign).toBe("OUTSIDE");
      expect(mockNodes[id].strokeWeight).toBe(3);
    }
    expect(res?.data.results.every((r: any) => !r.errors)).toBe(true);
  });

  it("reports an unsupported stroke property against that property alone", async () => {
    mockNodes["1:1"] = { id: "1:1", opacity: 1 };
    const res = await handleWriteModifyRequest(
      makeRequest("set_node_properties", ["1:1"], { opacity: 0.5, strokeCap: "ROUND" }),
    );
    expect(mockNodes["1:1"].opacity).toBe(0.5);
    expect(res?.data.results[0].errors.strokeCap).toContain("stroke caps");
  });

  // Figma throws for a value it will not take rather than ignoring it. The
  // other properties in the same call must still land.
  it("keeps the other properties when one assignment throws", async () => {
    const node: any = { id: "1:1", opacity: 1, strokeCap: "NONE" };
    Object.defineProperty(node, "dashPattern", {
      enumerable: true,
      get: () => [],
      set: () => { throw new Error("Cannot use a negative dash length"); },
    });
    mockNodes["1:1"] = node;
    const res = await handleWriteModifyRequest(
      makeRequest("set_node_properties", ["1:1"], {
        opacity: 0.25, strokeCap: "ROUND", dashPattern: [-1],
      }),
    );
    expect(node.opacity).toBe(0.25);
    expect(node.strokeCap).toBe("ROUND");
    expect(res?.data.results[0].errors.dashPattern).toContain("negative dash length");
  });
});

// ── set_export_settings ───────────────────────────────────────────────────────

describe("set_export_settings", () => {
  it("writes the presets in order onto every node", async () => {
    for (const id of ["1:1", "1:2"]) mockNodes[id] = { id, name: id, exportSettings: [] };
    const res = await handleWriteModifyRequest(
      makeRequest("set_export_settings", ["1:1", "1:2"], {
        settings: [
          { format: "PNG", suffix: "@2x", constraint: { type: "SCALE", value: 2 } },
          { format: "SVG", contentsOnly: false },
        ],
      }),
    );
    expect(mockNodes["1:1"].exportSettings).toEqual([
      { format: "PNG", suffix: "@2x", constraint: { type: "SCALE", value: 2 } },
      { format: "SVG", contentsOnly: false },
    ]);
    expect(mockNodes["1:2"].exportSettings.length).toBe(2);
    expect(res?.data.results[0].exportSettings).toBe(2);
    expect(commitUndoCalled).toBe(true);
  });

  // Figma rejects a constraint on a vector format, so passing one through would
  // turn a preset the caller can reasonably write into an error.
  it("drops a raster constraint from SVG and PDF presets", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "n", exportSettings: [] };
    await handleWriteModifyRequest(
      makeRequest("set_export_settings", ["1:1"], {
        settings: [{ format: "PDF", constraint: { type: "SCALE", value: 2 } }],
      }),
    );
    expect(mockNodes["1:1"].exportSettings[0].constraint).toBeUndefined();
  });

  it("clears the presets on an empty array", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "n", exportSettings: [{ format: "PNG" }] };
    await handleWriteModifyRequest(makeRequest("set_export_settings", ["1:1"], { settings: [] }));
    expect(mockNodes["1:1"].exportSettings).toEqual([]);
  });

  it("reports a node that cannot be exported without failing the call", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "page", type: "PAGE" };
    mockNodes["1:2"] = { id: "1:2", name: "frame", exportSettings: [] };
    const res = await handleWriteModifyRequest(
      makeRequest("set_export_settings", ["1:1", "1:2"], { settings: [{ format: "PNG" }] }),
    );
    expect(res?.data.results[0].error).toContain("cannot be exported");
    expect(mockNodes["1:2"].exportSettings.length).toBe(1);
  });
});

// ── find_replace_text progress ────────────────────────────────────────────────

describe("find_replace_text progress", () => {
  const buildPage = (count: number) => {
    const children = Array.from({ length: count }, (_, i) => ({
      id: `t:${i}`,
      name: `Text ${i}`,
      type: "TEXT",
      characters: "before",
      fontName: { family: "Inter", style: "Regular" },
    }));
    const page = { id: "0:1", name: "Page 1", type: "PAGE", children };
    mockNodes["0:1"] = page;
    return page;
  };

  const run = (requestId: string) =>
    handleWriteModifyRequest({
      ...makeRequest("find_replace_text", ["0:1"], { find: "before", replace: "after" }),
      requestId,
    });

  beforeEach(() => {
    progressPosts = [];
    (globalThis as any).figma = {
      getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
      commitUndo: () => { commitUndoCalled = true; },
      loadFontAsync: async () => {},
      ui: { postMessage: (msg: any) => progressPosts.push(msg) },
    };
  });

  it("reports while walking a page with many text nodes", async () => {
    buildPage(100);
    const res = await run("req-many");
    expect(res?.data.replaced).toBe(100);
    const updates = progressPosts.filter((m) => m.type === "progress_update");
    expect(updates.length).toBe(20);
    expect(updates[0].message).toContain("0/100 text nodes");
    expect(updates.every((u: any) => u.progress >= 1 && u.progress <= 99)).toBe(true);
  });

  // Under the threshold the work finishes before a message would be read.
  it("stays quiet on a small page", async () => {
    buildPage(5);
    await run("req-few");
    expect(progressPosts.filter((m) => m.type === "progress_update")).toEqual([]);
  });
});

// ── missing fonts ─────────────────────────────────────────────────────────────

describe("find_replace_text with a font the file lacks", () => {
  const textNode = (id: string, family: string) => ({
    id,
    name: id,
    type: "TEXT",
    characters: "before",
    fontName: { family, style: "Regular" },
  });

  const setup = (available: string[]) => {
    mockNodes["0:1"] = {
      id: "0:1",
      name: "Page 1",
      type: "PAGE",
      children: [textNode("t:1", "Inter"), textNode("t:2", "Ghost Sans"), textNode("t:3", "Phantom")],
    };
    (globalThis as any).figma = {
      getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
      commitUndo: () => { commitUndoCalled = true; },
      ui: { postMessage: () => {} },
      loadFontAsync: async (font: any) => {
        if (!available.includes(font.family)) throw new Error("unavailable");
      },
    };
  };

  const run = () =>
    handleWriteModifyRequest(
      makeRequest("find_replace_text", ["0:1"], { find: "before", replace: "after" }),
    );

  // The point of loading every font before writing any: a run that cannot
  // finish must not leave half the page rewritten.
  it("rewrites nothing when one node's font is missing", async () => {
    setup(["Inter"]);
    await expect(run()).rejects.toThrow(/not available in this file/);
    expect(mockNodes["0:1"].children.map((n: any) => n.characters)).toEqual([
      "before", "before", "before",
    ]);
  });

  it("names every missing font, so they are fixed in one round trip", async () => {
    setup(["Inter"]);
    await expect(run()).rejects.toThrow("Ghost Sans Regular, Phantom Regular");
  });

  it("rewrites everything when the fonts are all there", async () => {
    setup(["Inter", "Ghost Sans", "Phantom"]);
    const res = await run();
    expect(res?.data.replaced).toBe(3);
    expect(mockNodes["0:1"].children.every((n: any) => n.characters === "after")).toBe(true);
  });
});

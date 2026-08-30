import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteComponentRequest } from "./write-components";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;
let navigatedTo: any;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

beforeEach(() => {
  commitUndoCalled = false;
  navigatedTo = null;
  mockNodes = {};
  (globalThis as any).figma = {
    get currentPage() { return { id: "0:1", name: "Page 1" }; },
    setCurrentPageAsync: async (page: any) => { navigatedTo = page; },
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    group: (nodes: any[], parent: any) => {
      const group = { id: "grp:1", name: "Group 1", type: "GROUP", children: [...nodes] };
      (parent as any).children = (parent as any).children ?? [];
      (parent as any).children.push(group);
      return group;
    },
    root: {
      children: [
        { id: "0:1", name: "Page 1", type: "PAGE" },
        { id: "0:2", name: "Page 2", type: "PAGE" },
      ],
    },
    commitUndo: () => { commitUndoCalled = true; },
  };
});

// ── group_nodes ───────────────────────────────────────────────────────────────

describe("group_nodes", () => {
  it("groups nodes and returns the GROUP", async () => {
    const parent = { id: "0:1", children: [] as any[], appendChild: () => {} };
    mockNodes["1:1"] = { id: "1:1", type: "RECTANGLE", parent };
    mockNodes["2:2"] = { id: "2:2", type: "RECTANGLE", parent };
    const res = await handleWriteComponentRequest(makeRequest("group_nodes", ["1:1", "2:2"]));
    expect(res?.data.type).toBe("GROUP");
    expect(commitUndoCalled).toBe(true);
  });

  it("applies custom name to the group", async () => {
    const parent = { id: "0:1", children: [] as any[], appendChild: () => {} };
    mockNodes["1:1"] = { id: "1:1", type: "FRAME", parent };
    mockNodes["2:2"] = { id: "2:2", type: "FRAME", parent };
    const res = await handleWriteComponentRequest(
      makeRequest("group_nodes", ["1:1", "2:2"], { name: "My Group" })
    );
    expect(res?.data.name).toBe("My Group");
  });

  it("throws for empty nodeIds", async () => {
    await expect(handleWriteComponentRequest(makeRequest("group_nodes", []))).rejects.toThrow("nodeIds is required");
  });

  it("throws when no valid nodes found", async () => {
    await expect(
      handleWriteComponentRequest(makeRequest("group_nodes", ["9:9", "8:8"]))
    ).rejects.toThrow("No valid scene nodes found");
  });
});

// ── ungroup_nodes ─────────────────────────────────────────────────────────────

describe("ungroup_nodes", () => {
  it("ungroups a GROUP node and returns child IDs", async () => {
    const child1 = { id: "3:1", type: "RECTANGLE" };
    const child2 = { id: "3:2", type: "RECTANGLE" };
    let removed = false;
    const parent = {
      id: "0:1",
      children: [] as any[],
      insertChild(_idx: number, child: any) { this.children.push(child); },
    };
    const group = {
      id: "grp:1", type: "GROUP",
      children: [child1, child2],
      parent,
      remove() { removed = true; },
    };
    parent.children = [group];
    mockNodes["grp:1"] = group;

    const res = await handleWriteComponentRequest(makeRequest("ungroup_nodes", ["grp:1"]));
    expect(res?.data.results[0].childIds).toEqual(["3:1", "3:2"]);
    expect(removed).toBe(true);
    expect(commitUndoCalled).toBe(true);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteComponentRequest(makeRequest("ungroup_nodes", ["9:9"]));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error when node is not a GROUP", async () => {
    mockNodes["1:1"] = { id: "1:1", type: "FRAME" };
    const res = await handleWriteComponentRequest(makeRequest("ungroup_nodes", ["1:1"]));
    expect(res?.data.results[0].error).toBe("Node is not a GROUP");
  });

  it("throws for empty nodeIds", async () => {
    await expect(
      handleWriteComponentRequest(makeRequest("ungroup_nodes", []))
    ).rejects.toThrow("nodeIds is required");
  });

  it("returns null for unrecognised type", async () => {
    const res = await handleWriteComponentRequest(makeRequest("unknown_op"));
    expect(res).toBeNull();
  });
});

// ── set_annotations ───────────────────────────────────────────────────────────

// It absorbed clear_annotations, whose only advantage was taking several nodes.
// Clearing ten nodes must not cost ten calls.
describe("set_annotations", () => {
  const annotatable = (id: string) => ({ id, type: "FRAME", annotations: [{ label: "old" }] });

  it("sets the same annotations across several nodes in one undo entry", async () => {
    mockNodes["1:1"] = annotatable("1:1");
    mockNodes["1:2"] = annotatable("1:2");
    const res = await handleWriteComponentRequest(
      makeRequest("set_annotations", ["1:1", "1:2"], { annotations: [{ label: "Main Button" }] }),
    );
    expect(mockNodes["1:1"].annotations).toEqual([{ label: "Main Button" }]);
    expect(mockNodes["1:2"].annotations).toEqual([{ label: "Main Button" }]);
    expect(res?.data.results.map((r: any) => r.success)).toEqual([true, true]);
    expect(commitUndoCalled).toBe(true);
  });

  it("clears across several nodes with an empty array", async () => {
    mockNodes["1:1"] = annotatable("1:1");
    mockNodes["1:2"] = annotatable("1:2");
    await handleWriteComponentRequest(
      makeRequest("set_annotations", ["1:1", "1:2"], { annotations: [] }),
    );
    expect(mockNodes["1:1"].annotations).toEqual([]);
    expect(mockNodes["1:2"].annotations).toEqual([]);
  });

  it("reports an unsupported node against itself, leaving the others set", async () => {
    mockNodes["1:1"] = { id: "1:1", type: "SLICE" }; // no annotations property
    mockNodes["1:2"] = annotatable("1:2");
    const res = await handleWriteComponentRequest(
      makeRequest("set_annotations", ["1:1", "1:2"], { annotations: [{ label: "Btn" }] }),
    );
    expect(res?.data.results[0].error).toContain("does not support annotations");
    expect(mockNodes["1:2"].annotations).toEqual([{ label: "Btn" }]);
  });

  it("reports a missing node per node", async () => {
    const res = await handleWriteComponentRequest(
      makeRequest("set_annotations", ["9:9"], { annotations: [] }),
    );
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("throws when annotations is not an array", async () => {
    mockNodes["1:1"] = annotatable("1:1");
    await expect(
      handleWriteComponentRequest(makeRequest("set_annotations", ["1:1"], {})),
    ).rejects.toThrow("annotations array is required");
  });

  it("throws for empty nodeIds", async () => {
    await expect(
      handleWriteComponentRequest(makeRequest("set_annotations", [], { annotations: [] })),
    ).rejects.toThrow("nodeIds is required");
  });
});

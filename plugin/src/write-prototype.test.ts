import { describe, it, expect, beforeEach } from "bun:test";
import { handleWritePrototypeRequest } from "./write-prototype";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

const clickNavigate: any = {
  trigger: { type: "ON_CLICK" },
  action: {
    type: "NAVIGATE",
    destinationId: "1:3",
    transition: { type: "DISSOLVE", duration: 0.3, easing: { type: "EASE_OUT" } },
    preserveScrollPosition: false,
  },
};

beforeEach(() => {
  commitUndoCalled = false;
  mockNodes = {};
  (globalThis as any).figma = {
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    commitUndo: () => { commitUndoCalled = true; },
  };
});

// ── set_reactions ─────────────────────────────────────────────────────────────

describe("set_reactions", () => {
  it("replaces all reactions (default mode)", async () => {
    const existing = [{ trigger: { type: "ON_HOVER" }, action: { type: "BACK" } }];
    let stored: any[] = [...existing];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };

    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], { reactions: [clickNavigate] })
    );

    expect(mockNodes["1:2"].reactions).toHaveLength(1);
    expect(mockNodes["1:2"].reactions[0].trigger.type).toBe("ON_CLICK");
    expect(res?.data.reactionCount).toBe(1);
    expect(commitUndoCalled).toBe(true);
  });

  it("appends to existing reactions when mode is append", async () => {
    const existing = [{ trigger: { type: "ON_HOVER" }, action: { type: "BACK" } }];
    let stored: any[] = [...existing];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };

    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], { reactions: [clickNavigate], mode: "append" })
    );

    expect(mockNodes["1:2"].reactions).toHaveLength(2);
    expect(res?.data.reactionCount).toBe(2);
    expect(commitUndoCalled).toBe(true);
  });

  it("sets empty reactions array (clears all via replace)", async () => {
    let stored: any[] = [clickNavigate];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };

    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], { reactions: [] })
    );

    expect(mockNodes["1:2"].reactions).toHaveLength(0);
    expect(res?.data.reactionCount).toBe(0);
  });

  it("throws when node not found", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("set_reactions", ["9:9"], { reactions: [clickNavigate] }))
    ).rejects.toThrow("Node not found: 9:9");
  });

  it("throws when node does not support reactions", async () => {
    mockNodes["1:2"] = { id: "1:2", name: "Document" }; // no reactions property
    await expect(
      handleWritePrototypeRequest(makeRequest("set_reactions", ["1:2"], { reactions: [clickNavigate] }))
    ).rejects.toThrow("does not support reactions");
  });

  it("throws when nodeId is missing", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("set_reactions", [], { reactions: [] }))
    ).rejects.toThrow("nodeId is required");
  });
});

// ── set_reactions: the removal it absorbed ────────────────────────────────────

describe("set_reactions with removeIndices", () => {
  it("removes every reaction when removeIndices is empty", async () => {
    let stored: any[] = [clickNavigate, { trigger: { type: "ON_HOVER" }, action: { type: "BACK" } }];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };

    const res = await handleWritePrototypeRequest(makeRequest("set_reactions", ["1:2"], { removeIndices: [] }));

    expect(mockNodes["1:2"].reactions).toHaveLength(0);
    expect(res?.data.removed).toBe(2);
    expect(res?.data.reactionCount).toBe(0);
    expect(commitUndoCalled).toBe(true);
  });

  it("ignores an index past the end rather than throwing", async () => {
    let stored: any[] = [clickNavigate];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };

    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], { removeIndices: [7] })
    );

    expect(mockNodes["1:2"].reactions).toHaveLength(1);
    expect(res?.data.removed).toBe(0);
  });

  // The response says `removed` only when something was being removed, so a
  // caller can tell which half of the tool answered it.
  it("says nothing about removed when it is setting reactions", async () => {
    let stored: any[] = [];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };

    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], { reactions: [clickNavigate] })
    );

    expect(res?.data.reactionCount).toBe(1);
    expect(res?.data.removed).toBeUndefined();
  });

  it("removes only specified indices, keeps others", async () => {
    const r0 = { trigger: { type: "ON_CLICK" }, action: { type: "BACK" } };
    const r1 = clickNavigate;
    const r2 = { trigger: { type: "ON_HOVER" }, action: { type: "CLOSE" } };
    let stored: any[] = [r0, r1, r2];
    mockNodes["1:2"] = {
      id: "1:2", name: "Button", reactions: stored,
      setReactionsAsync: async (r: any[]) => { stored = r; mockNodes["1:2"].reactions = r; },
    };

    const res = await handleWritePrototypeRequest(
      makeRequest("set_reactions", ["1:2"], { removeIndices: [0, 2] })
    );

    expect(mockNodes["1:2"].reactions).toHaveLength(1);
    expect(mockNodes["1:2"].reactions[0].trigger.type).toBe("ON_CLICK"); // r1 (was index 1) remains
    expect(res?.data.removed).toBe(2);
    expect(res?.data.reactionCount).toBe(1);
    expect(commitUndoCalled).toBe(true);
  });

  it("throws when node not found", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("set_reactions", ["9:9"], { removeIndices: [] }))
    ).rejects.toThrow("Node not found: 9:9");
  });

  it("throws when node does not support reactions", async () => {
    mockNodes["1:2"] = { id: "1:2", name: "Document" };
    await expect(
      handleWritePrototypeRequest(makeRequest("set_reactions", ["1:2"], { removeIndices: [] }))
    ).rejects.toThrow("does not support reactions");
  });

  it("throws when nodeId is missing", async () => {
    await expect(
      handleWritePrototypeRequest(makeRequest("set_reactions", [], { removeIndices: [] }))
    ).rejects.toThrow("nodeId is required");
  });
});

// ── unknown type ──────────────────────────────────────────────────────────────

describe("unknown type", () => {
  it("returns null for unrecognised type", async () => {
    const res = await handleWritePrototypeRequest(makeRequest("unknown_prototype_op"));
    expect(res).toBeNull();
  });
});

import { describe, it, expect, beforeEach } from "bun:test";
import { handleWritePageRequest } from "./write-page";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;
let mockPages: any[];
let currentPage: any;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

beforeEach(() => {
  commitUndoCalled = false;
  mockNodes = {};
  currentPage = null;
  mockPages = [{ id: "0:1", name: "Page 1", type: "PAGE", remove: () => { mockPages.splice(mockPages.indexOf(page1), 1); } }];
  const page1 = mockPages[0];
  (globalThis as any).figma = {
    createPage: () => {
      const page: any = {
        id: `page:${mockPages.length + 1}`,
        name: "Page",
        type: "PAGE",
        remove() { mockPages.splice(mockPages.indexOf(this), 1); },
      };
      mockPages.push(page);
      return page;
    },
    getNodeByIdAsync: async (id: string) =>
      mockNodes[id] ?? mockPages.find(p => p.id === id) ?? null,
    setCurrentPageAsync: async (page: any) => { currentPage = page; },
    commitUndo: () => { commitUndoCalled = true; },
    root: {
      get children() { return mockPages; },
      insertChild(index: number, page: any) {
        const i = mockPages.indexOf(page);
        if (i !== -1) mockPages.splice(i, 1);
        mockPages.splice(index, 0, page);
      },
    },
  };
});

// ── add_page ──────────────────────────────────────────────────────────────────

describe("add_page", () => {
  it("creates a new page with name", async () => {
    const res = await handleWritePageRequest(makeRequest("add_page", [], { name: "Flows" }));
    expect(res?.data.name).toBe("Flows");
    expect(mockPages).toHaveLength(2);
    expect(commitUndoCalled).toBe(true);
  });

  it("creates page with default name when none provided", async () => {
    const res = await handleWritePageRequest(makeRequest("add_page", [], {}));
    expect(res?.data.id).toBeDefined();
    expect(mockPages).toHaveLength(2);
  });

  it("inserts page at specified index", async () => {
    // add a second page first
    mockPages.push({ id: "0:2", name: "Page 2", type: "PAGE", remove: () => {} });
    const res = await handleWritePageRequest(makeRequest("add_page", [], { name: "Inserted", index: 0 }));
    expect(mockPages[0].name).toBe("Inserted");
  });

  it("returns page id, name, and index", async () => {
    const res = await handleWritePageRequest(makeRequest("add_page", [], { name: "New Page" }));
    expect(res?.data.id).toBeDefined();
    expect(res?.data.name).toBe("New Page");
    expect(typeof res?.data.index).toBe("number");
  });
});

// ── delete_page ───────────────────────────────────────────────────────────────

describe("delete_page", () => {
  it("deletes page by pageId", async () => {
    const page2: any = { id: "0:2", name: "Page 2", type: "PAGE", remove() { mockPages.splice(mockPages.indexOf(this), 1); } };
    mockPages.push(page2);
    mockNodes["0:2"] = page2;
    const res = await handleWritePageRequest(makeRequest("delete_page", [], { pageId: "0:2" }));
    expect(res?.data.deleted).toBe(true);
    expect(mockPages).toHaveLength(1);
    expect(commitUndoCalled).toBe(true);
  });

  it("deletes page by pageName", async () => {
    const page2: any = { id: "0:2", name: "Flows", type: "PAGE", remove() { mockPages.splice(mockPages.indexOf(this), 1); } };
    mockPages.push(page2);
    const res = await handleWritePageRequest(makeRequest("delete_page", [], { pageName: "Flows" }));
    expect(res?.data.deleted).toBe(true);
    expect(mockPages).toHaveLength(1);
  });

  it("throws when trying to delete the only page", async () => {
    mockNodes["0:1"] = mockPages[0];
    await expect(handleWritePageRequest(makeRequest("delete_page", [], { pageId: "0:1" }))).rejects.toThrow("only page");
  });

  it("throws when page not found by id", async () => {
    mockPages.push({ id: "0:2", name: "P2", type: "PAGE" });
    await expect(handleWritePageRequest(makeRequest("delete_page", [], { pageId: "9:9" }))).rejects.toThrow("Page not found");
  });

  it("throws when page not found by name", async () => {
    await expect(handleWritePageRequest(makeRequest("delete_page", [], { pageName: "NonExistent" }))).rejects.toThrow("Page not found");
  });

  it("throws when neither pageId nor pageName given", async () => {
    await expect(handleWritePageRequest(makeRequest("delete_page", [], {}))).rejects.toThrow("pageId or pageName is required");
  });

  it("throws when node is not a PAGE", async () => {
    mockNodes["1:1"] = { id: "1:1", type: "FRAME" };
    mockPages.push({ id: "0:2", name: "P2" });
    await expect(handleWritePageRequest(makeRequest("delete_page", [], { pageId: "1:1" }))).rejects.toThrow("is not a PAGE");
  });
});

// ── rename_page ───────────────────────────────────────────────────────────────

describe("rename_page", () => {
  it("renames page by pageId", async () => {
    mockNodes["0:1"] = mockPages[0];
    const res = await handleWritePageRequest(makeRequest("rename_page", [], { pageId: "0:1", newName: "Redesign" }));
    expect(mockPages[0].name).toBe("Redesign");
    expect(res?.data.name).toBe("Redesign");
    expect(commitUndoCalled).toBe(true);
  });

  it("renames page by pageName", async () => {
    const res = await handleWritePageRequest(makeRequest("rename_page", [], { pageName: "Page 1", newName: "Updated" }));
    expect(mockPages[0].name).toBe("Updated");
    expect(res?.data.name).toBe("Updated");
  });

  it("returns oldName and new name", async () => {
    mockNodes["0:1"] = mockPages[0];
    const res = await handleWritePageRequest(makeRequest("rename_page", [], { pageId: "0:1", newName: "New Name" }));
    expect(res?.data.oldName).toBe("Page 1");
    expect(res?.data.name).toBe("New Name");
  });

  it("throws when newName is missing", async () => {
    mockNodes["0:1"] = mockPages[0];
    await expect(handleWritePageRequest(makeRequest("rename_page", [], { pageId: "0:1" }))).rejects.toThrow("newName is required");
  });

  it("throws when page not found by id", async () => {
    await expect(handleWritePageRequest(makeRequest("rename_page", [], { pageId: "9:9", newName: "X" }))).rejects.toThrow("Page not found");
  });

  it("throws when page not found by name", async () => {
    await expect(handleWritePageRequest(makeRequest("rename_page", [], { pageName: "Ghost", newName: "X" }))).rejects.toThrow("Page not found");
  });

  it("throws when neither pageId nor pageName given", async () => {
    await expect(handleWritePageRequest(makeRequest("rename_page", [], { newName: "X" }))).rejects.toThrow("pageId or pageName is required");
  });
});

// ── unknown type ──────────────────────────────────────────────────────────────

describe("handleWritePageRequest unknown", () => {
  it("returns null for unrecognised type", async () => {
    const res = await handleWritePageRequest(makeRequest("unknown_page_op"));
    expect(res).toBeNull();
  });
});

// ── navigate_to_page ──────────────────────────────────────────────────────────

describe("navigate_to_page", () => {
  it("navigates by pageId", async () => {
    mockNodes["0:2"] = { id: "0:2", name: "Page 2", type: "PAGE" };
    const res = await handleWritePageRequest(makeRequest("navigate_to_page", [], { pageId: "0:2" }));
    expect(currentPage?.id).toBe("0:2");
    expect(res?.data.id).toBe("0:2");
    expect(res?.data.name).toBe("Page 2");
  });

  it("navigates by pageName", async () => {
    const res = await handleWritePageRequest(makeRequest("navigate_to_page", [], { pageName: "Page 1" }));
    expect(currentPage?.name).toBe("Page 1");
    expect(res?.data.name).toBe("Page 1");
  });

  it("throws when pageId node not found", async () => {
    await expect(
      handleWritePageRequest(makeRequest("navigate_to_page", [], { pageId: "9:9" }))
    ).rejects.toThrow("Page not found: 9:9");
  });

  it("throws when pageId node is not a PAGE", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame", type: "FRAME" };
    await expect(
      handleWritePageRequest(makeRequest("navigate_to_page", [], { pageId: "1:1" }))
    ).rejects.toThrow("is not a PAGE");
  });

  it("throws when pageName not found", async () => {
    await expect(
      handleWritePageRequest(makeRequest("navigate_to_page", [], { pageName: "Nonexistent" }))
    ).rejects.toThrow("Page not found");
  });

  it("throws when neither pageId nor pageName provided", async () => {
    await expect(
      handleWritePageRequest(makeRequest("navigate_to_page", [], {}))
    ).rejects.toThrow("pageId or pageName is required");
  });
});

// manage_page replaced four page tools on the MCP surface. These check the
// router reaches each implementation and the arguments survive the trip.
describe("manage_page", () => {
  const manage = (params: any) =>
    handleWritePageRequest({ type: "manage_page", requestId: "req-1", nodeIds: [], params });

  it("routes add", async () => {
    const res = await manage({ action: "add", name: "Specs" });
    expect(res.data.name).toBe("Specs");
    expect(mockPages.some(p => p.name === "Specs")).toBe(true);
    // The response names the tool the caller actually called.
    expect(res.type).toBe("manage_page");
  });

  it("routes rename and carries newName", async () => {
    await manage({ action: "add", name: "Old" });
    const res = await manage({ action: "rename", pageName: "Old", newName: "New" });
    expect(res.data.name).toBe("New");
    expect(mockPages.some(p => p.name === "New")).toBe(true);
  });

  it("routes navigate", async () => {
    await manage({ action: "add", name: "Target" });
    await manage({ action: "navigate", pageName: "Target" });
    expect(currentPage.name).toBe("Target");
  });

  it("routes delete", async () => {
    await manage({ action: "add", name: "Doomed" });
    await manage({ action: "delete", pageName: "Doomed" });
    expect(mockPages.some(p => p.name === "Doomed")).toBe(false);
  });

  it("reports an unknown action rather than silently doing nothing", async () => {
    await expect(manage({ action: "duplicate" })).rejects.toThrow(/add, delete, rename, or navigate/);
  });
});

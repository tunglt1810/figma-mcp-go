import { describe, expect, it, beforeEach } from "bun:test";
import { pageOf, writeViewportHandlers } from "./write-viewport";

// ── Figma mock ────────────────────────────────────────────────────────────────

let nodes: Record<string, any>;
let currentPage: any;
let scrolledTo: any[] | null;
let pageSwitchedTo: any;

const makePage = (id: string, name: string) => {
  const page: any = { id, name, type: "PAGE", selection: [] as any[] };
  page.parent = { id: "0:0", type: "DOCUMENT" };
  return page;
};

const makeNode = (id: string, page: any, overrides: any = {}) => {
  const node = {
    id,
    name: `Node ${id}`,
    type: "FRAME",
    parent: page,
    x: 0,
    y: 0,
    width: 100,
    height: 50,
    absoluteBoundingBox: { x: 0, y: 0, width: 100, height: 50 },
    ...overrides,
  };
  nodes[id] = node;
  return node;
};

beforeEach(() => {
  nodes = {};
  scrolledTo = null;
  pageSwitchedTo = null;
  currentPage = makePage("1:0", "Page 1");
  nodes[currentPage.id] = currentPage;

  (globalThis as any).figma = {
    get currentPage() {
      return currentPage;
    },
    getNodeByIdAsync: async (id: string) => nodes[id] ?? null,
    setCurrentPageAsync: async (page: any) => {
      pageSwitchedTo = page;
      currentPage = page;
    },
    viewport: {
      scrollAndZoomIntoView: (target: any[]) => {
        scrolledTo = target;
      },
    },
  };
});

const call = (nodeIds: string[], params: any = {}) =>
  writeViewportHandlers["set_selection"]({
    type: "set_selection",
    requestId: "r1",
    nodeIds,
    params,
  });

// ── pageOf ────────────────────────────────────────────────────────────────────

describe("pageOf", () => {
  it("walks up to the page", () => {
    const group = makeNode("1:5", currentPage, { type: "GROUP" });
    const child = makeNode("1:6", group);
    expect(pageOf(child)).toBe(currentPage);
  });

  it("returns null for a node detached from the tree", () => {
    const orphan = makeNode("1:7", null, { parent: null });
    expect(pageOf(orphan)).toBeNull();
  });

  it("returns null when the walk reaches the document without a page", () => {
    const stray = makeNode("1:8", null, { parent: { type: "DOCUMENT", id: "0:0" } });
    expect(pageOf(stray)).toBeNull();
  });
});

// ── set_selection ─────────────────────────────────────────────────────────────

describe("set_selection", () => {
  it("selects and zooms by default", async () => {
    const node = makeNode("1:1", currentPage);
    const result = await call(["1:1"]);
    expect(currentPage.selection).toEqual([node]);
    expect(scrolledTo).toEqual([node]);
    expect(result.data.selected[0].id).toBe("1:1");
    expect(result.data.zoomed).toBe(true);
  });

  it("zooms without touching the selection when select is false", async () => {
    const node = makeNode("1:1", currentPage);
    currentPage.selection = [];
    await call(["1:1"], { select: false });
    expect(currentPage.selection).toEqual([]);
    expect(scrolledTo).toEqual([node]);
  });

  it("selects without moving the camera when zoom is false", async () => {
    const node = makeNode("1:1", currentPage);
    await call(["1:1"], { zoom: false });
    expect(currentPage.selection).toEqual([node]);
    expect(scrolledTo).toBeNull();
  });

  it("clears the selection when given no ids", async () => {
    makeNode("1:1", currentPage);
    currentPage.selection = [nodes["1:1"]];
    const result = await call([]);
    expect(currentPage.selection).toEqual([]);
    expect(result.data.cleared).toBe(true);
  });

  it("switches to the node's page first", async () => {
    const other = makePage("2:0", "Page 2");
    nodes[other.id] = other;
    const node = makeNode("2:1", other);
    await call(["2:1"]);
    expect(pageSwitchedTo).toBe(other);
    expect(other.selection).toEqual([node]);
  });

  it("does not switch pages when the node is already on the current one", async () => {
    makeNode("1:1", currentPage);
    await call(["1:1"]);
    expect(pageSwitchedTo).toBeNull();
  });

  it("refuses a selection spanning two pages", async () => {
    const other = makePage("2:0", "Page 2");
    nodes[other.id] = other;
    makeNode("1:1", currentPage);
    makeNode("2:1", other);
    expect(call(["1:1", "2:1"])).rejects.toThrow(/same page/);
  });

  it("names every missing node", async () => {
    makeNode("1:1", currentPage);
    expect(call(["1:1", "9:9"])).rejects.toThrow(/9:9/);
  });

  it("refuses to select a page", async () => {
    expect(call(["1:0"])).rejects.toThrow(/cannot be selected/);
  });

  it("refuses a node that is on no page", async () => {
    makeNode("1:9", null, { parent: null });
    expect(call(["1:9"])).rejects.toThrow(/not on any page/);
  });

  it("refuses to clear the selection when select is false", async () => {
    expect(call([], { select: false })).rejects.toThrow(/nodeIds is required/);
  });
});

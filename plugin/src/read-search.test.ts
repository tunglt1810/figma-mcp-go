import { describe, expect, it, beforeEach } from "bun:test";
import { readDocumentHandlers } from "./read-document";

// ── Figma mock ────────────────────────────────────────────────────────────────

let nodes: Record<string, any>;
let pages: any[];
let currentPage: any;
let loadedPages: string[];
let progressMessages: any[];

const makeNode = (id: string, name: string, type = "FRAME", children: any[] = []) => {
  const node: any = {
    id,
    name,
    type,
    x: 0,
    y: 0,
    width: 10,
    height: 10,
    absoluteBoundingBox: { x: 0, y: 0, width: 10, height: 10 },
  };
  if (children.length > 0 || type === "FRAME") node.children = children;
  nodes[id] = node;
  return node;
};

const makePage = (id: string, name: string, children: any[]) => {
  const page: any = {
    id,
    name,
    type: "PAGE",
    children,
    loadAsync: async () => {
      loadedPages.push(id);
    },
  };
  nodes[id] = page;
  return page;
};

beforeEach(() => {
  nodes = {};
  loadedPages = [];
  progressMessages = [];
  const pageOne = makePage("1:0", "Page 1", [
    makeNode("1:1", "Button Primary"),
    makeNode("1:2", "Card", "RECTANGLE"),
  ]);
  const pageTwo = makePage("2:0", "Page 2", [makeNode("2:1", "Button Ghost")]);
  pages = [pageOne, pageTwo];
  currentPage = pageOne;

  (globalThis as any).figma = {
    get currentPage() {
      return currentPage;
    },
    root: { get children() { return pages; } },
    getNodeByIdAsync: async (id: string) => nodes[id] ?? null,
    ui: {
      postMessage: (msg: any) => {
        progressMessages.push(msg);
      },
    },
  };
});

const search = (params: any) =>
  readDocumentHandlers["search_nodes"]({
    type: "search_nodes",
    requestId: "r1",
    params,
  });

// ── search_nodes ──────────────────────────────────────────────────────────────

describe("search_nodes", () => {
  it("searches the current page by default and ignores the others", async () => {
    const result = await search({ query: "button" });
    expect(result.data.nodes.map((n: any) => n.id)).toEqual(["1:1"]);
    expect(result.data.scope).toBe("page");
  });

  it("finds nodes on other pages when the scope is the document", async () => {
    const result = await search({ query: "button", scope: "document" });
    expect(result.data.nodes.map((n: any) => n.id)).toEqual(["1:1", "2:1"]);
    expect(result.data.scope).toBe("document");
  });

  // With documentAccess "dynamic-page", a page that is never loaded reports no
  // children at all — the search would answer "not found" instead of failing.
  it("loads every page before walking it", async () => {
    await search({ query: "button", scope: "document" });
    expect(loadedPages).toEqual(["1:0", "2:0"]);
  });

  it("labels each hit with its page only when the search spans pages", async () => {
    const spanning = await search({ query: "button", scope: "document" });
    expect(spanning.data.nodes[1].pageName).toBe("Page 2");

    const single = await search({ query: "button" });
    expect(single.data.nodes[0].pageName).toBeUndefined();
  });

  it("reports progress per page on a document search", async () => {
    await search({ query: "button", scope: "document" });
    const updates = progressMessages.filter((m) => m.type === "progress_update");
    expect(updates.length).toBe(2);
    expect(updates[updates.length - 1].progress).toBe(100);
  });

  it("stays silent about progress on a single-page search", async () => {
    await search({ query: "button" });
    expect(progressMessages.length).toBe(0);
  });

  it("filters by type", async () => {
    const result = await search({ types: ["RECTANGLE"], scope: "document" });
    expect(result.data.nodes.map((n: any) => n.id)).toEqual(["1:2"]);
  });

  it("flags a truncated answer, so a full page of hits is not read as complete", async () => {
    const result = await search({ scope: "document", limit: 1 });
    expect(result.data.count).toBe(1);
    expect(result.data.truncated).toBe(true);
  });

  it("does not flag an answer that fits", async () => {
    const result = await search({ query: "button", scope: "document", limit: 50 });
    expect(result.data.truncated).toBe(false);
  });

  it("stops walking further pages once the limit is reached", async () => {
    await search({ scope: "document", limit: 1 });
    expect(loadedPages).toEqual(["1:0"]);
  });

  it("scopes to a node when one is given, and reports that scope", async () => {
    const result = await search({ nodeId: "1:0", query: "card" });
    expect(result.data.nodes.map((n: any) => n.id)).toEqual(["1:2"]);
    expect(result.data.scope).toBe("node");
  });

  it("errors when the scope node does not exist", async () => {
    expect(search({ nodeId: "9:9" })).rejects.toThrow(/Node not found/);
  });
});

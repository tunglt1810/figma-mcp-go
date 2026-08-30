import { describe, expect, it, beforeEach } from "bun:test";
import { readDocumentHandlers } from "./read-document";
import { clearPinned, setPinned } from "./pinned";

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

  // The last page reports 99, not 100: 100 reads as "done", and the response
  // itself is what says the work finished. See clampProgress.
  it("reports progress per page on a document search", async () => {
    await search({ query: "button", scope: "document" });
    const updates = progressMessages.filter((m) => m.type === "progress_update");
    expect(updates.length).toBe(2);
    expect(updates[updates.length - 1].progress).toBe(99);
  });

  it("stays silent about progress on a single-page search", async () => {
    await search({ query: "button" });
    expect(progressMessages.length).toBe(0);
  });

  it("filters by type", async () => {
    const result = await search({ types: ["RECTANGLE"], scope: "document" });
    expect(result.data.nodes.map((n: any) => n.id)).toEqual(["1:2"]);
  });

  // ── what scan_nodes_by_types and scan_text_nodes used to do ────────────────

  // scan_nodes_by_types skipped hidden nodes; this tool never has. Both
  // behaviours have to be reachable, and the default has to stay the old one.
  it("searches hidden nodes by default and skips them on request", async () => {
    const hidden = makeNode("1:3", "Button Hidden", "FRAME");
    hidden.visible = false;
    currentPage.children.push(hidden);

    expect((await search({ query: "button" })).data.nodes.map((n: any) => n.id))
      .toEqual(["1:1", "1:3"]);
    expect((await search({ query: "button", includeHidden: false })).data.nodes.map((n: any) => n.id))
      .toEqual(["1:1"]);
  });

  it("skips everything under a hidden node, not just the node itself", async () => {
    const hidden = makeNode("1:3", "Hidden Wrapper", "FRAME", [makeNode("1:4", "Button Inside")]);
    hidden.visible = false;
    currentPage.children.push(hidden);

    const result = await search({ query: "button", includeHidden: false });
    expect(result.data.nodes.map((n: any) => n.id)).toEqual(["1:1"]);
  });

  // scan_text_nodes returned the copy itself, which this tool did not.
  it("reads the text of TEXT hits when asked, and nothing extra otherwise", async () => {
    const text = makeNode("1:3", "Label", "TEXT");
    text.characters = "Hello";
    text.fontSize = 14;
    text.fontName = { family: "Inter", style: "Regular" };
    currentPage.children.push(text);

    const withText = await search({ types: ["TEXT"], includeText: true });
    expect(withText.data.nodes[0].characters).toBe("Hello");
    expect(withText.data.nodes[0].fontSize).toBe(14);
    expect(withText.data.nodes[0].fontName).toEqual({ family: "Inter", style: "Regular" });

    const without = await search({ types: ["TEXT"] });
    expect(without.data.nodes[0].characters).toBeUndefined();
  });

  it("reports a mixed font as mixed rather than a symbol", async () => {
    const text = makeNode("1:3", "Label", "TEXT");
    text.characters = "Hello";
    text.fontSize = Symbol("figma.mixed");
    text.fontName = Symbol("figma.mixed");
    currentPage.children.push(text);

    const result = await search({ types: ["TEXT"], includeText: true });
    expect(result.data.nodes[0].fontSize).toBe("mixed");
    expect(result.data.nodes[0].fontName).toBe("mixed");
  });

  it("leaves a non-TEXT hit alone when includeText is on", async () => {
    const result = await search({ types: ["RECTANGLE"], includeText: true });
    expect(result.data.nodes[0].characters).toBeUndefined();
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

// ── get_document ──────────────────────────────────────────────────────────────

const getDocument = (params?: any) =>
  readDocumentHandlers["get_document"]({
    type: "get_document",
    requestId: "r2",
    params,
  });

describe("get_document", () => {
  it("serializes the current page by default", async () => {
    const result = await getDocument();
    expect(result.data.id).toBe("1:0");
    expect(result.data.scope).toBe("page");
    expect(loadedPages).toEqual([]);
  });

  it("serializes every page when the scope is the document", async () => {
    const result = await getDocument({ scope: "document" });
    expect(result.data.type).toBe("DOCUMENT");
    expect(result.data.scope).toBe("document");
    expect(result.data.pageCount).toBe(2);
    expect(result.data.children.map((p: any) => p.id)).toEqual(["1:0", "2:0"]);
  });

  // The same trap search_nodes fell into: an unloaded page reports no children,
  // so a document walk that skips loadAsync answers with empty pages.
  it("loads every page before serializing it", async () => {
    await getDocument({ scope: "document" });
    expect(loadedPages).toEqual(["1:0", "2:0"]);
  });

  it("shares one maxNodes budget across the pages and says it stopped short", async () => {
    const result = await getDocument({ scope: "document", maxNodes: 2 });
    expect(result.data.truncated).toBe(true);
  });

  // 99 rather than 100 — see the note on the search_nodes case above.
  it("reports progress per page", async () => {
    await getDocument({ scope: "document" });
    const updates = progressMessages.filter((m) => m.type === "progress_update");
    expect(updates.length).toBe(2);
    expect(updates[updates.length - 1].progress).toBe(99);
  });
});

// ── get_nodes_info ────────────────────────────────────────────────────────────

describe("get_nodes_info", () => {
  it("answers under a nodes key whether or not anything was deduped", async () => {
    const result = await readDocumentHandlers["get_nodes_info"]({
      type: "get_nodes_info",
      requestId: "r3",
      params: {},
      nodeIds: ["1:1", "1:2"],
    });
    expect(result.data.nodes.map((n: any) => n.id)).toEqual(["1:1", "1:2"]);
    expect(result.data.globalVars).toBeUndefined();
  });

  // It absorbed get_node, whose one advantage was throwing on an id that
  // matched nothing. Filtering such an id out in silence reads back as "that
  // node has no content" rather than "there is no such node".
  it("reports an id that matched nothing instead of dropping it", async () => {
    const result = await readDocumentHandlers["get_nodes_info"]({
      type: "get_nodes_info",
      requestId: "r6",
      params: {},
      nodeIds: ["1:1", "9:9"],
    });
    expect(result.data.nodes.map((n: any) => n.id)).toEqual(["1:1"]);
    expect(result.data.missing).toEqual(["9:9"]);
  });

  it("says nothing about missing when every id resolved", async () => {
    const result = await readDocumentHandlers["get_nodes_info"]({
      type: "get_nodes_info",
      requestId: "r7",
      params: {},
      nodeIds: ["1:1"],
    });
    expect(result.data.missing).toBeUndefined();
  });

  it("answers with an empty nodes list when every id was wrong", async () => {
    const result = await readDocumentHandlers["get_nodes_info"]({
      type: "get_nodes_info",
      requestId: "r8",
      params: {},
      nodeIds: ["8:8", "9:9"],
    });
    expect(result.data.nodes).toEqual([]);
    expect(result.data.missing).toEqual(["8:8", "9:9"]);
  });

  it("collapses a fill shared by several nodes into globalVars", async () => {
    const shared = [{ type: "SOLID", color: { r: 1, g: 0, b: 0 } }];
    nodes["1:1"].fills = shared;
    nodes["1:2"].fills = shared;
    const result = await readDocumentHandlers["get_nodes_info"]({
      type: "get_nodes_info",
      requestId: "r4",
      params: {},
      nodeIds: ["1:1", "1:2"],
    });
    expect(result.data.globalVars).toBeDefined();
    expect(result.data.nodes[0].styles.fills).toBe(result.data.nodes[1].styles.fills);
  });
});

// ── get_selection ─────────────────────────────────────────────────────────────

describe("get_selection", () => {
  const getSelection = (params?: any) =>
    readDocumentHandlers["get_selection"]({
      type: "get_selection",
      requestId: "r5",
      params,
    });

  beforeEach(() => {
    clearPinned();
    currentPage.selection = [nodes["1:1"]];
  });

  it("follows the live selection by default", async () => {
    const result = await getSelection();
    expect(result.data.map((n: any) => n.id)).toEqual(["1:1"]);
  });

  // The whole point of a pin: it holds still while the selection moves.
  it("reads the pinned set instead when asked for it", async () => {
    setPinned(["1:2"]);
    currentPage.selection = [nodes["1:1"]];
    const result = await getSelection({ source: "pinned" });
    expect(result.data.map((n: any) => n.id)).toEqual(["1:2"]);
  });

  it("answers empty rather than failing when nothing is pinned", async () => {
    expect((await getSelection({ source: "pinned" })).data).toEqual([]);
  });

  // A pinned node the user has since deleted must not fail every later call.
  it("skips a pinned node that is no longer in the file", async () => {
    setPinned(["1:2", "9:9"]);
    const result = await getSelection({ source: "pinned" });
    expect(result.data.map((n: any) => n.id)).toEqual(["1:2"]);
  });
});

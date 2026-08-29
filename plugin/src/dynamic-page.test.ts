import { beforeEach, describe, expect, it } from "bun:test";
import { installDynamicDocument, makeNode } from "./dynamic-page.fixture";
import { readDocumentHandlers } from "./read-document";
import { readStylesHandlers } from "./read-styles";
import { clearPinned } from "./pinned";

// Every handler that walks pages rather than the current one. Under
// documentAccess "dynamic-page" each of them has to call loadAsync first, and
// the fixture's pages report no children until it does — so a handler that
// forgets returns an empty answer here exactly as it would in Figma.

let doc: ReturnType<typeof installDynamicDocument>;

beforeEach(() => {
  clearPinned();
  doc = installDynamicDocument([
    {
      id: "1:0",
      name: "Page 1",
      children: [
        makeNode("1:1", "Button Primary"),
        makeNode("1:2", "Card", "RECTANGLE"),
      ],
    },
    {
      id: "2:0",
      name: "Page 2",
      children: [makeNode("2:1", "Button Ghost", "COMPONENT")],
    },
    {
      id: "3:0",
      name: "Page 3",
      children: [makeNode("3:1", "Button Danger", "COMPONENT")],
    },
  ]);
});

const call = (handlers: any, type: string, params?: any) =>
  handlers[type]({ type, requestId: "r1", params });

describe("handlers that walk every page", () => {
  it("search_nodes finds nodes on pages it had to load", async () => {
    const result = await call(readDocumentHandlers, "search_nodes", {
      query: "button",
      scope: "document",
    });
    expect(result.data.nodes.map((n: any) => n.id)).toEqual(["1:1", "2:1", "3:1"]);
    expect(doc.loadedPages).toEqual(["1:0", "2:0", "3:0"]);
  });

  it("get_document with scope document reaches every page", async () => {
    const result = await call(readDocumentHandlers, "get_document", { scope: "document" });
    expect(result.data.children.map((p: any) => p.id)).toEqual(["1:0", "2:0", "3:0"]);
    // Not just the pages: their contents, which is what an unloaded page hides.
    expect(result.data.children[1].children.map((n: any) => n.id)).toEqual(["2:1"]);
    expect(doc.loadedPages).toEqual(["1:0", "2:0", "3:0"]);
  });

  it("get_local_components finds components on other pages", async () => {
    const result = await call(readStylesHandlers, "get_local_components", {});
    expect(result.data.components.map((c: any) => c.id).sort()).toEqual(["2:1", "3:1"]);
    expect(doc.loadedPages).toEqual(["1:0", "2:0", "3:0"]);
  });
});

describe("handlers scoped to the current page", () => {
  // The current page is already loaded in Figma; these must not pay to load
  // every other page just to answer about this one.
  it("get_document defaults to the current page and loads nothing", async () => {
    doc.pages[0].loadAsync();
    doc.loadedPages.length = 0;
    const result = await call(readDocumentHandlers, "get_document");
    expect(result.data.id).toBe("1:0");
    expect(doc.loadedPages).toEqual([]);
  });

  it("search_nodes defaults to the current page", async () => {
    const result = await call(readDocumentHandlers, "search_nodes", { query: "button" });
    expect(result.data.nodes.map((n: any) => n.id)).toEqual(["1:1"]);
    expect(doc.loadedPages).toEqual(["1:0"]);
  });

  it("follows a page switch rather than the page it started on", async () => {
    doc.setCurrentPage("3:0");
    const result = await call(readDocumentHandlers, "search_nodes", { query: "button" });
    expect(result.data.nodes.map((n: any) => n.id)).toEqual(["3:1"]);
  });
});

// A Figma mock that behaves the way documentAccess "dynamic-page" actually
// does: a page that has not been loaded reports NO children.
//
// This is the trap that made search_nodes answer "not found" for every node on
// every page but the current one. A mock whose pages are always populated
// cannot catch it — the handler passes whether or not it calls loadAsync. Here,
// forgetting loadAsync produces the same empty answer it produces in Figma, so
// the test fails instead of the user.
//
// Test support only; nothing imports it from the plugin entry points.

export interface FixtureNode {
  id: string;
  name: string;
  type?: string;
  children?: FixtureNode[];
  [key: string]: any;
}

export interface DynamicDocument {
  /** The page ids that had loadAsync called on them, in order. */
  loadedPages: string[];
  /** Everything posted to the UI, progress updates included. */
  posted: any[];
  pages: any[];
  root: any;
  /** Switch the current page, as navigate_to_page does. */
  setCurrentPage(id: string): void;
}

export const makeNode = (
  id: string,
  name: string,
  type = "FRAME",
  children: FixtureNode[] = [],
): FixtureNode => {
  const node: FixtureNode = {
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
  return node;
};

/**
 * Install a `figma` global whose pages start unloaded.
 *
 * Every node is still reachable through getNodeByIdAsync, which is how Figma
 * behaves for an id you already hold; what an unloaded page withholds is its
 * children, and that is what this reproduces.
 */
export function installDynamicDocument(
  pageSpecs: Array<{ id: string; name: string; children: FixtureNode[] }>,
  options: { fileName?: string } = {},
): DynamicDocument {
  const loadedPages: string[] = [];
  const posted: any[] = [];
  const byId = new Map<string, any>();

  const index = (node: any) => {
    byId.set(node.id, node);
    for (const child of node.children ?? []) index(child);
  };

  const pages = pageSpecs.map((spec) => {
    let loaded = false;
    const page: any = {
      id: spec.id,
      name: spec.name,
      type: "PAGE",
      selection: [],
      get children() {
        // The whole point of the fixture.
        return loaded ? spec.children : [];
      },
      loadAsync: async () => {
        loaded = true;
        loadedPages.push(spec.id);
      },
      findAllWithCriteria({ types }: { types: string[] }) {
        const found: any[] = [];
        const walk = (node: any) => {
          if (types.includes(node.type)) found.push(node);
          for (const child of node.children ?? []) walk(child);
        };
        for (const child of page.children) walk(child);
        return found;
      },
    };
    byId.set(page.id, page);
    for (const child of spec.children) index(child);
    return page;
  });

  let currentPage = pages[0];

  const doc: DynamicDocument = {
    loadedPages,
    posted,
    pages,
    root: {
      id: "0:0",
      name: options.fileName ?? "Fixture",
      type: "DOCUMENT",
      get children() {
        return pages;
      },
    },
    setCurrentPage(id: string) {
      const next = pages.find((page) => page.id === id);
      if (!next) throw new Error(`No such page in the fixture: ${id}`);
      currentPage = next;
    },
  };

  (globalThis as any).figma = {
    get root() {
      return doc.root;
    },
    get currentPage() {
      return currentPage;
    },
    getNodeByIdAsync: async (id: string) => byId.get(id) ?? null,
    ui: { postMessage: (msg: any) => posted.push(msg) },
  };

  return doc;
}

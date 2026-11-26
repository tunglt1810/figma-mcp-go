import { describe, expect, it, beforeEach } from "bun:test";
import { markCancelled, resetCancellations } from "./cancellation";
import { resetWriteQueue } from "./write-queue";
import { resetUndoCheckpointState } from "./undo-checkpoint";

// main.ts runs startPanel() at import time, so the globals it uses must
// exist before the import. Vite defines __html__ and __APP_VERSION__ at build
// time. Here they are plain globals.
const posted: any[] = [];
let uiHandler: ((message: any) => any) | null = null;
let shownWith: any = null;

(globalThis as any).__html__ = "<html></html>";
(globalThis as any).__APP_VERSION__ = "0.0.0-test";
(globalThis as any).figma = {
  root: { name: "File", children: [] },
  currentPage: { name: "Page 1", selection: [] },
  ui: {
    postMessage: (message: any) => posted.push(message),
    get onmessage() {
      return uiHandler;
    },
    set onmessage(fn: any) {
      uiHandler = fn;
    },
    resize: () => {},
  },
  showUI: (_html: string, options: any) => { shownWith = options; },
  on: () => {},
  notify: () => {},
  getNodeByIdAsync: async () => null,
  clientStorage: { getAsync: async () => null, setAsync: async () => {} },
};

const { handleRequest } = await import("./main");

beforeEach(() => {
  resetWriteQueue();
  resetCancellations();
});

describe("startPanel", () => {
  // The dark palette depends on a `figma-dark` class that Figma adds only for
  // plugins that opt in here. Dropping this option silently leaves the panel
  // light in a dark editor. Nothing throws, and no other test would notice.
  it("opts into Figma's theme classes", () => {
    expect(shownWith?.themeColors).toBe(true);
  });
});

describe("handleRequest", () => {
  it("keeps a concurrent standalone write outside the pipeline undo step", async () => {
    const previous = (globalThis as any).figma;
    const node = { id: "1:1", name: "original" };
    const checkpoints: string[] = [];
    let entered!: () => void;
    let release!: () => void;
    const started = new Promise<void>(resolve => { entered = resolve; });
    const blocked = new Promise<void>(resolve => { release = resolve; });
    let firstLookup = true;
    const api = {
      ...previous,
      getNodeByIdAsync: async () => {
        if (firstLookup) {
          firstLookup = false;
          entered();
          await blocked;
        }
        return node;
      },
    };
    Object.defineProperty(api, "commitUndo", {
      value: () => checkpoints.push(node.name), writable: false,
    });
    (globalThis as any).figma = api;
    try {
      const pipeline = handleRequest({
        type: "batch_execute_pipeline", requestId: "pipeline",
        params: { steps: [
          { action: "batch_rename_nodes", params: { nodeId: node.id, name: "first" } },
          { action: "batch_rename_nodes", params: { nodeId: node.id, name: "pipeline" } },
        ] },
      });
      await Promise.race([
        started,
        new Promise((_, reject) => setTimeout(() => reject(new Error("pipeline never reached its first lookup")), 1000)),
      ]);
      const standalone = handleRequest({
        type: "batch_rename_nodes", requestId: "standalone", nodeIds: [node.id],
        params: { name: "standalone" },
      });
      expect(node.name).toBe("original");
      release();
      const results = await Promise.all([pipeline, standalone]);
      expect(results.every(result => !result.error)).toBe(true);
      expect(checkpoints).toEqual(["pipeline", "standalone"]);
    } finally {
      release();
      (globalThis as any).figma = previous;
      resetUndoCheckpointState();
    }
  });

  // The server's clock cannot see the plugin queue. A write waiting behind a
  // long pipeline sends no progress, so its 30s timer runs out, the caller is
  // told it failed, and a cancel arrives. Running it anyway applies the edit
  // for a caller that was told it did not happen, and has already retried.
  it("does not run a queued write the server has cancelled", async () => {
    markCancelled("r1");
    const response = await handleRequest({
      type: "set_text",
      requestId: "r1",
      nodeIds: ["1:1"],
      params: { text: "hello" },
    });
    expect(response.error).toBe("Request cancelled");
  });

  it("runs a queued write that was not cancelled", async () => {
    const response = await handleRequest({
      type: "set_text",
      requestId: "r2",
      nodeIds: ["1:1"],
      params: { text: "hello" },
    });
    // The mock has no such node, so it fails on the node lookup. That proves
    // the handler was reached, not skipped.
    expect(response.error).not.toBe("Request cancelled");
  });
});

describe("plugin-capabilities", () => {
  const capabilityMessages = () => posted.filter(m => m.type === "plugin-capabilities");

  it("lists the batch pipeline alongside the handlers", () => {
    expect(capabilityMessages()[0].handlers).toContain("batch_execute_pipeline");
    expect(capabilityMessages()[0].handlers).toContain("get_document");
  });

  // The panel's message listener is not installed yet when showUI returns, so
  // the first post can arrive in that gap. sendStatus is already sent twice for
  // that reason. The capability list needs the same second chance. Otherwise the
  // server thinks the plugin announced nothing, which it reads as "old plugin,
  // allow everything".
  it("is re-sent when the panel says it is ready", async () => {
    const before = capabilityMessages().length;
    expect(before).toBe(1);
    await uiHandler!({ type: "ui-ready" });
    expect(capabilityMessages().length).toBe(before + 1);
  });
});

describe("go_to_nodes", () => {
  const withNodes = async (nodes: Record<string, any>, run: () => Promise<void>) => {
    const previous = (globalThis as any).figma;
    const page = { id: "0:1", name: "Checkout", type: "PAGE", selection: [] as any[], parent: { type: "DOCUMENT" } };
    for (const node of Object.values(nodes)) node.parent = page;
    (globalThis as any).figma = {
      ...previous,
      currentPage: page,
      getNodeByIdAsync: async (id: string) => nodes[id] ?? null,
      setCurrentPageAsync: async () => {},
      viewport: { zoom: 1, scrollAndZoomIntoView() {} },
    };
    try {
      await run();
    } finally {
      (globalThis as any).figma = previous;
    }
  };

  it("replies with the count and the page", async () => {
    await withNodes({ "1:1": { id: "1:1", name: "Card", type: "FRAME" } }, async () => {
      posted.length = 0;
      await uiHandler!({ type: "go_to_nodes", nodeIds: ["1:1"], select: true });
      expect(posted).toContainEqual({ type: "go_to_nodes_result", count: 1, pageName: "Checkout" });
    });
  });

  it("replies with the error for a missing node", async () => {
    await withNodes({}, async () => {
      posted.length = 0;
      await uiHandler!({ type: "go_to_nodes", nodeIds: ["9:9"], select: false });
      const reply = posted.find(message => message.type === "go_to_nodes_result");
      expect(reply?.error).toContain("Node not found: 9:9");
    });
  });
});

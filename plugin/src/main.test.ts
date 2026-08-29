import { describe, expect, it, beforeEach } from "bun:test";
import { markCancelled, resetCancellations } from "./cancellation";
import { resetWriteQueue } from "./write-queue";

// main.ts runs startPanel() at import time, so the globals it touches have to
// exist before the import. Vite defines __html__ and __APP_VERSION__ at build
// time; here they are plain globals.
const posted: any[] = [];
let uiHandler: ((message: any) => any) | null = null;

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
  showUI: () => {},
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

describe("handleRequest", () => {
  // The plugin queue is invisible to the server's clock: a write waiting behind
  // a long pipeline emits no progress, so its 30s timer runs out, the caller is
  // told it failed, and a cancel arrives. Running it anyway lands the edit for a
  // caller that has already been told it did not happen, and has retried.
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
    // The mock has no such node, so it fails on the node lookup — which is proof
    // the handler was reached rather than skipped.
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
  // the first post can land in the gap. sendStatus is already sent twice for
  // that reason; the capability list needs the same second chance, or the server
  // is left thinking the plugin announced nothing — which it reads as "old
  // plugin, allow everything".
  it("is re-sent when the panel says it is ready", async () => {
    const before = capabilityMessages().length;
    expect(before).toBe(1);
    await uiHandler!({ type: "ui-ready" });
    expect(capabilityMessages().length).toBe(before + 1);
  });
});

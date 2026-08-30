import { describe, expect, it, beforeEach } from "bun:test";
import { writeDocumentHandlers } from "./write-document";

let saveCalls: any[];

beforeEach(() => {
  saveCalls = [];
  (globalThis as any).figma = {
    saveVersionHistoryAsync: async (title: string, description?: string) => {
      saveCalls.push({ title, description });
      return { id: "v123" };
    },
  };
});

const call = (params: any) =>
  writeDocumentHandlers["save_version_checkpoint"]({
    type: "save_version_checkpoint",
    requestId: "r1",
    params,
  });

describe("save_version_checkpoint", () => {
  it("saves a named version and returns its id", async () => {
    const result = await call({ title: "Before the redesign" });
    expect(saveCalls).toEqual([{ title: "Before the redesign", description: undefined }]);
    expect(result.data.id).toBe("v123");
    expect(result.data.title).toBe("Before the redesign");
  });

  it("passes the description through", async () => {
    await call({ title: "Checkpoint", description: "Generated before a batch run" });
    expect(saveCalls[0].description).toBe("Generated before a batch run");
  });

  it("requires a title", async () => {
    expect(call({})).rejects.toThrow(/title is required/);
  });

  // FigJam and Slides have no version history; the API is simply absent there.
  it("explains itself when the editor has no version history", async () => {
    (globalThis as any).figma = {};
    expect(call({ title: "x" })).rejects.toThrow(/not available in this editor/);
  });

  it("tolerates an API that returns nothing useful", async () => {
    (globalThis as any).figma = { saveVersionHistoryAsync: async () => undefined };
    const result = await call({ title: "x" });
    expect(result.data.id).toBeNull();
  });
});

// ── set_codegen_result ────────────────────────────────────────────────────────

describe("set_codegen_result", () => {
  let node: any;

  const setup = () => {
    const store: Record<string, string> = {};
    node = {
      id: "1:1",
      name: "Button",
      relaunch: null as any,
      getSharedPluginData: (ns: string, key: string) => store[`${ns}/${key}`] ?? "",
      setSharedPluginData: (ns: string, key: string, value: string) => {
        store[`${ns}/${key}`] = value;
      },
      setRelaunchData: (data: any) => { node.relaunch = data; },
    };
    (globalThis as any).figma = {
      getNodeByIdAsync: async (id: string) => (id === "1:1" ? node : null),
      commitUndo: () => {},
    };
    return store;
  };

  const call = (params: any, nodeIds = ["1:1"]) =>
    writeDocumentHandlers["set_codegen_result"]({
      type: "set_codegen_result",
      requestId: "r1",
      nodeIds,
      params,
    });

  it("stores blocks as shared data so the whole team's Dev Mode sees them", async () => {
    const store = setup();
    const res = await call({ blocks: [{ title: "Button.tsx", language: "typescript", code: "const x = 1" }] });
    expect(JSON.parse(store["figma-mcp-go/codegen"])).toEqual([
      { title: "Button.tsx", language: "TYPESCRIPT", code: "const x = 1" },
    ]);
    expect(res.data.blocks[0].bytes).toBe(11);
  });

  it("falls back for a language Figma does not know", async () => {
    const store = setup();
    await call({ blocks: [{ title: "x", language: "cobol", code: "y" }] });
    expect(JSON.parse(store["figma-mcp-go/codegen"])[0].language).toBe("PLAINTEXT");
  });

  it("defaults a missing title", async () => {
    const store = setup();
    await call({ blocks: [{ language: "css", code: "a{}" }] });
    expect(JSON.parse(store["figma-mcp-go/codegen"])[0].title).toBe("Code");
  });

  it("adds a relaunch button so the node can reopen the plugin", async () => {
    setup();
    await call({ blocks: [{ title: "x", language: "css", code: "a{}" }] });
    expect(node.relaunch.open).toContain("1 code block");
  });

  // An empty array must clear, and shared plugin data is cleared with "".
  it("clears the stored code and the relaunch button", async () => {
    const store = setup();
    await call({ blocks: [{ title: "x", language: "css", code: "a{}" }] });
    const res = await call({ blocks: [] });
    expect(store["figma-mcp-go/codegen"]).toBe("");
    expect(res.data.cleared).toBe(true);
    expect(node.relaunch).toEqual({});
  });

  it("refuses a block with no code", async () => {
    setup();
    expect(call({ blocks: [{ title: "x", language: "css" }] })).rejects.toThrow(/non-empty code/);
    expect(call({ blocks: [{ title: "x", code: "" }] })).rejects.toThrow(/non-empty code/);
  });

  it("reports a missing node", async () => {
    setup();
    expect(call({ blocks: [] }, ["9:9"])).rejects.toThrow(/Node not found/);
  });
});

// ── manage_plugin_data ────────────────────────────────────────────────────────

describe("manage_plugin_data", () => {
  let node: any;
  let store: Record<string, string>;

  const setup = () => {
    store = {};
    node = {
      id: "1:1",
      name: "Card",
      getSharedPluginData: (ns: string, key: string) => store[`${ns}/${key}`] ?? "",
      setSharedPluginData: (ns: string, key: string, value: string) => {
        if (value === "") delete store[`${ns}/${key}`];
        else store[`${ns}/${key}`] = value;
      },
      getSharedPluginDataKeys: (ns: string) =>
        Object.keys(store).filter(k => k.startsWith(`${ns}/`)).map(k => k.slice(ns.length + 1)),
    };
    (globalThis as any).figma = {
      getNodeByIdAsync: async (id: string) => (id === "1:1" ? node : null),
      commitUndo: () => {},
    };
  };

  const call = (params: any) =>
    writeDocumentHandlers["manage_plugin_data"]({
      type: "manage_plugin_data",
      requestId: "r1",
      nodeIds: ["1:1"],
      params,
    });

  it("round-trips a value", async () => {
    setup();
    await call({ action: "set", key: "component", value: "src/Button.tsx" });
    const res = await call({ action: "get", key: "component" });
    expect(res.data.value).toBe("src/Button.tsx");
  });

  // Figma cannot tell "unset" from "set to empty"; null is the honest answer.
  it("reports an unset key as null", async () => {
    setup();
    expect((await call({ action: "get", key: "missing" })).data.value).toBeNull();
  });

  it("lists the keys in a namespace", async () => {
    setup();
    await call({ action: "set", key: "a", value: "1" });
    await call({ action: "set", key: "b", value: "2" });
    expect((await call({ action: "keys" })).data.keys.sort()).toEqual(["a", "b"]);
  });

  it("deletes a key", async () => {
    setup();
    await call({ action: "set", key: "a", value: "1" });
    await call({ action: "delete", key: "a" });
    expect((await call({ action: "get", key: "a" })).data.value).toBeNull();
  });

  it("honours a custom namespace", async () => {
    setup();
    await call({ action: "set", key: "a", value: "1", namespace: "acme" });
    expect(store["acme/a"]).toBe("1");
    expect((await call({ action: "get", key: "a" })).data.value).toBeNull();
  });

  it("requires a key for everything but keys", async () => {
    setup();
    expect(call({ action: "get" })).rejects.toThrow(/key is required/);
    expect(call({ action: "set", value: "x" })).rejects.toThrow(/key is required/);
  });

  it("requires a string value on set", async () => {
    setup();
    expect(call({ action: "set", key: "a" })).rejects.toThrow(/value is required/);
    expect(call({ action: "set", key: "a", value: 5 })).rejects.toThrow(/must be a string/);
  });

  it("rejects an unknown action", async () => {
    setup();
    expect(call({ action: "increment", key: "a" })).rejects.toThrow(/must be get, set, delete, or keys/);
  });
});

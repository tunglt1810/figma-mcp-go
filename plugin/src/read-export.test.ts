import { beforeEach, describe, expect, it } from "bun:test";
import { readExportHandlers } from "./read-export";

let progressMessages: any[];
let nodes: Record<string, any>;

const makeNode = (id: string, name: string) => {
  const node = {
    id,
    name,
    type: "FRAME",
    width: 100,
    height: 50,
    exportAsync: async () => new Uint8Array([1, 2, 3]),
  };
  nodes[id] = node;
  return node;
};

beforeEach(() => {
  progressMessages = [];
  nodes = {};
  makeNode("1:1", "Cover");
  makeNode("1:2", "Contents");
  makeNode("1:3", "Summary");
  (globalThis as any).figma = {
    getNodeByIdAsync: async (id: string) => nodes[id] ?? null,
    base64Encode: () => "AQID",
    currentPage: { selection: [] },
    ui: { postMessage: (msg: any) => progressMessages.push(msg) },
  };
});

const updates = () => progressMessages.filter((m) => m.type === "progress_update");

describe("get_screenshot progress", () => {
  it("reports the node it is about to render", async () => {
    const result = await readExportHandlers["get_screenshot"]({
      type: "get_screenshot",
      requestId: "r1",
      nodeIds: ["1:1", "1:2", "1:3"],
      params: {},
    });
    expect(result.data.exports.map((e: any) => e.nodeId)).toEqual(["1:1", "1:2", "1:3"]);
    expect(updates().length).toBe(3);
    expect(updates()[0].message).toBe("Exporting Cover (1/3)");
    // The response says the work finished; a progress message never claims it.
    expect(updates().every((u: any) => u.progress < 100)).toBe(true);
  });

  it("stays quiet for a single node", async () => {
    await readExportHandlers["get_screenshot"]({
      type: "get_screenshot",
      requestId: "r2",
      nodeIds: ["1:1"],
      params: {},
    });
    expect(updates()).toEqual([]);
  });

  it("asks for a scale constraint only on the raster formats", async () => {
    const seen: any[] = [];
    nodes["1:1"].exportAsync = async (settings: any) => {
      seen.push(settings);
      return new Uint8Array([1]);
    };
    for (const format of ["PNG", "JPG", "SVG", "PDF"]) {
      await readExportHandlers["get_screenshot"]({
        type: "get_screenshot",
        requestId: "r3",
        nodeIds: ["1:1"],
        params: { format, scale: 3 },
      });
    }
    expect(seen.map((s) => s.format)).toEqual(["PNG", "JPG", "SVG", "PDF"]);
    expect(seen[0].constraint).toEqual({ type: "SCALE", value: 3 });
    expect(seen[1].constraint).toEqual({ type: "SCALE", value: 3 });
    expect(seen[2].constraint).toBeUndefined();
    expect(seen[3].constraint).toBeUndefined();
  });
});

describe("export_frames_to_pdf progress", () => {
  it("reports a page at a time", async () => {
    const result = await readExportHandlers["export_frames_to_pdf"]({
      type: "export_frames_to_pdf",
      requestId: "r4",
      nodeIds: ["1:1", "1:2"],
      params: {},
    });
    expect(result.data.frames.length).toBe(2);
    expect(updates().map((u: any) => u.message)).toEqual([
      "Rendering page 1/2",
      "Rendering page 2/2",
    ]);
  });
});

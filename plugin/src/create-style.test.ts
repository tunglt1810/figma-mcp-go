import { describe, it, expect, beforeEach } from "vitest";
import { handleWriteStyleRequest } from "./write-styles";

// create_style replaced four create_*_style tools on the MCP surface. These
// check the router reaches each of the four implementations, and that the
// arguments survive the trip — in particular effectType, which is unwrapped
// back to the `type` the effect implementation reads.

let created: Record<string, any>;

const makeStyle = (kind: string) => {
  const style: any = { id: `S:${kind}`, name: "", description: "", remove() {} };
  created[kind] = style;
  return style;
};

const makeRequest = (params: any) => ({
  type: "create_style",
  requestId: "req-test-1",
  nodeIds: [],
  params,
});

beforeEach(() => {
  created = {};
  (globalThis as any).figma = {
    getLocalPaintStylesAsync: async () => [],
    getLocalTextStylesAsync: async () => [],
    getLocalEffectStylesAsync: async () => [],
    getLocalGridStylesAsync: async () => [],
    createPaintStyle: () => makeStyle("PAINT"),
    createTextStyle: () => makeStyle("TEXT"),
    createEffectStyle: () => makeStyle("EFFECT"),
    createGridStyle: () => makeStyle("GRID"),
    loadFontAsync: async () => {},
    commitUndo: () => {},
  };
});

describe("create_style", () => {
  it("routes PAINT to the paint style implementation", async () => {
    const res = await handleWriteStyleRequest(
      makeRequest({ type: "PAINT", name: "Brand/Primary", color: "#ff0000" }),
    );
    expect(created.PAINT).toBeDefined();
    expect(created.PAINT.name).toBe("Brand/Primary");
    expect(created.PAINT.paints[0].color.r).toBeCloseTo(1);
    // The response names the tool the caller actually called.
    expect(res.type).toBe("create_style");
  });

  it("routes TEXT and carries the font arguments", async () => {
    await handleWriteStyleRequest(
      makeRequest({ type: "TEXT", name: "Heading/H1", fontSize: 32, fontFamily: "Inter" }),
    );
    expect(created.TEXT).toBeDefined();
    expect(created.TEXT.fontSize).toBe(32);
  });

  it("routes EFFECT and unwraps effectType back to type", async () => {
    await handleWriteStyleRequest(
      makeRequest({ type: "EFFECT", name: "Shadow/Card", effectType: "INNER_SHADOW", radius: 4 }),
    );
    expect(created.EFFECT).toBeDefined();
    expect(created.EFFECT.effects[0].type).toBe("INNER_SHADOW");
  });

  it("routes GRID", async () => {
    await handleWriteStyleRequest(
      makeRequest({ type: "GRID", name: "Grid/Desktop", pattern: "COLUMNS", count: 12 }),
    );
    expect(created.GRID).toBeDefined();
    expect(created.GRID.layoutGrids[0].count).toBe(12);
  });

  it("reports an unknown kind rather than silently doing nothing", async () => {
    await expect(
      handleWriteStyleRequest(makeRequest({ type: "SHADOW", name: "x" })),
    ).rejects.toThrow(/PAINT, TEXT, EFFECT, or GRID/);
  });
});

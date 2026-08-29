import { describe, expect, it, beforeEach } from "bun:test";
import { resolveRange, resolveRangeFont, sortRanges, writeTextHandlers } from "./write-text";

// ── resolveRange ──────────────────────────────────────────────────────────────

describe("resolveRange", () => {
  it("accepts a range inside the text", () => {
    expect(resolveRange({ start: 2, end: 5 }, 10)).toEqual([2, 5]);
  });

  it("accepts a range covering the whole text", () => {
    expect(resolveRange({ start: 0, end: 10 }, 10)).toEqual([0, 10]);
  });

  // Figma's own error names neither the range nor the length.
  it("names the range and the length when it runs past the end", () => {
    expect(() => resolveRange({ start: 0, end: 20 }, 10)).toThrow(/20 is outside.*10 character/);
  });

  it("rejects a negative start", () => {
    expect(() => resolveRange({ start: -1, end: 5 }, 10)).toThrow(/outside the text/);
  });

  it("rejects an empty range", () => {
    expect(() => resolveRange({ start: 3, end: 3 }, 10)).toThrow(/empty/);
    expect(() => resolveRange({ start: 5, end: 2 }, 10)).toThrow(/empty/);
  });

  it("rejects fractional offsets", () => {
    expect(() => resolveRange({ start: 0.5, end: 3 }, 10)).toThrow(/whole numbers/);
  });
});

// ── resolveRangeFont ──────────────────────────────────────────────────────────

describe("resolveRangeFont", () => {
  const current = { family: "Inter", style: "Regular" };

  it("returns nothing when the range asks for no font change", () => {
    expect(resolveRangeFont({ start: 0, end: 1 }, current)).toBeNull();
  });

  it("inherits the family when only a style is given", () => {
    expect(resolveRangeFont({ start: 0, end: 1, fontStyle: "Bold" }, current))
      .toEqual({ family: "Inter", style: "Bold" });
  });

  it("inherits the style when only a family is given", () => {
    expect(resolveRangeFont({ start: 0, end: 1, fontFamily: "Roboto" }, current))
      .toEqual({ family: "Roboto", style: "Regular" });
  });

  it("takes both when both are given", () => {
    expect(resolveRangeFont({ start: 0, end: 1, fontFamily: "Roboto", fontStyle: "Bold" }, current))
      .toEqual({ family: "Roboto", style: "Bold" });
  });

  // A mixed range reports figma.mixed, so there is no font to inherit from.
  it("asks for both halves when the existing font is mixed", () => {
    expect(() => resolveRangeFont({ start: 0, end: 1, fontStyle: "Bold" }, Symbol("mixed")))
      .toThrow(/both required/);
  });
});

// ── sortRanges ────────────────────────────────────────────────────────────────

describe("sortRanges", () => {
  it("orders by start so overlapping edits apply as written", () => {
    const sorted = sortRanges([
      { start: 5, end: 8 },
      { start: 0, end: 3 },
      { start: 3, end: 5 },
    ]);
    expect(sorted.map(r => r.start)).toEqual([0, 3, 5]);
  });

  it("does not mutate the caller's array", () => {
    const input = [{ start: 5, end: 8 }, { start: 0, end: 3 }];
    sortRanges(input);
    expect(input[0].start).toBe(5);
  });
});

// ── set_text_ranges ───────────────────────────────────────────────────────────

let node: any;
let loadedFonts: any[];
let calls: any[];

beforeEach(() => {
  loadedFonts = [];
  calls = [];
  const record = (name: string) => (...args: any[]) => calls.push({ name, args });
  node = {
    id: "1:1",
    name: "Body",
    type: "TEXT",
    characters: "Hello world",
    getRangeAllFontNames: () => [{ family: "Inter", style: "Regular" }],
    getRangeFontName: () => ({ family: "Inter", style: "Regular" }),
    setRangeFontName: record("setRangeFontName"),
    setRangeFontSize: record("setRangeFontSize"),
    setRangeFills: record("setRangeFills"),
    setRangeTextDecoration: record("setRangeTextDecoration"),
    setRangeTextCase: record("setRangeTextCase"),
    setRangeLetterSpacing: record("setRangeLetterSpacing"),
    setRangeLineHeight: record("setRangeLineHeight"),
    setRangeListOptions: record("setRangeListOptions"),
    setRangeIndentation: record("setRangeIndentation"),
    setRangeHyperlink: record("setRangeHyperlink"),
  };
  (globalThis as any).figma = {
    getNodeByIdAsync: async (id: string) => (id === "1:1" ? node : null),
    loadFontAsync: async (font: any) => { loadedFonts.push(font); },
    commitUndo: () => {},
  };
});

const call = (params: any, nodeIds = ["1:1"]) =>
  writeTextHandlers["set_text_ranges"]({
    type: "set_text_ranges",
    requestId: "r1",
    nodeIds,
    params,
  });

const callsNamed = (name: string) => calls.filter(c => c.name === name);

describe("set_text_ranges", () => {
  it("makes one word bold without touching the rest", async () => {
    const result = await call({ ranges: [{ start: 6, end: 11, fontStyle: "Bold" }] });
    expect(callsNamed("setRangeFontName")).toEqual([
      { name: "setRangeFontName", args: [6, 11, { family: "Inter", style: "Bold" }] },
    ]);
    expect(result.data.rangesApplied).toBe(1);
  });

  // Figma refuses to edit a text node while any font in it is unloaded, not
  // just the fonts being written.
  it("loads every font already in the node", async () => {
    node.getRangeAllFontNames = () => [
      { family: "Inter", style: "Regular" },
      { family: "Roboto", style: "Bold" },
    ];
    await call({ ranges: [{ start: 0, end: 5, fontSize: 20 }] });
    expect(loadedFonts).toEqual([
      { family: "Inter", style: "Regular" },
      { family: "Roboto", style: "Bold" },
    ]);
  });

  it("sets a colour as a solid fill", async () => {
    await call({ ranges: [{ start: 0, end: 5, color: "#ff0000" }] });
    const fills = callsNamed("setRangeFills")[0].args[2];
    expect(fills[0].type).toBe("SOLID");
    expect(fills[0].color.r).toBeCloseTo(1);
  });

  it("sets a hyperlink", async () => {
    await call({ ranges: [{ start: 0, end: 5, hyperlink: "https://example.com" }] });
    expect(callsNamed("setRangeHyperlink")[0].args[2])
      .toEqual({ type: "URL", value: "https://example.com" });
  });

  it("clears a hyperlink when it is explicitly null", async () => {
    await call({ ranges: [{ start: 0, end: 5, hyperlink: null }] });
    expect(callsNamed("setRangeHyperlink")[0].args[2]).toBeNull();
  });

  it("leaves the hyperlink alone when the range does not mention it", async () => {
    await call({ ranges: [{ start: 0, end: 5, fontSize: 12 }] });
    expect(callsNamed("setRangeHyperlink")).toEqual([]);
  });

  it("sets a list type", async () => {
    await call({ ranges: [{ start: 0, end: 5, listType: "UNORDERED" }] });
    expect(callsNamed("setRangeListOptions")[0].args[2]).toEqual({ type: "UNORDERED" });
  });

  it("takes AUTO line height without a value", async () => {
    await call({ ranges: [{ start: 0, end: 5, lineHeight: "AUTO" }] });
    expect(callsNamed("setRangeLineHeight")[0].args[2]).toEqual({ unit: "AUTO" });
  });

  it("applies several ranges in text order", async () => {
    await call({
      ranges: [
        { start: 6, end: 11, fontSize: 20 },
        { start: 0, end: 5, fontSize: 10 },
      ],
    });
    expect(callsNamed("setRangeFontSize").map(c => c.args[0])).toEqual([0, 6]);
  });

  it("requires at least one range", async () => {
    expect(call({ ranges: [] })).rejects.toThrow(/ranges is required/);
  });

  it("refuses a node with no text", async () => {
    node.characters = "";
    expect(call({ ranges: [{ start: 0, end: 1 }] })).rejects.toThrow(/no text/);
  });

  it("refuses a node that is not TEXT", async () => {
    node.type = "FRAME";
    expect(call({ ranges: [{ start: 0, end: 1 }] })).rejects.toThrow(/not a TEXT node/);
  });

  it("reports a missing node", async () => {
    expect(call({ ranges: [{ start: 0, end: 1 }] }, ["9:9"])).rejects.toThrow(/Node not found/);
  });
});

describe("set_text_ranges with a font the file lacks", () => {
  beforeEach(() => {
    (globalThis as any).figma.loadFontAsync = async (font: any) => {
      loadedFonts.push(font);
      if (font.family !== "Inter") throw new Error("unavailable");
    };
  });

  // A range asking for a missing font used to fail on that range, after the
  // ranges before it had already been applied.
  it("applies no range at all when a later one asks for a missing font", async () => {
    await expect(
      call({
        ranges: [
          { start: 0, end: 5, fontStyle: "Bold" },
          { start: 6, end: 11, fontFamily: "Phantom", fontStyle: "Regular" },
        ],
      }),
    ).rejects.toThrow("Font not available in this file: Phantom Regular");
    expect(callsNamed("setRangeFontName")).toEqual([]);
    expect(callsNamed("setRangeFills")).toEqual([]);
  });

  it("names every missing font in one error", async () => {
    await expect(
      call({
        ranges: [
          { start: 0, end: 5, fontFamily: "Phantom", fontStyle: "Regular" },
          { start: 6, end: 11, fontFamily: "Ghost Sans", fontStyle: "Bold" },
        ],
      }),
    ).rejects.toThrow("Phantom Regular, Ghost Sans Bold");
  });

  it("asks Figma for each font once, however many ranges want it", async () => {
    await call({
      ranges: [
        { start: 0, end: 5, fontStyle: "Regular" },
        { start: 6, end: 11, fontStyle: "Regular" },
      ],
    });
    expect(loadedFonts).toEqual([{ family: "Inter", style: "Regular" }]);
  });
});

import { describe, it, expect, beforeEach } from "bun:test";
import {
  isMixed,
  toHex,
  serializePaints,
  getBounds,
  deduplicateStyles,
  serializeVariableValue,
  serializeLineHeight,
  serializeLetterSpacing,
  serializeStyles,
  serializeText,
  serializeNode,
  invertTransform,
} from "./serializers";
import { makeGradientPaint } from "./write-helpers";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockGetStyleByIdAsync: (id: string) => Promise<{ name: string } | null>;

beforeEach(() => {
  mockGetStyleByIdAsync = async (_id: string) => null;
  (globalThis as any).figma = {
    getStyleByIdAsync: (id: string) => mockGetStyleByIdAsync(id),
  };
});

// ── isMixed ──────────────────────────────────────────────────────────────────

describe("isMixed", () => {
  it("returns true for symbols", () => {
    expect(isMixed(Symbol())).toBe(true);
  });
  it("returns false for non-symbols", () => {
    expect(isMixed(14)).toBe(false);
    expect(isMixed("hello")).toBe(false);
    expect(isMixed(null)).toBe(false);
    expect(isMixed(undefined)).toBe(false);
  });
});

// ── toHex ────────────────────────────────────────────────────────────────────

describe("toHex", () => {
  it("converts full white", () => {
    expect(toHex({ r: 1, g: 1, b: 1 })).toBe("#ffffff");
  });
  it("converts full black", () => {
    expect(toHex({ r: 0, g: 0, b: 0 })).toBe("#000000");
  });
  it("converts a mid-range color", () => {
    expect(toHex({ r: 1, g: 0, b: 0 })).toBe("#ff0000");
  });
  it("clamps values above 1", () => {
    expect(toHex({ r: 2, g: 0, b: 0 })).toBe("#ff0000");
  });
  it("clamps values below 0", () => {
    expect(toHex({ r: -1, g: 0, b: 0 })).toBe("#000000");
  });
  it("rounds fractional values", () => {
    // 0.5 * 255 = 127.5 → rounds to 128 = 0x80
    expect(toHex({ r: 0.5, g: 0.5, b: 0.5 })).toBe("#808080");
  });
});

// ── serializePaints ───────────────────────────────────────────────────────────

describe("serializePaints", () => {
  it("returns 'mixed' for symbol input", () => {
    expect(serializePaints(Symbol())).toBe("mixed");
  });
  it("returns undefined for null/non-array", () => {
    expect(serializePaints(null)).toBeUndefined();
    expect(serializePaints("red")).toBeUndefined();
  });
  it("returns undefined for empty array", () => {
    expect(serializePaints([])).toBeUndefined();
  });
  it("filters out unsupported paints like IMAGE", () => {
    const paints = [{ type: "IMAGE" }];
    expect(serializePaints(paints)).toBeUndefined();
  });
  it("serializes GRADIENT_RADIAL to geometry", () => {
    const node = { width: 100, height: 100 };
    // M = identity transform for simplicity
    const paints = [{
      type: "GRADIENT_RADIAL",
      gradientTransform: [[1, 0, 0], [0, 1, 0]],
      gradientStops: [
        { position: 0, color: { r: 1, g: 0.5, b: 0, a: 1 } },
        { position: 1, color: { r: 0, g: 0, b: 0, a: 1 } }
      ]
    }];
    const result = serializePaints(paints, node) as any[];
    expect(result[0].type).toBe("GRADIENT_RADIAL");
    expect(result[0].geometry.center.percentX).toBe(50);
    expect(result[0].geometry.center.percentY).toBe(50);
    expect(result[0].geometry.radius.percentX).toBe(50);
    expect(result[0].geometry.radius.percentY).toBe(50);
    expect(result[0].stops[0].color).toBe("#ff8000");
    expect(result[0].cssString).toBe("radial-gradient(50% 50% at 50% 50%, #ff8000 0%, #000000 100%)");
  });
  it("serializes GRADIENT_RADIAL ellipse (rx != ry)", () => {
    // T_inv maps gradient space → node-normalized space.
    // Scale X by 0.6, Y by 0.3 → ellipse with rx=30%, ry=15%
    const paints = [{
      type: "GRADIENT_RADIAL",
      gradientTransform: invertTransform([[0.6, 0, 0.2], [0, 0.3, 0.35]]),
      gradientStops: [
        { position: 0, color: { r: 1, g: 0, b: 0, a: 1 } },
        { position: 1, color: { r: 0, g: 0, b: 1, a: 1 } }
      ]
    }];
    const result = serializePaints(paints) as any[];
    expect(result[0].geometry.radius.percentX).toBe(30);
    expect(result[0].geometry.radius.percentY).toBe(15);
    expect(result[0].geometry.rotation).toBe(0);
  });
  it("serializes GRADIENT_RADIAL with rotation", () => {
    const theta = 45 * Math.PI / 180;
    const rx = 0.4, ry = 0.2;
    // M = R(theta) · diag(2*rx, 2*ry)
    const T_inv = [
      [2*rx*Math.cos(theta), -2*ry*Math.sin(theta), 0.5 - rx*Math.cos(theta) + ry*Math.sin(theta)],
      [2*rx*Math.sin(theta),  2*ry*Math.cos(theta), 0.5 - rx*Math.sin(theta) - ry*Math.cos(theta)]
    ];
    const paints = [{
      type: "GRADIENT_RADIAL",
      gradientTransform: invertTransform(T_inv),
      gradientStops: [
        { position: 0, color: { r: 1, g: 1, b: 0, a: 1 } }
      ]
    }];
    const result = serializePaints(paints) as any[];
    expect(result[0].geometry.center.percentX).toBe(50);
    expect(result[0].geometry.center.percentY).toBe(50);
    expect(result[0].geometry.radius.percentX).toBe(Math.round(rx * 100));
    expect(result[0].geometry.radius.percentY).toBe(Math.round(ry * 100));
    expect(result[0].geometry.rotation).toBe(45);
  });
  it("serializes GRADIENT_RADIAL with off-center position", () => {
    // Center at (-0.28, -0.13) in normalized space
    const T_inv = [[1.5, 0, -0.28 - 1.5*0.5], [0, 1.5, -0.13 - 1.5*0.5]];
    const paints = [{
      type: "GRADIENT_RADIAL",
      gradientTransform: invertTransform(T_inv),
      gradientStops: [
        { position: 0, color: { r: 1, g: 1, b: 1, a: 1 } }
      ]
    }];
    const result = serializePaints(paints) as any[];
    expect(result[0].geometry.center.percentX).toBe(-28);
    expect(result[0].geometry.center.percentY).toBe(-13);
    expect(result[0].geometry.radius.percentX).toBe(75);
    expect(result[0].geometry.radius.percentY).toBe(75);
  });
  it("roundtrips radial gradient through serialize → makeGradientPaint → serialize", () => {
    const theta = 30 * Math.PI / 180;
    const rx = 0.35, ry = 0.2;
    const T_inv = [
      [2*rx*Math.cos(theta), -2*ry*Math.sin(theta), 0.5 - rx*Math.cos(theta) + ry*Math.sin(theta)],
      [2*rx*Math.sin(theta),  2*ry*Math.cos(theta), 0.5 - rx*Math.sin(theta) - ry*Math.cos(theta)]
    ];
    const paints = [{
      type: "GRADIENT_RADIAL",
      gradientTransform: invertTransform(T_inv),
      gradientStops: [
        { position: 0, color: { r: 1, g: 0, b: 0, a: 1 } },
        { position: 1, color: { r: 0, g: 0, b: 1, a: 1 } }
      ]
    }];
    // First serialize
    const result1 = serializePaints(paints) as any[];
    const g1 = result1[0].geometry;

    // Reconstruct paint via write path
    const reconstructed = makeGradientPaint("GRADIENT_RADIAL", result1[0].stops, g1);

    // Second serialize
    const result2 = serializePaints([{
      type: "GRADIENT_RADIAL",
      gradientTransform: reconstructed.gradientTransform,
      gradientStops: reconstructed.gradientStops
    }]) as any[];
    const g2 = result2[0].geometry;

    expect(g2.center.percentX).toBe(g1.center.percentX);
    expect(g2.center.percentY).toBe(g1.center.percentY);
    expect(g2.radius.percentX).toBe(g1.radius.percentX);
    expect(g2.radius.percentY).toBe(g1.radius.percentY);
    expect(g2.rotation).toBe(g1.rotation);
  });
  it("serializes GRADIENT_LINEAR to geometry", () => {
    const node = { width: 200, height: 100 };
    // M = [[1, 0, 0], [0, 1, 0]]
    const paints = [{
      type: "GRADIENT_LINEAR",
      gradientTransform: [[1, 0, 0], [0, 1, 0]],
      gradientStops: [
        { position: 0, color: { r: 1, g: 1, b: 1, a: 1 } }
      ]
    }];
    const result = serializePaints(paints, node) as any[];
    expect(result[0].type).toBe("GRADIENT_LINEAR");
    expect(result[0].geometry.start.percentX).toBe(0);
    expect(result[0].geometry.end.percentX).toBe(100);
    expect(result[0].geometry.angle).toBe(0);
    expect(result[0].cssString).toBe("linear-gradient(90deg, #ffffff 0%)");
  });
  it("serializes a solid paint with opacity 1 as plain hex", () => {
    const paints = [{ type: "SOLID", color: { r: 1, g: 0, b: 0 }, opacity: 1 }];
    expect(serializePaints(paints)).toEqual(["#ff0000"]);
  });
  it("appends alpha hex when opacity < 1", () => {
    // opacity 0.5 → Math.round(0.5 * 255) = 128 = 0x80
    const paints = [{ type: "SOLID", color: { r: 1, g: 0, b: 0 }, opacity: 0.5 }];
    const result = serializePaints(paints) as string[];
    expect(result[0]).toBe("#ff000080");
  });
  it("defaults opacity to 1 when not provided", () => {
    const paints = [{ type: "SOLID", color: { r: 0, g: 0, b: 1 } }];
    expect(serializePaints(paints)).toEqual(["#0000ff"]);
  });
  it("serializes multiple solid paints", () => {
    const paints = [
      { type: "SOLID", color: { r: 1, g: 0, b: 0 } },
      { type: "SOLID", color: { r: 0, g: 1, b: 0 } },
    ];
    expect(serializePaints(paints)).toEqual(["#ff0000", "#00ff00"]);
  });
});

// ── getBounds ─────────────────────────────────────────────────────────────────

describe("getBounds", () => {
  it("returns bounds for a node with x/y/width/height", () => {
    expect(getBounds({ x: 10, y: 20, width: 100, height: 50 })).toEqual({
      x: 10, y: 20, width: 100, height: 50,
    });
  });
  it("rounds floating point values to 2 decimal places", () => {
    const bounds = getBounds({ x: 10.999, y: 0, width: 99.999, height: 50 });
    expect(bounds?.x).toBe(11);
    expect(bounds?.width).toBe(100);
  });
  it("returns undefined when coordinates are missing", () => {
    expect(getBounds({ name: "page" })).toBeUndefined();
    expect(getBounds({ x: 0, y: 0 })).toBeUndefined();
  });
});

// ── serializeLineHeight ───────────────────────────────────────────────────────

describe("serializeLineHeight", () => {
  it("returns 'mixed' for symbol", () => {
    expect(serializeLineHeight(Symbol())).toBe("mixed");
  });
  it("returns undefined for AUTO unit", () => {
    expect(serializeLineHeight({ unit: "AUTO" })).toBeUndefined();
  });
  it("returns undefined for null/falsy", () => {
    expect(serializeLineHeight(null)).toBeUndefined();
    expect(serializeLineHeight(undefined)).toBeUndefined();
  });
  it("returns value and unit for PIXELS", () => {
    expect(serializeLineHeight({ value: 24, unit: "PIXELS" })).toEqual({ value: 24, unit: "PIXELS" });
  });
  it("returns value and unit for PERCENT", () => {
    expect(serializeLineHeight({ value: 150, unit: "PERCENT" })).toEqual({ value: 150, unit: "PERCENT" });
  });
});

// ── serializeLetterSpacing ────────────────────────────────────────────────────

describe("serializeLetterSpacing", () => {
  it("returns 'mixed' for symbol", () => {
    expect(serializeLetterSpacing(Symbol())).toBe("mixed");
  });
  it("returns undefined when value is 0", () => {
    expect(serializeLetterSpacing({ value: 0, unit: "PIXELS" })).toBeUndefined();
  });
  it("returns undefined for null/falsy", () => {
    expect(serializeLetterSpacing(null)).toBeUndefined();
  });
  it("returns value and unit for non-zero spacing", () => {
    expect(serializeLetterSpacing({ value: 1.5, unit: "PIXELS" })).toEqual({ value: 1.5, unit: "PIXELS" });
  });
});

// ── deduplicateStyles ─────────────────────────────────────────────────────────

describe("deduplicateStyles", () => {
  it("returns original tree and undefined globalVars when nothing is repeated", () => {
    const tree = {
      children: [
        { styles: { fills: ["#ff0000"] } },
        { styles: { fills: ["#00ff00"] } },
      ],
    };
    const { tree: result, globalVars } = deduplicateStyles(tree);
    expect(globalVars).toBeUndefined();
    expect(result).toBe(tree);
  });

  it("deduplicates fills that appear more than once", () => {
    const sharedFill = ["#ff0000"];
    const tree = {
      children: [
        { styles: { fills: sharedFill } },
        { styles: { fills: sharedFill } },
      ],
    };
    const { tree: result, globalVars } = deduplicateStyles(tree);
    expect(globalVars).toBeDefined();
    const refs = Object.keys(globalVars!.styles);
    expect(refs.length).toBe(1);
    // Both nodes should now reference the short key instead of the array
    const children = (result as any).children;
    expect(typeof children[0].styles.fills).toBe("string");
    expect(children[0].styles.fills).toBe(children[1].styles.fills);
  });

  it("deduplicates strokes that appear more than once", () => {
    const sharedStroke = ["#0000ff"];
    const tree = {
      children: [
        { styles: { strokes: sharedStroke } },
        { styles: { strokes: sharedStroke } },
      ],
    };
    const { globalVars } = deduplicateStyles(tree);
    expect(globalVars).toBeDefined();
  });

  it("preserves unique fills as-is", () => {
    const tree = {
      children: [
        { styles: { fills: ["#ff0000"] } },
        { styles: { fills: ["#00ff00"] } },
        { styles: { fills: ["#ff0000"] } },
        { styles: { fills: ["#00ff00"] } },
      ],
    };
    const { globalVars } = deduplicateStyles(tree);
    // Both colors appear twice so both should be deduped
    expect(Object.keys(globalVars!.styles).length).toBe(2);
  });

  it("handles empty tree without errors", () => {
    const { tree, globalVars } = deduplicateStyles({});
    expect(globalVars).toBeUndefined();
    expect(tree).toEqual({});
  });
});

// ── serializeVariableValue ────────────────────────────────────────────────────

describe("serializeVariableValue", () => {
  it("passes through primitives unchanged", () => {
    expect(serializeVariableValue(42)).toBe(42);
    expect(serializeVariableValue("hello")).toBe("hello");
    expect(serializeVariableValue(true)).toBe(true);
    expect(serializeVariableValue(null)).toBe(null);
  });

  it("serializes VARIABLE_ALIAS objects", () => {
    const val = { type: "VARIABLE_ALIAS", id: "abc123", extra: "ignored" };
    expect(serializeVariableValue(val)).toEqual({ type: "VARIABLE_ALIAS", id: "abc123" });
  });

  it("serializes color objects to COLOR type", () => {
    const val = { r: 1, g: 0, b: 0, a: 1 };
    expect(serializeVariableValue(val)).toEqual({ type: "COLOR", r: 1, g: 0, b: 0, a: 1 });
  });

  it("defaults alpha to 1 when missing from color", () => {
    const val = { r: 0, g: 1, b: 0 };
    expect(serializeVariableValue(val)).toEqual({ type: "COLOR", r: 0, g: 1, b: 0, a: 1 });
  });

  it("passes through unknown objects unchanged", () => {
    const val = { foo: "bar" };
    expect(serializeVariableValue(val)).toEqual({ foo: "bar" });
  });
});

// ── serializeStyles ───────────────────────────────────────────────────────────

describe("serializeStyles", () => {
  it("returns empty object for node with no relevant properties", async () => {
    const result = await serializeStyles({ id: "1", name: "box" });
    expect(result).toEqual({});
  });

  it("includes fills when fills is a solid paint array", async () => {
    const node = { fills: [{ type: "SOLID", color: { r: 1, g: 0, b: 0 } }] };
    const result = await serializeStyles(node);
    expect(result.fills).toEqual(["#ff0000"]);
  });

  it("includes fillStyle name when fillStyleId resolves to a style", async () => {
    mockGetStyleByIdAsync = async (id) => (id === "style-1" ? { name: "Red" } : null);
    const node = {
      fills: [{ type: "SOLID", color: { r: 1, g: 0, b: 0 } }],
      fillStyleId: "style-1",
    };
    const result = await serializeStyles(node);
    expect(result.fillStyle).toBe("Red");
    expect(result.fills).toEqual(["#ff0000"]);
  });

  it("skips fillStyle when fillStyleId resolves to null", async () => {
    const node = {
      fills: [{ type: "SOLID", color: { r: 1, g: 0, b: 0 } }],
      fillStyleId: "missing",
    };
    const result = await serializeStyles(node);
    expect(result.fillStyle).toBeUndefined();
    expect(result.fills).toEqual(["#ff0000"]);
  });

  it("skips fillStyle when fillStyleId is not a string", async () => {
    const node = {
      fills: [{ type: "SOLID", color: { r: 0, g: 0, b: 1 } }],
      fillStyleId: Symbol(),
    };
    const result = await serializeStyles(node);
    expect(result.fillStyle).toBeUndefined();
  });

  it("includes strokes and strokeStyle", async () => {
    mockGetStyleByIdAsync = async (id) => (id === "s-1" ? { name: "Border" } : null);
    const node = {
      strokes: [{ type: "SOLID", color: { r: 0, g: 0, b: 0 } }],
      strokeStyleId: "s-1",
    };
    const result = await serializeStyles(node);
    expect(result.strokeStyle).toBe("Border");
    expect(result.strokes).toEqual(["#000000"]);
  });

  it("omits cornerRadius when value is 0", async () => {
    const result = await serializeStyles({ cornerRadius: 0 });
    expect(result.cornerRadius).toBeUndefined();
  });

  it("includes cornerRadius when non-zero", async () => {
    const result = await serializeStyles({ cornerRadius: 8 });
    expect(result.cornerRadius).toBe(8);
  });

  it("sets cornerRadius to 'mixed' for symbol", async () => {
    const result = await serializeStyles({ cornerRadius: Symbol() });
    expect(result.cornerRadius).toBe("mixed");
  });

  it("includes padding when paddingLeft is present", async () => {
    const node = { paddingLeft: 10, paddingRight: 20, paddingTop: 5, paddingBottom: 15 };
    const result = await serializeStyles(node);
    expect(result.padding).toEqual({ top: 5, right: 20, bottom: 15, left: 10 });
  });
});

// ── serializeText ─────────────────────────────────────────────────────────────

describe("serializeText", () => {
  const makeBase = () => ({ id: "t1", name: "Text", type: "TEXT", bounds: undefined, styles: {} });

  it("handles mixed font name", async () => {
    const node = {
      fontName: Symbol(),
      fontSize: 16,
      fontWeight: 400,
      textDecoration: "NONE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      characters: "hello",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.fontFamily).toBe("mixed");
    expect(result.styles.fontStyle).toBe("mixed");
  });

  it("handles regular font name", async () => {
    const node = {
      fontName: { family: "Inter", style: "Regular" },
      fontSize: 14,
      fontWeight: 400,
      textDecoration: "NONE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      characters: "hello",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.fontFamily).toBe("Inter");
    expect(result.styles.fontStyle).toBe("Regular");
    expect(result.characters).toBe("hello");
  });

  it("includes vertical text alignment", async () => {
    const node = {
      fontName: { family: "Inter", style: "Regular" },
      fontSize: 14,
      fontWeight: 400,
      textDecoration: "NONE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "CENTER",
      textAlignVertical: "BOTTOM",
      characters: "aligned",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.textAlignHorizontal).toBe("CENTER");
    expect(result.styles.textAlignVertical).toBe("BOTTOM");
  });

  it("includes textStyle when textStyleId resolves", async () => {
    mockGetStyleByIdAsync = async (id) => (id === "ts-1" ? { name: "Heading 1" } : null);
    const node = {
      fontName: { family: "Inter", style: "Bold" },
      fontSize: 32,
      fontWeight: 700,
      textDecoration: "NONE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      textStyleId: "ts-1",
      characters: "Title",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.textStyle).toBe("Heading 1");
  });

  it("omits textStyle when textStyleId is not a string", async () => {
    const node = {
      fontName: { family: "Inter", style: "Regular" },
      fontSize: 14,
      fontWeight: 400,
      textDecoration: "NONE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      textStyleId: Symbol(),
      characters: "hi",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.textStyle).toBeUndefined();
  });

  it("serializes mixed text properties", async () => {
    const node = {
      fontName: { family: "Inter", style: "Regular" },
      fontSize: Symbol(),
      fontWeight: Symbol(),
      textDecoration: Symbol(),
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: Symbol(),
      textAlignVertical: Symbol(),
      characters: "mixed",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.fontSize).toBe("mixed");
    expect(result.styles.fontWeight).toBe("mixed");
    expect(result.styles.textDecoration).toBe("mixed");
    expect(result.styles.textAlignHorizontal).toBe("mixed");
    expect(result.styles.textAlignVertical).toBe("mixed");
  });

  it("omits textDecoration when value is NONE", async () => {
    const node = {
      fontName: { family: "Inter", style: "Regular" },
      fontSize: 14,
      fontWeight: 400,
      textDecoration: "NONE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      characters: "plain",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.textDecoration).toBeUndefined();
  });

  it("includes textDecoration when not NONE", async () => {
    const node = {
      fontName: { family: "Inter", style: "Regular" },
      fontSize: 14,
      fontWeight: 400,
      textDecoration: "UNDERLINE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      characters: "underlined",
    };
    const result = await serializeText(node, makeBase());
    expect(result.styles.textDecoration).toBe("UNDERLINE");
  });
});

// ── serializeNode ─────────────────────────────────────────────────────────────

describe("serializeNode", () => {
  it("serializes a plain node with bounds", async () => {
    const node = { id: "1:1", name: "Box", type: "RECTANGLE", x: 0, y: 0, width: 100, height: 50 };
    const result = await serializeNode(node);
    expect(result.id).toBe("1:1");
    expect(result.type).toBe("RECTANGLE");
    expect(result.bounds).toEqual({ x: 0, y: 0, width: 100, height: 50 });
  });

  it("serializes a TEXT node", async () => {
    const node = {
      id: "1:2",
      name: "Label",
      type: "TEXT",
      x: 0, y: 0, width: 50, height: 20,
      fontName: { family: "Inter", style: "Regular" },
      fontSize: 14,
      fontWeight: 400,
      textDecoration: "NONE",
      lineHeight: { unit: "AUTO" },
      letterSpacing: { value: 0, unit: "PIXELS" },
      textAlignHorizontal: "LEFT",
      characters: "Hello",
    };
    const result = await serializeNode(node);
    expect(result.type).toBe("TEXT");
    expect(result.characters).toBe("Hello");
  });

  it("recursively serializes children", async () => {
    const node = {
      id: "1:3",
      name: "Frame",
      type: "FRAME",
      x: 0, y: 0, width: 200, height: 200,
      children: [
        { id: "1:4", name: "Child", type: "RECTANGLE", x: 10, y: 10, width: 50, height: 50 },
      ],
    };
    const result = await serializeNode(node);
    expect(result.children).toHaveLength(1);
    expect(result.children[0].id).toBe("1:4");
  });
});

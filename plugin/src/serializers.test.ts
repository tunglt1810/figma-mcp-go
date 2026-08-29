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
  serializeStyledSegments,
  serializeComponentPropertyDefinitions,
  serializeNode,
  makeBudget,
  serializeEffects,
  serializeNodeProperties,
  invertTransform,
} from "./serializers";
import { makeGradientPaint, makeEffect } from "./write-helpers";

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
  it("drops paints whose eye is toggled off", () => {
    const paints = [
      { type: "SOLID", color: { r: 1, g: 0, b: 0 }, visible: false },
      { type: "SOLID", color: { r: 0, g: 1, b: 0 } },
    ];
    expect(serializePaints(paints)).toEqual(["#00ff00"]);
  });
  it("returns undefined when every paint is toggled off", () => {
    const paints = [{ type: "SOLID", color: { r: 1, g: 0, b: 0 }, visible: false }];
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
  it("strips 32-bit float noise from stop positions", () => {
    // Figma reports an exact 40% stop as 0.4000000059604645
    const paints = [{
      type: "GRADIENT_RADIAL",
      gradientTransform: [[1, 0, 0], [0, 1, 0]],
      gradientStops: [
        { position: 0.4000000059604645, color: { r: 0.973, g: 0.784, b: 0.863, a: 1 } },
        { position: 1, color: { r: 1, g: 0.953, b: 0.953, a: 1 } }
      ]
    }];
    const result = serializePaints(paints) as any[];
    expect(result[0].stops[0].position).toBe(0.4);
    expect(result[0].stops[1].position).toBe(1);
  });
  it("reports paint-level opacity on a gradient when it is not 1", () => {
    const paints = [{
      type: "GRADIENT_RADIAL",
      opacity: 0.5,
      gradientTransform: [[1, 0, 0], [0, 1, 0]],
      gradientStops: [{ position: 0, color: { r: 1, g: 0, b: 0, a: 1 } }]
    }];
    const result = serializePaints(paints) as any[];
    expect(result[0].opacity).toBe(0.5);
  });
  it("folds paint opacity into cssString but leaves stops[] raw", () => {
    const paints = [{
      type: "GRADIENT_LINEAR",
      opacity: 0.5,
      gradientTransform: [[1, 0, 0], [0, 1, 0]],
      gradientStops: [
        { position: 0, color: { r: 1, g: 0, b: 0, a: 1 } },
        { position: 1, color: { r: 0, g: 0, b: 1, a: 0.5 } },
      ]
    }];
    const result = serializePaints(paints) as any[];
    // stops[] must stay writable back through set_paint, which takes opacity separately.
    expect(result[0].stops[0].color).toBe("#ff0000");
    expect(result[0].stops[1].color).toBe("#0000ff80");
    // cssString renders, so it carries the opacity: 1*0.5 = 0x80, 0.5*0.5 = 0x40.
    expect(result[0].cssString).toBe("linear-gradient(90deg, #ff000080 0%, #0000ff40 100%)");
  });
  it("leaves cssString unchanged for a fully opaque gradient", () => {
    const paints = [{
      type: "GRADIENT_LINEAR",
      gradientTransform: [[1, 0, 0], [0, 1, 0]],
      gradientStops: [{ position: 0, color: { r: 1, g: 0, b: 0, a: 1 } }]
    }];
    const result = serializePaints(paints) as any[];
    expect(result[0].cssString).toBe("linear-gradient(90deg, #ff0000 0%)");
  });
  it("omits gradient opacity when the fill is fully opaque", () => {
    const paints = [{
      type: "GRADIENT_LINEAR",
      opacity: 1,
      gradientTransform: [[1, 0, 0], [0, 1, 0]],
      gradientStops: [{ position: 0, color: { r: 1, g: 0, b: 0, a: 1 } }]
    }];
    const result = serializePaints(paints) as any[];
    expect(result[0]).not.toHaveProperty("opacity");
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

  // cornerRadius now lives only under geometry, which already owns the per-corner
  // variants. Reporting it in both places was duplication.
  it("does not report cornerRadius, which belongs to geometry", async () => {
    const result = await serializeStyles({ cornerRadius: 8 });
    expect(result.cornerRadius).toBeUndefined();
  });

  it("reports stroke weight and alignment alongside the stroke colour", async () => {
    const node = {
      strokes: [{ type: "SOLID", color: { r: 0, g: 0, b: 0 } }],
      strokeWeight: 2,
      strokeAlign: "INSIDE",
      dashPattern: [4, 2],
    };
    const result = await serializeStyles(node);
    expect(result.strokeWeight).toBe(2);
    expect(result.strokeAlign).toBe("INSIDE");
    expect(result.dashPattern).toEqual([4, 2]);
  });

  it("omits stroke weight when there is no stroke to draw", async () => {
    const result = await serializeStyles({ strokes: [], strokeWeight: 2, strokeAlign: "INSIDE" });
    expect(result.strokeWeight).toBeUndefined();
    expect(result.strokeAlign).toBeUndefined();
  });

  it("includes padding when paddingLeft is present", async () => {
    const node = { paddingLeft: 10, paddingRight: 20, paddingTop: 5, paddingBottom: 15 };
    const result = await serializeStyles(node);
    expect(result.padding).toEqual({ top: 5, right: 20, bottom: 15, left: 10 });
  });

  it("omits padding when every side is zero", async () => {
    const node = { paddingLeft: 0, paddingRight: 0, paddingTop: 0, paddingBottom: 0 };
    const result = await serializeStyles(node);
    expect(result.padding).toBeUndefined();
  });

  it("includes effects and effectStyle", async () => {
    mockGetStyleByIdAsync = async (id) => (id === "e-1" ? { name: "Card shadow" } : null);
    const node = {
      effects: [{ type: "DROP_SHADOW", color: { r: 0, g: 0, b: 0, a: 0.25 }, radius: 8, offset: { x: 0, y: 4 }, spread: 0 }],
      effectStyleId: "e-1",
    };
    const result = await serializeStyles(node);
    expect(result.effectStyle).toBe("Card shadow");
    expect(result.effects).toEqual([
      { type: "DROP_SHADOW", radius: 8, color: "#000000", opacity: 0.25, offsetX: 0, offsetY: 4 },
    ]);
  });

  it("omits effects when the node has none", async () => {
    const result = await serializeStyles({ effects: [] });
    expect(result.effects).toBeUndefined();
  });

  it("reports auto-layout settings", async () => {
    const node = {
      layoutMode: "HORIZONTAL",
      itemSpacing: 12,
      primaryAxisAlignItems: "CENTER",
      counterAxisAlignItems: "MIN",
      primaryAxisSizingMode: "AUTO",
      counterAxisSizingMode: "FIXED",
    };
    const result = await serializeStyles(node);
    expect(result.layout).toEqual({
      mode: "HORIZONTAL",
      itemSpacing: 12,
      primaryAxisAlignItems: "CENTER",
      primaryAxisSizingMode: "AUTO",
      counterAxisSizingMode: "FIXED",
    });
  });

  it("omits layout for a frame that does not use auto-layout", async () => {
    const result = await serializeStyles({ layoutMode: "NONE", itemSpacing: 0 });
    expect(result.layout).toBeUndefined();
  });
});

// ── serializeEffects ──────────────────────────────────────────────────────────

describe("serializeEffects", () => {
  it("returns 'mixed' for symbol input", () => {
    expect(serializeEffects(Symbol())).toBe("mixed");
  });

  it("returns undefined for a node with no effects", () => {
    expect(serializeEffects([])).toBeUndefined();
    expect(serializeEffects(undefined)).toBeUndefined();
  });

  it("reports a blur as type and radius only", () => {
    expect(serializeEffects([{ type: "LAYER_BLUR", radius: 10 }])).toEqual([
      { type: "LAYER_BLUR", radius: 10 },
    ]);
  });

  // This file's "Cosun Glass" style is a GLASS effect. Listing only the parameters of
  // shadows and blurs would report it as a bare radius and lose the whole effect.
  it("keeps the parameters of effect types beyond shadows and blurs", () => {
    const glass = {
      type: "GLASS", radius: 4, depth: 20, dispersion: 0.5, lightAngle: -45,
      lightIntensity: 0.800000011920929, refraction: 0.800000011920929, splay: 0,
      visible: true, boundVariables: {},
    };
    // splay is 0, the identity for that parameter, so it is omitted like any default.
    expect(serializeEffects([glass])).toEqual([{
      type: "GLASS", radius: 4, depth: 20, dispersion: 0.5, lightAngle: -45,
      lightIntensity: 0.8, refraction: 0.8,
    }]);
  });

  it("keeps a zero radius, which is a hard-edged shadow rather than no shadow", () => {
    expect(serializeEffects([{ type: "DROP_SHADOW", radius: 0, spread: 0 }])).toEqual([
      { type: "DROP_SHADOW", radius: 0 },
    ]);
  });

  // NoiseEffectMultitone has an effect-level opacity of its own. Reporting it as
  // `opacity` would overwrite the alpha lifted out of `color`.
  it("keeps multitone noise opacity apart from the colour's alpha", () => {
    const noise = {
      type: "NOISE", noiseType: "MULTITONE", radius: 0, density: 0.5, noiseSize: 2,
      color: { r: 0, g: 0, b: 0, a: 0.25 }, opacity: 0.8, blendMode: "NORMAL",
    };
    const result = serializeEffects([noise]) as any[];
    expect(result[0].opacity).toBe(0.25);
    expect(result[0].noiseOpacity).toBe(0.8);
  });

  it("converts the second tone of a duotone noise effect to hex", () => {
    const noise = {
      type: "NOISE", noiseType: "DUOTONE", noiseSize: 2, density: 0.5,
      color: { r: 0, g: 0, b: 0, a: 1 }, secondaryColor: { r: 0, g: 1, b: 0, a: 1 },
    };
    const result = serializeEffects([noise]) as any[];
    expect(result[0].secondaryColor).toBe("#00ff00");
  });

  // Vector-valued fields are objects, so a scalar-only pass-through would drop them.
  it("keeps the start and end points of a progressive blur", () => {
    const blur = {
      type: "LAYER_BLUR", blurType: "PROGRESSIVE", radius: 12, startRadius: 2,
      startOffset: { x: 0, y: 0.2 }, endOffset: { x: 0, y: 0.9 },
    };
    expect(serializeEffects([blur])).toEqual([{
      type: "LAYER_BLUR", blurType: "PROGRESSIVE", radius: 12, startRadius: 2,
      startOffset: { x: 0, y: 0.2 }, endOffset: { x: 0, y: 0.9 },
    }]);
  });

  it("omits blurType NORMAL, which is what an untagged blur means", () => {
    expect(serializeEffects([{ type: "LAYER_BLUR", blurType: "NORMAL", radius: 4 }])).toEqual([
      { type: "LAYER_BLUR", radius: 4 },
    ]);
  });

  it("round-trips every writable effect type through makeEffect", () => {
    const figmaEffects = [
      { type: "DROP_SHADOW", color: { r: 0, g: 0, b: 0, a: 0.3 }, offset: { x: 2, y: 4 },
        radius: 8, spread: 1, visible: true, blendMode: "MULTIPLY", showShadowBehindNode: true },
      { type: "LAYER_BLUR", blurType: "PROGRESSIVE", radius: 12, startRadius: 2,
        startOffset: { x: 0, y: 0.2 }, endOffset: { x: 0, y: 0.9 }, visible: true },
      { type: "GLASS", radius: 4, depth: 20, dispersion: 0.5, lightAngle: -45,
        lightIntensity: 0.8, refraction: 0.8, visible: true },
      { type: "NOISE", noiseType: "MULTITONE", color: { r: 1, g: 0, b: 0, a: 0.25 },
        opacity: 0.8, blendMode: "NORMAL", noiseSize: 3, density: 0.7, visible: true },
      { type: "TEXTURE", noiseSize: 5, radius: 3, clipToShape: true, visible: true },
    ];

    const first = serializeEffects(figmaEffects) as any[];
    // Feeding a read result back through the write path and reading it again must
    // land on the same thing, or effects cannot be copied between nodes.
    const second = serializeEffects(first.map((e: any) => makeEffect(e)));
    expect(second).toEqual(first);
  });

  it("drops plugin bookkeeping and a default blend mode", () => {
    const effect = {
      type: "DROP_SHADOW", radius: 4, blendMode: "NORMAL",
      visible: true, boundVariables: { radius: "v1" },
    };
    expect(serializeEffects([effect])).toEqual([{ type: "DROP_SHADOW", radius: 4 }]);
  });

  it("keeps a blend mode that is not the default", () => {
    const effect = { type: "DROP_SHADOW", radius: 4, blendMode: "MULTIPLY" };
    expect(serializeEffects([effect])).toEqual([
      { type: "DROP_SHADOW", radius: 4, blendMode: "MULTIPLY" },
    ]);
  });

  it("drops effects that are toggled off", () => {
    const effects = [
      { type: "DROP_SHADOW", radius: 4, visible: false },
      { type: "LAYER_BLUR", radius: 2 },
    ];
    expect(serializeEffects(effects)).toEqual([{ type: "LAYER_BLUR", radius: 2 }]);
  });

  it("round-trips into the shape set_effects accepts", () => {
    const serialized = serializeEffects([
      { type: "INNER_SHADOW", color: { r: 1, g: 0, b: 0, a: 1 }, radius: 6, offset: { x: 2, y: 3 }, spread: 1 },
    ]) as any[];
    // These are exactly the keys set_effects reads off its params.
    expect(serialized[0]).toEqual({
      type: "INNER_SHADOW", color: "#ff0000", radius: 6, offsetX: 2, offsetY: 3, spread: 1,
    });
  });
});

// ── serializeNodeProperties ───────────────────────────────────────────────────

describe("serializeNodeProperties", () => {
  it("reports nothing for a node at every default", () => {
    const node = { opacity: 1, visible: true, locked: false, blendMode: "PASS_THROUGH",
      constraints: { horizontal: "MIN", vertical: "MIN" } };
    expect(serializeNodeProperties(node)).toEqual({});
  });

  it("reports opacity, visibility, lock and blend mode when they differ", () => {
    const node = { opacity: 0.5, visible: false, locked: true, blendMode: "MULTIPLY" };
    expect(serializeNodeProperties(node)).toEqual({
      opacity: 0.5, visible: false, locked: true, blendMode: "MULTIPLY",
    });
  });

  it("strips float noise from opacity", () => {
    expect(serializeNodeProperties({ opacity: 0.4000000059604645 }).opacity).toBe(0.4);
  });

  it("treats NORMAL blend mode as the default for leaf nodes", () => {
    expect(serializeNodeProperties({ blendMode: "NORMAL" })).toEqual({});
  });

  it("reports constraints only when they are not top-left", () => {
    expect(serializeNodeProperties({ constraints: { horizontal: "STRETCH", vertical: "MIN" } })).toEqual({
      constraints: { horizontal: "STRETCH", vertical: "MIN" },
    });
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

  describe("geometry", () => {
    it("extracts rotation", async () => {
      const node = { id: "1", type: "RECTANGLE", rotation: 45 };
      const result = await serializeNode(node);
      expect(result.geometry.rotation).toBe(45);
    });

    it("extracts cornerRadius", async () => {
      const node = { id: "2", type: "RECTANGLE", cornerRadius: 8 };
      const result = await serializeNode(node);
      expect(result.geometry.cornerRadius).toBe(8);
    });

    it("extracts mixed cornerRadius", async () => {
      const originalFigma = (globalThis as any).figma;
      const mixed = Symbol();
      (globalThis as any).figma = { mixed, getStyleByIdAsync: mockGetStyleByIdAsync };
      try {
        const node = { id: "3", type: "RECTANGLE", cornerRadius: mixed };
        const result = await serializeNode(node);
        expect(result.geometry.cornerRadius).toBe("mixed");
      } finally {
        (globalThis as any).figma = originalFigma;
      }
    });

    it("extracts STAR geometry", async () => {
      const node = { id: "4", type: "STAR", width: 100, pointCount: 5, innerRadius: 0.5 };
      const result = await serializeNode(node);
      expect(result.geometry.pointCount).toBe(5);
      expect(result.geometry.innerRadiusRatio).toBe(0.5);
      expect(result.geometry.outerRadiusPixel).toBe(50);
      expect(result.geometry.innerRadiusPixel).toBe(25);
    });

    it("extracts POLYGON geometry", async () => {
      const node = { id: "5", type: "POLYGON", pointCount: 6 };
      const result = await serializeNode(node);
      expect(result.geometry.pointCount).toBe(6);
    });

    it("extracts ELLIPSE geometry", async () => {
      const arcData = { startingAngle: 0, endingAngle: Math.PI, innerRadius: 0 };
      const node = { id: "6", type: "ELLIPSE", arcData };
      const result = await serializeNode(node);
      expect(result.geometry.arcData).toEqual(arcData);
    });

    it("extracts corner radii for RECTANGLE", async () => {
      const node = { id: "7", type: "RECTANGLE", topLeftRadius: 1, topRightRadius: 2, bottomLeftRadius: 3, bottomRightRadius: 4 };
      const result = await serializeNode(node);
      expect(result.geometry.topLeftRadius).toBe(1);
      expect(result.geometry.topRightRadius).toBe(2);
      expect(result.geometry.bottomLeftRadius).toBe(3);
      expect(result.geometry.bottomRightRadius).toBe(4);
    });

    it("returns undefined geometry if no properties match", async () => {
      const node = { id: "8", type: "FRAME" };
      const result = await serializeNode(node);
      expect(result.geometry).toBeUndefined();
    });

    it("collapses uniform corner radii to cornerRadius alone", async () => {
      const node = { id: "9", type: "RECTANGLE", cornerRadius: 16,
        topLeftRadius: 16, topRightRadius: 16, bottomLeftRadius: 16, bottomRightRadius: 16 };
      const result = await serializeNode(node);
      expect(result.geometry).toEqual({ cornerRadius: 16 });
    });

    it("omits rotation and radii entirely for an unrotated square-cornered frame", async () => {
      const node = { id: "10", type: "FRAME", rotation: 0, cornerRadius: 0,
        topLeftRadius: 0, topRightRadius: 0, bottomLeftRadius: 0, bottomRightRadius: 0 };
      const result = await serializeNode(node);
      expect(result.geometry).toBeUndefined();
    });
  });

  describe("node properties", () => {
    it("reports opacity and visibility on the node itself", async () => {
      const node = { id: "1", type: "RECTANGLE", opacity: 0.5, visible: false };
      const result = await serializeNode(node);
      expect(result.opacity).toBe(0.5);
      expect(result.visible).toBe(false);
    });

    it("stays silent for a node at every default", async () => {
      const node = { id: "1", type: "RECTANGLE", opacity: 1, visible: true, locked: false };
      const result = await serializeNode(node);
      expect(result).not.toHaveProperty("opacity");
      expect(result).not.toHaveProperty("visible");
      expect(result).not.toHaveProperty("locked");
    });

    it("omits styles entirely when there is nothing to report", async () => {
      const node = { id: "1", name: "Group", type: "GROUP" };
      const result = await serializeNode(node);
      expect(result).not.toHaveProperty("styles");
    });

    it("omits children for an empty container", async () => {
      const node = { id: "1", name: "Empty frame", type: "FRAME", children: [] };
      const result = await serializeNode(node);
      expect(result).not.toHaveProperty("children");
    });
  });
});

// ── serializeStyledSegments ───────────────────────────────────────────────────

describe("serializeStyledSegments", () => {
  const nodeWith = (segments: any) => ({
    getStyledTextSegments: () => segments,
  });

  it("returns nothing for a node with one uniform style", () => {
    // The node-level style fields already say everything a single segment could.
    expect(serializeStyledSegments(nodeWith([
      { start: 0, end: 5, characters: "Hello", fontName: { family: "Inter", style: "Regular" } },
    ]))).toBeUndefined();
  });

  it("returns nothing when the API is absent", () => {
    expect(serializeStyledSegments({})).toBeUndefined();
  });

  it("reports the font of each segment", () => {
    const segments = serializeStyledSegments(nodeWith([
      { start: 0, end: 6, characters: "Hello ", fontName: { family: "Inter", style: "Regular" }, fontSize: 14 },
      { start: 6, end: 11, characters: "world", fontName: { family: "Inter", style: "Bold" }, fontSize: 14 },
    ]))!;
    expect(segments.length).toBe(2);
    expect(segments[1]).toMatchObject({
      start: 6,
      end: 11,
      characters: "world",
      fontFamily: "Inter",
      fontStyle: "Bold",
      fontSize: 14,
    });
  });

  it("carries a hyperlink through as its URL", () => {
    const segments = serializeStyledSegments(nodeWith([
      { start: 0, end: 4, characters: "docs", hyperlink: { type: "URL", value: "https://x.dev" } },
      { start: 4, end: 8, characters: " here" },
    ]))!;
    expect(segments[0].hyperlink).toBe("https://x.dev");
  });

  it("omits the defaults that mean 'nothing special'", () => {
    const segments = serializeStyledSegments(nodeWith([
      { start: 0, end: 2, characters: "ab", textDecoration: "NONE", textCase: "ORIGINAL", listOptions: { type: "NONE" }, indentation: 0 },
      { start: 2, end: 4, characters: "cd", textDecoration: "UNDERLINE", listOptions: { type: "UNORDERED" } },
    ]))!;
    expect(segments[0].textDecoration).toBeUndefined();
    expect(segments[0].textCase).toBeUndefined();
    expect(segments[0].listType).toBeUndefined();
    expect(segments[0].indentation).toBeUndefined();
    expect(segments[1].textDecoration).toBe("UNDERLINE");
    expect(segments[1].listType).toBe("UNORDERED");
  });
});

// ── serializeComponentPropertyDefinitions ─────────────────────────────────────

describe("serializeComponentPropertyDefinitions", () => {
  it("ignores a node that is not a component", () => {
    expect(serializeComponentPropertyDefinitions({
      type: "FRAME",
      componentPropertyDefinitions: { "X#1:0": { type: "BOOLEAN", defaultValue: true } },
    })).toBeUndefined();
  });

  it("returns nothing for a component that exposes nothing", () => {
    expect(serializeComponentPropertyDefinitions({ type: "COMPONENT", componentPropertyDefinitions: {} }))
      .toBeUndefined();
    expect(serializeComponentPropertyDefinitions({ type: "COMPONENT" })).toBeUndefined();
  });

  // The suffixed key is what every write has to quote back; the bare name is
  // what a reader recognises.
  it("keeps the full id as the key and the bare name alongside", () => {
    const defs = serializeComponentPropertyDefinitions({
      type: "COMPONENT_SET",
      componentPropertyDefinitions: {
        "Size#1:0": { type: "VARIANT", defaultValue: "Large", variantOptions: ["Small", "Large"] },
        "Disabled#2:0": { type: "BOOLEAN", defaultValue: false },
      },
    })!;
    expect(defs["Size#1:0"]).toEqual({
      name: "Size",
      type: "VARIANT",
      defaultValue: "Large",
      variantOptions: ["Small", "Large"],
    });
    expect(defs["Disabled#2:0"].name).toBe("Disabled");
  });
});

// ── serialization budget ──────────────────────────────────────────────────────

describe("serializeNode budget", () => {
  // A tree of `breadth` children, each with `breadth` children of its own.
  const makeTree = (breadth: number, levels: number, prefix = "1"): any => {
    const node: any = {
      id: `${prefix}:0`,
      name: `Node ${prefix}`,
      type: "FRAME",
      x: 0, y: 0, width: 10, height: 10,
      absoluteBoundingBox: { x: 0, y: 0, width: 10, height: 10 },
      children: [],
    };
    if (levels > 0) {
      for (let i = 0; i < breadth; i++) {
        node.children.push(makeTree(breadth, levels - 1, `${prefix}${i}`));
      }
    }
    return node;
  };

  const countNodes = (tree: any): number =>
    1 + (tree.children ?? []).reduce((sum: number, child: any) => sum + countNodes(child), 0);

  beforeEach(() => {
    (globalThis as any).figma = {
      getStyleByIdAsync: async () => null,
      mixed: Symbol("mixed"),
    };
  });

  it("walks the whole tree when no budget is given", async () => {
    const tree = await serializeNode(makeTree(2, 2));
    expect(countNodes(tree)).toBe(7);
  });

  it("stops at the depth limit and says what it withheld", async () => {
    const budget = makeBudget(undefined, 1);
    const tree = await serializeNode(makeTree(2, 2), budget);
    expect(budget.truncated).toBe(true);
    expect(tree.children.length).toBe(2);
    // The depth-1 nodes are reported; their own children are not.
    expect(tree.children[0].children).toBeUndefined();
    expect(tree.children[0].childCount).toBe(2);
    expect(tree.children[0].childrenOmitted).toBe(true);
  });

  it("depth 0 reports the root alone", async () => {
    const budget = makeBudget(undefined, 0);
    const tree = await serializeNode(makeTree(2, 2), budget);
    expect(tree.children).toBeUndefined();
    expect(tree.childCount).toBe(2);
  });

  it("stops at the node limit", async () => {
    const budget = makeBudget(3);
    const tree = await serializeNode(makeTree(2, 2), budget);
    expect(budget.truncated).toBe(true);
    // The root plus the three children the budget paid for.
    expect(countNodes(tree)).toBe(4);
  });

  // Spending the budget in tree order is what makes a truncated answer
  // reproducible rather than a race between promises.
  it("spends the budget in tree order", async () => {
    const budget = makeBudget(2);
    const tree = await serializeNode(makeTree(2, 2), budget);
    expect(tree.children[0].id).toBe("10:0");
    expect(tree.children[0].children[0].id).toBe("100:0");
  });

  it("leaves an untruncated tree unmarked", async () => {
    const budget = makeBudget(100, 10);
    const tree = await serializeNode(makeTree(2, 2), budget);
    expect(budget.truncated).toBe(false);
    expect(tree.childrenOmitted).toBeUndefined();
  });

  it("treats a zero or negative limit as no limit", async () => {
    expect(makeBudget(0).remaining).toBe(Infinity);
    expect(makeBudget(-5).remaining).toBe(Infinity);
    expect(makeBudget(undefined, -1).maxDepth).toBe(Infinity);
  });
});

// Serializers — shared read/write helpers for converting Figma node data to JSON.

export const isMixed = (value: any) => typeof value === "symbol";

// Round floating-point pixel values to 2 decimal places.
// Figma sometimes returns values like 123.99999999999999 instead of 124.
const pixelRound = (v: number) => Math.round(v * 100) / 100;

// Round a 0–1 ratio (gradient stop position, paint opacity) to 4 decimals.
// Figma returns 32-bit floats, so an exact 40% arrives as 0.4000000059604645.
const ratioRound = (v: number) => Math.round(v * 10000) / 10000;

// Append a two-digit alpha to a #rrggbb hex, leaving fully opaque colours bare.
const withAlpha = (hex: string, alpha: number) =>
  alpha >= 1
    ? hex
    : hex + Math.round(Math.max(0, alpha) * 255).toString(16).padStart(2, "0");

export const toHex = (color: any) => {
  const clamp = (v: any) => Math.min(255, Math.max(0, Math.round(v * 255)));
  const [r, g, b] = [clamp(color.r), clamp(color.g), clamp(color.b)];
  return `#${[r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("")}`;
};

export function invertTransform(t: number[][]): number[][] {
  const [[a, b, c], [d, e, f]] = t;
  const det = a * e - b * d;
  if (det === 0) return [[1, 0, 0], [0, 1, 0]];
  return [
    [e / det, -b / det, (b * f - c * e) / det],
    [-d / det, a / det, (c * d - a * f) / det],
  ];
}

export const serializePaints = (paints: any, node?: any) => {
  if (isMixed(paints)) return "mixed";

  if (!paints || !Array.isArray(paints)) return undefined;

  const result = paints
    // A paint with the eye toggled off contributes nothing to the render, so drop it
    // rather than reporting a colour the node does not actually show.
    .filter((paint: any) => paint.visible !== false)
    .filter((paint: any) => {
      return (paint.type === "SOLID" && "color" in paint) || 
             paint.type === "GRADIENT_LINEAR" || 
             paint.type === "GRADIENT_RADIAL";
    })
    .map((paint: any) => {
      const paintOpacity = paint.opacity != null ? paint.opacity : 1;

      if (paint.type === "SOLID") {
        return withAlpha(toHex(paint.color), paintOpacity);
      }

      // GRADIENT
      const inv = invertTransform(paint.gradientTransform);
      // Keep position as a 0–1 float for precision, but strip float noise:
      // Figma reports a 40% stop as 0.4000000059604645. cssString rounds to integer %.
      const rawStops = paint.gradientStops.map((stop: any) => ({
        position: ratioRound(stop.position),
        hex: toHex(stop.color),
        alpha: stop.color.a != null ? stop.color.a : 1,
      }));
      // stops[] stays raw — it is what set_paint accepts back, so folding the
      // paint-level opacity in here would corrupt a read-then-write round trip.
      const stops = rawStops.map((s: any) => ({ position: s.position, color: withAlpha(s.hex, s.alpha) }));
      // Paint-level opacity is what Figma shows next to the fill type ("Radial 100 %").
      // A gradient has no single colour to carry it, so it gets its own field.
      const gradientOpacity = paintOpacity !== 1 ? { opacity: ratioRound(paintOpacity) } : {};
      // cssString is the ready-to-render form, so it does fold paint opacity into each
      // stop's alpha. Without this a half-transparent gradient would render solid.
      const stopStrings = rawStops
        .map((s: any) => `${withAlpha(s.hex, s.alpha * paintOpacity)} ${Math.round(s.position * 100)}%`)
        .join(", ");

      if (paint.type === "GRADIENT_RADIAL") {
        // Center: mapped from (0.5, 0.5)
        const cx = inv[0][0] * 0.5 + inv[0][1] * 0.5 + inv[0][2];
        const cy = inv[1][0] * 0.5 + inv[1][1] * 0.5 + inv[1][2];
        
        // 2x2 transformation matrix M
        const ma = inv[0][0];
        const mb = inv[0][1];
        const mc = inv[1][0];
        const md = inv[1][1];

        // M M^T for SVD to find principal axes
        const A = ma*ma + mb*mb;
        const B = ma*mc + mb*md;
        const C = mc*mc + md*md;

        // Angle of the principal axis
        let theta = 0.5 * Math.atan2(2 * B, A - C);

        // Eigenvalues of M M^T
        const E = (A + C) / 2;
        const F = Math.sqrt( Math.pow(A - C, 2) / 4 + B*B );
        
        // Singular values (true radii of the ellipse when mapping a radius 0.5 circle)
        // Since gradient circle has radius 0.5 in gradient space, we multiply by 0.5
        const rx = 0.5 * Math.sqrt(E + F);
        const ry = 0.5 * Math.sqrt(E - F);

        const rotation = theta * 180 / Math.PI;

        const rxPercent = Math.round(rx * 100);
        const ryPercent = Math.round(ry * 100);
        const cxPercent = Math.round(cx * 100);
        const cyPercent = Math.round(cy * 100);
        
        // CSS radial-gradient: rx% is relative to element width, ry% to element height.
        // Omit shape keyword — browser defaults to ellipse which accepts % values.
        const cssString = `radial-gradient(${rxPercent}% ${ryPercent}% at ${cxPercent}% ${cyPercent}%, ${stopStrings})`;

        return {
          type: "GRADIENT_RADIAL",
          ...gradientOpacity,
          stops,
          geometry: {
            center: { percentX: cxPercent, percentY: cyPercent },
            radius: { percentX: rxPercent, percentY: ryPercent },
            rotation: Math.round(rotation)
          },
          cssString
        };
      }

      if (paint.type === "GRADIENT_LINEAR") {
        // Start: 0, 0.5. End: 1, 0.5
        const sx = inv[0][0] * 0.0 + inv[0][1] * 0.5 + inv[0][2];
        const sy = inv[1][0] * 0.0 + inv[1][1] * 0.5 + inv[1][2];
        const ex = inv[0][0] * 1.0 + inv[0][1] * 0.5 + inv[0][2];
        const ey = inv[1][0] * 1.0 + inv[1][1] * 0.5 + inv[1][2];

        const angle = Math.atan2(ey - sy, ex - sx) * 180 / Math.PI;

        let cssAngle = Math.round(angle + 90);
        if (cssAngle < 0) cssAngle += 360;
        cssAngle = cssAngle % 360;
        const cssString = `linear-gradient(${cssAngle}deg, ${stopStrings})`;

        return {
          type: "GRADIENT_LINEAR",
          ...gradientOpacity,
          stops,
          geometry: {
            start: { percentX: Math.round(sx * 100), percentY: Math.round(sy * 100) },
            end: { percentX: Math.round(ex * 100), percentY: Math.round(ey * 100) },
            angle: Math.round(angle)
          },
          cssString
        };
      }
    });

  return result.length > 0 ? result : undefined;
};

export const getBounds = (node: any) => {
  if ("x" in node && "y" in node && "width" in node && "height" in node) {
    return {
      x: pixelRound(node.x),
      y: pixelRound(node.y),
      width: pixelRound(node.width),
      height: pixelRound(node.height),
    };
  }

  return undefined;
};

// Keys serializeEffects renames or handles itself, plus plugin-internal bookkeeping
// that carries no design meaning.
//
// `opacity` is in here because NoiseEffectMultitone has an effect-level opacity of its
// own. Letting the generic pass-through copy it would clobber the alpha we lift out of
// `color`, so it is surfaced as noiseOpacity instead.
const effectKeysHandledElsewhere = new Set([
  "type", "color", "secondaryColor", "offset", "opacity", "visible", "boundVariables",
]);

// Values Figma reports at these keys are already the type's default, so reporting them
// is noise. A missing blurType means NORMAL, which is what serializeEffects omits here.
const effectDefaults: Record<string, unknown> = { blendMode: "NORMAL", blurType: "NORMAL" };

const isVector = (v: any) => v && typeof v.x === "number" && typeof v.y === "number";

// Serialize node.effects into the shape set_effects accepts, so a read result can be
// fed straight back into a write without translation.
//
// Colour and offset are normalised into hex and offsetX/offsetY. Everything else is
// passed through: Figma keeps adding effect types (GLASS, NOISE, TEXTURE, SHADER) with
// parameters of their own, and listing only the ones we recognise would drop them.
export const serializeEffects = (effects: any) => {
  if (isMixed(effects)) return "mixed";
  if (!effects || !Array.isArray(effects)) return undefined;

  const result = effects
    .filter((effect: any) => effect.visible !== false)
    .map((effect: any) => {
      const out: any = { type: effect.type };

      if (effect.color) {
        out.color = toHex(effect.color);
        const a = effect.color.a != null ? effect.color.a : 1;
        if (a !== 1) out.opacity = ratioRound(a);
      }
      // The second tone of a DUOTONE noise effect.
      if (effect.secondaryColor) out.secondaryColor = toHex(effect.secondaryColor);
      if (effect.offset) {
        out.offsetX = pixelRound(effect.offset.x);
        out.offsetY = pixelRound(effect.offset.y);
      }
      // NOISE MULTITONE carries an opacity that is not the colour's alpha.
      if (effect.type === "NOISE" && typeof effect.opacity === "number") {
        out.noiseOpacity = ratioRound(effect.opacity);
      }

      for (const key of Object.keys(effect)) {
        if (effectKeysHandledElsewhere.has(key)) continue;
        const value = effect[key];
        if (effectDefaults[key] === value) continue;
        if (typeof value === "number") {
          // Zero is the identity for these parameters (spread, dispersion, density …),
          // so it is a default worth omitting. Radius always applies: a zero-radius
          // shadow is a hard edge, which is not the same as no shadow.
          if (value === 0 && key !== "radius") continue;
          out[key] = pixelRound(value);
        } else if (typeof value === "string") {
          out[key] = value;
        } else if (typeof value === "boolean") {
          // false is the off state for every boolean effect field Figma exposes
          // (showShadowBehindNode, clipToShape), so it is the default.
          if (value) out[key] = true;
        } else if (isVector(value)) {
          // Progressive blur start/end points and the noise size vector.
          out[key] = { x: pixelRound(value.x), y: pixelRound(value.y) };
        }
      }

      return out;
    });

  return result.length > 0 ? result : undefined;
};

// Auto-layout settings, mirroring the arguments create_node accepts.
// Only emitted for frames that actually use auto-layout.
export const serializeLayout = (node: any) => {
  if (!("layoutMode" in node) || node.layoutMode === "NONE") return undefined;

  const layout: any = { mode: node.layoutMode };
  if (node.itemSpacing) layout.itemSpacing = node.itemSpacing;
  if (node.primaryAxisAlignItems && node.primaryAxisAlignItems !== "MIN")
    layout.primaryAxisAlignItems = node.primaryAxisAlignItems;
  if (node.counterAxisAlignItems && node.counterAxisAlignItems !== "MIN")
    layout.counterAxisAlignItems = node.counterAxisAlignItems;
  if (node.primaryAxisSizingMode) layout.primaryAxisSizingMode = node.primaryAxisSizingMode;
  if (node.counterAxisSizingMode) layout.counterAxisSizingMode = node.counterAxisSizingMode;
  if (node.layoutWrap && node.layoutWrap !== "NO_WRAP") {
    layout.layoutWrap = node.layoutWrap;
    if (node.counterAxisSpacing) layout.counterAxisSpacing = node.counterAxisSpacing;
  }
  return layout;
};

export const serializeStyles = async (node: any) => {
  const styles: any = {};

  if ("fills" in node) {
    // Prefer named style over raw fill values when a style is applied.
    if (node.fillStyleId && typeof node.fillStyleId === "string") {
      const style = await figma.getStyleByIdAsync(node.fillStyleId);
      if (style) styles.fillStyle = style.name;
    }
    const fills = serializePaints(node.fills);
    if (fills !== undefined) styles.fills = fills;
  }

  if ("strokes" in node) {
    if (node.strokeStyleId && typeof node.strokeStyleId === "string") {
      const style = await figma.getStyleByIdAsync(node.strokeStyleId);
      if (style) styles.strokeStyle = style.name;
    }
    const strokes = serializePaints(node.strokes);
    if (strokes !== undefined) {
      styles.strokes = strokes;
      // Weight and alignment only mean something when there is a stroke to draw.
      if ("strokeWeight" in node)
        styles.strokeWeight = isMixed(node.strokeWeight) ? "mixed" : node.strokeWeight;
      if (node.strokeAlign) styles.strokeAlign = node.strokeAlign;
      if (Array.isArray(node.dashPattern) && node.dashPattern.length > 0)
        styles.dashPattern = node.dashPattern;
      // The rest of the stroke geometry set_node_properties writes. Read back
      // so a caller can round-trip a stroke rather than only half of one.
      if ("strokeCap" in node && node.strokeCap)
        styles.strokeCap = isMixed(node.strokeCap) ? "mixed" : node.strokeCap;
      if ("strokeJoin" in node && node.strokeJoin)
        styles.strokeJoin = isMixed(node.strokeJoin) ? "mixed" : node.strokeJoin;
      // 4 is Figma's default and says nothing, so only a changed limit is worth
      // the tokens.
      if (typeof node.strokeMiterLimit === "number" && node.strokeMiterLimit !== 4)
        styles.strokeMiterLimit = node.strokeMiterLimit;
    }
  }

  if ("effects" in node) {
    if (node.effectStyleId && typeof node.effectStyleId === "string") {
      const style = await figma.getStyleByIdAsync(node.effectStyleId);
      if (style) styles.effectStyle = style.name;
    }
    const effects = serializeEffects(node.effects);
    if (effects !== undefined) styles.effects = effects;
  }

  const layout = serializeLayout(node);
  if (layout) styles.layout = layout;

  // Padding only matters under auto-layout, and only when something is non-zero.
  if ("paddingLeft" in node) {
    const padding = {
      top: node.paddingTop,
      right: node.paddingRight,
      bottom: node.paddingBottom,
      left: node.paddingLeft,
    };
    if (padding.top || padding.right || padding.bottom || padding.left)
      styles.padding = padding;
  }

  return styles;
};

export const serializeLineHeight = (lineHeight: any) => {
  if (isMixed(lineHeight)) return "mixed";

  if (!lineHeight || lineHeight.unit === "AUTO") return undefined;

  return { value: lineHeight.value, unit: lineHeight.unit };
};

export const serializeLetterSpacing = (letterSpacing: any) => {
  if (isMixed(letterSpacing)) return "mixed";

  if (!letterSpacing || letterSpacing.value === 0) return undefined;

  return { value: letterSpacing.value, unit: letterSpacing.unit };
};

/**
 * Per-range styling, but only when the node actually has more than one.
 *
 * The node-level style fields report "mixed" for a paragraph with one bold
 * word, which tells a code generator that something varies and nothing about
 * what — so bold, links and colour changes were lost on the way to code. A node
 * styled uniformly returns nothing here: a single segment repeating what the
 * fields above already say is pure noise in every serialized tree.
 */
export const serializeStyledSegments = (node: any) => {
  if (typeof node.getStyledTextSegments !== "function") return undefined;
  const segments = node.getStyledTextSegments([
    "fontName",
    "fontSize",
    "fills",
    "textDecoration",
    "textCase",
    "hyperlink",
    "listOptions",
    "indentation",
  ]);
  if (!Array.isArray(segments) || segments.length <= 1) return undefined;
  return segments.map((segment: any) => {
    const out: any = {
      start: segment.start,
      end: segment.end,
      characters: segment.characters,
    };
    if (segment.fontName) {
      out.fontFamily = segment.fontName.family;
      out.fontStyle = segment.fontName.style;
    }
    if (segment.fontSize != null) out.fontSize = segment.fontSize;
    if (Array.isArray(segment.fills) && segment.fills.length > 0) {
      out.fills = serializePaints(segment.fills);
    }
    if (segment.textDecoration && segment.textDecoration !== "NONE") {
      out.textDecoration = segment.textDecoration;
    }
    if (segment.textCase && segment.textCase !== "ORIGINAL") out.textCase = segment.textCase;
    if (segment.hyperlink) out.hyperlink = segment.hyperlink.value ?? segment.hyperlink;
    if (segment.listOptions && segment.listOptions.type !== "NONE") {
      out.listType = segment.listOptions.type;
    }
    if (segment.indentation) out.indentation = segment.indentation;
    return out;
  });
};

export const serializeText = async (node: any, base: any) => {
  let fontFamily: any;
  let fontStyle: any;

  if (typeof node.fontName === "symbol") {
    fontFamily = "mixed";
    fontStyle = "mixed";
  } else if (node.fontName) {
    fontFamily = node.fontName.family;
    fontStyle = node.fontName.style;
  }

  const textStyleName =
    node.textStyleId && typeof node.textStyleId === "string"
      ? ((await figma.getStyleByIdAsync(node.textStyleId))?.name ?? undefined)
      : undefined;

  const styledSegments = serializeStyledSegments(node);

  return Object.assign({}, base, {
    characters: node.characters,
    ...(styledSegments ? { styledSegments } : {}),
    styles: Object.assign({}, base.styles, {
      ...(textStyleName ? { textStyle: textStyleName } : {}),
      fontSize: isMixed(node.fontSize) ? "mixed" : node.fontSize,
      fontFamily,
      fontStyle,
      fontWeight: isMixed(node.fontWeight) ? "mixed" : node.fontWeight,
      textDecoration: isMixed(node.textDecoration)
        ? "mixed"
        : node.textDecoration !== "NONE"
          ? node.textDecoration
          : undefined,
      lineHeight: serializeLineHeight(node.lineHeight),
      letterSpacing: serializeLetterSpacing(node.letterSpacing),
      textAlignHorizontal: isMixed(node.textAlignHorizontal)
        ? "mixed"
        : node.textAlignHorizontal,
      textAlignVertical: isMixed(node.textAlignVertical)
        ? "mixed"
        : node.textAlignVertical,
    }),
  });
};

// Per-corner radii, but only when they actually differ from the uniform cornerRadius.
// Figma reports all four on every rectangle and frame; repeating them when they all
// match is pure noise.
const getCornerRadii = (node: any) => {
  const corners = ["topLeftRadius", "topRightRadius", "bottomLeftRadius", "bottomRightRadius"];
  if (!corners.every((c) => c in node)) return undefined;
  const values = corners.map((c) => node[c]);
  if (values.every((v) => v === values[0])) return undefined;
  return { topLeftRadius: values[0], topRightRadius: values[1], bottomLeftRadius: values[2], bottomRightRadius: values[3] };
};

const getGeometry = (node: any) => {
  const geom: any = {};

  // Omit defaults: an unrotated node with square corners has nothing to report.
  if ("rotation" in node && node.rotation !== 0) {
    geom.rotation = pixelRound(node.rotation);
  }
  if ("cornerRadius" in node) {
    const cr = isMixed(node.cornerRadius) ? "mixed" : node.cornerRadius;
    if (cr !== 0) geom.cornerRadius = cr;
  }

  switch (node.type) {
    case "STAR":
      if ("pointCount" in node) geom.pointCount = node.pointCount;
      if ("innerRadius" in node) {
        geom.innerRadiusRatio = node.innerRadius;
        geom.outerRadiusPixel = node.width / 2;
        geom.innerRadiusPixel = (node.width / 2) * node.innerRadius;
      }
      break;
    case "POLYGON":
      if ("pointCount" in node) geom.pointCount = node.pointCount;
      break;
    case "ELLIPSE":
      if ("arcData" in node) geom.arcData = node.arcData;
      break;
    case "RECTANGLE":
    case "FRAME":
    case "COMPONENT":
      Object.assign(geom, getCornerRadii(node));
      break;
  }
  
  return Object.keys(geom).length > 0 ? geom : undefined;
};

// Node-level properties, mirroring what set_node_properties can write so that reads
// and writes cover the same ground. Rotation is left to getGeometry, which already
// reports it. Every field is omitted at its default value to keep output small.
export const serializeNodeProperties = (node: any) => {
  const props: any = {};

  if ("opacity" in node && node.opacity !== 1) props.opacity = ratioRound(node.opacity);
  if ("visible" in node && !node.visible) props.visible = false;
  if ("locked" in node && node.locked) props.locked = true;
  // Frames and groups default to PASS_THROUGH, leaf nodes to NORMAL.
  if (node.blendMode && node.blendMode !== "NORMAL" && node.blendMode !== "PASS_THROUGH")
    props.blendMode = node.blendMode;
  if (node.constraints && (node.constraints.horizontal !== "MIN" || node.constraints.vertical !== "MIN"))
    props.constraints = { horizontal: node.constraints.horizontal, vertical: node.constraints.vertical };

  return props;
};

/**
 * What a component or variant set exposes to its instances.
 *
 * Reading a component told you its layers and nothing about its API, so the
 * only way to learn a property's name was to place an instance and inspect it.
 * The `#1:2` suffix Figma appends is kept, because that is the id every write
 * has to quote back, with the bare name alongside for reading.
 */
export const serializeComponentPropertyDefinitions = (node: any) => {
  if (node.type !== "COMPONENT" && node.type !== "COMPONENT_SET") return undefined;
  const defs = node.componentPropertyDefinitions;
  if (!defs || Object.keys(defs).length === 0) return undefined;
  const out: Record<string, any> = {};
  for (const [key, def] of Object.entries<any>(defs)) {
    out[key] = {
      name: key.split("#")[0],
      type: def.type,
      defaultValue: def.defaultValue,
      ...(def.variantOptions ? { variantOptions: def.variantOptions } : {}),
      ...(def.preferredValues ? { preferredValues: def.preferredValues } : {}),
    };
  }
  return out;
};

/**
 * Bounds on how much of a tree serializeNode will walk.
 *
 * get_document on a busy page produced one object per node with no ceiling,
 * which is a payload nothing on the far side asked for and an LLM context
 * nothing survives. A budget is shared across the whole walk rather than
 * applied per branch, so the cost of an answer is bounded by the answer and
 * not by the shape of the tree.
 */
export interface SerializeBudget {
  /** How many more nodes may be serialized. Mutated as the walk spends it. */
  remaining: number;
  /** How deep the walk may go below the root; Infinity for no limit. */
  maxDepth: number;
  /** Set when something was left out, so a half tree is never reported as whole. */
  truncated: boolean;
}

export const makeBudget = (maxNodes?: number, maxDepth?: number): SerializeBudget => ({
  remaining: maxNodes != null && Number(maxNodes) > 0 ? Number(maxNodes) : Infinity,
  maxDepth: maxDepth != null && Number(maxDepth) >= 0 ? Number(maxDepth) : Infinity,
  truncated: false,
});

export const serializeNode = async (
  node: any,
  budget?: SerializeBudget,
  depth = 0,
): Promise<any> => {
  const styles = await serializeStyles(node);
  const base: any = {
    id: node.id,
    name: node.name,
    type: node.type,
    bounds: getBounds(node),
    geometry: getGeometry(node),
    ...serializeNodeProperties(node),
  };
  if (Object.keys(styles).length > 0) base.styles = styles;
  const componentProperties = serializeComponentPropertyDefinitions(node);
  if (componentProperties) base.componentProperties = componentProperties;
  if (node.type === "TEXT") return serializeText(node, base);
  // An empty children array says nothing a reader cannot infer from the node type,
  // so only containers that actually hold something report children.
  if ("children" in node && node.children.length > 0) {
    // The node itself is still reported; only its children are withheld, and
    // saying how many keeps the answer honest.
    const omitted = () =>
      Object.assign({}, base, { childCount: node.children.length, childrenOmitted: true });

    if (budget && depth >= budget.maxDepth) {
      budget.truncated = true;
      return omitted();
    }

    const children: any[] = [];
    for (const child of node.children) {
      if (budget) {
        if (budget.remaining <= 0) {
          budget.truncated = true;
          break;
        }
        budget.remaining--;
      }
      // Sequential rather than Promise.all: the budget is spent in tree order,
      // or which nodes survive would depend on how the promises happen to settle.
      children.push(await serializeNode(child, budget, depth + 1));
    }

    if (children.length < node.children.length) {
      return Object.assign({}, base, {
        children,
        childCount: node.children.length,
        childrenOmitted: true,
      });
    }
    return Object.assign({}, base, { children });
  }
  return base;
};

// deduplicateStyles does a two-pass walk over a serialized node tree.
// First pass: count how many times each fills/strokes array value appears.
// Second pass: replace values that appear more than once with a short ref key.
// Returns the rewritten tree and a globalVars.styles map (or undefined if nothing was deduped).
export const deduplicateStyles = (tree: any): { tree: any; globalVars: Record<string, any> | undefined } => {
  // Pass 1: count occurrences of each serialized fill/stroke value
  const counts = new Map<string, number>();
  const countWalk = (node: any) => {
    if (!node || typeof node !== "object") return;
    const s = node.styles;
    if (s) {
      if (Array.isArray(s.fills)) counts.set(JSON.stringify(s.fills), (counts.get(JSON.stringify(s.fills)) ?? 0) + 1);
      if (Array.isArray(s.strokes)) counts.set(JSON.stringify(s.strokes), (counts.get(JSON.stringify(s.strokes)) ?? 0) + 1);
    }
    if (Array.isArray(node.children)) node.children.forEach(countWalk);
  };
  countWalk(tree);

  // Build ref map for values that appear more than once
  let counter = 0;
  const keyToRef = new Map<string, string>();
  const refs: Record<string, any> = {};
  for (const [key, count] of counts) {
    if (count > 1) {
      const ref = `s${++counter}`;
      keyToRef.set(key, ref);
      refs[ref] = JSON.parse(key);
    }
  }
  if (keyToRef.size === 0) return { tree, globalVars: undefined };

  // Pass 2: replace repeated values with ref keys
  const replaceWalk = (node: any): any => {
    if (!node || typeof node !== "object") return node;
    let result = node;
    const s = node.styles;
    if (s) {
      let newStyles = s;
      if (Array.isArray(s.fills)) {
        const ref = keyToRef.get(JSON.stringify(s.fills));
        if (ref) newStyles = { ...newStyles, fills: ref };
      }
      if (Array.isArray(s.strokes)) {
        const ref = keyToRef.get(JSON.stringify(s.strokes));
        if (ref) newStyles = { ...newStyles, strokes: ref };
      }
      if (newStyles !== s) result = { ...node, styles: newStyles };
    }
    if (Array.isArray(node.children)) {
      const newChildren = node.children.map(replaceWalk);
      result = { ...result, children: newChildren };
    }
    return result;
  };

  return { tree: replaceWalk(tree), globalVars: { styles: refs } };
};

export const serializeVariableValue = (value: any) => {
  if (typeof value !== "object" || value === null) return value;

  if ("type" in value && value.type === "VARIABLE_ALIAS") {
    return { type: "VARIABLE_ALIAS", id: value.id };
  }

  if ("r" in value && "g" in value && "b" in value) {
    return {
      type: "COLOR",
      r: value.r,
      g: value.g,
      b: value.b,
      a: "a" in value ? value.a : 1,
    };
  }

  return value;
};

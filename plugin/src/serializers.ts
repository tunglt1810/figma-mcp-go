// Serializers — shared read/write helpers for converting Figma node data to JSON.

export const isMixed = (value: any) => typeof value === "symbol";

// Round floating-point pixel values to 2 decimal places.
// Figma sometimes returns values like 123.99999999999999 instead of 124.
const pixelRound = (v: number) => Math.round(v * 100) / 100;

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
    .filter((paint: any) => {
      return (paint.type === "SOLID" && "color" in paint) || 
             paint.type === "GRADIENT_LINEAR" || 
             paint.type === "GRADIENT_RADIAL";
    })
    .map((paint: any) => {
      if (paint.type === "SOLID") {
        const hex = toHex(paint.color);
        const opacity = paint.opacity != null ? paint.opacity : 1;
        if (opacity === 1) return hex;
        return (
          hex +
          Math.round(opacity * 255)
            .toString(16)
            .padStart(2, "0")
        );
      }

      // GRADIENT
      const inv = invertTransform(paint.gradientTransform);
      const stops = paint.gradientStops.map((stop: any) => {
        const colorHex = toHex(stop.color);
        const a = stop.color.a != null ? stop.color.a : 1;
        const colorStr = a === 1 ? colorHex : colorHex + Math.round(a * 255).toString(16).padStart(2, "0");
        return { position: stop.position, color: colorStr };
      });

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

        const stopStrings = stops.map((s: any) => `${s.color} ${Math.round(s.position * 100)}%`).join(', ');
        const rxPercent = Math.round(rx * 100);
        const ryPercent = Math.round(ry * 100);
        const cxPercent = Math.round(cx * 100);
        const cyPercent = Math.round(cy * 100);
        
        const cssString = Math.abs(rx - ry) < 0.01 
          ? `radial-gradient(circle ${rxPercent}% at ${cxPercent}% ${cyPercent}%, ${stopStrings})`
          : `radial-gradient(ellipse ${rxPercent}% ${ryPercent}% at ${cxPercent}% ${cyPercent}%, ${stopStrings})`;

        return {
          type: "GRADIENT_RADIAL",
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

        const stopStrings = stops.map((s: any) => `${s.color} ${Math.round(s.position * 100)}%`).join(', ');
        let cssAngle = Math.round(angle + 90);
        if (cssAngle < 0) cssAngle += 360;
        cssAngle = cssAngle % 360;
        const cssString = `linear-gradient(${cssAngle}deg, ${stopStrings})`;

        return {
          type: "GRADIENT_LINEAR",
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
    if (strokes !== undefined) styles.strokes = strokes;
  }

  if ("cornerRadius" in node) {
    const cr = isMixed(node.cornerRadius) ? "mixed" : node.cornerRadius;
    if (cr !== 0) styles.cornerRadius = cr;
  }

  if ("paddingLeft" in node) {
    styles.padding = {
      top: node.paddingTop,
      right: node.paddingRight,
      bottom: node.paddingBottom,
      left: node.paddingLeft,
    };
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

  return Object.assign({}, base, {
    characters: node.characters,
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

export const serializeNode = async (node: any): Promise<any> => {
  const styles = await serializeStyles(node);
  const base = {
    id: node.id,
    name: node.name,
    type: node.type,
    bounds: getBounds(node),
    styles,
  };
  if (node.type === "TEXT") return serializeText(node, base);
  if ("children" in node) {
    return Object.assign({}, base, {
      children: await Promise.all(node.children.map((child: any) => serializeNode(child))),
    });
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

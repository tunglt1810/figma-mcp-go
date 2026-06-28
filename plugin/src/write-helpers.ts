import { invertTransform } from "./serializers";

// Write helpers — utilities used exclusively by write handlers.

export const hexToRgb = (hex: string) => {
  const clean = hex.replace("#", "");
  return {
    r: parseInt(clean.slice(0, 2), 16) / 255,
    g: parseInt(clean.slice(2, 4), 16) / 255,
    b: parseInt(clean.slice(4, 6), 16) / 255,
    a: clean.length >= 8 ? parseInt(clean.slice(6, 8), 16) / 255 : 1,
  };
};

export const makeSolidPaint = (colorInput: any, opacityOverride?: number): SolidPaint => {
  const { r, g, b, a } = typeof colorInput === "string"
    ? hexToRgb(colorInput)
    : { r: colorInput.r, g: colorInput.g, b: colorInput.b, a: colorInput.a != null ? colorInput.a : 1 };
  const eff = opacityOverride != null ? opacityOverride : a;
  const paint: any = { type: "SOLID", color: { r, g, b } };
  if (eff !== 1) paint.opacity = eff;
  return paint;
};

export const getParentNode = async (parentId: string | undefined) => {
  if (!parentId) return figma.currentPage;
  const parent = await figma.getNodeByIdAsync(parentId);
  if (!parent) throw new Error(`Parent node not found: ${parentId}`);
  if (!("appendChild" in parent)) throw new Error(`Node ${parentId} cannot have children`);
  return parent as ChildrenMixin & BaseNode;
};

export const applyAutoLayout = (frame: FrameNode, p: any) => {
  if (p.layoutMode != null) frame.layoutMode = p.layoutMode;
  if (p.paddingTop != null) frame.paddingTop = Number(p.paddingTop);
  if (p.paddingRight != null) frame.paddingRight = Number(p.paddingRight);
  if (p.paddingBottom != null) frame.paddingBottom = Number(p.paddingBottom);
  if (p.paddingLeft != null) frame.paddingLeft = Number(p.paddingLeft);
  if (p.itemSpacing != null) frame.itemSpacing = Number(p.itemSpacing);
  if (frame.layoutMode !== "NONE") {
    if (p.primaryAxisAlignItems) frame.primaryAxisAlignItems = p.primaryAxisAlignItems;
    if (p.counterAxisAlignItems) frame.counterAxisAlignItems = p.counterAxisAlignItems;
    if (p.primaryAxisSizingMode) frame.primaryAxisSizingMode = p.primaryAxisSizingMode;
    if (p.counterAxisSizingMode) frame.counterAxisSizingMode = p.counterAxisSizingMode;
    if (p.layoutWrap) frame.layoutWrap = p.layoutWrap;
    if (p.counterAxisSpacing != null && frame.layoutWrap === "WRAP") {
      frame.counterAxisSpacing = Number(p.counterAxisSpacing);
    }
  }
};

export const base64ToBytes = (b64: string) => {
  const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  const lookup: Record<string, number> = {};
  for (let i = 0; i < chars.length; i++) lookup[chars[i]] = i;
  const padded = b64.replace(/[^A-Za-z0-9+/=]/g, "");
  const clean = padded.replace(/=/g, "");
  let outLen = Math.floor(padded.length * 3 / 4);
  if (padded.endsWith("==")) outLen -= 2;
  else if (padded.endsWith("=")) outLen -= 1;
  const bytes = new Uint8Array(outLen);
  let j = 0;
  for (let i = 0; i < clean.length; i += 4) {
    const a = lookup[clean[i]] || 0;
    const bv = lookup[clean[i + 1]] || 0;
    const c = lookup[clean[i + 2]] || 0;
    const d = lookup[clean[i + 3]] || 0;
    bytes[j++] = (a << 2) | (bv >> 4);
    if (j < outLen) bytes[j++] = ((bv & 15) << 4) | (c >> 2);
    if (j < outLen) bytes[j++] = ((c & 3) << 6) | d;
  }
  return bytes;
};

export const makeGradientPaint = (type: string, stops: any[], geometry: any): GradientPaint => {
  const gradientStops: ReadonlyArray<ColorStop> = stops.map((stop: any) => {
    const { r, g, b, a } = typeof stop.color === "string" ? hexToRgb(stop.color) : stop.color;
    return {
      position: stop.position,
      color: { r, g, b, a: a != null ? a : 1 }
    };
  });

  let T_inv: number[][] = [[1, 0, 0], [0, 1, 0]];

  if (type === "GRADIENT_RADIAL") {
    const cx = (geometry.center?.percentX || 50) / 100;
    const cy = (geometry.center?.percentY || 50) / 100;
    const rx = (geometry.radius?.percentX || 50) / 100;
    const ry = (geometry.radius?.percentY || 50) / 100;
    const theta = ((geometry.rotation || 0) * Math.PI) / 180;

    // Place 3 control points in normalized node space:
    //   center      = gradient center
    //   rxHandle    = tip of the X-radius axis (rotated by theta)
    //   ryHandle    = tip of the Y-radius axis (perpendicular to X-axis)
    const centerNorm = { x: cx, y: cy };
    const rxHandleNorm = {
      x: cx + rx * Math.cos(theta),
      y: cy + rx * Math.sin(theta)
    };
    const ryHandleNorm = {
      x: cx - ry * Math.sin(theta),
      y: cy + ry * Math.cos(theta)
    };

    // Solve affine transform T_inv that maps 3 gradient-space control points
    // to their positions in normalized node space:
    //   (0.5, 0.5) → center    (gradient center)
    //   (1.0, 0.5) → rxHandle  (end of X-radius axis)
    //   (0.5, 1.0) → ryHandle  (end of Y-radius axis)
    //
    // T_inv = [[A, B, C], [D, E, F]] where A·gx + B·gy + C = nx
    // Coefficients derived by substituting the 3 point pairs and solving.
    const A = 2 * (rxHandleNorm.x - centerNorm.x);
    const B = 2 * (ryHandleNorm.x - centerNorm.x);
    const C = 3 * centerNorm.x - rxHandleNorm.x - ryHandleNorm.x;

    const D = 2 * (rxHandleNorm.y - centerNorm.y);
    const E = 2 * (ryHandleNorm.y - centerNorm.y);
    const F = 3 * centerNorm.y - rxHandleNorm.y - ryHandleNorm.y;

    T_inv = [[A, B, C], [D, E, F]];
  } else if (type === "GRADIENT_LINEAR") {
    const sx = (geometry.start?.percentX || 0) / 100;
    const sy = (geometry.start?.percentY || 0) / 100;
    const ex = (geometry.end?.percentX || 100) / 100;
    const ey = (geometry.end?.percentY || 100) / 100;

    const startNorm = { x: sx, y: sy };
    const endNorm = { x: ex, y: ey };
    
    // Perpendicular vector for the Y handle mapping
    const dx = endNorm.x - startNorm.x;
    const dy = endNorm.y - startNorm.y;
    const perpNorm = { x: startNorm.x - dy, y: startNorm.y + dx };

    const A = endNorm.x - startNorm.x;
    const B = 2 * (perpNorm.x - startNorm.x);
    const C = 2 * startNorm.x - perpNorm.x;

    const D = endNorm.y - startNorm.y;
    const E = 2 * (perpNorm.y - startNorm.y);
    const F = 2 * startNorm.y - perpNorm.y;

    T_inv = [[A, B, C], [D, E, F]];
  }

  const gradientTransform = invertTransform(T_inv) as Transform;

  return {
    type: type as any,
    gradientStops,
    gradientTransform
  };
};

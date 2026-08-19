import { invertTransform } from "./serializers";

// Write helpers — utilities used exclusively by write handlers.

// Accepts #RGB, #RGBA, #RRGGBB and #RRGGBBAA, with or without the leading #.
// Anything else is an error: the old version returned NaN channels, which Figma
// painted as a broken fill without reporting anything.
export const hexToRgb = (hex: string) => {
  const clean = typeof hex === "string" ? hex.replace(/^#/, "") : "";
  if (!/^([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/.test(clean)) {
    throw new Error(
      `Invalid hex color: ${JSON.stringify(hex)} — expected #RGB, #RGBA, #RRGGBB, or #RRGGBBAA`,
    );
  }
  // Shorthand doubles each digit: #f80 is #ff8800.
  const full = clean.length <= 4
    ? clean.split("").map(c => c + c).join("")
    : clean;
  return {
    r: parseInt(full.slice(0, 2), 16) / 255,
    g: parseInt(full.slice(2, 4), 16) / 255,
    b: parseInt(full.slice(4, 6), 16) / 255,
    a: full.length >= 8 ? parseInt(full.slice(6, 8), 16) / 255 : 1,
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

export const makeGradientPaint = (type: string, stops: any[], geometry: any, opacity?: number): GradientPaint => {
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
    gradientTransform,
    ...(opacity != null ? { opacity } : {})
  };
};

// Every effect type Figma's Effect union covers except SHADER, which needs a shader
// imported through figma.importShaderById first and so cannot be built from params.
export const supportedEffectTypes = [
  "DROP_SHADOW", "INNER_SHADOW", "LAYER_BLUR", "BACKGROUND_BLUR",
  "NOISE", "TEXTURE", "GLASS",
];

const num = (value: any, fallback: number) => (value != null ? Number(value) : fallback);

// Read a {x, y} pair, which is how serializeEffects reports Figma's Vector fields.
const vec = (value: any, fallback: { x: number; y: number }) =>
  value && value.x != null && value.y != null
    ? { x: Number(value.x), y: Number(value.y) }
    : fallback;

// Build one Figma Effect from the flat parameter object set_effects accepts.
//
// The shape mirrors what serializeEffects produces, so a node's effects can be read
// and written back unchanged. Colour alpha travels as `opacity`; NOISE MULTITONE's own
// effect opacity is `noiseOpacity`, because the two are different values.
export const makeEffect = (e: any): Effect => {
  const visible = e.visible ?? true;

  switch (e.type) {
    case "DROP_SHADOW":
    case "INNER_SHADOW": {
      const { r, g, b } = hexToRgb(e.color || "#000000");
      const shadow: any = {
        type: e.type,
        color: { r, g, b, a: num(e.opacity, 0.25) },
        offset: { x: num(e.offsetX, 0), y: num(e.offsetY, 4) },
        radius: num(e.radius, 4),
        spread: num(e.spread, 0),
        visible,
        blendMode: (e.blendMode || "NORMAL") as BlendMode,
      };
      if (e.type === "DROP_SHADOW" && e.showShadowBehindNode != null) {
        shadow.showShadowBehindNode = !!e.showShadowBehindNode;
      }
      return shadow as DropShadowEffect;
    }

    case "LAYER_BLUR":
    case "BACKGROUND_BLUR": {
      // blurType is part of the BlurEffect union; omitting it leaves the effect
      // underspecified, so it defaults to NORMAL here rather than being left out.
      if (e.blurType === "PROGRESSIVE") {
        return {
          type: e.type,
          blurType: "PROGRESSIVE",
          radius: num(e.radius, 4),
          startRadius: num(e.startRadius, 0),
          startOffset: vec(e.startOffset, { x: 0, y: 0 }),
          endOffset: vec(e.endOffset, { x: 0, y: 1 }),
          visible,
        } as BlurEffect;
      }
      return {
        type: e.type,
        blurType: "NORMAL",
        radius: num(e.radius, 4),
        visible,
      } as BlurEffect;
    }

    case "NOISE": {
      const { r, g, b } = hexToRgb(e.color || "#000000");
      const noise: any = {
        type: "NOISE",
        noiseType: e.noiseType || "MONOTONE",
        color: { r, g, b, a: num(e.opacity, 1) },
        noiseSize: num(e.noiseSize, 2),
        density: num(e.density, 0.5),
        visible,
      };
      // NoiseEffectBase declares blendMode, but the Figma runtime rejects the key on
      // noise effects ("Unrecognized key(s) in object: 'blendMode'"). Only forward it
      // when a caller asks for it, so the common path stays writable.
      if (e.blendMode) noise.blendMode = e.blendMode as BlendMode;
      if (noise.noiseType === "DUOTONE") {
        const s = hexToRgb(e.secondaryColor || "#ffffff");
        noise.secondaryColor = { r: s.r, g: s.g, b: s.b, a: s.a };
      }
      if (noise.noiseType === "MULTITONE") noise.opacity = num(e.noiseOpacity, 1);
      return noise as NoiseEffect;
    }

    case "TEXTURE":
      return {
        type: "TEXTURE",
        noiseSize: num(e.noiseSize, 2),
        radius: num(e.radius, 4),
        clipToShape: !!e.clipToShape,
        visible,
      } as TextureEffect;

    case "GLASS":
      return {
        type: "GLASS",
        lightIntensity: num(e.lightIntensity, 0.5),
        lightAngle: num(e.lightAngle, -45),
        refraction: num(e.refraction, 0.5),
        depth: num(e.depth, 20),
        dispersion: num(e.dispersion, 0),
        radius: num(e.radius, 4),
        visible,
      } as GlassEffect;

    case "SHADER":
      throw new Error(
        "SHADER effects cannot be created from parameters: the shader has to be imported with figma.importShaderById first",
      );

    default:
      throw new Error(
        `Unknown effect type: ${e.type}. Must be one of ${supportedEffectTypes.join(", ")}`,
      );
  }
};

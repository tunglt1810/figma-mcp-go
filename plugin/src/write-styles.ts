import { makeSolidPaint, hexToRgb, makeEffect } from "./write-helpers";
import { HandlerMap } from "./dispatch";

// create_style replaced four create_*_style tools on the MCP surface. The four
// implementations stay separate below, because they genuinely are: only the
// surface merged. `effectType` is unwrapped back to `type`, which the style kind
// took over at the outer level.
const STYLE_ACTIONS: Record<string, string> = {
  PAINT: "create_paint_style",
  TEXT: "create_text_style",
  EFFECT: "create_effect_style",
  GRID: "create_grid_style",
};

export const writeStylesHandlers: HandlerMap = {
  "create_style": async (request) => {
  const { type, effectType, ...rest } = request.params || {};
  const action = STYLE_ACTIONS[type];
  if (!action) {
    throw new Error(`type must be PAINT, TEXT, EFFECT, or GRID, got: ${type}`);
  }
  const params = effectType != null ? { ...rest, type: effectType } : rest;
  const result = await handleWriteStyleRequest({ ...request, type: action, params });
  // Answer under the name the caller used, not the one we delegated to.
  return { ...result, type: request.type }
  },

  "create_paint_style": async (request) => {
    const p = request.params || {};
    if (!p.name) throw new Error("name is required");
    if (!p.color) throw new Error("color is required");
    const existing = (await figma.getLocalPaintStylesAsync()).find(s => s.name === p.name);
    if (existing) {
      return { type: request.type, requestId: request.requestId, data: { id: existing.id, name: existing.name } };
    }
    const style = figma.createPaintStyle();
    style.name = p.name;
    style.paints = [makeSolidPaint(p.color)];
    if (p.description) style.description = p.description;
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: style.id, name: style.name },
    };
  },

  "create_text_style": async (request) => {
    const p = request.params || {};
    if (!p.name) throw new Error("name is required");
    const existing = (await figma.getLocalTextStylesAsync()).find(s => s.name === p.name);
    if (existing) {
      return { type: request.type, requestId: request.requestId, data: { id: existing.id, name: existing.name } };
    }
    const family = p.fontFamily || "Inter";
    const fontStyle = p.fontStyle || "Regular";
    await figma.loadFontAsync({ family, style: fontStyle });
    const style = figma.createTextStyle();
    style.name = p.name;
    style.fontName = { family, style: fontStyle };
    if (p.fontSize != null) style.fontSize = Number(p.fontSize);
    if (p.description) style.description = p.description;
    if (p.textDecoration && p.textDecoration !== "NONE") {
      style.textDecoration = p.textDecoration;
    }
    if (p.lineHeightValue != null) {
      style.lineHeight = { value: Number(p.lineHeightValue), unit: p.lineHeightUnit || "PIXELS" };
    }
    if (p.letterSpacingValue != null) {
      style.letterSpacing = { value: Number(p.letterSpacingValue), unit: p.letterSpacingUnit || "PIXELS" };
    }
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: style.id, name: style.name },
    };
  },

  "create_effect_style": async (request) => {
    const p = request.params || {};
    if (!p.name) throw new Error("name is required");
    const existing = (await figma.getLocalEffectStylesAsync()).find(s => s.name === p.name);
    if (existing) {
      return { type: request.type, requestId: request.requestId, data: { id: existing.id, name: existing.name } };
    }
    const effectType = p.type || "DROP_SHADOW";
    let effect: Effect;
    if (effectType === "LAYER_BLUR") {
      effect = { type: "LAYER_BLUR", blurType: "NORMAL", radius: Number(p.radius ?? 4), visible: true };
    } else if (effectType === "BACKGROUND_BLUR") {
      effect = { type: "BACKGROUND_BLUR", blurType: "NORMAL", radius: Number(p.radius ?? 4), visible: true };
    } else {
      // DROP_SHADOW or INNER_SHADOW
      const { r, g, b, a } = hexToRgb(p.color || "#000000");
      const alpha = p.opacity != null ? Number(p.opacity) : (a !== 1 ? a : 0.25);
      effect = {
        type: effectType as "DROP_SHADOW" | "INNER_SHADOW",
        color: { r, g, b, a: alpha },
        offset: { x: Number(p.offsetX ?? 0), y: Number(p.offsetY ?? 4) },
        radius: Number(p.radius ?? 8),
        spread: Number(p.spread ?? 0),
        visible: true,
        blendMode: "NORMAL",
      };
    }
    const style = figma.createEffectStyle();
    style.name = p.name;
    style.effects = [effect];
    if (p.description) style.description = p.description;
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: style.id, name: style.name },
    };
  },

  "create_grid_style": async (request) => {
    const p = request.params || {};
    if (!p.name) throw new Error("name is required");
    const existing = (await figma.getLocalGridStylesAsync()).find(s => s.name === p.name);
    if (existing) {
      return { type: request.type, requestId: request.requestId, data: { id: existing.id, name: existing.name } };
    }
    const pattern = p.pattern || "GRID";
    let grid: LayoutGrid;
    if (pattern === "COLUMNS" || pattern === "ROWS") {
      grid = {
        pattern,
        count: Number(p.count ?? 12),
        gutterSize: Number(p.gutterSize ?? 16),
        offset: Number(p.offset ?? 0),
        alignment: p.alignment || "STRETCH",
        visible: true,
      };
    } else {
      // GRID
      const { r, g, b, a } = hexToRgb(p.color || "#FF0000");
      grid = {
        pattern: "GRID",
        sectionSize: Number(p.sectionSize ?? 8),
        visible: true,
        color: { r, g, b, a: p.opacity != null ? Number(p.opacity) : (a !== 1 ? a : 0.1) },
      };
    }
    const style = figma.createGridStyle();
    style.name = p.name;
    style.layoutGrids = [grid];
    if (p.description) style.description = p.description;
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: style.id, name: style.name },
    };
  },

  "update_paint_style": async (request) => {
    const p = request.params || {};
    if (!p.styleId) throw new Error("styleId is required");
    const style = await figma.getStyleByIdAsync(p.styleId);
    if (!style) throw new Error(`Style not found: ${p.styleId}`);
    if (style.type !== "PAINT") throw new Error(`Style ${p.styleId} is not a paint style`);
    if (p.name) style.name = p.name;
    if (p.color) (style as PaintStyle).paints = [makeSolidPaint(p.color)];
    if (p.description != null) style.description = p.description;
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: style.id, name: style.name },
    };
  },

  "delete_style": async (request) => {
    const p = request.params || {};
    if (!p.styleId) throw new Error("styleId is required");
    const style = await figma.getStyleByIdAsync(p.styleId);
    if (!style) throw new Error(`Style not found: ${p.styleId}`);
    style.remove();
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { styleId: p.styleId, deleted: true },
    };
  },

  "apply_style_to_node": async (request) => {
    const p = request.params || {};
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeId is required");
    if (!p.styleId) throw new Error("styleId is required");
    const node = await figma.getNodeByIdAsync(nodeId);
    if (!node) throw new Error(`Node not found: ${nodeId}`);
    const style = await figma.getStyleByIdAsync(p.styleId);
    if (!style) throw new Error(`Style not found: ${p.styleId}`);
    const n = node as any;
    switch (style.type) {
      case "PAINT": {
        const target = p.target || "fill";
        if (target === "stroke") {
          if (!("strokeStyleId" in node)) throw new Error(`Node ${nodeId} does not support stroke styles`);
          await n.setStrokeStyleIdAsync(p.styleId);
        } else {
          if (!("fillStyleId" in node)) throw new Error(`Node ${nodeId} does not support fill styles`);
          await n.setFillStyleIdAsync(p.styleId);
        }
        break;
      }
      case "TEXT":
        if (!("textStyleId" in node)) throw new Error(`Node ${nodeId} does not support text styles`);
        await n.setTextStyleIdAsync(p.styleId);
        break;
      case "EFFECT":
        if (!("effectStyleId" in node)) throw new Error(`Node ${nodeId} does not support effect styles`);
        await n.setEffectStyleIdAsync(p.styleId);
        break;
      case "GRID":
        if (!("gridStyleId" in node)) throw new Error(`Node ${nodeId} does not support grid styles`);
        await n.setGridStyleIdAsync(p.styleId);
        break;
      default:
        throw new Error(`Unknown style type: ${(style as any).type}`);
    }
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: n.id, name: n.name, styleId: p.styleId, styleType: style.type },
    };
  },

  "set_effects": async (request) => {
    const p = request.params || {};
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeId is required");
    if (!Array.isArray(p.effects)) throw new Error("effects array is required");
    const node = await figma.getNodeByIdAsync(nodeId) as any;
    if (!node) throw new Error(`Node not found: ${nodeId}`);
    if (!("effects" in node)) throw new Error(`Node ${nodeId} does not support effects`);
    const effects: Effect[] = p.effects.map((e: any) => makeEffect(e));
    node.effects = effects;
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: node.id, name: node.name, effectCount: effects.length },
    };
  },

  "bind_variable_to_node": async (request) => {
    const p = request.params || {};
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeId is required");
    if (!p.variableId) throw new Error("variableId is required");
    if (!p.field) throw new Error("field is required");
    const node = await figma.getNodeByIdAsync(nodeId) as any;
    if (!node) throw new Error(`Node not found: ${nodeId}`);
    const variable = await figma.variables.getVariableByIdAsync(p.variableId);
    if (!variable) throw new Error(`Variable not found: ${p.variableId}`);
    if (p.field === "fillColor") {
      if (!("fills" in node)) throw new Error(`Node ${nodeId} does not support fills`);
      const fills = [...(node.fills as Paint[])];
      const base = fills.length > 0 ? fills[0] : makeSolidPaint("#000000");
      const paint = figma.variables.setBoundVariableForPaint(base as SolidPaint, "color", variable);
      node.fills = [paint];
    } else if (p.field === "strokeColor") {
      if (!("strokes" in node)) throw new Error(`Node ${nodeId} does not support strokes`);
      const strokes = [...(node.strokes as Paint[])];
      const base = strokes.length > 0 ? strokes[0] : makeSolidPaint("#000000");
      const paint = figma.variables.setBoundVariableForPaint(base as SolidPaint, "color", variable);
      node.strokes = [paint];
    } else {
      if (!(p.field in node)) throw new Error(`Node ${nodeId} does not have field: ${p.field}`);
      node.setBoundVariable(p.field, variable);
    }
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: node.id, name: node.name, variableId: p.variableId, field: p.field },
    };
  },
};

export const handleWriteStyleRequest = async (request: any): Promise<any> => {
  const handler = writeStylesHandlers[request.type];
  return handler ? handler(request) : null;
};

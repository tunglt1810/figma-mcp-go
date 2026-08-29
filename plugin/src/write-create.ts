import { getBounds } from "./serializers";
import { makeSolidPaint, getParentNode, base64ToBytes, applyAutoLayout } from "./write-helpers";
import { HandlerMap } from "./dispatch";

// create_node replaced seven create_* tools on the MCP surface. The seven
// implementations stay separate below, because these shapes genuinely differ;
// only the surface merged.
const NODE_ACTIONS: Record<string, string> = {
  FRAME: "create_frame",
  RECTANGLE: "create_rectangle",
  ELLIPSE: "create_ellipse",
  STAR: "create_star",
  POLYGON: "create_polygon",
  LINE: "create_line",
  SECTION: "create_section",
};

/**
 * The size to give a newly placed image.
 *
 * An explicit width and height win. Otherwise the image's own dimensions are
 * used, scaled down to fit a sensible box so a 4000px photo does not land as a
 * 4000px rectangle. getSizeAsync can fail on a malformed image, and a square
 * placeholder is a better outcome there than a failed import.
 */
export const imageSize = async (image: any, p: any) => {
  if (p.width != null && p.height != null) {
    return { width: Number(p.width), height: Number(p.height) };
  }
  const MAX = 1000;
  try {
    const size = await image.getSizeAsync();
    const scale = Math.min(1, MAX / Math.max(size.width, size.height));
    return { width: Math.round(size.width * scale), height: Math.round(size.height * scale) };
  } catch {
    return { width: Number(p.width ?? 200), height: Number(p.height ?? 200) };
  }
};

// Figma expresses a crop as the 2x3 affine transform that maps the fill's unit
// square onto a region of the image, so {x, y, width, height} in fractions of
// the image is scale-then-translate. Callers get the rectangle; the matrix
// stays here, where the one formula lives.
export const cropToTransform = (crop: any): [[number, number, number], [number, number, number]] => [
  [Number(crop.width), 0, Number(crop.x)],
  [0, Number(crop.height), Number(crop.y)],
];

export const writeCreateHandlers: HandlerMap = {
  "create_node": async (request) => {
  const { type, ...params } = request.params || {};
  const action = NODE_ACTIONS[type];
  if (!action) {
    throw new Error(
      `type must be FRAME, RECTANGLE, ELLIPSE, STAR, POLYGON, LINE, or SECTION, got: ${type}`,
    );
  }
  const result = await handleWriteCreateRequest({ ...request, type: action, params });
  // Answer under the name the caller used, not the one we delegated to.
  return { ...result, type: request.type }
  },

  "create_frame": async (request) => {
    const p = request.params || {};
    const parent = await getParentNode(p.parentId);
    const frame = figma.createFrame();
    frame.resize(p.width || 100, p.height || 100);
    frame.x = p.x != null ? p.x : 0;
    frame.y = p.y != null ? p.y : 0;
    if (p.name) frame.name = p.name;
    if (p.fillColor) frame.fills = [makeSolidPaint(p.fillColor)];
    applyAutoLayout(frame, p);
    (parent as any).appendChild(frame);
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: frame.id, name: frame.name, type: frame.type, bounds: getBounds(frame) },
    };
  },

  "create_rectangle": async (request) => {
    const p = request.params || {};
    const parent = await getParentNode(p.parentId);
    const rect = figma.createRectangle();
    rect.resize(p.width || 100, p.height || 100);
    rect.x = p.x != null ? p.x : 0;
    rect.y = p.y != null ? p.y : 0;
    if (p.name) rect.name = p.name;
    if (p.fillColor) rect.fills = [makeSolidPaint(p.fillColor)];
    if (p.cornerRadius != null) rect.cornerRadius = p.cornerRadius;
    (parent as any).appendChild(rect);
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: rect.id, name: rect.name, type: rect.type, bounds: getBounds(rect) },
    };
  },

  "create_ellipse": async (request) => {
    const p = request.params || {};
    const parent = await getParentNode(p.parentId);
    const ellipse = figma.createEllipse();
    ellipse.resize(p.width || 100, p.height || 100);
    ellipse.x = p.x != null ? p.x : 0;
    ellipse.y = p.y != null ? p.y : 0;
    if (p.name) ellipse.name = p.name;
    if (p.fillColor) ellipse.fills = [makeSolidPaint(p.fillColor)];
    // startAngle/endAngle/innerRadiusRatio were declared by the tool but
    // never read here, so arcs and rings silently came out as plain
    // ellipses. Assemble Figma's arcData from them.
    if (p.startAngle != null || p.endAngle != null || p.innerRadiusRatio != null) {
      ellipse.arcData = {
        startingAngle: p.startAngle ?? 0,
        endingAngle: p.endAngle ?? Math.PI * 2,
        innerRadius: p.innerRadiusRatio ?? 0,
      };
    } else if (p.arcData) {
      ellipse.arcData = p.arcData;
    }
    (parent as any).appendChild(ellipse);
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: ellipse.id, name: ellipse.name, type: ellipse.type, bounds: getBounds(ellipse) },
    };
  },

  "create_star": async (request) => {
    const p = request.params || {};
    const parent = await getParentNode(p.parentId);
    const star = figma.createStar();
    const pointCount = p.pointCount != null ? Number(p.pointCount) : 5;
    const outerRadius = p.outerRadius != null ? Number(p.outerRadius) : 50;
    star.pointCount = pointCount;
    
    const width = outerRadius * 2;
    star.resize(width, width);
    
    if (p.innerRadius != null) {
      star.innerRadius = Number(p.innerRadius) / outerRadius;
    }
    
    star.x = p.x != null ? p.x : 0;
    star.y = p.y != null ? p.y : 0;
    if (p.name) star.name = p.name;
    if (p.fillColor) star.fills = [makeSolidPaint(p.fillColor)];
    if (p.cornerRadius != null) star.cornerRadius = p.cornerRadius;
    (parent as any).appendChild(star);
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: star.id, name: star.name, type: star.type, bounds: getBounds(star) },
    };
  },

  "create_polygon": async (request) => {
    const p = request.params || {};
    const parent = await getParentNode(p.parentId);
    const polygon = figma.createPolygon();
    const pointCount = p.pointCount != null ? Number(p.pointCount) : 3;
    const radius = p.radius != null ? Number(p.radius) : 50;
    polygon.pointCount = pointCount;
    
    const width = radius * 2;
    polygon.resize(width, width);
    
    polygon.x = p.x != null ? p.x : 0;
    polygon.y = p.y != null ? p.y : 0;
    if (p.name) polygon.name = p.name;
    if (p.fillColor) polygon.fills = [makeSolidPaint(p.fillColor)];
    if (p.cornerRadius != null) polygon.cornerRadius = p.cornerRadius;
    (parent as any).appendChild(polygon);
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: polygon.id, name: polygon.name, type: polygon.type, bounds: getBounds(polygon) },
    };
  },

  "create_line": async (request) => {
    const p = request.params || {};
    const parent = await getParentNode(p.parentId);
    const line = figma.createLine();
    const length = p.length != null ? Number(p.length) : 100;
    
    line.resize(length, 0);
    
    if (p.rotation != null) line.rotation = Number(p.rotation);
    
    line.x = p.x != null ? p.x : 0;
    line.y = p.y != null ? p.y : 0;
    if (p.name) line.name = p.name;
    if (p.strokeColor) line.strokes = [makeSolidPaint(p.strokeColor)];
    if (p.strokeWeight != null) line.strokeWeight = p.strokeWeight;
    (parent as any).appendChild(line);
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: line.id, name: line.name, type: line.type, bounds: getBounds(line) },
    };
  },

  "create_text": async (request) => {
    const p = request.params || {};
    const parent = await getParentNode(p.parentId);
    const fontFamily = p.fontFamily || "Inter";
    const fontStyle = p.fontStyle || "Regular";
    await figma.loadFontAsync({ family: fontFamily, style: fontStyle });
    const textNode = figma.createText();
    textNode.fontName = { family: fontFamily, style: fontStyle };
    if (p.fontSize != null) textNode.fontSize = Number(p.fontSize);
    textNode.characters = p.text || "";
    textNode.x = p.x != null ? p.x : 0;
    textNode.y = p.y != null ? p.y : 0;
    if (p.name) textNode.name = p.name;
    if (p.fillColor) textNode.fills = [makeSolidPaint(p.fillColor)];
    (parent as any).appendChild(textNode);
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: textNode.id, name: textNode.name, type: textNode.type, bounds: getBounds(textNode) },
    };
  },

  "import_image": async (request) => {
    const p = request.params || {};
    if (!p.imageData && !p.imageUrl) {
      throw new Error("imageData (base64) or imageUrl is required");
    }

    // A URL avoids pushing the whole file through the WebSocket as base64,
    // which is the expensive half of placing an image.
    const image = p.imageUrl
      ? await figma.createImageAsync(p.imageUrl)
      : figma.createImage(base64ToBytes(p.imageData));
    const fill: any = {
      type: "IMAGE",
      imageHash: image.hash,
      scaleMode: p.scaleMode || (p.crop ? "CROP" : "FILL"),
    };
    if (p.crop) fill.imageTransform = cropToTransform(p.crop);
    if (p.filters) fill.filters = { ...p.filters };

    // Painting an existing node beats making a rectangle beside it: an avatar
    // or a hero slot is usually already there, waiting for its picture.
    if (p.nodeId) {
      const target = await figma.getNodeByIdAsync(p.nodeId) as any;
      if (!target) throw new Error(`Node not found: ${p.nodeId}`);
      if (!("fills" in target)) throw new Error(`Node ${p.nodeId} does not support fills`);
      target.fills = p.mode === "append" ? [...target.fills, fill] : [fill];
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: target.id, name: target.name, type: target.type, bounds: getBounds(target) },
      };
    }

    const parent = await getParentNode(p.parentId);
    const rect = figma.createRectangle();
    // Fall back to the image's own size rather than a fixed 200x200, so an
    // imported picture is not silently squashed into a square.
    const { width, height } = await imageSize(image, p);
    rect.resize(width, height);
    rect.x = p.x != null ? p.x : 0;
    rect.y = p.y != null ? p.y : 0;
    if (p.name) rect.name = p.name;
    rect.fills = [fill];
    (parent as any).appendChild(rect);
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: rect.id, name: rect.name, type: rect.type, bounds: getBounds(rect) },
    };
  },

  "create_component": async (request) => {
    const p = request.params || {};
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeId is required");
    const node = await figma.getNodeByIdAsync(nodeId) as any;
    if (!node) throw new Error(`Node not found: ${nodeId}`);
    if (node.type !== "FRAME") throw new Error(`Node ${nodeId} is not a FRAME — only frames can be converted to components`);

    const parent = node.parent as any;
    const index = parent.children.indexOf(node);

    const component = figma.createComponent();
    component.name = p.name || node.name;
    component.resize(node.width, node.height);
    component.x = node.x;
    component.y = node.y;
    component.fills = node.fills as Paint[];
    component.strokes = node.strokes as Paint[];
    if (node.cornerRadius != null && node.cornerRadius !== figma.mixed) {
      component.cornerRadius = node.cornerRadius as number;
    }
    if (node.layoutMode && node.layoutMode !== "NONE") {
      component.layoutMode = node.layoutMode;
      component.paddingTop = node.paddingTop;
      component.paddingRight = node.paddingRight;
      component.paddingBottom = node.paddingBottom;
      component.paddingLeft = node.paddingLeft;
      component.itemSpacing = node.itemSpacing;
      component.primaryAxisAlignItems = node.primaryAxisAlignItems;
      component.counterAxisAlignItems = node.counterAxisAlignItems;
    }
    // Move children from frame into component
    for (const child of [...node.children]) {
      component.appendChild(child);
    }
    parent.insertChild(index, component);
    node.remove();

    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: component.id, name: component.name, type: component.type, bounds: getBounds(component) },
    };
  },

  "create_section": async (request) => {
    const p = request.params || {};
    const section = figma.createSection();
    if (p.name) section.name = p.name;
    if (p.x != null) section.x = p.x;
    if (p.y != null) section.y = p.y;
    if (p.width != null || p.height != null) {
      section.resizeWithoutConstraints(p.width || section.width, p.height || section.height);
    }
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: section.id, name: section.name, type: section.type, bounds: getBounds(section) },
    };
  },
};

export const handleWriteCreateRequest = async (request: any): Promise<any> => {
  const handler = writeCreateHandlers[request.type];
  return handler ? handler(request) : null;
};

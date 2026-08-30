import { getBounds } from "./serializers";
import { makeSolidPaint, getParentNode, applyAutoLayout, makeGradientPaint, makeLayoutGrid } from "./write-helpers";
import { HandlerMap } from "./dispatch";
import { reportProgress, stepProgress } from "./progress";
import { loadFonts } from "./fonts";

// What Figma itself falls back to when a node's font is mixed.
const FALLBACK_FONT = { family: "Inter", style: "Regular" };

const REORDER_ORDERS = ["bringToFront", "sendToBack", "bringForward", "sendBackward"];

// Directly assignable node properties, with the label used in the
// "does not support X" message the eight predecessor tools produced.
const SIMPLE_NODE_PROPS: Array<{ key: string; label: string }> = [
  { key: "visible", label: "visibility" },
  { key: "locked", label: "locking" },
  { key: "opacity", label: "opacity" },
  { key: "rotation", label: "rotation" },
  { key: "blendMode", label: "blend mode" },
  { key: "isMask", label: "masking" },
  { key: "maskType", label: "mask type" },
  // Stroke geometry. These describe the line itself rather than its paint, so
  // they live on the node, not in a Paint — which is why they belong here and
  // not on set_paint, whose arguments all describe one paint.
  { key: "strokeWeight", label: "stroke weight" },
  { key: "strokeAlign", label: "stroke alignment" },
  { key: "strokeCap", label: "stroke caps" },
  { key: "strokeJoin", label: "stroke joins" },
  { key: "strokeMiterLimit", label: "stroke miter limit" },
  { key: "dashPattern", label: "dash pattern" },
];

const reorderIndex = (order: string, currentIndex: number, siblingCount: number): number => {
  switch (order) {
    case "bringToFront": return siblingCount - 1;
    case "sendToBack": return 0;
    case "bringForward": return Math.min(currentIndex + 1, siblingCount - 1);
    case "sendBackward": return Math.max(currentIndex - 1, 0);
    default: return currentIndex;
  }
};

/**
 * Apply every requested property to one node, collecting per-property outcomes.
 * A node can support opacity but not rotation, so failures are reported against
 * the individual property rather than the whole node.
 */
const applyNodeProperties = (n: any, p: any) => {
  const applied: Record<string, any> = {};
  const errors: Record<string, string> = {};

  for (const { key, label } of SIMPLE_NODE_PROPS) {
    if (p[key] === undefined) continue;
    if (!(key in n)) {
      errors[key] = `Node does not support ${label}`;
      continue;
    }
    // Figma rejects some values for a node's current shape rather than
    // ignoring them — a dash pattern with a negative length, a stroke cap on a
    // node whose caps are mixed. Reporting that against the one property keeps
    // the others in the same call applied, which is what the per-property
    // errors above already promise.
    try {
      n[key] = p[key];
      applied[key] = n[key];
    } catch (e: any) {
      errors[key] = e?.message ?? `Could not set ${label}`;
    }
  }

  if (p.constraints !== undefined) {
    if (!("constraints" in n)) {
      errors.constraints = "Node does not support constraints";
    } else {
      const updated: any = { ...n.constraints };
      if (p.constraints.horizontal) updated.horizontal = p.constraints.horizontal;
      if (p.constraints.vertical) updated.vertical = p.constraints.vertical;
      n.constraints = updated;
      applied.constraints = n.constraints;
    }
  }

  if (p.order !== undefined) {
    const parent = n.parent as any;
    if (!parent || !("children" in parent)) {
      errors.order = "Node has no reorderable parent";
    } else {
      const siblings = parent.children as any[];
      const newIndex = reorderIndex(p.order, siblings.indexOf(n), siblings.length);
      parent.insertChild(newIndex, n);
      applied.index = newIndex;
    }
  }

  return { applied, errors };
};

// set_paint replaced set_fills, set_gradient_fills and set_strokes on the MCP
// surface. The three implementations stay separate below; only the surface
// merged. `type` names the kind of paint, which the gradient implementation
// reads directly and the solid ones do not take at all.
/**
 * Text settings that belong to the whole node rather than a range.
 *
 * Range-level styling lives in set_text_ranges; these have no range form in
 * Figma at all, so they stay with the tool that owns the node's text.
 */
const applyParagraphProperties = (node: any, p: any) => {
  if (p.textAutoResize) node.textAutoResize = p.textAutoResize;
  if (p.textTruncation) node.textTruncation = p.textTruncation;
  // maxLines only means anything with truncation on, and null clears the cap.
  if (p.maxLines !== undefined) {
    node.maxLines = p.maxLines === null ? null : Number(p.maxLines);
  }
  if (p.paragraphSpacing != null) node.paragraphSpacing = Number(p.paragraphSpacing);
  if (p.paragraphIndent != null) node.paragraphIndent = Number(p.paragraphIndent);
  if (p.textAlignHorizontal) node.textAlignHorizontal = p.textAlignHorizontal;
  if (p.textAlignVertical) node.textAlignVertical = p.textAlignVertical;
};

export const writeModifyHandlers: HandlerMap = {
  "set_paint": async (request) => {
  const { target = "fill", type, ...rest } = request.params || {};
  let action: string;
  let params: any;
  if (type === "SOLID") {
    action = target === "stroke" ? "set_strokes" : "set_fills";
    params = rest;
  } else if (type === "GRADIENT_LINEAR" || type === "GRADIENT_RADIAL") {
    if (target === "stroke") throw new Error("gradients can only target fill, not stroke");
    action = "set_gradient_fills";
    params = { ...rest, type };
  } else {
    throw new Error(`type must be SOLID, GRADIENT_LINEAR, or GRADIENT_RADIAL, got: ${type}`);
  }
  const result = await handleWriteModifyRequest({ ...request, type: action, params });
  // Answer under the name the caller used, not the one we delegated to.
  return { ...result, type: request.type }
  },

  "set_node_properties": async (request) => {
    const p = request.params || {};
    const nodeIds = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");

    const requested = [...SIMPLE_NODE_PROPS.map(s => s.key), "constraints", "order"];
    if (!requested.some(key => p[key] !== undefined)) {
      throw new Error(`at least one property is required: ${requested.join(", ")}`);
    }
    if (p.order !== undefined && !REORDER_ORDERS.includes(p.order)) {
      throw new Error(`order must be ${REORDER_ORDERS.join(", ")}`);
    }

    const results: any[] = [];
    for (const nid of nodeIds) {
      const n = await figma.getNodeByIdAsync(nid) as any;
      if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
      const { applied, errors } = applyNodeProperties(n, p);
      const entry: any = { nodeId: nid, applied };
      if (Object.keys(errors).length > 0) entry.errors = errors;
      results.push(entry);
    }
    figma.commitUndo();
    return { type: request.type, requestId: request.requestId, data: { results } };
  },

  "set_text": async (request) => {
    const p = request.params || {};
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeId is required");
    const node = await figma.getNodeByIdAsync(nodeId);
    if (!node) throw new Error(`Node not found: ${nodeId}`);
    if (node.type !== "TEXT") throw new Error(`Node ${nodeId} is not a TEXT node`);
    const fontName = typeof node.fontName === "symbol"
      ? { family: "Inter", style: "Regular" }
      : node.fontName;
    await loadFonts([fontName]);
    // text is optional now that this tool also carries paragraph settings — a
    // call that only changes the wrap mode should not blank the node.
    if (p.text != null) node.characters = p.text;
    applyParagraphProperties(node, p);
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: node.id, name: node.name, characters: node.characters },
    };
  },

  "set_gradient_fills": async (request) => {
    const p = request.params || {};
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeId is required");
    const node = await figma.getNodeByIdAsync(nodeId);
    if (!node) throw new Error(`Node not found: ${nodeId}`);
    if (!("fills" in node)) throw new Error(`Node ${nodeId} does not support fills`);
    const newFill = makeGradientPaint(p.type, p.stops, p.geometry, p.opacity);
    if (p.mode === "append") {
      const existing = Array.isArray((node as any).fills) ? [...(node as any).fills] : [];
      (node as any).fills = [...existing, newFill];
    } else {
      (node as any).fills = [newFill];
    }
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: node.id, name: node.name },
    };
  },

  "set_fills": async (request) => {
    const p = request.params || {};
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeId is required");
    const node = await figma.getNodeByIdAsync(nodeId);
    if (!node) throw new Error(`Node not found: ${nodeId}`);
    if (!("fills" in node)) throw new Error(`Node ${nodeId} does not support fills`);
    const newFill = makeSolidPaint(p.color, p.opacity != null ? p.opacity : undefined);
    if (p.mode === "append") {
      // Mixed fills come back as figma.mixed, a symbol, which cannot be spread.
      const existing = Array.isArray((node as any).fills) ? [...(node as any).fills] : [];
      (node as any).fills = [...existing, newFill];
    } else {
      (node as any).fills = [newFill];
    }
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: node.id, name: node.name },
    };
  },

  "set_strokes": async (request) => {
    const p = request.params || {};
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeId is required");
    const node = await figma.getNodeByIdAsync(nodeId);
    if (!node) throw new Error(`Node not found: ${nodeId}`);
    if (!("strokes" in node)) throw new Error(`Node ${nodeId} does not support strokes`);
    const newStroke = makeSolidPaint(p.color);
    if (p.mode === "append") {
      // As with fills, mixed strokes are a symbol and cannot be spread.
      const existing = Array.isArray((node as any).strokes) ? [...(node as any).strokes] : [];
      (node as any).strokes = [...existing, newStroke];
    } else {
      (node as any).strokes = [newStroke];
    }
    if (p.strokeWeight != null) (node as any).strokeWeight = p.strokeWeight;
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: node.id, name: node.name },
    };
  },

  "move_nodes": async (request) => {
    const p = request.params || {};
    const nodeIds = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");
    const results: any[] = [];
    for (const nid of nodeIds) {
      const n = await figma.getNodeByIdAsync(nid) as any;
      if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
      if (!("x" in n)) { results.push({ nodeId: nid, error: "Node does not support position" }); continue; }
      if (p.x != null) n.x = p.x;
      if (p.y != null) n.y = p.y;
      results.push({ nodeId: nid, x: n.x, y: n.y });
    }
    figma.commitUndo();
    return { type: request.type, requestId: request.requestId, data: { results } };
  },

  "resize_nodes": async (request) => {
    const p = request.params || {};
    const nodeIds = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");
    const results: any[] = [];
    for (const nid of nodeIds) {
      const n = await figma.getNodeByIdAsync(nid) as any;
      if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
      if (!("resize" in n)) { results.push({ nodeId: nid, error: "Node does not support resize" }); continue; }
      const w = p.width != null ? p.width : n.width;
      const h = p.height != null ? p.height : n.height;
      n.resize(w, h);
      results.push({ nodeId: nid, width: n.width, height: n.height });
    }
    figma.commitUndo();
    return { type: request.type, requestId: request.requestId, data: { results } };
  },

  "clone_node": async (request) => {
    const p = request.params || {};
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeId is required");
    const node = await figma.getNodeByIdAsync(nodeId) as any;
    if (!node) throw new Error(`Node not found: ${nodeId}`);
    const clone = node.clone();
    if (p.x != null) clone.x = p.x;
    if (p.y != null) clone.y = p.y;
    if (p.parentId) {
      const parent = await getParentNode(p.parentId);
      (parent as any).appendChild(clone);
    }
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: clone.id, name: clone.name, type: clone.type, bounds: getBounds(clone) },
    };
  },

  "set_corner_radius": async (request) => {
    const p = request.params || {};
    const nodeIds = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");
    const results: any[] = [];
    for (const nid of nodeIds) {
      const n = await figma.getNodeByIdAsync(nid) as any;
      if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
      if (!("cornerRadius" in n)) { results.push({ nodeId: nid, error: "Node does not support corner radius" }); continue; }
      if (p.cornerRadius != null) n.cornerRadius = p.cornerRadius;
      if (p.topLeftRadius != null) n.topLeftRadius = p.topLeftRadius;
      if (p.topRightRadius != null) n.topRightRadius = p.topRightRadius;
      if (p.bottomLeftRadius != null) n.bottomLeftRadius = p.bottomLeftRadius;
      if (p.bottomRightRadius != null) n.bottomRightRadius = p.bottomRightRadius;
      results.push({ nodeId: nid, cornerRadius: n.cornerRadius });
    }
    figma.commitUndo();
    return { type: request.type, requestId: request.requestId, data: { results } };
  },

  "set_layout_grids": async (request) => {
    const p = request.params || {};
    const nodeIds = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");
    if (!Array.isArray(p.grids)) throw new Error("grids is required and must be an array");
    // An empty array is how a caller removes the grids a frame already has.
    const grids = p.grids.map(makeLayoutGrid);

    const results: any[] = [];
    for (const nid of nodeIds) {
      const node = await figma.getNodeByIdAsync(nid) as any;
      if (!node) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
      if (!("layoutGrids" in node)) {
        results.push({ nodeId: nid, error: `${node.type} does not support layout grids` });
        continue;
      }
      node.layoutGrids = p.mode === "append" ? [...node.layoutGrids, ...grids] : grids;
      results.push({ nodeId: nid, gridCount: node.layoutGrids.length });
    }
    figma.commitUndo();
    return { type: request.type, requestId: request.requestId, data: { results } };
  },

  // Absorbed set_layout_sizing, which was these same arguments over many nodes.
  // A row of siblings that should all FILL is the case that made the plural form
  // worth having: one round trip per sibling was the alternative.
  "set_auto_layout": async (request) => {
    const p = request.params || {};
    const nodeIds = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");

    const results: any[] = [];
    for (const nid of nodeIds) {
      const n = await figma.getNodeByIdAsync(nid) as any;
      if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
      // Components, component sets and instances carry auto layout too, and the
      // FRAME-only check used to turn a perfectly valid call on a component into
      // an error. Ask for the property instead of the type.
      //
      // layoutMode is the frame's own layout; a node that only sizes itself
      // inside its parent's layout does not have it, and that is what the
      // layoutSizing/layoutAlign/layoutGrow arguments are for.
      if (!("layoutMode" in n) && !("layoutSizingHorizontal" in n)) {
        results.push({
          nodeId: nid,
          error: `Node ${nid} is a ${n.type} and does not support auto layout — expected a FRAME, COMPONENT, COMPONENT_SET, or INSTANCE`,
        });
        continue;
      }
      try {
        applyAutoLayout(n, p);
        results.push({
          nodeId: nid,
          name: n.name,
          layoutSizingHorizontal: n.layoutSizingHorizontal,
          layoutSizingVertical: n.layoutSizingVertical,
        });
      } catch (e: any) {
        // One sibling that cannot FILL — because its parent has no auto layout
        // — must not undo the ones that could.
        results.push({ nodeId: nid, error: e.message });
      }
    }
    figma.commitUndo();
    return { type: request.type, requestId: request.requestId, data: { results } };
  },

  // Writes the presets, it does not export. get_screenshot and save_screenshots
  // still do the exporting; this is what a designer sees under Export in the
  // right-hand panel, and what a handoff pipeline reads.
  "set_export_settings": async (request) => {
    const p = request.params || {};
    const nodeIds = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");
    if (!Array.isArray(p.settings)) throw new Error("settings must be an array of export presets");

    const presets = p.settings.map((entry: any) => {
      const preset: any = { format: entry.format };
      if (entry.suffix != null) preset.suffix = String(entry.suffix);
      if (entry.contentsOnly != null) preset.contentsOnly = !!entry.contentsOnly;
      if (entry.useAbsoluteBounds != null) preset.useAbsoluteBounds = !!entry.useAbsoluteBounds;
      // SVG and PDF have no raster size, and Figma rejects a constraint on them.
      if (entry.constraint && entry.format !== "SVG" && entry.format !== "PDF") {
        preset.constraint = {
          type: entry.constraint.type,
          value: Number(entry.constraint.value),
        };
      }
      return preset;
    });

    const results: any[] = [];
    for (const nid of nodeIds) {
      const n = await figma.getNodeByIdAsync(nid) as any;
      if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
      if (!("exportSettings" in n)) {
        results.push({ nodeId: nid, error: `Node ${nid} is a ${n.type} and cannot be exported` });
        continue;
      }
      try {
        n.exportSettings = presets;
        results.push({ nodeId: nid, name: n.name, exportSettings: n.exportSettings.length });
      } catch (e: any) {
        results.push({ nodeId: nid, error: e.message });
      }
    }
    figma.commitUndo();
    return { type: request.type, requestId: request.requestId, data: { results } };
  },

  "reparent_nodes": async (request) => {
    const p = request.params || {};
    const nodeIds = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");
    if (!p.parentId) throw new Error("parentId is required");
    const newParent = await figma.getNodeByIdAsync(p.parentId) as any;
    if (!newParent) throw new Error(`Parent not found: ${p.parentId}`);
    if (!("appendChild" in newParent)) throw new Error(`Node ${p.parentId} cannot contain children`);
    const results: any[] = [];
    for (const nid of nodeIds) {
      const n = await figma.getNodeByIdAsync(nid) as any;
      if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
      try {
        newParent.appendChild(n);
        results.push({ nodeId: nid, newParentId: p.parentId });
      } catch (e: any) {
        results.push({ nodeId: nid, error: e.message });
      }
    }
    figma.commitUndo();
    return { type: request.type, requestId: request.requestId, data: { results } };
  },

  "batch_rename_nodes": async (request) => {
    const p = request.params || {};
    const nodeIds = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");
    const results: any[] = [];
    for (const nid of nodeIds) {
      const n = await figma.getNodeByIdAsync(nid) as any;
      if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
      const oldName: string = n.name;
      let newName = oldName;
      // Absorbed rename_node: a literal name wins outright, and the schema
      // rejects it alongside a substitution rather than defining an order.
      if (p.name !== undefined) {
        n.name = p.name;
        results.push({ nodeId: nid, oldName, name: p.name });
        continue;
      }
      if (p.find !== undefined && p.replace !== undefined) {
        if (p.useRegex) {
          try {
            const regex = new RegExp(p.find, p.regexFlags || "g");
            newName = newName.replace(regex, p.replace);
          } catch (e: any) {
            results.push({ nodeId: nid, error: `Invalid regex: ${e.message}` }); continue;
          }
        } else {
          newName = newName.split(p.find).join(p.replace);
        }
      }
      if (p.prefix) newName = p.prefix + newName;
      if (p.suffix) newName = newName + p.suffix;
      n.name = newName;
      results.push({ nodeId: nid, oldName, name: newName });
    }
    figma.commitUndo();
    return { type: request.type, requestId: request.requestId, data: { results } };
  },

  "find_replace_text": async (request) => {
    const p = request.params || {};
    if (!p.find) throw new Error("find is required");
    if (p.replace === undefined) throw new Error("replace is required");
    const rootNodeId = request.nodeIds && request.nodeIds[0];
    const root: any = rootNodeId
      ? await figma.getNodeByIdAsync(rootNodeId)
      : figma.currentPage;
    if (!root) throw new Error(`Root node not found: ${rootNodeId}`);
    const textNodes: any[] = [];
    const collect = (node: any) => {
      if (node.type === "TEXT") textNodes.push(node);
      if ("children" in node) (node.children as any[]).forEach(collect);
    };
    collect(root);
    const results: any[] = [];
    // Collected first, written after every font has loaded.
    const pending: Array<{ node: any; originalText: string; newText: string }> = [];
    // A find-and-replace over a whole page is the write that most often runs
    // past the server's timeout, and it is the one write where the caller
    // cannot guess how much there is to do.
    const reportEvery = Math.max(1, Math.floor(textNodes.length / 20));
    for (let i = 0; i < textNodes.length; i++) {
      const tn = textNodes[i];
      if (textNodes.length > 20 && i % reportEvery === 0) {
        await reportProgress(
          request.requestId,
          stepProgress(i, textNodes.length),
          `Scanned ${i}/${textNodes.length} text nodes, ${results.length} changed`,
        );
      }
      const originalText: string = tn.characters;
      let newText: string;
      if (p.useRegex) {
        try {
          const regex = new RegExp(p.find, p.regexFlags || "g");
          newText = originalText.replace(regex, p.replace);
        } catch (e: any) {
          results.push({ nodeId: tn.id, nodeName: tn.name, error: `Invalid regex: ${e.message}` });
          continue;
        }
      } else {
        newText = originalText.split(p.find).join(p.replace);
      }
      if (newText !== originalText) {
        pending.push({ node: tn, originalText, newText });
      }
    }

    // Every font first, then every write. Loading inside the loop meant one node
    // in a font the file lacks aborted the run after the nodes before it had
    // already been rewritten — and reported one missing font per attempt.
    await loadFonts(
      pending.map(({ node }) =>
        typeof node.fontName === "symbol" ? FALLBACK_FONT : node.fontName,
      ),
    );
    for (const { node, originalText, newText } of pending) {
      node.characters = newText;
      results.push({ nodeId: node.id, nodeName: node.name, oldText: originalText, newText });
    }
    figma.commitUndo();
    const successCount = results.filter((r: any) => !r.error).length;
    return { type: request.type, requestId: request.requestId, data: { replaced: successCount, results } };
  },
};

export const handleWriteModifyRequest = async (request: any): Promise<any> => {
  const handler = writeModifyHandlers[request.type];
  return handler ? handler(request) : null;
};

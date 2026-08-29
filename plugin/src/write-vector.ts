import { HandlerMap } from "./dispatch";
import { getBounds } from "./serializers";
import { getParentNode, hexToRgb } from "./write-helpers";

// Vector and boolean geometry.
//
// None of this existed, which meant no icons: an icon is almost always two
// shapes combined, a stroke turned into a fill, or a path from an SVG. Every
// operation here is destructive in the same way — it consumes the shapes it
// takes and hands back one new node — so the pipeline's rollback log cannot
// reverse them. save_version_checkpoint is the way back.

const BOOLEAN_OPERATIONS: Record<string, string> = {
  UNION: "union",
  SUBTRACT: "subtract",
  INTERSECT: "intersect",
  EXCLUDE: "exclude",
};

/**
 * The parent the result should land in.
 *
 * Figma needs one, and the only answer that does not move the user's work is
 * the parent the shapes already share. Shapes from different parents have no
 * such answer, so the caller is asked rather than guessed at.
 */
export function commonParent(nodes: any[]): any {
  const parent = nodes[0]?.parent;
  if (!parent) {
    throw new Error(`Node ${nodes[0]?.id} has no parent — it may already have been removed`);
  }
  for (const node of nodes) {
    if (node.parent !== parent) {
      throw new Error(
        "All nodes must share a parent — move them together first, or the result would have to jump out of one of them",
      );
    }
  }
  return parent;
}

const resolveNodes = async (nodeIds: string[], minimum: number, what: string) => {
  if (nodeIds.length < minimum) {
    throw new Error(`${what} needs at least ${minimum} node(s), got ${nodeIds.length}`);
  }
  const found = await Promise.all(nodeIds.map((id) => figma.getNodeByIdAsync(id)));
  const missing = nodeIds.filter((_, i) => found[i] === null);
  if (missing.length > 0) throw new Error(`Node not found: ${missing.join(", ")}`);
  return found as any[];
};

const describe = (node: any) => ({
  id: node.id,
  name: node.name,
  type: node.type,
  bounds: getBounds(node),
});

export const writeVectorHandlers: HandlerMap = {
  "boolean_operation": async (request) => {
    const p = request.params || {};
    const operation = String(p.operation || "").toUpperCase();
    const method = BOOLEAN_OPERATIONS[operation];
    if (!method) {
      throw new Error(
        `operation must be UNION, SUBTRACT, INTERSECT, or EXCLUDE, got: ${p.operation}`,
      );
    }
    const nodes = await resolveNodes(request.nodeIds || [], 2, operation);
    // SUBTRACT and EXCLUDE read the order they are given — the first shape is
    // the one the others are cut out of — so the ids stay in the caller's order.
    const parent = commonParent(nodes);
    const result = (figma as any)[method](nodes, parent);
    if (p.name) result.name = p.name;
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: describe(result),
    };
  },

  "flatten_nodes": async (request) => {
    const p = request.params || {};
    const nodes = await resolveNodes(request.nodeIds || [], 1, "flatten_nodes");
    const parent = commonParent(nodes);
    const result = figma.flatten(nodes, parent);
    if (p.name) result.name = p.name;
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: describe(result),
    };
  },

  "outline_stroke": async (request) => {
    const nodes = await resolveNodes(request.nodeIds || [], 1, "outline_stroke");
    const outlined: any[] = [];
    const skipped: { id: string; reason: string }[] = [];
    for (const node of nodes) {
      if (typeof node.outlineStroke !== "function") {
        skipped.push({ id: node.id, reason: `${node.type} has no stroke to outline` });
        continue;
      }
      const result = node.outlineStroke();
      // Figma returns null for a node whose stroke is empty or zero-width.
      // That is a no-op, not a failure, and one such node in a batch should not
      // lose the caller the ones that worked.
      if (result) outlined.push(describe(result));
      else skipped.push({ id: node.id, reason: "no visible stroke" });
    }
    if (outlined.length > 0) figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { outlined, skipped },
    };
  },

  "create_vector": async (request) => {
    const p = request.params || {};
    if (!p.svg) throw new Error("svg is required");
    const parent = await getParentNode(p.parentId);
    // createNodeFromSvg always wraps its paths in a frame. A single-path icon
    // is far more useful as the vector itself, so the wrapper is unwrapped when
    // it holds exactly one child.
    const imported = figma.createNodeFromSvg(p.svg);
    let node: any = imported;
    if (imported.children.length === 1) {
      node = imported.children[0];
      imported.parent?.appendChild(node);
      imported.remove();
    }
    (parent as any).appendChild(node);
    if (p.name) node.name = p.name;
    if (p.x != null) node.x = Number(p.x);
    if (p.y != null) node.y = Number(p.y);
    if (p.width != null && p.height != null) {
      node.resize(Number(p.width), Number(p.height));
    }
    if (p.fillColor && "fills" in node) {
      const { r, g, b, a } = hexToRgb(p.fillColor);
      node.fills = [{ type: "SOLID", color: { r, g, b }, opacity: a }];
    }
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: describe(node),
    };
  },
};

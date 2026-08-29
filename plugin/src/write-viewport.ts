import { HandlerMap } from "./dispatch";
import { getBounds } from "./serializers";

// Selection and viewport. Neither touches the document, so neither commits an
// undo step — putting a camera move on the undo stack would make Ctrl+Z scroll
// the canvas instead of reversing the user's last real edit.

/** The page a node lives on, or null for a node detached from the tree. */
export const pageOf = (node: any): any => {
  let current = node;
  while (current && current.type !== "PAGE") {
    if (current.type === "DOCUMENT") return null;
    current = current.parent;
  }
  return current ?? null;
};

export const writeViewportHandlers: HandlerMap = {
  "set_selection": async (request) => {
    const p = request.params || {};
    const select = p.select !== false;
    const zoom = p.zoom !== false;
    const nodeIds: string[] = request.nodeIds || [];

    // No ids at all means "deselect everything". Only meaningful for select;
    // there is nothing to zoom to.
    if (nodeIds.length === 0) {
      if (!select) {
        throw new Error("nodeIds is required unless select is true and you are clearing the selection");
      }
      figma.currentPage.selection = [];
      return {
        type: request.type,
        requestId: request.requestId,
        data: { selected: [], pageId: figma.currentPage.id, pageName: figma.currentPage.name, cleared: true },
      };
    }

    const found = await Promise.all(nodeIds.map((id) => figma.getNodeByIdAsync(id)));
    const missing = nodeIds.filter((_, i) => found[i] === null);
    if (missing.length > 0) {
      throw new Error(`Node not found: ${missing.join(", ")}`);
    }

    const nodes = found as any[];
    for (const node of nodes) {
      if (node.type === "PAGE" || node.type === "DOCUMENT") {
        throw new Error(`Node ${node.id} is a ${node.type} and cannot be selected — use manage_page to switch pages`);
      }
    }

    // A selection belongs to one page, so mixed pages cannot be honoured. Say
    // so rather than silently selecting whichever subset happens to survive.
    const pages = nodes.map(pageOf);
    const detached = nodes.filter((_, i) => pages[i] === null);
    if (detached.length > 0) {
      throw new Error(`Node ${detached[0].id} is not on any page`);
    }
    const targetPage = pages[0];
    if (pages.some((page) => page.id !== targetPage.id)) {
      throw new Error(
        "All nodes must be on the same page — a Figma selection cannot span pages",
      );
    }

    if (targetPage.id !== figma.currentPage.id) {
      await figma.setCurrentPageAsync(targetPage);
    }

    if (select) figma.currentPage.selection = nodes;
    if (zoom) figma.viewport.scrollAndZoomIntoView(nodes);

    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        selected: nodes.map((node) => ({
          id: node.id,
          name: node.name,
          type: node.type,
          bounds: getBounds(node),
        })),
        pageId: targetPage.id,
        pageName: targetPage.name,
        zoomed: zoom,
      },
    };
  },
};

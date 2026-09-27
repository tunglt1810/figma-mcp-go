import { HandlerMap } from "./dispatch";

function buildReaction(r: any): Reaction {
  // `actions` (plural array) is the current API. `action` (singular) is deprecated.
  // Accept either form, so callers do not need to care about the difference.
  const actions: Action[] = r.actions ?? (r.action != null ? [r.action] : []);
  return { trigger: r.trigger ?? null, actions } as Reaction;
}

// The MCP framework may pass array params as a JSON string, so parse carefully.
function parseArray(v: any): any[] {
  if (Array.isArray(v)) return v;
  if (typeof v === "string") {
    try { return JSON.parse(v); } catch { return []; }
  }
  return [];
}

// setReactionsAsync is required when documentAccess is "dynamic-page".
// Fall back to direct assignment only when setReactionsAsync is missing (older Figma).
async function setReactions(node: any, reactions: Reaction[]): Promise<void> {
  if (typeof node.setReactionsAsync === "function") {
    await node.setReactionsAsync(reactions);
    return;
  }
  try {
    node.reactions = reactions;
  } catch (e) {
    throw new Error(`Failed to set reactions: ${e instanceof Error ? e.message : String(e)}`);
  }
}

export const writePrototypeHandlers: HandlerMap = {
  // This took over remove_reactions. Removing everything is already
  // set_reactions(replace, []). But removing #1 and #3 by index needed a
  // get, filter, set round trip, so removeIndices is what was carried over.
  "set_reactions": async (request) => {
    const p = request.params || {};
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeId is required");
    const node = await figma.getNodeByIdAsync(nodeId);
    if (!node) throw new Error(`Node not found: ${nodeId}`);
    if (!("reactions" in node)) throw new Error(`Node ${nodeId} does not support reactions`);

    const current: Reaction[] = (node as any).reactions;
    let final: Reaction[];
    let removing = false;

    if (p.removeIndices !== undefined) {
      removing = true;
      const indices = parseArray(p.removeIndices);
      // An empty array means remove everything, not remove nothing. That is
      // what remove_reactions did, and it is easy to flip by mistake in a rewrite.
      if (indices.length === 0) {
        final = [];
      } else {
        const toRemove = new Set<number>(indices);
        final = current.filter((_: any, i: number) => !toRemove.has(i));
      }
    } else {
      const incoming: Reaction[] = parseArray(p.reactions).map(buildReaction);
      final = p.mode === "append" ? [...current, ...incoming] : incoming;
    }

    await setReactions(node, final);
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        id: node.id,
        name: (node as any).name,
        reactionCount: final.length,
        ...(removing ? { removed: current.length - final.length } : {}),
      },
    };
  },
};

export const handleWritePrototypeRequest = async (request: any): Promise<any> => {
  const handler = writePrototypeHandlers[request.type];
  return handler ? handler(request) : null;
};

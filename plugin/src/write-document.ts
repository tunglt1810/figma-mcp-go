import { HandlerMap } from "./dispatch";
import { CODEGEN_KEY, PLUGIN_DATA_NAMESPACE, normalizeLanguage } from "./codegen";

// File-level operations. The pipeline's rollback lives in memory and dies with
// the plugin, so a checkpoint here is the only safety net that survives a
// crashed run or a change noticed an hour later.

const getNode = async (nodeId: string | undefined) => {
  if (!nodeId) throw new Error("nodeId is required");
  const node = await figma.getNodeByIdAsync(nodeId);
  if (!node) throw new Error(`Node not found: ${nodeId}`);
  if (typeof (node as any).setSharedPluginData !== "function") {
    throw new Error(`Node ${nodeId} cannot carry plugin data`);
  }
  return node as any;
};

export const writeDocumentHandlers: HandlerMap = {
  "set_codegen_result": async (request) => {
    const p = request.params || {};
    const node = await getNode(request.nodeIds && request.nodeIds[0]);
    const blocks = Array.isArray(p.blocks) ? p.blocks : [];
    for (const block of blocks) {
      if (!block || typeof block.code !== "string" || block.code === "") {
        throw new Error("every block needs a non-empty code string");
      }
    }

    // Shared rather than private plugin data: the point is that the whole team
    // sees the code in Dev Mode, not only the machine that generated it.
    const stored = blocks.map((block: any) => ({
      title: String(block.title ?? "Code"),
      language: normalizeLanguage(block.language),
      code: block.code,
    }));
    node.setSharedPluginData(
      PLUGIN_DATA_NAMESPACE,
      CODEGEN_KEY,
      // An empty array would still read back as "has code"; an empty string is
      // how shared plugin data is cleared.
      stored.length > 0 ? JSON.stringify(stored) : "",
    );

    // A node carrying code gets a button in Figma's own UI, so a designer can
    // reopen the plugin from the layer rather than hunting through the menu.
    if (typeof node.setRelaunchData === "function") {
      node.setRelaunchData(
        stored.length > 0 ? { open: `${stored.length} code block(s) from your AI tool` } : {},
      );
    }

    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        id: node.id,
        name: node.name,
        blocks: stored.map((block: any) => ({
          title: block.title,
          language: block.language,
          bytes: block.code.length,
        })),
        cleared: stored.length === 0,
      },
    };
  },

  "manage_plugin_data": async (request) => {
    const p = request.params || {};
    const action = String(p.action || "");
    const node = await getNode(request.nodeIds && request.nodeIds[0]);
    const namespace = p.namespace || PLUGIN_DATA_NAMESPACE;

    if (action === "keys") {
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, namespace, keys: node.getSharedPluginDataKeys(namespace) },
      };
    }

    if (!p.key) throw new Error(`key is required when action is ${action}`);

    if (action === "get") {
      const value = node.getSharedPluginData(namespace, p.key);
      return {
        type: request.type,
        requestId: request.requestId,
        // "" is both "not set" and "set to empty" in Figma's model; reporting
        // null for it is the closest honest answer.
        data: { id: node.id, namespace, key: p.key, value: value === "" ? null : value },
      };
    }

    if (action === "set" || action === "delete") {
      if (action === "set" && typeof p.value !== "string") {
        throw new Error("value is required and must be a string when action is set");
      }
      node.setSharedPluginData(namespace, p.key, action === "set" ? p.value : "");
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, namespace, key: p.key, ...(action === "set" ? { stored: true } : { deleted: true }) },
      };
    }

    throw new Error(`action must be get, set, delete, or keys, got: ${p.action}`);
  },

  "save_version_checkpoint": async (request) => {
    const p = request.params || {};
    if (!p.title) throw new Error("title is required");
    if (typeof (figma as any).saveVersionHistoryAsync !== "function") {
      throw new Error(
        "Version history is not available in this editor — it requires a Figma design file",
      );
    }
    const version = await (figma as any).saveVersionHistoryAsync(
      p.title,
      p.description,
    );
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        id: version && version.id ? version.id : null,
        title: p.title,
        description: p.description ?? null,
      },
    };
  },
};

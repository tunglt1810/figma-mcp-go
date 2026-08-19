import { serializeNode, getBounds, serializeStyles, serializeNodeProperties, isMixed, deduplicateStyles } from "./serializers";
import { HandlerMap } from "./dispatch";

export const readDocumentHandlers: HandlerMap = {
  "get_document": async (request) => {
    const raw = await serializeNode(figma.currentPage);
    const { tree, globalVars } = deduplicateStyles(raw);
    return {
      type: request.type,
      requestId: request.requestId,
      data: globalVars ? { ...tree, globalVars } : tree,
    };
  },

  "get_selection": async (request) => {
    return {
      type: request.type,
      requestId: request.requestId,
      data: await Promise.all(figma.currentPage.selection.map((node) => serializeNode(node))),
    };

  },

  "get_node": async (request) => {
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeIds is required for get_node");
    const node = await figma.getNodeByIdAsync(nodeId);
    if (!node || node.type === "DOCUMENT")
      throw new Error(`Node not found: ${nodeId}`);
    return {
      type: request.type,
      requestId: request.requestId,
      data: await serializeNode(node),
    };
  },

  "get_instance_overrides": async (request) => {
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeIds is required for get_instance_overrides");
    const node = await figma.getNodeByIdAsync(nodeId);
    if (!node) throw new Error(`Node not found: ${nodeId}`);
    if (node.type !== "INSTANCE") throw new Error(`Node ${nodeId} is not an INSTANCE`);
    
    const componentProperties: Record<string, any> = {};
    if (node.componentProperties) {
      for (const [key, prop] of Object.entries(node.componentProperties)) {
        componentProperties[key] = {
          type: prop.type,
          value: prop.value,
        };
        if (prop.preferredValues) {
          componentProperties[key].preferredValues = prop.preferredValues;
        }
      }
    }
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        id: node.id,
        name: node.name,
        componentProperties,
      },
    };
  },

  "get_nodes_info": async (request) => {
    if (!request.nodeIds || request.nodeIds.length === 0)
      throw new Error("nodeIds is required for get_nodes_info");
    const nodes = await Promise.all(
      request.nodeIds.map((id: string) => figma.getNodeByIdAsync(id)),
    );
    return {
      type: request.type,
      requestId: request.requestId,
      data: await Promise.all(
        nodes
          .filter((n) => n !== null && n.type !== "DOCUMENT")
          .map((n) => serializeNode(n)),
      ),
    };
  },

  "get_design_context": async (request) => {
    const depth =
      request.params && request.params.depth != null
        ? request.params.depth
        : 2;
    const detail = (request.params && request.params.detail) || "full";
    const dedupeComponents = !!(request.params && request.params.dedupeComponents);
    const componentDefs = new Map<string, any>();

    const serializeForDetail = async (n: any) => {
      const base = { id: n.id, name: n.name, type: n.type, bounds: getBounds(n) };
      if (detail === "minimal") return base;
      // "full" reports the same node properties plus geometry and children, and runs
      // its own style lookups. Building styles here first would double every
      // getStyleByIdAsync round trip on the way to throwing the result away.
      if (detail !== "compact") return await serializeNode(n);
      const styles = await serializeStyles(n);
      const result: any = Object.assign({}, base, serializeNodeProperties(n));
      if (Object.keys(styles).length > 0) result.styles = styles;
      return result;
    };

    const extractInstanceOverrides = async (
      instanceNode: any,
      componentNode: any,
    ): Promise<{ id: string; name: string; type: string; characters?: string; mainComponentId?: string | null; visible?: boolean; opacity?: number; fills?: any }[]> => {
      const overrides: any[] = [];
      if (!instanceNode?.children || !componentNode?.children) return overrides;
      for (let i = 0; i < instanceNode.children.length; i++) {
        const instChild = instanceNode.children[i];
        const compChild = componentNode.children[i];
        if (!instChild || !compChild) continue;

        // Detect property overrides (visible, opacity, fills) for all node types
        const propChanges: any = {};
        if ("visible" in instChild && "visible" in compChild && instChild.visible !== compChild.visible) {
          propChanges.visible = instChild.visible;
        }
        if ("opacity" in instChild && "opacity" in compChild && instChild.opacity !== compChild.opacity) {
          propChanges.opacity = instChild.opacity;
        }
        if ("fills" in instChild && "fills" in compChild && !isMixed(instChild.fills) && !isMixed(compChild.fills)) {
          if (JSON.stringify(instChild.fills) !== JSON.stringify(compChild.fills)) {
            propChanges.fills = instChild.fills;
          }
        }

        if (instChild.type === "TEXT") {
          const override: any = { id: instChild.id, name: instChild.name, type: "TEXT" };
          let hasChange = false;
          if (instChild.characters !== compChild.characters) {
            override.characters = instChild.characters;
            hasChange = true;
          }
          if (Object.keys(propChanges).length > 0) {
            Object.assign(override, propChanges);
            hasChange = true;
          }
          if (hasChange) overrides.push(override);
          continue;
        }

        if (instChild.type === "INSTANCE") {
          const [nestedMc, compMc] = await Promise.all([
            instChild.getMainComponentAsync(),
            compChild.type === "INSTANCE" ? compChild.getMainComponentAsync() : Promise.resolve(null),
          ]);
          if (nestedMc?.id !== compMc?.id) {
            const override: any = { id: instChild.id, name: instChild.name, type: "INSTANCE", mainComponentId: nestedMc?.id ?? null };
            if (Object.keys(propChanges).length > 0) Object.assign(override, propChanges);
            overrides.push(override);
            continue;
          }
          if (Object.keys(propChanges).length > 0) {
            overrides.push({ id: instChild.id, name: instChild.name, type: "INSTANCE", mainComponentId: nestedMc?.id ?? null, ...propChanges });
          }
          if (nestedMc) overrides.push(...await extractInstanceOverrides(instChild, nestedMc));
          continue;
        }

        if (Object.keys(propChanges).length > 0) {
          overrides.push({ id: instChild.id, name: instChild.name, type: instChild.type, ...propChanges });
        }
        if ("children" in instChild) {
          overrides.push(...await extractInstanceOverrides(instChild, compChild));
        }
      }
      return overrides;
    };

    const serializeWithDepth = async (node: any, currentDepth: number): Promise<any> => {
      if (dedupeComponents && node.type === "INSTANCE") {
        const mc = await node.getMainComponentAsync();
        if (mc && !componentDefs.has(mc.id)) {
          componentDefs.set(mc.id, await serializeNode(mc));
        }
        const props: Record<string, any> = {};
        if (node.componentProperties) {
          for (const [key, prop] of Object.entries(node.componentProperties)) {
            props[key] = (prop as any).value;
          }
        }
        const result: any = {
          id: node.id,
          name: node.name,
          type: node.type,
          bounds: getBounds(node),
          mainComponentId: mc?.id ?? null,
        };
        if (Object.keys(props).length > 0) result.componentProperties = props;
        const overrides = await extractInstanceOverrides(node, mc);
        if (overrides.length > 0) result.overrides = overrides;
        return result;
      }
      if (detail === "full") {
        const serialized = await serializeNode(node);
        if (currentDepth >= depth && serialized.children) {
          return Object.assign({}, serialized, {
            children: undefined,
            childCount: node.children ? node.children.length : 0,
          });
        }
        if (serialized.children) {
          const childNodes = await Promise.all(
            serialized.children.map((child: any) =>
              figma.getNodeByIdAsync(child.id),
            ),
          );
          const serializedChildren = await Promise.all(
            childNodes
              .filter((n) => n !== null && n.type !== "DOCUMENT")
              .map((n) => serializeWithDepth(n, currentDepth + 1)),
          );
          return Object.assign({}, serialized, { children: serializedChildren });
        }
        return serialized;
      }

      const serialized = await serializeForDetail(node);
      const hasChildren = "children" in node && node.children.length > 0;
      if (!hasChildren) return serialized;
      if (currentDepth >= depth) {
        return Object.assign({}, serialized, { childCount: node.children.length });
      }
      const serializedChildren = await Promise.all(
        node.children
          .filter((n: any) => n.type !== "DOCUMENT")
          .map((n: any) => serializeWithDepth(n, currentDepth + 1)),
      );
      return Object.assign({}, serialized, { children: serializedChildren });
    };

    const selection = figma.currentPage.selection;
    const rawContextNodes =
      selection.length > 0
        ? await Promise.all(
            selection.map((node) => serializeWithDepth(node, 0)),
          )
        : [await serializeWithDepth(figma.currentPage, 0)];
    const { tree: dedupedNodes, globalVars } = deduplicateStyles({ children: rawContextNodes });
    const contextNodes = (dedupedNodes as any).children;
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        fileName: figma.root.name,
        currentPage: {
          id: figma.currentPage.id,
          name: figma.currentPage.name,
        },
        selectionCount: selection.length,
        context: contextNodes,
        ...(componentDefs.size > 0 ? { componentDefs: Object.fromEntries(componentDefs) } : {}),
        ...(globalVars ? { globalVars } : {}),
      },
    };
  },

  "get_metadata": async (request) => {
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        fileName: figma.root.name,
        currentPageId: figma.currentPage.id,
        currentPageName: figma.currentPage.name,
        pageCount: figma.root.children.length,
        pages: figma.root.children.map((page) => ({
          id: page.id,
          name: page.name,
        })),
      },
    };

  },

  "get_pages": async (request) => {
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        currentPageId: figma.currentPage.id,
        pages: figma.root.children.map((page) => ({
          id: page.id,
          name: page.name,
        })),
      },
    };

  },

  "get_viewport": async (request) => {
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        center: { x: figma.viewport.center.x, y: figma.viewport.center.y },
        zoom: figma.viewport.zoom,
        bounds: {
          x: figma.viewport.bounds.x,
          y: figma.viewport.bounds.y,
          width: figma.viewport.bounds.width,
          height: figma.viewport.bounds.height,
        },
      },
    };

  },

  "get_fonts": async (request) => {
    const fontMap = new Map<string, any>();
    const collectFonts = (n: any) => {
      if (n.type === "TEXT") {
        const fontName = n.fontName;
        if (typeof fontName !== "symbol" && fontName) {
          const key = `${fontName.family}::${fontName.style}`;
          if (!fontMap.has(key)) {
            fontMap.set(key, { family: fontName.family, style: fontName.style, nodeCount: 0 });
          }
          fontMap.get(key).nodeCount++;
        }
      }
      if ("children" in n) n.children.forEach(collectFonts);
    };
    collectFonts(figma.currentPage);
    const fonts = Array.from(fontMap.values()).sort((a, b) => b.nodeCount - a.nodeCount);
    return {
      type: request.type,
      requestId: request.requestId,
      data: { count: fonts.length, fonts },
    };
  },

  "search_nodes": async (request) => {
    const query = request.params && request.params.query
      ? request.params.query.toLowerCase()
      : "";
    const scopeNodeId = request.params && request.params.nodeId;
    const types = request.params && request.params.types ? request.params.types : [];
    const limit = request.params && request.params.limit ? request.params.limit : 50;
    const root = scopeNodeId
      ? await figma.getNodeByIdAsync(scopeNodeId)
      : figma.currentPage;
    if (!root) throw new Error(`Node not found: ${scopeNodeId}`);
    const results: any[] = [];
    const search = async (n: any) => {
      if (results.length >= limit) return;
      if (n !== root) {
        const nameMatch = !query || n.name.toLowerCase().includes(query);
        const typeMatch = types.length === 0 || types.includes(n.type);
        if (nameMatch && typeMatch) {
          results.push({
            id: n.id,
            name: n.name,
            type: n.type,
            bounds: getBounds(n),
          });
        }
      }
      if (results.length < limit && "children" in n) {
        for (const child of n.children) await search(child);
      }
    };
    await search(root);
    return {
      type: request.type,
      requestId: request.requestId,
      data: { count: results.length, nodes: results },
    };
  },

  "get_reactions": async (request) => {
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeId is required for get_reactions");
    const node = await figma.getNodeByIdAsync(nodeId);
    if (!node || node.type === "DOCUMENT") throw new Error(`Node not found: ${nodeId}`);
    const reactions = "reactions" in node ? node.reactions : [];
    return {
      type: request.type,
      requestId: request.requestId,
      data: { nodeId: node.id, name: node.name, reactions },
    };
  },

  "scan_text_nodes": async (request) => {
    const nodeId = request.params && request.params.nodeId;
    if (!nodeId) throw new Error("nodeId is required for scan_text_nodes");
    const root = await figma.getNodeByIdAsync(nodeId);
    if (!root) throw new Error(`Node not found: ${nodeId}`);
    const textNodes: any[] = [];
    const findText = async (n: any) => {
      if (n.type === "TEXT") {
        textNodes.push({
          id: n.id,
          name: n.name,
          characters: n.characters,
          fontSize: isMixed(n.fontSize) ? "mixed" : n.fontSize,
          fontName: isMixed(n.fontName) ? "mixed" : n.fontName,
        });
      }
      if ("children" in n)
        for (const child of n.children) await findText(child);
    };
    figma.ui.postMessage({
      type: "progress_update",
      requestId: request.requestId,
      progress: 10,
      message: "Scanning text nodes...",
    });
    await new Promise((r) => setTimeout(r, 0));
    await findText(root);
    return {
      type: request.type,
      requestId: request.requestId,
      data: { count: textNodes.length, textNodes },
    };
  },

  "scan_nodes_by_types": async (request) => {
    const nodeId = request.params && request.params.nodeId;
    const types =
      request.params && request.params.types ? request.params.types : [];
    if (!nodeId)
      throw new Error("nodeId is required for scan_nodes_by_types");
    if (types.length === 0)
      throw new Error("types must be a non-empty array");
    const root = await figma.getNodeByIdAsync(nodeId);
    if (!root) throw new Error(`Node not found: ${nodeId}`);
    const matchingNodes: any[] = [];
    const findByTypes = async (n: any) => {
      if ("visible" in n && !n.visible) return;
      if (types.includes(n.type)) {
        matchingNodes.push({
          id: n.id,
          name: n.name,
          type: n.type,
          bbox: {
            x: "x" in n ? n.x : 0,
            y: "y" in n ? n.y : 0,
            width: "width" in n ? n.width : 0,
            height: "height" in n ? n.height : 0,
          },
        });
      }
      if ("children" in n)
        for (const child of n.children) await findByTypes(child);
    };
    figma.ui.postMessage({
      type: "progress_update",
      requestId: request.requestId,
      progress: 10,
      message: `Scanning for types: ${types.join(", ")}...`,
    });
    await new Promise((r) => setTimeout(r, 0));
    await findByTypes(root);
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        count: matchingNodes.length,
        matchingNodes,
        searchedTypes: types,
      },
    };
  },
};

export const handleReadDocumentRequest = async (request: any): Promise<any> => {
  const handler = readDocumentHandlers[request.type];
  return handler ? handler(request) : null;
};

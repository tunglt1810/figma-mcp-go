import { HandlerMap } from "./dispatch";
export const writeComponentsHandlers: HandlerMap = {
  "swap_component": async (request) => {
    const p = request.params || {};
    const nodeId = request.nodeIds && request.nodeIds[0];
    if (!nodeId) throw new Error("nodeId is required");
    if (!p.componentId) throw new Error("componentId is required");
    const node = await figma.getNodeByIdAsync(nodeId);
    if (!node) throw new Error(`Node not found: ${nodeId}`);
    if (node.type !== "INSTANCE") throw new Error(`Node ${nodeId} is not a component INSTANCE`);
    const component = await figma.getNodeByIdAsync(p.componentId);
    if (!component) throw new Error(`Component not found: ${p.componentId}`);
    if (component.type !== "COMPONENT") throw new Error(`Node ${p.componentId} is not a COMPONENT`);
    node.mainComponent = component;
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: node.id, name: node.name, componentId: component.id, componentName: component.name },
    };
  },

  "detach_instance": async (request) => {
    const nodeIds = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");
    const results: any[] = [];
    for (const nid of nodeIds) {
      const n = await figma.getNodeByIdAsync(nid);
      if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
      if (n.type !== "INSTANCE") { results.push({ nodeId: nid, error: "Node is not an INSTANCE" }); continue; }
      const frame = n.detachInstance();
      results.push({ nodeId: nid, newId: frame.id, name: frame.name });
    }
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { results },
    };
  },

  "delete_nodes": async (request) => {
    const nodeIds = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");
    const results: any[] = [];
    for (const nid of nodeIds) {
      const n = await figma.getNodeByIdAsync(nid);
      if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
      n.remove();
      results.push({ nodeId: nid, deleted: true });
    }
    figma.commitUndo();
    return { type: request.type, requestId: request.requestId, data: { results } };
  },

  "group_nodes": async (request) => {
    const p = request.params || {};
    const nodeIds = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");
    const nodes = await Promise.all(nodeIds.map((id: string) => figma.getNodeByIdAsync(id)));
    const validNodes = nodes.filter((n): n is SceneNode => n !== null && n.type !== "DOCUMENT" && n.type !== "PAGE");
    if (validNodes.length === 0) throw new Error("No valid scene nodes found");
    const parent = validNodes[0].parent;
    if (!parent) throw new Error("Nodes must have a parent");
    const group = figma.group(validNodes, parent as any);
    if (p.name) group.name = p.name;
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: group.id, name: group.name, type: group.type },
    };
  },

  "ungroup_nodes": async (request) => {
    const nodeIds = request.nodeIds || [];
    if (nodeIds.length === 0) throw new Error("nodeIds is required");
    const results: any[] = [];
    for (const nid of nodeIds) {
      const n = await figma.getNodeByIdAsync(nid);
      if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
      if (n.type !== "GROUP") { results.push({ nodeId: nid, error: "Node is not a GROUP" }); continue; }
      const group = n as GroupNode;
      const parent = group.parent as any;
      const index = parent.children.indexOf(group);
      const childIds: string[] = [];
      for (const child of [...group.children]) {
        parent.insertChild(index, child as SceneNode);
        childIds.push(child.id);
      }
      group.remove();
      results.push({ nodeId: nid, childIds });
    }
    figma.commitUndo();
    return { type: request.type, requestId: request.requestId, data: { results } };
  },

  "create_component_instance": async (request) => {
    const p = request.params || {};
    let baseComponent: ComponentNode | null = null;

    if (p.componentId) {
      const node = await figma.getNodeByIdAsync(p.componentId);
      if (!node) throw new Error(`Component not found: ${p.componentId}`);
      if (node.type === "COMPONENT_SET") {
        baseComponent = (node as ComponentSetNode).defaultVariant;
        if (!baseComponent && (node as ComponentSetNode).children.length > 0) {
          baseComponent = (node as ComponentSetNode).children[0] as ComponentNode;
        }
      } else if (node.type === "COMPONENT") {
        baseComponent = node as ComponentNode;
      } else {
        throw new Error(`Node ${p.componentId} is not a COMPONENT or COMPONENT_SET`);
      }
    } else if (p.componentKey) {
      baseComponent = await figma.importComponentByKeyAsync(p.componentKey);
    } else {
      throw new Error("componentId or componentKey is required");
    }

    if (!baseComponent) throw new Error("Could not resolve a ComponentNode to instantiate");

    const instance = baseComponent.createInstance();

    let parent: BaseNode = figma.currentPage;
    if (p.parentId) {
      const pNode = await figma.getNodeByIdAsync(p.parentId);
      if (!pNode) {
        instance.remove();
        throw new Error(`Parent not found: ${p.parentId}`);
      }
      parent = pNode;
    }
    if ("appendChild" in parent) {
      (parent as any).appendChild(instance);
    } else if (p.parentId) {
      instance.remove();
      throw new Error(`Parent node does not support children`);
    }

    if (p.x !== undefined && p.y !== undefined) {
      instance.x = p.x;
      instance.y = p.y;
    } else if (p.x === undefined && p.y === undefined && parent.type === "PAGE") {
      instance.x = figma.viewport.center.x - instance.width / 2;
      instance.y = figma.viewport.center.y - instance.height / 2;
    }

    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: instance.id, name: instance.name },
    };
  },

  "set_instance_overrides": async (request) => {
    const nodeId = request.nodeIds && request.nodeIds[0];
    const p = request.params || {};
    if (!nodeId) throw new Error("nodeId is required");
    if (!p.properties) throw new Error("properties object is required");
    const node = await figma.getNodeByIdAsync(nodeId);
    if (!node) throw new Error(`Node not found: ${nodeId}`);
    if (node.type !== "INSTANCE") throw new Error(`Node ${nodeId} is not an INSTANCE`);

    try {
      (node as InstanceNode).setProperties(p.properties);
    } catch (err: any) {
      throw new Error(`Failed to set properties: ${err.message || err}`);
    }
    
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: node.id, name: node.name, success: true },
    };
  },

  "create_connector": async (request) => {
    if (figma.editorType !== "figjam") {
      throw new Error("create_connector is only supported in FigJam files");
    }
    const p = request.params || {};
    let startPoint: any = null;
    let endPoint: any = null;
    
    if (p.startNodeId) {
      const startNode = await figma.getNodeByIdAsync(p.startNodeId);
      if (!startNode) throw new Error(`startNodeId not found: ${p.startNodeId}`);
      startPoint = { endpointNodeId: startNode.id, magnet: "AUTO" };
    } else if (p.startPosition) {
      startPoint = { position: p.startPosition };
    }
    
    if (p.endNodeId) {
      const endNode = await figma.getNodeByIdAsync(p.endNodeId);
      if (!endNode) throw new Error(`endNodeId not found: ${p.endNodeId}`);
      endPoint = { endpointNodeId: endNode.id, magnet: "AUTO" };
    } else if (p.endPosition) {
      endPoint = { position: p.endPosition };
    }
    
    const connector = figma.createConnector();
    if (startPoint) connector.connectorStart = startPoint;
    if (endPoint) connector.connectorEnd = endPoint;
    
    if (p.lineType) {
      connector.connectorLineType = p.lineType;
    }
    
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { id: connector.id },
    };
  },

  // Absorbed clear_annotations, which was this with an empty array — but over
  // many nodes, where this took one. Clearing ten nodes must not cost ten calls,
  // so the arity of the tool that clears is the one that survived.
  "set_annotations": async (request) => {
    const nodeIds = request.nodeIds || [];
    const p = request.params || {};
    if (nodeIds.length === 0) throw new Error("nodeIds is required");
    if (!Array.isArray(p.annotations)) throw new Error("annotations array is required");

    const results: any[] = [];
    for (const nid of nodeIds) {
      const n = await figma.getNodeByIdAsync(nid);
      if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
      if (!("annotations" in n)) {
        results.push({ nodeId: nid, error: `Node type ${n.type} does not support annotations` });
        continue;
      }
      try {
        (n as any).annotations = p.annotations;
        results.push({ nodeId: nid, success: true });
      } catch (e: any) {
        // Annotations need a paid Dev Mode seat, and Figma reports that by
        // throwing. One node's refusal is every node's, but reporting it per
        // node keeps the shape the same either way.
        results.push({ nodeId: nid, error: e.message });
      }
    }
    figma.commitUndo();
    return { type: request.type, requestId: request.requestId, data: { results } };
  },
};

export const handleWriteComponentRequest = async (request: any): Promise<any> => {
  const handler = writeComponentsHandlers[request.type];
  return handler ? handler(request) : null;
};

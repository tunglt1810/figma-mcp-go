export const handleWriteComponentRequest = async (request: any) => {
  switch (request.type) {
    case "swap_component": {
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
    }

    case "detach_instance": {
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
    }

    case "delete_nodes": {
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
    }

    case "navigate_to_page": {
      const p = request.params || {};
      let page: PageNode | undefined;
      if (p.pageId) {
        const found = await figma.getNodeByIdAsync(p.pageId);
        if (!found) throw new Error(`Page not found: ${p.pageId}`);
        if (found.type !== "PAGE") throw new Error(`Node ${p.pageId} is not a PAGE`);
        page = found as PageNode;
      } else if (p.pageName) {
        page = figma.root.children.find(pg => pg.name === p.pageName) as PageNode | undefined;
        if (!page) throw new Error(`Page not found with name: ${p.pageName}`);
      } else {
        throw new Error("pageId or pageName is required");
      }
      await figma.setCurrentPageAsync(page);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: page.id, name: page.name },
      };
    }

    case "group_nodes": {
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
    }

    case "ungroup_nodes": {
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
    }

    case "create_component_instance": {
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
    }

    case "set_instance_overrides": {
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
    }

    case "create_connector": {
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
    }

    case "set_annotations": {
      const nodeId = request.nodeIds && request.nodeIds[0];
      const p = request.params || {};
      if (!nodeId) throw new Error("nodeId is required");
      if (!Array.isArray(p.annotations)) throw new Error("annotations array is required");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      
      if (!("annotations" in node)) {
         throw new Error(`Node type ${node.type} does not support annotations`);
      }
      
      (node as any).annotations = p.annotations;
      
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, success: true },
      };
    }

    case "clear_annotations": {
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid);
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("annotations" in n)) { results.push({ nodeId: nid, error: "Node does not support annotations" }); continue; }
        (n as any).annotations = [];
        results.push({ nodeId: nid, success: true });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    default:
      return null;
  }
};

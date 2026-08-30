import { hexToRgb, makeSolidPaint } from "./write-helpers";
import { HandlerMap } from "./dispatch";

const parseVariableValue = (type: string, value: any): VariableValue => {
  if (type === "COLOR") {
    if (typeof value === "string") {
      const { r, g, b, a } = hexToRgb(value);
      return { r, g, b, a };
    }
    return value as RGBA;
  }
  if (type === "FLOAT") return typeof value === "number" ? value : parseFloat(String(value));
  if (type === "BOOLEAN") return value === true || value === "true";
  return String(value); // STRING
};

// manage_variable replaced the six single-purpose variable tools on the MCP
// surface. The implementations stay separate below; only the surface merged.
const VARIABLE_ACTIONS: Record<string, string> = {
  create_collection: "create_variable_collection",
  add_mode: "add_variable_mode",
  create: "create_variable",
  set_value: "set_variable_value",
  delete: "delete_variable",
  bind: "bind_variable_to_node",
};

export const writeVariablesHandlers: HandlerMap = {
  "manage_variable": async (request) => {
    const { action, ...params } = request.params || {};
    const type = VARIABLE_ACTIONS[action];
    if (!type) {
      throw new Error(
        `action must be ${Object.keys(VARIABLE_ACTIONS).join(", ")}, got: ${action}`,
      );
    }
    const result = await handleWriteVariableRequest({ ...request, type, params });
    // Answer under the name the caller used, not the one we delegated to.
    return { ...result, type: request.type };
  },

  "create_variable_collection": async (request) => {
    const p = request.params || {};
    if (!p.name) throw new Error("name is required");
    const collection = figma.variables.createVariableCollection(p.name);
    if (p.initialModeName && collection.modes.length > 0) {
      collection.renameMode(collection.modes[0].modeId, p.initialModeName);
    }
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        id: collection.id,
        name: collection.name,
        modes: collection.modes.map((m) => ({ modeId: m.modeId, name: m.name })),
      },
    };
  },

  "add_variable_mode": async (request) => {
    const p = request.params || {};
    if (!p.collectionId) throw new Error("collectionId is required");
    if (!p.modeName) throw new Error("modeName is required");
    const collection = await figma.variables.getVariableCollectionByIdAsync(p.collectionId);
    if (!collection) throw new Error(`Collection not found: ${p.collectionId}`);
    const modeId = collection.addMode(p.modeName);
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { collectionId: p.collectionId, modeId, modeName: p.modeName },
    };
  },

  "create_variable": async (request) => {
    const p = request.params || {};
    if (!p.name) throw new Error("name is required");
    if (!p.collectionId) throw new Error("collectionId is required");
    const validTypes = ["COLOR", "FLOAT", "STRING", "BOOLEAN"];
    if (!p.type || !validTypes.includes(p.type)) {
      throw new Error("type is required: COLOR, FLOAT, STRING, or BOOLEAN");
    }
    const collection = await figma.variables.getVariableCollectionByIdAsync(p.collectionId);
    if (!collection) throw new Error(`Collection not found: ${p.collectionId}`);
    const variable = figma.variables.createVariable(p.name, collection, p.type as VariableResolvedDataType);
    if (p.value != null && collection.modes.length > 0) {
      const modeId = collection.modes[0].modeId;
      variable.setValueForMode(modeId, parseVariableValue(p.type, p.value));
    }
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        id: variable.id,
        name: variable.name,
        resolvedType: variable.resolvedType,
        collectionId: p.collectionId,
      },
    };
  },

  "set_variable_value": async (request) => {
    const p = request.params || {};
    if (!p.variableId) throw new Error("variableId is required");
    if (!p.modeId) throw new Error("modeId is required");
    if (p.value == null) throw new Error("value is required");
    const variable = await figma.variables.getVariableByIdAsync(p.variableId);
    if (!variable) throw new Error(`Variable not found: ${p.variableId}`);
    variable.setValueForMode(p.modeId, parseVariableValue(variable.resolvedType, p.value));
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: { variableId: variable.id, name: variable.name, modeId: p.modeId },
    };
  },

  "delete_variable": async (request) => {
    const p = request.params || {};
    if (p.variableId) {
      const variable = await figma.variables.getVariableByIdAsync(p.variableId);
      if (!variable) throw new Error(`Variable not found: ${p.variableId}`);
      variable.remove();
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { variableId: p.variableId, deleted: true },
      };
    } else if (p.collectionId) {
      const collection = await figma.variables.getVariableCollectionByIdAsync(p.collectionId);
      if (!collection) throw new Error(`Collection not found: ${p.collectionId}`);
      collection.remove();
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { collectionId: p.collectionId, deleted: true },
      };
    } else {
      throw new Error("variableId or collectionId is required");
    }
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

export const handleWriteVariableRequest = async (request: any): Promise<any> => {
  const handler = writeVariablesHandlers[request.type];
  return handler ? handler(request) : null;
};

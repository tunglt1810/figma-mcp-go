import { HandlerMap } from "./dispatch";

// Component properties and variants.
//
// The plugin could make a component and swap an instance, but not define what a
// component exposes — so a design system could be read and never built. These
// four actions are the missing half: turn components into a variant set, then
// declare the properties instances can set, and point the component's own
// layers at them.

const PROPERTY_TYPES = ["BOOLEAN", "TEXT", "INSTANCE_SWAP", "VARIANT"];

/** The property reference field that matches a property type. */
const REFERENCE_FIELDS: Record<string, string> = {
  BOOLEAN: "visible",
  TEXT: "characters",
  INSTANCE_SWAP: "mainComponent",
};

/**
 * Property ids are `Name#123:4`, and Figma changes the id whenever the property
 * is renamed. Callers work in names, so a name is resolved against the current
 * definitions on every call rather than being remembered.
 */
export function resolvePropertyId(
  definitions: Record<string, any> | undefined,
  nameOrId: string,
): string {
  const keys = Object.keys(definitions ?? {});
  if (keys.includes(nameOrId)) return nameOrId;
  const matches = keys.filter((key) => key.split("#")[0] === nameOrId);
  if (matches.length === 1) return matches[0];
  if (matches.length > 1) {
    throw new Error(
      `Property name "${nameOrId}" is ambiguous — it matches ${matches.join(", ")}; pass the full id`,
    );
  }
  throw new Error(
    keys.length === 0
      ? `No component properties are defined; cannot find "${nameOrId}"`
      : `Property not found: "${nameOrId}". Defined: ${keys.join(", ")}`,
  );
}

const getComponentNode = async (nodeId: string | undefined) => {
  if (!nodeId) throw new Error("nodeId is required");
  const node = await figma.getNodeByIdAsync(nodeId);
  if (!node) throw new Error(`Node not found: ${nodeId}`);
  if (node.type !== "COMPONENT" && node.type !== "COMPONENT_SET") {
    throw new Error(
      `Node ${nodeId} is a ${node.type} — component properties live on a COMPONENT or COMPONENT_SET`,
    );
  }
  return node as any;
};

const definitionsOf = (node: any) => {
  const defs = node.componentPropertyDefinitions ?? {};
  const out: Record<string, any> = {};
  for (const [key, def] of Object.entries<any>(defs)) {
    out[key] = {
      name: key.split("#")[0],
      type: def.type,
      defaultValue: def.defaultValue,
      ...(def.variantOptions ? { variantOptions: def.variantOptions } : {}),
      ...(def.preferredValues ? { preferredValues: def.preferredValues } : {}),
    };
  }
  return out;
};

const ACTIONS: Record<string, (node: any, p: any) => Promise<any> | any> = {
  add: (node, p) => {
    if (!p.name) throw new Error("name is required to add a property");
    const type = String(p.type || "").toUpperCase();
    if (!PROPERTY_TYPES.includes(type)) {
      throw new Error(`type must be one of ${PROPERTY_TYPES.join(", ")}, got: ${p.type}`);
    }
    // A VARIANT property is what distinguishes the members of a set, so it has
    // nowhere to live on a lone component.
    if (type === "VARIANT" && node.type !== "COMPONENT_SET") {
      throw new Error(
        "A VARIANT property belongs to a COMPONENT_SET — use combine_as_variants first",
      );
    }
    if (p.defaultValue === undefined) {
      throw new Error("defaultValue is required to add a property");
    }
    const options = p.preferredValues ? { preferredValues: p.preferredValues } : undefined;
    const id = node.addComponentProperty(p.name, type, p.defaultValue, options);
    return { propertyId: id };
  },

  edit: (node, p) => {
    if (!p.property) throw new Error("property is required to edit a property");
    const id = resolvePropertyId(node.componentPropertyDefinitions, p.property);
    const changes: any = {};
    if (p.name != null) changes.name = p.name;
    if (p.defaultValue !== undefined) changes.defaultValue = p.defaultValue;
    if (p.preferredValues != null) changes.preferredValues = p.preferredValues;
    if (Object.keys(changes).length === 0) {
      throw new Error("edit needs at least one of name, defaultValue, or preferredValues");
    }
    // Renaming mints a new id, so the caller gets the one that is now valid.
    const newId = node.editComponentProperty(id, changes);
    return { propertyId: newId };
  },

  delete: (node, p) => {
    if (!p.property) throw new Error("property is required to delete a property");
    const id = resolvePropertyId(node.componentPropertyDefinitions, p.property);
    node.deleteComponentProperty(id);
    return { deleted: id };
  },

  bind: async (node, p) => {
    if (!p.property) throw new Error("property is required to bind a property");
    if (!p.targetNodeId) throw new Error("targetNodeId is required to bind a property");
    const id = resolvePropertyId(node.componentPropertyDefinitions, p.property);
    const type = node.componentPropertyDefinitions[id].type;
    const field = REFERENCE_FIELDS[type];
    if (!field) {
      throw new Error(
        `A ${type} property is chosen on the instance and has no layer to bind to`,
      );
    }
    const target = await figma.getNodeByIdAsync(p.targetNodeId);
    if (!target) throw new Error(`Node not found: ${p.targetNodeId}`);
    // Merged rather than replaced: a layer can carry one reference per field,
    // and overwriting the object would drop the others.
    (target as any).componentPropertyReferences = {
      ...((target as any).componentPropertyReferences ?? {}),
      [field]: id,
    };
    return { propertyId: id, boundTo: target.id, field };
  },
};

export const writeComponentPropertyHandlers: HandlerMap = {
  "manage_component_properties": async (request) => {
    const p = request.params || {};
    const action = String(p.action || "");
    const run = ACTIONS[action];
    if (!run) {
      throw new Error(`action must be add, edit, delete, or bind, got: ${p.action}`);
    }
    const node = await getComponentNode(request.nodeIds && request.nodeIds[0]);
    const result = await run(node, p);
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        id: node.id,
        name: node.name,
        ...result,
        properties: definitionsOf(node),
      },
    };
  },

  "combine_as_variants": async (request) => {
    const p = request.params || {};
    const nodeIds: string[] = request.nodeIds || [];
    if (nodeIds.length < 2) {
      throw new Error(`combine_as_variants needs at least 2 components, got ${nodeIds.length}`);
    }
    const found = await Promise.all(nodeIds.map((id) => figma.getNodeByIdAsync(id)));
    const missing = nodeIds.filter((_, i) => found[i] === null);
    if (missing.length > 0) throw new Error(`Node not found: ${missing.join(", ")}`);

    const components = found as any[];
    const wrong = components.filter((node) => node.type !== "COMPONENT");
    if (wrong.length > 0) {
      throw new Error(
        `Only COMPONENT nodes can be combined; ${wrong[0].id} is a ${wrong[0].type}`,
      );
    }
    const parent = components[0].parent;
    if (!parent) throw new Error(`Component ${components[0].id} has no parent`);
    for (const node of components) {
      if (node.parent !== parent) {
        throw new Error("All components must share a parent before they can be combined");
      }
    }

    const set = figma.combineAsVariants(components, parent);
    if (p.name) set.name = p.name;
    figma.commitUndo();
    return {
      type: request.type,
      requestId: request.requestId,
      data: {
        id: set.id,
        name: set.name,
        type: set.type,
        variantCount: set.children.length,
        properties: definitionsOf(set),
      },
    };
  },
};

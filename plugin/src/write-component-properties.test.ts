import { describe, expect, it, beforeEach } from "bun:test";
import { resolvePropertyId, writeComponentPropertyHandlers } from "./write-component-properties";

// ── resolvePropertyId ─────────────────────────────────────────────────────────

describe("resolvePropertyId", () => {
  const defs = { "Size#1:0": {}, "Disabled#2:0": {} };

  it("takes a full id as it is", () => {
    expect(resolvePropertyId(defs, "Size#1:0")).toBe("Size#1:0");
  });

  // Figma mints a new id on every rename, so callers work in names.
  it("resolves a bare name to its current id", () => {
    expect(resolvePropertyId(defs, "Size")).toBe("Size#1:0");
  });

  it("refuses an ambiguous name rather than picking one", () => {
    expect(() => resolvePropertyId({ "Size#1:0": {}, "Size#9:0": {} }, "Size"))
      .toThrow(/ambiguous/);
  });

  it("lists what is defined when the name is unknown", () => {
    expect(() => resolvePropertyId(defs, "Colour")).toThrow(/Defined: Size#1:0, Disabled#2:0/);
  });

  it("says so when nothing is defined at all", () => {
    expect(() => resolvePropertyId({}, "Size")).toThrow(/No component properties are defined/);
    expect(() => resolvePropertyId(undefined, "Size")).toThrow(/No component properties are defined/);
  });
});

// ── handlers ──────────────────────────────────────────────────────────────────

let nodes: Record<string, any>;
let component: any;
let combineCalls: any[];

const makeComponent = (id: string, parent: any, type = "COMPONENT") => {
  const node: any = {
    id,
    name: `Comp ${id}`,
    type,
    parent,
    componentPropertyDefinitions: {} as Record<string, any>,
    addComponentProperty(name: string, propType: string, defaultValue: any, options?: any) {
      const propId = `${name}#1:${Object.keys(this.componentPropertyDefinitions).length}`;
      this.componentPropertyDefinitions[propId] = {
        type: propType,
        defaultValue,
        ...(options?.preferredValues ? { preferredValues: options.preferredValues } : {}),
      };
      return propId;
    },
    editComponentProperty(propId: string, changes: any) {
      const existing = this.componentPropertyDefinitions[propId];
      delete this.componentPropertyDefinitions[propId];
      const newId = changes.name ? `${changes.name}#1:9` : propId;
      this.componentPropertyDefinitions[newId] = { ...existing, ...changes };
      return newId;
    },
    deleteComponentProperty(propId: string) {
      delete this.componentPropertyDefinitions[propId];
    },
  };
  nodes[id] = node;
  return node;
};

beforeEach(() => {
  nodes = {};
  combineCalls = [];
  const page = { id: "1:0", type: "PAGE" };
  nodes[page.id] = page;
  component = makeComponent("1:1", page);

  (globalThis as any).figma = {
    getNodeByIdAsync: async (id: string) => nodes[id] ?? null,
    combineAsVariants: (components: any[], parent: any) => {
      combineCalls.push({ ids: components.map(c => c.id), parent });
      const set = makeComponent("2:0", parent, "COMPONENT_SET");
      set.children = components;
      return set;
    },
    commitUndo: () => {},
  };
});

const manage = (params: any, nodeIds = ["1:1"]) =>
  writeComponentPropertyHandlers["manage_component_properties"]({
    type: "manage_component_properties",
    requestId: "r1",
    nodeIds,
    params,
  });

const combine = (nodeIds: string[], params: any = {}) =>
  writeComponentPropertyHandlers["combine_as_variants"]({
    type: "combine_as_variants",
    requestId: "r1",
    nodeIds,
    params,
  });

describe("manage_component_properties: add", () => {
  it("adds a boolean property and reports the definitions back", async () => {
    const res = await manage({ action: "add", name: "Disabled", type: "BOOLEAN", defaultValue: false });
    expect(res.data.propertyId).toBe("Disabled#1:0");
    expect(res.data.properties["Disabled#1:0"]).toMatchObject({ name: "Disabled", type: "BOOLEAN" });
  });

  it("accepts a lowercase type", async () => {
    const res = await manage({ action: "add", name: "Label", type: "text", defaultValue: "Hi" });
    expect(res.data.properties[res.data.propertyId].type).toBe("TEXT");
  });

  it("carries preferredValues through for an instance swap", async () => {
    const res = await manage({
      action: "add", name: "Icon", type: "INSTANCE_SWAP", defaultValue: "1:9",
      preferredValues: [{ type: "COMPONENT", key: "abc" }],
    });
    expect(res.data.properties[res.data.propertyId].preferredValues).toEqual([
      { type: "COMPONENT", key: "abc" },
    ]);
  });

  // A VARIANT property is what tells the members of a set apart, so a lone
  // component has nowhere to put it.
  it("refuses a VARIANT property on a lone component", async () => {
    expect(manage({ action: "add", name: "Size", type: "VARIANT", defaultValue: "Large" }))
      .rejects.toThrow(/COMPONENT_SET/);
  });

  it("allows a VARIANT property on a set", async () => {
    const set = makeComponent("3:0", null, "COMPONENT_SET");
    const res = await manage(
      { action: "add", name: "Size", type: "VARIANT", defaultValue: "Large" },
      [set.id],
    );
    expect(res.data.properties[res.data.propertyId].type).toBe("VARIANT");
  });

  it("requires a name, a known type, and a default", async () => {
    expect(manage({ action: "add", type: "BOOLEAN", defaultValue: false })).rejects.toThrow(/name is required/);
    expect(manage({ action: "add", name: "X", type: "COLOUR", defaultValue: 1 })).rejects.toThrow(/type must be one of/);
    expect(manage({ action: "add", name: "X", type: "BOOLEAN" })).rejects.toThrow(/defaultValue is required/);
  });
});

describe("manage_component_properties: edit", () => {
  beforeEach(async () => {
    await manage({ action: "add", name: "Disabled", type: "BOOLEAN", defaultValue: false });
  });

  it("renames by name and returns the id that is now valid", async () => {
    const res = await manage({ action: "edit", property: "Disabled", name: "Inactive" });
    expect(res.data.propertyId).toBe("Inactive#1:9");
    expect(res.data.properties["Inactive#1:9"].name).toBe("Inactive");
  });

  it("changes only the default when that is all that is given", async () => {
    const res = await manage({ action: "edit", property: "Disabled", defaultValue: true });
    expect(res.data.properties[res.data.propertyId].defaultValue).toBe(true);
  });

  it("needs something to change", async () => {
    expect(manage({ action: "edit", property: "Disabled" })).rejects.toThrow(/at least one of/);
  });
});

describe("manage_component_properties: delete", () => {
  it("removes the property", async () => {
    await manage({ action: "add", name: "Disabled", type: "BOOLEAN", defaultValue: false });
    const res = await manage({ action: "delete", property: "Disabled" });
    expect(res.data.deleted).toBe("Disabled#1:0");
    expect(res.data.properties).toEqual({});
  });
});

describe("manage_component_properties: bind", () => {
  it("points a layer's visibility at a boolean property", async () => {
    await manage({ action: "add", name: "Disabled", type: "BOOLEAN", defaultValue: false });
    const layer: any = { id: "1:5", name: "Badge" };
    nodes["1:5"] = layer;
    const res = await manage({ action: "bind", property: "Disabled", targetNodeId: "1:5" });
    expect(layer.componentPropertyReferences).toEqual({ visible: "Disabled#1:0" });
    expect(res.data.field).toBe("visible");
  });

  it("keeps references the layer already had", async () => {
    await manage({ action: "add", name: "Disabled", type: "BOOLEAN", defaultValue: false });
    const layer: any = { id: "1:5", componentPropertyReferences: { characters: "Label#9:9" } };
    nodes["1:5"] = layer;
    await manage({ action: "bind", property: "Disabled", targetNodeId: "1:5" });
    expect(layer.componentPropertyReferences).toEqual({
      characters: "Label#9:9",
      visible: "Disabled#1:0",
    });
  });

  it("says a VARIANT property has no layer to bind to", async () => {
    const set = makeComponent("3:0", null, "COMPONENT_SET");
    await manage({ action: "add", name: "Size", type: "VARIANT", defaultValue: "L" }, [set.id]);
    nodes["3:5"] = { id: "3:5" };
    expect(manage({ action: "bind", property: "Size", targetNodeId: "3:5" }, [set.id]))
      .rejects.toThrow(/no layer to bind to/);
  });

  it("reports a missing target", async () => {
    await manage({ action: "add", name: "Disabled", type: "BOOLEAN", defaultValue: false });
    expect(manage({ action: "bind", property: "Disabled", targetNodeId: "9:9" }))
      .rejects.toThrow(/Node not found/);
  });
});

describe("manage_component_properties: guards", () => {
  it("rejects an unknown action", async () => {
    expect(manage({ action: "rename" })).rejects.toThrow(/must be add, edit, delete, or bind/);
  });

  it("rejects a node that is not a component", async () => {
    nodes["1:7"] = { id: "1:7", type: "FRAME" };
    expect(manage({ action: "add", name: "X", type: "BOOLEAN", defaultValue: false }, ["1:7"]))
      .rejects.toThrow(/is a FRAME/);
  });
});

describe("combine_as_variants", () => {
  beforeEach(() => {
    makeComponent("1:2", nodes["1:0"]);
  });

  it("combines components into a set under their shared parent", async () => {
    const res = await combine(["1:1", "1:2"], { name: "Button" });
    expect(combineCalls[0].ids).toEqual(["1:1", "1:2"]);
    expect(res.data.type).toBe("COMPONENT_SET");
    expect(res.data.name).toBe("Button");
    expect(res.data.variantCount).toBe(2);
  });

  it("needs at least two components", async () => {
    expect(combine(["1:1"])).rejects.toThrow(/at least 2/);
  });

  it("refuses anything that is not a COMPONENT", async () => {
    nodes["1:3"] = { id: "1:3", type: "FRAME", parent: nodes["1:0"] };
    expect(combine(["1:1", "1:3"])).rejects.toThrow(/1:3 is a FRAME/);
  });

  it("refuses components from different parents", async () => {
    makeComponent("1:4", { id: "9:0", type: "FRAME" });
    expect(combine(["1:1", "1:4"])).rejects.toThrow(/share a parent/);
  });

  it("names a missing node", async () => {
    expect(combine(["1:1", "9:9"])).rejects.toThrow(/9:9/);
  });
});

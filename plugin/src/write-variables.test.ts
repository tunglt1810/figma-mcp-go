import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteVariableRequest } from "./write-variables";

// manage_variable replaced six tools on the MCP surface by dispatching on an
// action. The six implementations are unchanged, so what needs pinning is the
// routing: every action reaches the right one, an unknown action says so rather
// than doing nothing, and the answer comes back under the name the caller used.

let collections: Record<string, any>;
let variables: Record<string, any>;
let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;

const makeRequest = (type: string, nodeIds: string[], params: any) => ({
  type,
  requestId: "req-1",
  nodeIds,
  params,
});

const manage = (params: any, nodeIds: string[] = []) =>
  handleWriteVariableRequest(makeRequest("manage_variable", nodeIds, params));

beforeEach(() => {
  commitUndoCalled = false;
  collections = {};
  variables = {};
  mockNodes = {};

  const makeCollection = (id: string, name: string) => {
    const collection: any = {
      id,
      name,
      modes: [{ modeId: `${id}:m0`, name: "Mode 1" }],
      renameMode: (modeId: string, newName: string) => {
        const mode = collection.modes.find((m: any) => m.modeId === modeId);
        if (mode) mode.name = newName;
      },
      addMode: (modeName: string) => {
        const modeId = `${id}:m${collection.modes.length}`;
        collection.modes.push({ modeId, name: modeName });
        return modeId;
      },
      remove: () => { collection.removed = true; },
    };
    collections[id] = collection;
    return collection;
  };
  makeCollection("c1", "Brand");

  (globalThis as any).figma = {
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    commitUndo: () => { commitUndoCalled = true; },
    variables: {
      createVariableCollection: (name: string) => makeCollection(`c${Object.keys(collections).length + 1}`, name),
      getVariableCollectionByIdAsync: async (id: string) => collections[id] ?? null,
      getVariableByIdAsync: async (id: string) => variables[id] ?? null,
      createVariable: (name: string, collection: any, type: string) => {
        const variable: any = {
          id: `v${Object.keys(variables).length + 1}`,
          name,
          resolvedType: type,
          values: {} as Record<string, any>,
          setValueForMode: (modeId: string, value: any) => { variable.values[modeId] = value; },
          remove: () => { variable.removed = true; },
        };
        variables[variable.id] = variable;
        return variable;
      },
      setBoundVariableForPaint: (paint: any, _field: string, variable: any) => ({ ...paint, boundTo: variable.id }),
    },
  };
});

describe("manage_variable routing", () => {
  it("create_collection reaches create_variable_collection", async () => {
    const res = await manage({ action: "create_collection", name: "Semantic", initialModeName: "Light" });
    expect(res?.data.name).toBe("Semantic");
    expect(res?.data.modes[0].name).toBe("Light");
    expect(commitUndoCalled).toBe(true);
  });

  it("add_mode reaches add_variable_mode", async () => {
    const res = await manage({ action: "add_mode", collectionId: "c1", modeName: "Dark" });
    expect(res?.data.modeName).toBe("Dark");
    expect(collections["c1"].modes.map((m: any) => m.name)).toEqual(["Mode 1", "Dark"]);
  });

  it("create reaches create_variable and sets the first mode's value", async () => {
    const res = await manage({
      action: "create", name: "Color/Primary", collectionId: "c1", type: "COLOR", value: "#FF5733",
    });
    expect(res?.data.name).toBe("Color/Primary");
    expect(variables[res!.data.id].values["c1:m0"]).toBeDefined();
  });

  it("set_value reaches set_variable_value", async () => {
    await manage({ action: "create", name: "Spacing/MD", collectionId: "c1", type: "FLOAT" });
    const res = await manage({ action: "set_value", variableId: "v1", modeId: "c1:m0", value: "16" });
    expect(res?.data.variableId).toBe("v1");
    expect(variables["v1"].values["c1:m0"]).toBe(16);
  });

  it("delete removes a variable", async () => {
    await manage({ action: "create", name: "Spacing/MD", collectionId: "c1", type: "FLOAT" });
    const res = await manage({ action: "delete", variableId: "v1" });
    expect(res?.data.deleted).toBe(true);
    expect(variables["v1"].removed).toBe(true);
  });

  it("delete removes a whole collection", async () => {
    const res = await manage({ action: "delete", collectionId: "c1" });
    expect(res?.data.deleted).toBe(true);
    expect(collections["c1"].removed).toBe(true);
  });

  it("bind reaches bind_variable_to_node, which reads the node from nodeIds", async () => {
    await manage({ action: "create", name: "Color/Primary", collectionId: "c1", type: "COLOR" });
    mockNodes["1:1"] = { id: "1:1", name: "Card", fills: [] };
    const res = await manage(
      { action: "bind", variableId: "v1", field: "fillColor" },
      ["1:1"],
    );
    expect(res?.data.field).toBe("fillColor");
    expect(mockNodes["1:1"].fills[0].boundTo).toBe("v1");
  });

  // Delegating must not leak the name it delegated to.
  it("answers under manage_variable, not the handler it routed to", async () => {
    const res = await manage({ action: "create_collection", name: "Semantic" });
    expect(res?.type).toBe("manage_variable");
  });

  it("names the actions it knows when given one it does not", async () => {
    await expect(manage({ action: "rename" })).rejects.toThrow(/action must be .*create_collection/);
  });

  it("returns null for an unrecognised request type", async () => {
    expect(await handleWriteVariableRequest(makeRequest("unknown_op", [], {}))).toBeNull();
  });
});

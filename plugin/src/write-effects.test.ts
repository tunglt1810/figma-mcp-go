import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteStyleRequest } from "./write-styles";
import { handleWriteVariableRequest } from "./write-variables";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

beforeEach(() => {
  commitUndoCalled = false;
  mockNodes = {};
  (globalThis as any).figma = {
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    commitUndo: () => { commitUndoCalled = true; },
    getLocalPaintStylesAsync: async () => [],
    getLocalTextStylesAsync:  async () => [],
    getLocalEffectStylesAsync: async () => [],
    getLocalGridStylesAsync:  async () => [],
    getStyleByIdAsync: async () => null,
    loadFontAsync: async () => {},
    variables: {
      getVariableByIdAsync: async () => null,
      getVariableCollectionByIdAsync: async () => null,
    },
  };
});

// ── set_effects ───────────────────────────────────────────────────────────────

describe("set_effects", () => {
  it("sets a drop shadow effect", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Card", effects: [] };
    const res = await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "DROP_SHADOW", color: "#000000", opacity: 0.3, radius: 8, offsetX: 0, offsetY: 4 }],
    }));
    expect(mockNodes["1:1"].effects).toHaveLength(1);
    expect(mockNodes["1:1"].effects[0].type).toBe("DROP_SHADOW");
    expect(mockNodes["1:1"].effects[0].radius).toBe(8);
    expect(mockNodes["1:1"].effects[0].color.a).toBe(0.3);
    expect(res?.data.effectCount).toBe(1);
    expect(commitUndoCalled).toBe(true);
  });

  it("sets an inner shadow effect", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "INNER_SHADOW", radius: 4 }],
    }));
    expect(mockNodes["1:1"].effects[0].type).toBe("INNER_SHADOW");
  });

  it("sets a layer blur effect", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "LAYER_BLUR", radius: 10 }],
    }));
    expect(mockNodes["1:1"].effects[0].type).toBe("LAYER_BLUR");
    expect(mockNodes["1:1"].effects[0].radius).toBe(10);
  });

  it("sets a background blur effect", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "BACKGROUND_BLUR", radius: 20 }],
    }));
    expect(mockNodes["1:1"].effects[0].type).toBe("BACKGROUND_BLUR");
  });

  it("sets multiple effects at once", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [
        { type: "DROP_SHADOW", radius: 4 },
        { type: "LAYER_BLUR", radius: 2 },
      ],
    }));
    expect(mockNodes["1:1"].effects).toHaveLength(2);
  });

  it("clears effects when empty array provided", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [{ type: "DROP_SHADOW" }] };
    const res = await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], { effects: [] }));
    expect(mockNodes["1:1"].effects).toHaveLength(0);
    expect(res?.data.effectCount).toBe(0);
  });

  it("uses default values for shadow", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "DROP_SHADOW" }],
    }));
    const shadow = mockNodes["1:1"].effects[0];
    expect(shadow.radius).toBe(4);
    expect(shadow.offset.x).toBe(0);
    expect(shadow.offset.y).toBe(4);
    expect(shadow.color.a).toBe(0.25); // default opacity
  });

  it("throws for unknown effect type", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await expect(handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "GLOW" }],
    }))).rejects.toThrow("Unknown effect type");
  });

  // Figma's Effect union covers more than shadows and blurs, and get_nodes_info reports all
  // of it, so set_effects has to accept the whole set back.

  it("tags a plain blur with blurType NORMAL", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "LAYER_BLUR", radius: 6 }],
    }));
    // blurType is part of the BlurEffect union; leaving it out underspecifies the effect.
    expect(mockNodes["1:1"].effects[0].blurType).toBe("NORMAL");
  });

  it("sets a progressive blur with its start and end points", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{
        type: "LAYER_BLUR", blurType: "PROGRESSIVE", radius: 12, startRadius: 2,
        startOffset: { x: 0, y: 0.2 }, endOffset: { x: 0, y: 0.9 },
      }],
    }));
    const blur = mockNodes["1:1"].effects[0];
    expect(blur.blurType).toBe("PROGRESSIVE");
    expect(blur.startRadius).toBe(2);
    expect(blur.startOffset).toEqual({ x: 0, y: 0.2 });
    expect(blur.endOffset).toEqual({ x: 0, y: 0.9 });
  });

  it("sets a glass effect", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{
        type: "GLASS", radius: 4, depth: 20, dispersion: 0.5,
        lightAngle: -45, lightIntensity: 0.8, refraction: 0.8,
      }],
    }));
    expect(mockNodes["1:1"].effects[0]).toEqual({
      type: "GLASS", radius: 4, depth: 20, dispersion: 0.5,
      lightAngle: -45, lightIntensity: 0.8, refraction: 0.8, visible: true,
    });
  });

  it("sets a monotone noise effect", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "NOISE", color: "#ff0000", opacity: 0.4, noiseSize: 3, density: 0.7 }],
    }));
    const noise = mockNodes["1:1"].effects[0];
    expect(noise.noiseType).toBe("MONOTONE");
    expect(noise.color).toEqual({ r: 1, g: 0, b: 0, a: 0.4 });
    expect(noise.noiseSize).toBe(3);
    expect(noise.density).toBe(0.7);
  });

  // NoiseEffectBase declares blendMode, but the Figma runtime rejects the key outright:
  // "Unrecognized key(s) in object: 'blendMode'". Sending a default would make every
  // noise effect unwritable, so it only goes out when a caller asks for it.
  it("leaves blendMode off a noise effect unless asked", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "NOISE" }],
    }));
    expect(mockNodes["1:1"].effects[0]).not.toHaveProperty("blendMode");

    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "NOISE", blendMode: "MULTIPLY" }],
    }));
    expect(mockNodes["1:1"].effects[0].blendMode).toBe("MULTIPLY");
  });

  it("sets a duotone noise effect with its second colour", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "NOISE", noiseType: "DUOTONE", color: "#000000", secondaryColor: "#00ff00" }],
    }));
    expect(mockNodes["1:1"].effects[0].secondaryColor).toEqual({ r: 0, g: 1, b: 0, a: 1 });
  });

  // NOISE MULTITONE has an effect-level opacity that is not the colour's alpha, so the
  // two travel under different names.
  it("keeps multitone noise opacity separate from colour alpha", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "NOISE", noiseType: "MULTITONE", opacity: 0.25, noiseOpacity: 0.8 }],
    }));
    const noise = mockNodes["1:1"].effects[0];
    expect(noise.color.a).toBe(0.25);
    expect(noise.opacity).toBe(0.8);
  });

  it("sets a texture effect", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "TEXTURE", noiseSize: 5, radius: 3, clipToShape: true }],
    }));
    expect(mockNodes["1:1"].effects[0]).toEqual({
      type: "TEXTURE", noiseSize: 5, radius: 3, clipToShape: true, visible: true,
    });
  });

  it("carries showShadowBehindNode through on a drop shadow", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "DROP_SHADOW", showShadowBehindNode: true }],
    }));
    expect(mockNodes["1:1"].effects[0].showShadowBehindNode).toBe(true);
  });

  it("explains that a SHADER effect needs an imported shader", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await expect(handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "SHADER", id: "s1" }],
    }))).rejects.toThrow("importShaderById");
  });

  it("throws if nodeId is missing", async () => {
    await expect(handleWriteStyleRequest(makeRequest("set_effects", [], {
      effects: [{ type: "DROP_SHADOW" }],
    }))).rejects.toThrow("nodeId is required");
  });

  it("throws if effects is not an array", async () => {
    mockNodes["1:1"] = { id: "1:1", effects: [] };
    await expect(handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: "shadow",
    }))).rejects.toThrow("effects array is required");
  });

  it("throws if node not found", async () => {
    await expect(handleWriteStyleRequest(makeRequest("set_effects", ["9:9"], {
      effects: [{ type: "DROP_SHADOW" }],
    }))).rejects.toThrow("Node not found");
  });

  it("throws if node does not support effects", async () => {
    mockNodes["1:1"] = { id: "1:1" }; // no effects property
    await expect(handleWriteStyleRequest(makeRequest("set_effects", ["1:1"], {
      effects: [{ type: "DROP_SHADOW" }],
    }))).rejects.toThrow("does not support effects");
  });
});

// ── bind_variable_to_node – strokeColor ──────────────────────────────────────
//
// It moved to the variables module when manage_variable absorbed it.

describe("bind_variable_to_node strokeColor", () => {
  const mockVariable = { id: "v1", name: "color/primary", resolvedType: "COLOR" };
  const mockPaint = { type: "SOLID", color: { r: 1, g: 0, b: 0 } };

  beforeEach(() => {
    (globalThis as any).figma.variables = {
      getVariableByIdAsync: async (id: string) => id === "v1" ? mockVariable : null,
      setBoundVariableForPaint: (_paint: any, _field: string, _variable: any) => mockPaint,
    };
  });

  it("binds a variable to strokeColor", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame", strokes: [], setBoundVariable: () => {} };
    const res = await handleWriteVariableRequest(makeRequest("bind_variable_to_node", ["1:1"], {
      variableId: "v1", field: "strokeColor",
    }));
    expect(res?.data.field).toBe("strokeColor");
    expect(mockNodes["1:1"].strokes).toHaveLength(1);
    expect(commitUndoCalled).toBe(true);
  });

  it("uses existing stroke as base when binding strokeColor", async () => {
    const existingStroke = { type: "SOLID", color: { r: 0, g: 0, b: 0 } };
    mockNodes["1:1"] = { id: "1:1", strokes: [existingStroke], setBoundVariable: () => {} };
    await handleWriteVariableRequest(makeRequest("bind_variable_to_node", ["1:1"], {
      variableId: "v1", field: "strokeColor",
    }));
    expect(mockNodes["1:1"].strokes).toHaveLength(1);
  });

  it("throws if node does not support strokes", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Text" }; // no strokes
    await expect(handleWriteVariableRequest(makeRequest("bind_variable_to_node", ["1:1"], {
      variableId: "v1", field: "strokeColor",
    }))).rejects.toThrow("does not support strokes");
  });
});

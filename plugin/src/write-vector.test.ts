import { describe, expect, it, beforeEach } from "bun:test";
import { commonParent, writeVectorHandlers } from "./write-vector";

let nodes: Record<string, any>;
let page: any;
let opCalls: any[];
let created: any;

const makeNode = (id: string, parent: any, overrides: any = {}) => {
  const node: any = {
    id,
    name: `Node ${id}`,
    type: "RECTANGLE",
    parent,
    x: 0,
    y: 0,
    width: 10,
    height: 10,
    absoluteBoundingBox: { x: 0, y: 0, width: 10, height: 10 },
    ...overrides,
  };
  nodes[id] = node;
  return node;
};

const resultNode = (name: string, type: string) => ({
  id: "9:1",
  name,
  type,
  x: 0,
  y: 0,
  width: 10,
  height: 10,
  absoluteBoundingBox: { x: 0, y: 0, width: 10, height: 10 },
  children: [] as any[],
  fills: [] as any[],
  resize(w: number, h: number) {
    this.width = w;
    this.height = h;
  },
});

beforeEach(() => {
  nodes = {};
  opCalls = [];
  page = { id: "1:0", name: "Page 1", type: "PAGE", appendChild: (n: any) => { n.parent = page; } };
  nodes[page.id] = page;
  created = null;

  const op = (name: string) => (targets: any[], parent: any) => {
    opCalls.push({ name, ids: targets.map(t => t.id), parent });
    return resultNode("Boolean", "BOOLEAN_OPERATION");
  };

  (globalThis as any).figma = {
    get currentPage() { return page; },
    getNodeByIdAsync: async (id: string) => nodes[id] ?? null,
    union: op("union"),
    subtract: op("subtract"),
    intersect: op("intersect"),
    exclude: op("exclude"),
    flatten: (targets: any[], parent: any) => {
      opCalls.push({ name: "flatten", ids: targets.map(t => t.id), parent });
      return resultNode("Vector", "VECTOR");
    },
    createNodeFromSvg: (svg: string) => {
      created = { svg, ...resultNode("SVG", "FRAME"), remove() { this.removed = true; } };
      return created;
    },
    commitUndo: () => {},
  };
});

const call = (type: string, nodeIds: string[], params: any = {}) =>
  writeVectorHandlers[type]({ type, requestId: "r1", nodeIds, params });

// ── commonParent ──────────────────────────────────────────────────────────────

describe("commonParent", () => {
  it("returns the shared parent", () => {
    const a = makeNode("1:1", page);
    const b = makeNode("1:2", page);
    expect(commonParent([a, b])).toBe(page);
  });

  it("refuses shapes from different parents", () => {
    const other = { id: "1:9", type: "FRAME" };
    expect(() => commonParent([makeNode("1:1", page), makeNode("1:2", other)]))
      .toThrow(/share a parent/);
  });

  it("reports a node that has already been removed", () => {
    expect(() => commonParent([makeNode("1:1", null)])).toThrow(/no parent/);
  });
});

// ── boolean_operation ─────────────────────────────────────────────────────────

describe("boolean_operation", () => {
  beforeEach(() => {
    makeNode("1:1", page);
    makeNode("1:2", page);
  });

  it("runs each operation against the shared parent", async () => {
    for (const [operation, method] of Object.entries({
      UNION: "union", SUBTRACT: "subtract", INTERSECT: "intersect", EXCLUDE: "exclude",
    })) {
      opCalls = [];
      await call("boolean_operation", ["1:1", "1:2"], { operation });
      expect(opCalls[0].name).toBe(method);
      expect(opCalls[0].parent).toBe(page);
    }
  });

  it("accepts a lowercase operation name", async () => {
    await call("boolean_operation", ["1:1", "1:2"], { operation: "union" });
    expect(opCalls[0].name).toBe("union");
  });

  // SUBTRACT cuts the later shapes out of the first, so order is meaning.
  it("keeps the caller's node order", async () => {
    await call("boolean_operation", ["1:2", "1:1"], { operation: "SUBTRACT" });
    expect(opCalls[0].ids).toEqual(["1:2", "1:1"]);
  });

  it("names the result when asked", async () => {
    const res = await call("boolean_operation", ["1:1", "1:2"], { operation: "UNION", name: "Icon" });
    expect(res.data.name).toBe("Icon");
  });

  it("needs two shapes", async () => {
    expect(call("boolean_operation", ["1:1"], { operation: "UNION" }))
      .rejects.toThrow(/at least 2/);
  });

  it("rejects an unknown operation", async () => {
    expect(call("boolean_operation", ["1:1", "1:2"], { operation: "MERGE" }))
      .rejects.toThrow(/must be UNION/);
  });

  it("names a missing node", async () => {
    expect(call("boolean_operation", ["1:1", "9:9"], { operation: "UNION" }))
      .rejects.toThrow(/9:9/);
  });
});

// ── flatten_nodes ─────────────────────────────────────────────────────────────

describe("flatten_nodes", () => {
  it("flattens into the shared parent", async () => {
    makeNode("1:1", page);
    makeNode("1:2", page);
    const res = await call("flatten_nodes", ["1:1", "1:2"]);
    expect(opCalls[0].name).toBe("flatten");
    expect(res.data.type).toBe("VECTOR");
  });

  it("accepts a single node", async () => {
    makeNode("1:1", page);
    await call("flatten_nodes", ["1:1"]);
    expect(opCalls[0].ids).toEqual(["1:1"]);
  });
});

// ── outline_stroke ────────────────────────────────────────────────────────────

describe("outline_stroke", () => {
  it("outlines what it can", async () => {
    makeNode("1:1", page, { outlineStroke: () => resultNode("Outline", "VECTOR") });
    const res = await call("outline_stroke", ["1:1"]);
    expect(res.data.outlined.length).toBe(1);
    expect(res.data.skipped).toEqual([]);
  });

  // Figma returns null for a node whose stroke is empty; that is a no-op, and
  // one such node must not cost the caller the ones that worked.
  it("reports a node with no visible stroke without failing the call", async () => {
    makeNode("1:1", page, { outlineStroke: () => resultNode("Outline", "VECTOR") });
    makeNode("1:2", page, { outlineStroke: () => null });
    const res = await call("outline_stroke", ["1:1", "1:2"]);
    expect(res.data.outlined.length).toBe(1);
    expect(res.data.skipped).toEqual([{ id: "1:2", reason: "no visible stroke" }]);
  });

  it("reports a node type that cannot be outlined", async () => {
    makeNode("1:1", page, { type: "GROUP" });
    const res = await call("outline_stroke", ["1:1"]);
    expect(res.data.outlined).toEqual([]);
    expect(res.data.skipped[0].reason).toMatch(/no stroke to outline/);
  });
});

// ── create_vector ─────────────────────────────────────────────────────────────

describe("create_vector", () => {
  const svg = '<svg><path d="M0 0 L10 10"/></svg>';

  it("unwraps a single-path import so the caller gets the vector itself", async () => {
    const inner = resultNode("path", "VECTOR");
    (globalThis as any).figma.createNodeFromSvg = () => {
      const frame: any = resultNode("SVG", "FRAME");
      frame.children = [inner];
      inner.parent = frame;
      frame.parent = { appendChild: (n: any) => { n.parent = page; } };
      frame.remove = () => { frame.removed = true; };
      return frame;
    };
    const res = await call("create_vector", [], { svg });
    expect(res.data.type).toBe("VECTOR");
  });

  it("keeps the wrapper when the SVG has several paths", async () => {
    (globalThis as any).figma.createNodeFromSvg = () => {
      const frame: any = resultNode("SVG", "FRAME");
      frame.children = [resultNode("a", "VECTOR"), resultNode("b", "VECTOR")];
      return frame;
    };
    const res = await call("create_vector", [], { svg });
    expect(res.data.type).toBe("FRAME");
  });

  it("applies name, position, size, and fill", async () => {
    const res = await call("create_vector", [], {
      svg, name: "Icon", x: 5, y: 7, width: 24, height: 24, fillColor: "#ff0000",
    });
    expect(res.data.name).toBe("Icon");
    expect(created.x).toBe(5);
    expect(created.y).toBe(7);
    expect(created.width).toBe(24);
    expect(created.fills[0].color.r).toBeCloseTo(1);
  });

  it("requires the svg", async () => {
    expect(call("create_vector", [], {})).rejects.toThrow(/svg is required/);
  });
});

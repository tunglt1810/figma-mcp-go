import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteCreateRequest } from "./write-create";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;
let createdComponents: any[];

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

let mockCurrentPage: any;

beforeEach(() => {
  commitUndoCalled = false;
  createdComponents = [];
  mockNodes = {};
  mockCurrentPage = { id: "0:1", name: "Page 1", appendChild: () => {} };
  (globalThis as any).figma = {
    get currentPage() { return mockCurrentPage; },
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    createComponent: () => {
      const comp: any = {
        id: "comp:new",
        name: "Component",
        type: "COMPONENT",
        x: 0, y: 0, width: 100, height: 100,
        fills: [], strokes: [], cornerRadius: 0, layoutMode: "NONE",
        children: [] as any[],
        resize(w: number, h: number) { this.width = w; this.height = h; },
        appendChild(child: any) { this.children.push(child); },
      };
      createdComponents.push(comp);
      return comp;
    },
    createStar: () => ({ id: "star:new", type: "STAR", name: "Star", resize(w: number, h: number) { this.width = w; this.height = h; } }),
    createPolygon: () => ({ id: "poly:new", type: "POLYGON", name: "Polygon", resize(w: number, h: number) { this.width = w; this.height = h; } }),
    createLine: () => ({ id: "line:new", type: "LINE", name: "Line", resize(w: number, h: number) { this.width = w; this.height = h; } }),
    createEllipse: () => ({ id: "ellipse:new", type: "ELLIPSE", name: "Ellipse", resize(w: number, h: number) { this.width = w; this.height = h; } }),
    commitUndo: () => { commitUndoCalled = true; },
    mixed: Symbol("mixed"),
  };
});

// ── create_component ──────────────────────────────────────────────────────────

describe("create_component", () => {
  const makeParent = () => ({
    id: "0:1",
    children: [] as any[],
    insertChild(_: number, c: any) { this.children.push(c); },
  });

  it("converts a FRAME to a COMPONENT in place", async () => {
    const child = { id: "2:1", type: "RECTANGLE" };
    let frameRemoved = false;
    const parent = makeParent();
    const frame = {
      id: "1:1", name: "Card", type: "FRAME",
      x: 10, y: 20, width: 200, height: 100,
      fills: [{ type: "SOLID" }], strokes: [],
      cornerRadius: 8, layoutMode: "NONE",
      children: [child], parent,
      remove() { frameRemoved = true; },
    };
    parent.children = [frame];
    mockNodes["1:1"] = frame;

    const res = await handleWriteCreateRequest(makeRequest("create_component", ["1:1"]));
    expect(res?.data.type).toBe("COMPONENT");
    expect(createdComponents[0].name).toBe("Card");
    expect(createdComponents[0].cornerRadius).toBe(8);
    expect(createdComponents[0].children).toContain(child);
    expect(frameRemoved).toBe(true);
    expect(commitUndoCalled).toBe(true);
  });

  it("copies frame dimensions", async () => {
    const parent = makeParent();
    const frame = {
      id: "1:1", name: "Banner", type: "FRAME",
      x: 0, y: 0, width: 320, height: 64,
      fills: [], strokes: [], cornerRadius: 0, layoutMode: "NONE",
      children: [], parent,
      remove() {},
    };
    parent.children = [frame];
    mockNodes["1:1"] = frame;

    await handleWriteCreateRequest(makeRequest("create_component", ["1:1"]));
    expect(createdComponents[0].width).toBe(320);
    expect(createdComponents[0].height).toBe(64);
  });

  it("uses custom name when provided", async () => {
    const parent = makeParent();
    const frame = {
      id: "1:1", name: "Frame", type: "FRAME",
      x: 0, y: 0, width: 100, height: 100,
      fills: [], strokes: [], cornerRadius: 0, layoutMode: "NONE",
      children: [], parent,
      remove() {},
    };
    parent.children = [frame];
    mockNodes["1:1"] = frame;

    await handleWriteCreateRequest(makeRequest("create_component", ["1:1"], { name: "Button" }));
    expect(createdComponents[0].name).toBe("Button");
  });

  it("copies auto-layout properties when layoutMode is set", async () => {
    const parent = makeParent();
    const frame = {
      id: "1:1", name: "Row", type: "FRAME",
      x: 0, y: 0, width: 200, height: 48,
      fills: [], strokes: [], cornerRadius: 0,
      layoutMode: "HORIZONTAL",
      paddingTop: 8, paddingRight: 16, paddingBottom: 8, paddingLeft: 16,
      itemSpacing: 12,
      primaryAxisAlignItems: "CENTER",
      counterAxisAlignItems: "CENTER",
      children: [], parent,
      remove() {},
    };
    parent.children = [frame];
    mockNodes["1:1"] = frame;

    await handleWriteCreateRequest(makeRequest("create_component", ["1:1"]));
    const comp = createdComponents[0];
    expect(comp.layoutMode).toBe("HORIZONTAL");
    expect(comp.paddingTop).toBe(8);
    expect(comp.paddingRight).toBe(16);
    expect(comp.itemSpacing).toBe(12);
    expect(comp.primaryAxisAlignItems).toBe("CENTER");
  });

  it("throws when nodeId not found", async () => {
    await expect(
      handleWriteCreateRequest(makeRequest("create_component", ["9:9"]))
    ).rejects.toThrow("Node not found: 9:9");
  });

  it("throws when node is not a FRAME", async () => {
    mockNodes["1:1"] = { id: "1:1", type: "RECTANGLE" };
    await expect(
      handleWriteCreateRequest(makeRequest("create_component", ["1:1"]))
    ).rejects.toThrow("is not a FRAME");
  });

  it("throws when no nodeId provided", async () => {
    await expect(
      handleWriteCreateRequest(makeRequest("create_component", []))
    ).rejects.toThrow("nodeId is required");
  });
});

// ── create_section ────────────────────────────────────────────────────────────

describe("create_section", () => {
  let createdSection: any;

  beforeEach(() => {
    createdSection = null;
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      currentPage: { id: "0:1", name: "Page 1", appendChild: () => {} },
      createSection: () => {
        createdSection = {
          id: "section:new", name: "Section", type: "SECTION",
          x: 0, y: 0, width: 200, height: 200,
          resizeWithoutConstraints(w: number, h: number) { this.width = w; this.height = h; },
        };
        return createdSection;
      },
    };
  });

  it("creates a section with a name", async () => {
    const res = await handleWriteCreateRequest(makeRequest("create_section", [], { name: "Sprint 1" }));
    expect(createdSection.name).toBe("Sprint 1");
    expect(res?.data.type).toBe("SECTION");
    expect(res?.data.id).toBe("section:new");
    expect(commitUndoCalled).toBe(true);
  });

  it("creates a section at a specific position", async () => {
    const res = await handleWriteCreateRequest(makeRequest("create_section", [], { x: 100, y: 200 }));
    expect(createdSection.x).toBe(100);
    expect(createdSection.y).toBe(200);
  });

  it("creates a section with custom size", async () => {
    await handleWriteCreateRequest(makeRequest("create_section", [], { width: 800, height: 600 }));
    expect(createdSection.width).toBe(800);
    expect(createdSection.height).toBe(600);
  });

  it("creates a section with default values when no params given", async () => {
    const res = await handleWriteCreateRequest(makeRequest("create_section", [], {}));
    expect(res?.data.id).toBe("section:new");
  });
});

// ── create_star ───────────────────────────────────────────────────────────────

describe("create_star", () => {
  it("creates a star with default values", async () => {
    let appendedChild: any = null;
    (globalThis as any).figma.currentPage.appendChild = (child: any) => { appendedChild = child; };

    const res = await handleWriteCreateRequest(makeRequest("create_star"));
    expect(res?.data.type).toBe("STAR");
    expect(appendedChild.type).toBe("STAR");
    expect(appendedChild.pointCount).toBe(5);
    expect(appendedChild.width).toBe(100);
    expect(appendedChild.height).toBe(100);
    expect(commitUndoCalled).toBe(true);
  });

  it("creates a star with specified params", async () => {
    let appendedChild: any = null;
    (globalThis as any).figma.currentPage.appendChild = (child: any) => { appendedChild = child; };

    await handleWriteCreateRequest(makeRequest("create_star", [], {
      pointCount: 6,
      outerRadius: 60,
      innerRadius: 30,
      x: 10,
      y: 20,
      name: "MyStar",
      fillColor: "ff0000",
      cornerRadius: 5
    }));
    expect(appendedChild.pointCount).toBe(6);
    expect(appendedChild.width).toBe(120); // 60 * 2
    expect(appendedChild.innerRadius).toBe(0.5); // 30 / 60
    expect(appendedChild.x).toBe(10);
    expect(appendedChild.y).toBe(20);
    expect(appendedChild.name).toBe("MyStar");
    expect(appendedChild.fills[0].color.r).toBe(1);
    expect(appendedChild.cornerRadius).toBe(5);
  });
});

// ── create_polygon ────────────────────────────────────────────────────────────

describe("create_polygon", () => {
  it("creates a polygon with default values", async () => {
    let appendedChild: any = null;
    (globalThis as any).figma.currentPage.appendChild = (child: any) => { appendedChild = child; };

    const res = await handleWriteCreateRequest(makeRequest("create_polygon"));
    expect(res?.data.type).toBe("POLYGON");
    expect(appendedChild.type).toBe("POLYGON");
    expect(appendedChild.pointCount).toBe(3);
    expect(appendedChild.width).toBe(100);
    expect(appendedChild.height).toBe(100);
    expect(commitUndoCalled).toBe(true);
  });

  it("creates a polygon with specified params", async () => {
    let appendedChild: any = null;
    (globalThis as any).figma.currentPage.appendChild = (child: any) => { appendedChild = child; };

    await handleWriteCreateRequest(makeRequest("create_polygon", [], {
      pointCount: 8,
      radius: 40,
      x: 5,
      y: 15,
      name: "Octagon",
      fillColor: "00ff00",
      cornerRadius: 2
    }));
    expect(appendedChild.pointCount).toBe(8);
    expect(appendedChild.width).toBe(80); // 40 * 2
    expect(appendedChild.x).toBe(5);
    expect(appendedChild.y).toBe(15);
    expect(appendedChild.name).toBe("Octagon");
    expect(appendedChild.fills[0].color.g).toBe(1);
    expect(appendedChild.cornerRadius).toBe(2);
  });
});

// ── create_line ───────────────────────────────────────────────────────────────

describe("create_line", () => {
  it("creates a line with default values", async () => {
    let appendedChild: any = null;
    (globalThis as any).figma.currentPage.appendChild = (child: any) => { appendedChild = child; };

    const res = await handleWriteCreateRequest(makeRequest("create_line"));
    expect(res?.data.type).toBe("LINE");
    expect(appendedChild.type).toBe("LINE");
    expect(appendedChild.width).toBe(100);
    expect(appendedChild.height).toBe(0);
    expect(commitUndoCalled).toBe(true);
  });

  it("creates a line with specified params", async () => {
    let appendedChild: any = null;
    (globalThis as any).figma.currentPage.appendChild = (child: any) => { appendedChild = child; };

    await handleWriteCreateRequest(makeRequest("create_line", [], {
      length: 200,
      rotation: 45,
      x: 10,
      y: 10,
      name: "Divider",
      strokeColor: "0000ff",
      strokeWeight: 4
    }));
    expect(appendedChild.width).toBe(200);
    expect(appendedChild.rotation).toBe(45);
    expect(appendedChild.x).toBe(10);
    expect(appendedChild.y).toBe(10);
    expect(appendedChild.name).toBe("Divider");
    expect(appendedChild.strokes[0].color.b).toBe(1);
    expect(appendedChild.strokeWeight).toBe(4);
  });
});

// ── create_ellipse ────────────────────────────────────────────────────────────

describe("create_ellipse", () => {
  it("sets arcData when provided", async () => {
    let appendedChild: any = null;
    (globalThis as any).figma.currentPage.appendChild = (child: any) => { appendedChild = child; };

    const arcData = { startingAngle: 0, endingAngle: Math.PI, innerRadius: 0.5 };
    await handleWriteCreateRequest(makeRequest("create_ellipse", [], {
      arcData
    }));
    expect(appendedChild.type).toBe("ELLIPSE");
    expect(appendedChild.arcData).toEqual(arcData);
  });
});

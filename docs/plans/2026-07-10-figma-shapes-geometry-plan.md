# Figma Shapes Geometry Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fully support the geometric properties of shape types (Star, Polygon, Ellipse, Rectangle, and Line) in both directions: reading (serialization) and writing (creation tools) in the Figma MCP Server.

**Architecture:** Extend `serializers.ts` to extract a `geometry` object from a Node. Extend `tools_write_create.go` and `write-create.ts` with the `create_star`, `create_polygon`, and `create_line` tools, and update `create_ellipse`. Pixel properties such as radius are automatically converted into `width`/`height` and ratios for compatibility with the Figma API.

**Tech Stack:** Go (MCP Server), TypeScript (Figma Plugin)

## Global Constraints

- All Go changes must follow the `mark3labs/mcp-go` library conventions.
- Do not remove `cornerRadius` from `styles`; copy it into `geometry` to preserve backward compatibility.

---

### Task 1: Upgrade Serialization (Read Geometry Properties)

**Files:**
- Modify: `plugin/src/serializers.ts`

**Interfaces:**
- Produces: `serializeNode` now returns an additional `geometry` property containing geometric attributes (`rotation`, `cornerRadius`, `pointCount`, `innerRadiusPixel`, `outerRadiusPixel`, `arcData`, and so on).

- [ ] **Step 1: Add Geometry Extraction Logic**

Open `plugin/src/serializers.ts` and add the following logic before or inside `serializeNode`:

```typescript
const getGeometry = (node: any) => {
  const geom: any = {};
  
  if ("rotation" in node) {
    geom.rotation = node.rotation;
  }
  if ("cornerRadius" in node) {
    geom.cornerRadius = node.cornerRadius === figma.mixed ? "mixed" : node.cornerRadius;
  }

  switch (node.type) {
    case "STAR":
      if ("pointCount" in node) geom.pointCount = node.pointCount;
      if ("innerRadius" in node) {
        geom.innerRadiusRatio = node.innerRadius;
        geom.outerRadiusPixel = node.width / 2;
        geom.innerRadiusPixel = (node.width / 2) * node.innerRadius;
      }
      break;
    case "POLYGON":
      if ("pointCount" in node) geom.pointCount = node.pointCount;
      break;
    case "ELLIPSE":
      if ("arcData" in node) geom.arcData = node.arcData;
      break;
    case "RECTANGLE":
    case "FRAME":
    case "COMPONENT":
      if ("topLeftRadius" in node) geom.topLeftRadius = node.topLeftRadius;
      if ("topRightRadius" in node) geom.topRightRadius = node.topRightRadius;
      if ("bottomLeftRadius" in node) geom.bottomLeftRadius = node.bottomLeftRadius;
      if ("bottomRightRadius" in node) geom.bottomRightRadius = node.bottomRightRadius;
      break;
  }
  
  return Object.keys(geom).length > 0 ? geom : undefined;
};
```

- [ ] **Step 2: Update `serializeNode`**

Add the `geometry` field to the `base` object in `serializeNode`:

```typescript
export const serializeNode = async (node: any): Promise<any> => {
  const styles = await serializeStyles(node);
  const base = {
    id: node.id,
    name: node.name,
    type: node.type,
    bounds: getBounds(node),
    styles,
    geometry: getGeometry(node),
  };
  // ... (keep the remaining code unchanged)
```

- [ ] **Step 3: Build and Test Compilation**

Run ` npm run build` in the `plugin/` directory to ensure there are no syntax errors.

- [ ] **Step 4: Commit**

```bash
 git commit -am "feat: add geometry extraction to serializers"
```

---

### Task 2: Register the New Tools (Go Server)

**Files:**
- Modify: `internal/tools_write_create.go`

**Interfaces:**
- Produces: New MCP tools are registered (`create_star`, `create_polygon`, and `create_line`).

- [ ] **Step 1: Declare `create_star`**

Add the following to `registerWriteCreateTools`:

```go
	s.AddTool(mcp.NewTool("create_star",
		mcp.WithDescription("Create a new star shape."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("pointCount", mcp.Description("Number of points (default 5)")),
		mcp.WithNumber("outerRadius", mcp.Description("Outer radius in pixels (default 50)")),
		mcp.WithNumber("innerRadius", mcp.Description("Inner radius in pixels (default calculated based on 0.3819 ratio)")),
		mcp.WithString("fillColor", mcp.Description("Fill color as hex e.g. #FF5733")),
		mcp.WithNumber("cornerRadius", mcp.Description("Corner radius in pixels")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_star", nil, params)
		return renderResponse(resp, err)
	})
```

- [ ] **Step 2: Declare `create_polygon` and `create_line`**

```go
	s.AddTool(mcp.NewTool("create_polygon",
		mcp.WithDescription("Create a new polygon shape."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("pointCount", mcp.Description("Number of sides (default 3)")),
		mcp.WithNumber("radius", mcp.Description("Radius in pixels (default 50)")),
		mcp.WithString("fillColor", mcp.Description("Fill color as hex e.g. #FF5733")),
		mcp.WithNumber("cornerRadius", mcp.Description("Corner radius in pixels")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_polygon", nil, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_line",
		mcp.WithDescription("Create a new line."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("length", mcp.Description("Length in pixels (default 100)")),
		mcp.WithNumber("rotation", mcp.Description("Rotation in degrees (default 0)")),
		mcp.WithString("strokeColor", mcp.Description("Stroke color as hex e.g. #000000")),
		mcp.WithNumber("strokeWeight", mcp.Description("Stroke weight in pixels (default 1)")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_line", nil, params)
		return renderResponse(resp, err)
	})
```

- [ ] **Step 3: Update `create_ellipse`**

Update `create_ellipse` to support `arcData` by adding `startAngle`, `endAngle`, and `innerRadiusRatio`.

- [ ] **Step 4: Build the Go Server**

Run ` go build ./cmd/figma-mcp-go` to test compilation.

- [ ] **Step 5: Commit**

```bash
 git commit -am "feat: register new creation tools for shapes in MCP server"
```

---

### Task 3: Add Handlers to the Figma Plugin

**Files:**
- Modify: `plugin/src/write-create.ts`

**Interfaces:**
- Consumes: Requests of type `create_star`, `create_polygon`, and `create_line`, along with their parameters.
- Produces: Calls the Figma API to create the shape and returns the node as JSON.

- [ ] **Step 1: Handle `create_star`**

Add the following case to `switch (request.type)`:

```typescript
    case "create_star": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const star = figma.createStar();
      const pointCount = p.pointCount != null ? Number(p.pointCount) : 5;
      const outerRadius = p.outerRadius != null ? Number(p.outerRadius) : 50;
      star.pointCount = pointCount;
      
      const width = outerRadius * 2;
      star.resize(width, width);
      
      if (p.innerRadius != null) {
        star.innerRadius = Number(p.innerRadius) / outerRadius;
      }
      
      star.x = p.x != null ? p.x : 0;
      star.y = p.y != null ? p.y : 0;
      if (p.name) star.name = p.name;
      if (p.fillColor) star.fills = [makeSolidPaint(p.fillColor)];
      if (p.cornerRadius != null) star.cornerRadius = p.cornerRadius;
      (parent as any).appendChild(star);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: star.id, name: star.name, type: star.type, bounds: getBounds(star) },
      };
    }
```

- [ ] **Step 2: Handle `create_polygon` and `create_line`**

```typescript
    case "create_polygon": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const polygon = figma.createPolygon();
      const pointCount = p.pointCount != null ? Number(p.pointCount) : 3;
      const radius = p.radius != null ? Number(p.radius) : 50;
      polygon.pointCount = pointCount;
      
      const width = radius * 2;
      polygon.resize(width, width);
      
      polygon.x = p.x != null ? p.x : 0;
      polygon.y = p.y != null ? p.y : 0;
      if (p.name) polygon.name = p.name;
      if (p.fillColor) polygon.fills = [makeSolidPaint(p.fillColor)];
      if (p.cornerRadius != null) polygon.cornerRadius = p.cornerRadius;
      (parent as any).appendChild(polygon);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: polygon.id, name: polygon.name, type: polygon.type, bounds: getBounds(polygon) },
      };
    }

    case "create_line": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const line = figma.createLine();
      const length = p.length != null ? Number(p.length) : 100;
      
      line.resize(length, 0);
      
      if (p.rotation != null) line.rotation = Number(p.rotation);
      
      line.x = p.x != null ? p.x : 0;
      line.y = p.y != null ? p.y : 0;
      if (p.name) line.name = p.name;
      if (p.strokeColor) line.strokes = [makeSolidPaint(p.strokeColor)];
      if (p.strokeWeight != null) line.strokeWeight = p.strokeWeight;
      (parent as any).appendChild(line);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: line.id, name: line.name, type: line.type, bounds: getBounds(line) },
      };
    }
```

- [ ] **Step 3: Update `create_ellipse`**

Update the `create_ellipse` handler block to set `ellipse.arcData` when the parameter is provided.

- [ ] **Step 4: Build and Commit**

Run ` npm run build` in `plugin/` and commit:

```bash
 git commit -am "feat: handle create_star, create_polygon, create_line in plugin"
```

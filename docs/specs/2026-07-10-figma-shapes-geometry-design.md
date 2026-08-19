# Figma Shapes Geometry Support Design

## Purpose

Upgrade the Figma MCP Server (`figma-mcp-go`) to fully support the geometric properties of every shape type (Star, Polygon, Ellipse, Rectangle, and Line). This gives AI coding agents enough precise data to automatically generate SVG or CSS from a Figma drawing, while also providing a complete MCP toolset for drawing these shapes back into Figma.

## Architecture & Data Flow

### 1. Read (Serialization) — Returned Data Structure

When the MCP server responds to `get_node` or another read command, the Node structure gains a `geometry` object at the same level as `bounds` and `styles`.

**Expected `geometry` structure:**
- **Every Shape**: Contains `rotation`.
- **StarNode**: `pointCount`, `innerRadiusPixel`, `outerRadiusPixel`, and `cornerRadius` for rounded points.
- **PolygonNode**: `pointCount` and `cornerRadius`.
- **EllipseNode**: `arcData`, containing `startingAngle`, `endingAngle`, and the proportional `innerRadius` useful for pie and donut charts.
- **RectangleNode / FrameNode**: Extract `topLeftRadius`, `topRightRadius`, `bottomLeftRadius`, `bottomRightRadius`, and the shared `cornerRadius`.
- **LineNode**: Length and rotation may be added in the future; for now, rotation is represented by `rotation`.

*Compatibility note*: `cornerRadius` currently lives in `styles`; it will be copied into `geometry` to support more precise shape-specific logic without breaking existing clients.

### 2. Write (Creation Tools) — Add MCP Tools

The Go Server registers the new tools and the TypeScript Plugin executes them:

- **`create_star`**:
  - Parameters: `x`, `y`, `pointCount` (default 5), `outerRadius` (default 50, in pixels), `innerRadius` (in pixels), `fillColor`, and `cornerRadius`.
  - Processing: The Plugin calculates `width = outerRadius * 2`, `height = outerRadius * 2`, and sets `node.innerRadius = innerRadius / outerRadius`.
- **`create_polygon`**:
  - Parameters: `x`, `y`, `pointCount` (default 3), `radius` (circumradius, default 50), `fillColor`, and `cornerRadius`.
  - Processing: `width = radius * 2`, `height = radius * 2`.
- **`create_line`**:
  - Parameters: `x`, `y`, `length` (default 100), `rotation` (degrees, default 0), `strokeColor`, and `strokeWeight`.
  - Processing: Set `width = length`, `height = 0`, and apply `rotation`.
- **`create_ellipse` (enhancement)**:
  - Additional parameters: `startAngle`, `endAngle`, and `innerRadiusRatio`.

## Edge Cases / Error Handling

- **`create_star`**: If `outerRadius = 0`, fall back or return an error to avoid division by zero when calculating `innerRadiusRatio`.
- Mixed data (for example, mixed Rectangle corner radii) is serialized as the string `"mixed"`.

## Testing

- Use `get_node` to inspect a star-shaped node with rounded points in Figma and verify that `geometry` is present with accurate pixel values.
- Run `create_star` with `outerRadius=100`, `innerRadius=50`, and `pointCount=5`. Verify in Figma that the generated node is 200x200 and that the `innerRadius` ratio is 0.5.

# Specs: Gradient Support for the Figma MCP Plugin

## 1. Introduction

Figma uses a 2x3 Transform matrix (`gradientTransform`) to represent the position, rotation, and size of gradient types (Linear, Radial, Angular, and Diamond). However, this matrix is not friendly to frontend developers or LLMs when they need to export CSS or a React Native component, which requires parameters such as `center`, `radius`, `start`, `end`, and `angle`.

This feature automatically converts between `gradientTransform` and a geometry coordinate system in both directions.

## 2. Read Process (Serialization — `serializePaints`)

When reading a Figma Node, `serializePaints` calculates and returns the `fills`/`strokes` arrays.

For a solid color, it returns a Hex string `"#RRGGBB"` (or `"#RRGGBBAA"`).

For a gradient, it returns a JSON object:

### Radial Gradient Output

```json
{
  "type": "GRADIENT_RADIAL",
  "stops": [
    { "position": 0, "color": "#FFBE45FF" },
    { "position": 1, "color": "#131313FF" }
  ],
  "geometry": {
    "center": { "percentX": 50, "percentY": 50 },
    "radius": { "percentX": 50, "percentY": 50 },
    "rotation": 0
  }
}
```

### Linear Gradient Output

```json
{
  "type": "GRADIENT_LINEAR",
  "stops": [ ... ],
  "geometry": {
    "start": { "percentX": 0, "percentY": 0 },
    "end": { "percentX": 100, "percentY": 100 },
    "angle": 135
  }
}
```

### 2.1. Mathematical Formula: Transform Matrix → Geometry

Figma stores `gradientTransform` as matrix $M$. The matrix maps from Normalized Node Space $N$ (`[0..1], [0..1]`) to Gradient Local Space $L$.

$$ M \times N = L \implies N = M^{-1} \times L $$

Inverse of a 2x3 matrix:

```typescript
function invertTransform(t: Transform): Transform {
  const [[a, b, c], [d, e, f]] = t;
  const det = a * e - b * d;
  if (det === 0) return [[1, 0, 0], [0, 1, 0]];
  return [
    [e / det, -b / det, (b * f - c * e) / det],
    [-d / det, a / det, (c * d - a * f) / det],
  ];
}
```

**Radial Gradient Local Handles:**
- Center: `(0.5, 0.5)`
- Rx (radius-X handle): `(1, 0.5)`
- Ry (radius-Y handle): `(0.5, 1)`

Multiply $M^{-1}$ by the three points to obtain `centerNorm`, `rxNorm`, and `ryNorm` (coordinates in `[0, 1]` space).

Then calculate percentages:
- `center.percentX = centerNorm.x * 100`
- `radius.percentX = length(rxNorm - centerNorm) * 100`
- `rotation = atan2(rxNorm.y - centerNorm.y, rxNorm.x - centerNorm.x) * 180 / Math.PI`

**Linear Gradient Local Handles:**
- Start: `(0, 0.5)`
- End: `(1, 0.5)`

Similarly, multiply by $M^{-1}$ to obtain `startNorm` and `endNorm`. Calculate `angle` with `atan2` between `start` and `end`.

## 3. Write Process (Mutation — `set_gradient_fills`)

### Input Payload

Use the schema from the `set_gradient_fills` MCP tool:

```json
{
  "nodeId": "1:1",
  "type": "GRADIENT_RADIAL",
  "stops": [ { "position": 0, "color": "#FF0000" }, { "position": 1, "color": "#00FF00" } ],
  "geometry": {
    "center": { "percentX": 50, "percentY": 50 },
    "radius": { "percentX": 50, "percentY": 50 },
    "rotation": 0
  }
}
```

### 3.1. Mathematical Formula: Geometry → Transform Matrix

Convert the input geometry (expressed in `%`) by dividing by 100 to obtain Normalized Coordinates ($N$).

Then find the matrix $T_{inv}$ (which is $M^{-1}$) that maps Local Handles to $N$.

**Radial:**

Let $cx, cy$ be the center coordinates, $rx, ry$ the radius magnitudes along X and Y, and $\theta$ the rotation.

The `centerNorm`, `rxHandleNorm`, and `ryHandleNorm` points are:

```typescript
rxHandleNorm.x = cx + rx * cos(theta)
rxHandleNorm.y = cy + rx * sin(theta)
// Assume the ry handle is perpendicular:
ryHandleNorm.x = cx - ry * sin(theta)
ryHandleNorm.y = cy + ry * cos(theta)
```

Map $T_{inv}$ as follows:
`(0.5, 0.5) -> centerNorm`
`(1, 0.5) -> rxHandleNorm`
`(0.5, 1) -> ryHandleNorm`

Solve the three-point system to find $T_{inv} = [[A,B,C], [D,E,F]]$:

```typescript
A = 2 * (rxHandleNorm.x - centerNorm.x)
B = 2 * (ryHandleNorm.x - centerNorm.x)
C = 3 * centerNorm.x - rxHandleNorm.x - ryHandleNorm.x

D = 2 * (rxHandleNorm.y - centerNorm.y)
E = 2 * (ryHandleNorm.y - centerNorm.y)
F = 3 * centerNorm.y - rxHandleNorm.y - ryHandleNorm.y
```

Finally, `gradientTransform = invertTransform(T_inv)`.

**Linear:**

Map $T_{inv}$ as follows:
`(0, 0.5) -> startNorm`
`(1, 0.5) -> endNorm`
`(0, 1) -> perpNorm` (a perpendicular point used to create a virtual width, `perpNorm = startNorm + [-dy, dx]`)

Solve the system:

```typescript
A = endNorm.x - startNorm.x
B = 2 * (perpNorm.x - startNorm.x)
C = 2 * startNorm.x - perpNorm.x

D = endNorm.y - startNorm.y
E = 2 * (perpNorm.y - startNorm.y)
F = 2 * startNorm.y - perpNorm.y
```

`gradientTransform = invertTransform(T_inv)`

## 4. MCP Schema for the `set_gradient_fills` Tool

Define the tool to accept an object with these arguments:
- `nodeId`: string
- `type`: string (`GRADIENT_LINEAR`, `GRADIENT_RADIAL`)
- `stops`: Array<{ color: string, position: number }>
- `geometry`: Object containing `center`, `radius`, and `rotation` for RADIAL; `start` and `end` for LINEAR. Coordinates are expressed as `percentX` and `percentY`.

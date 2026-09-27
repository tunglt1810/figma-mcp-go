# Specs: Gradient Support for the Figma MCP Plugin

## 1. Introduction

Figma stores the position, rotation, and size of a gradient (Linear, Radial, Angular, or Diamond) as a 2x3 transform matrix, `gradientTransform`. This matrix is hard for frontend developers and LLMs to use. CSS and React Native need simple values such as `center`, `radius`, `start`, `end`, and `angle`.

This feature converts between `gradientTransform` and these geometry values, in both directions.

## 2. Read Process (Serialization — `serializePaints`)

When reading a Figma node, `serializePaints` builds and returns the `fills`/`strokes` arrays.

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

Figma stores `gradientTransform` as matrix $M$. It maps normalized node space $N$ (`[0..1], [0..1]`) to gradient local space $L$.

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

Multiply $M^{-1}$ by the three points to get `centerNorm`, `rxNorm`, and `ryNorm` (coordinates in `[0, 1]` space).

Then compute the percentages:
- `center.percentX = centerNorm.x * 100`
- `radius.percentX = length(rxNorm - centerNorm) * 100`
- `rotation = atan2(rxNorm.y - centerNorm.y, rxNorm.x - centerNorm.x) * 180 / Math.PI`

**Linear Gradient Local Handles:**
- Start: `(0, 0.5)`
- End: `(1, 0.5)`

In the same way, multiply by $M^{-1}$ to get `startNorm` and `endNorm`. Compute `angle` with `atan2` from `start` to `end`.

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

The input geometry is in `%`. Divide it by 100 to get normalized coordinates ($N$).

Then find the matrix $T_{inv}$ (that is, $M^{-1}$) that maps the local handles to $N$.

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
`(0, 1) -> perpNorm` (a perpendicular point that gives the gradient a virtual width: `perpNorm = startNorm + [-dy, dx]`)

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

The tool accepts an object with these arguments:
- `nodeId`: string
- `type`: string (`GRADIENT_LINEAR`, `GRADIENT_RADIAL`)
- `stops`: Array<{ color: string, position: number }>
- `geometry`: an object with `center`, `radius`, and `rotation` for RADIAL, or `start` and `end` for LINEAR. Coordinates are given as `percentX` and `percentY`.


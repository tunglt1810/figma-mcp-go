# Specs: Hỗ trợ Gradient cho Figma MCP Plugin

## 1. Giới thiệu
Figma sử dụng ma trận Transform 2x3 (`gradientTransform`) để biểu diễn vị trí, góc xoay và kích thước của các loại gradient (Linear, Radial, Angular, Diamond). Tuy nhiên, ma trận này không thân thiện với các Frontend Developers và LLMs khi cần xuất CSS hoặc React Native component (cần các tham số như `center`, `radius`, `start`, `end`, `angle`).
Tính năng này sẽ tự động phân tích và quy đổi qua lại giữa `gradientTransform` và hệ tọa độ `geometry`.

## 2. Quá trình Đọc (Serialization - `serializePaints`)

Khi gọi đọc Figma Node, `serializePaints` sẽ tính toán và trả về mảng `fills`/`strokes`.
Với Solid màu trơn: trả về chuỗi Hex `"#RRGGBB"` (hoặc `"#RRGGBBAA"`).
Với Gradient, trả về JSON Object:

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

### 2.1. Công thức Toán: Transform Matrix -> Geometry
Figma lưu `gradientTransform` là ma trận $M$. Ma trận này map từ Normalize Node Space $N$ `([0..1], [0..1])` sang Gradient Local Space $L$.
$$ M \times N = L \implies N = M^{-1} \times L $$

Hàm nghịch đảo ma trận 2x3:
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
- Rx (Radius X handle): `(1, 0.5)`
- Ry (Radius Y handle): `(0.5, 1)`

Nhân $M^{-1}$ với 3 điểm trên để lấy `centerNorm`, `rxNorm`, `ryNorm` (tọa độ trên không gian `[0, 1]`).
Sau đó tính phần trăm:
- `center.percentX = centerNorm.x * 100`
- `radius.percentX = length(rxNorm - centerNorm) * 100`
- `rotation = atan2(rxNorm.y - centerNorm.y, rxNorm.x - centerNorm.x) * 180 / Math.PI`

**Linear Gradient Local Handles:**
- Start: `(0, 0.5)`
- End: `(1, 0.5)`

Tương tự, nhân $M^{-1}$ để lấy `startNorm` và `endNorm`.
Tính `angle` bằng `atan2` giữa `start` và `end`.

## 3. Quá trình Ghi (Mutation - `set_gradient_fills`)

### Input Payload
Sử dụng schema từ MCP tool `set_gradient_fills`:
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

### 3.1. Công thức Toán: Geometry -> Transform Matrix
Từ Input Geometry (tính theo `%`), chia 100 để có được Normalize Coordinates ($N$).
Tiếp tục tìm ma trận $T_{inv}$ (chính là $M^{-1}$) sao cho $T_{inv}$ map Local Handles sang $N$.

**Radial:**
Gọi $cx, cy$ là tọa độ center, $rx, ry$ là độ lớn radius theo X và Y, $\theta$ là rotation.
Ta có điểm `centerNorm`, `rxHandleNorm`, `ryHandleNorm`:
```typescript
rxHandleNorm.x = cx + rx * cos(theta)
rxHandleNorm.y = cy + rx * sin(theta)
// Giả định ryHandle vuông góc:
ryHandleNorm.x = cx - ry * sin(theta)
ryHandleNorm.y = cy + ry * cos(theta)
```

Map $T_{inv}$:
`(0.5, 0.5) -> centerNorm`
`(1, 0.5) -> rxHandleNorm`
`(0.5, 1) -> ryHandleNorm`

Giải hệ 3 điểm để tìm được $T_{inv} = [[A,B,C], [D,E,F]]$:
```typescript
A = 2 * (rxHandleNorm.x - centerNorm.x)
B = 2 * (ryHandleNorm.x - centerNorm.x)
C = 3 * centerNorm.x - rxHandleNorm.x - ryHandleNorm.x

D = 2 * (rxHandleNorm.y - centerNorm.y)
E = 2 * (ryHandleNorm.y - centerNorm.y)
F = 3 * centerNorm.y - rxHandleNorm.y - ryHandleNorm.y
```
Cuối cùng: `gradientTransform = invertTransform(T_inv)`

**Linear:**
Map $T_{inv}$:
`(0, 0.5) -> startNorm`
`(1, 0.5) -> endNorm`
`(0, 1) -> perpNorm` (điểm vuông góc để tạo bề dày ảo, `perpNorm = startNorm + [-dy, dx]`)

Giải hệ:
```typescript
A = endNorm.x - startNorm.x
B = 2 * (perpNorm.x - startNorm.x)
C = 2 * startNorm.x - perpNorm.x

D = endNorm.y - startNorm.y
E = 2 * (perpNorm.y - startNorm.y)
F = 2 * startNorm.y - perpNorm.y
```
`gradientTransform = invertTransform(T_inv)`

## 4. MCP Schema cho tool `set_gradient_fills`
Định nghĩa tool nhận argument dạng Object như sau:
- `nodeId`: string
- `type`: string (GRADIENT_LINEAR, GRADIENT_RADIAL)
- `stops`: Array<{ color: string, position: number }>
- `geometry`: Object (chứa center, radius, rotation cho RADIAL; start, end cho LINEAR). Các toạ độ này tính bằng `percentX`, `percentY`.

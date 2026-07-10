# Figma Shapes Geometry Support Design

## Purpose
Nâng cấp Figma MCP Server (`figma-mcp-go`) để hỗ trợ đầy đủ các thuộc tính hình học (geometry) của mọi loại shape (Star, Polygon, Ellipse, Rectangle, Line). Điều này cho phép AI coding agents có đủ thông số chính xác để tự động tạo ra file SVG hoặc CSS từ bản vẽ Figma, và đồng thời cung cấp bộ công cụ MCP đầy đủ để AI có thể vẽ lại các shape này vào Figma.

## Architecture & Data Flow

### 1. Đọc (Serialization) - Cấu trúc dữ liệu trả về
Khi MCP server phản hồi lệnh `get_node` hoặc các lệnh đọc khác, cấu trúc Node sẽ được bổ sung thêm một object `geometry` nằm ngang hàng với `bounds` và `styles`.

**Cấu trúc `geometry` mong đợi:**
- **Mọi Shape**: Chứa `rotation` (góc xoay).
- **StarNode**: `pointCount` (số cánh), `innerRadiusPixel` (bán kính trong tính bằng pixel), `outerRadiusPixel` (bán kính ngoài tính bằng pixel), `cornerRadius` (bo góc các đỉnh).
- **PolygonNode**: `pointCount` (số cạnh), `cornerRadius`.
- **EllipseNode**: `arcData` (chứa `startingAngle`, `endingAngle`, `innerRadius` tỷ lệ - hữu ích cho biểu đồ quạt/donut).
- **RectangleNode / FrameNode**: Bóc tách `topLeftRadius`, `topRightRadius`, `bottomLeftRadius`, `bottomRightRadius`, và `cornerRadius` chung.
- **LineNode**: (Tương lai có thể thêm chiều dài và góc xoay, tuy nhiên hiện tại góc xoay sẽ nằm trong `rotation`).

*Ghi chú tương thích*: Thuộc tính `cornerRadius` hiện đang nằm trong `styles` sẽ được copy sang `geometry` để đảm bảo logic phân mảnh tốt nhất mà không làm hỏng các client cũ.

### 2. Ghi (Creation Tools) - Bổ sung MCP Tools
Server Go sẽ đăng ký các tools mới và Plugin TypeScript sẽ nhận lệnh thực thi:

- **`create_star`**:
  - Tham số: `x`, `y`, `pointCount` (mặc định 5), `outerRadius` (mặc định 50, đơn vị pixel), `innerRadius` (đơn vị pixel), `fillColor`, `cornerRadius`.
  - Xử lý: Plugin sẽ tính toán `width = outerRadius * 2`, `height = outerRadius * 2` và `node.innerRadius = innerRadius / outerRadius`.
- **`create_polygon`**:
  - Tham số: `x`, `y`, `pointCount` (mặc định 3), `radius` (bán kính ngoại tiếp vòng tròn, mặc định 50), `fillColor`, `cornerRadius`.
  - Xử lý: `width = radius * 2`, `height = radius * 2`.
- **`create_line`**:
  - Tham số: `x`, `y`, `length` (mặc định 100), `rotation` (góc xoay bằng độ, mặc định 0), `strokeColor`, `strokeWeight`.
  - Xử lý: Đặt `width = length`, `height = 0`, áp dụng `rotation`.
- **`create_ellipse` (Nâng cấp)**:
  - Tham số bổ sung: `startAngle`, `endAngle`, `innerRadiusRatio`.

## Edge Cases / Error Handling
- **`create_star`**: Nếu `outerRadius` = 0, cần fallback hoặc báo lỗi để tránh chia cho 0 khi tính `innerRadiusRatio`.
- Dữ liệu `mixed` (ví dụ: góc bo của Rectangle là mixed) sẽ được serialize thành chuỗi `"mixed"`.

## Testing
- Dùng `get_node` kiểm tra một node hình sao có bo góc trong Figma, đảm bảo `geometry` xuất hiện và có các giá trị pixel chính xác.
- Chạy `create_star` với `outerRadius=100`, `innerRadius=50`, `pointCount=5`. Kiểm tra Figma xem node sinh ra có kích thước 200x200 và tỷ lệ innerRadius là 0.5 hay không.

# Specs: FigJam Connectors cho Figma MCP Plugin

## 1. Giới thiệu
Connectors (Đường nối) là một tính năng đặc trưng của FigJam, cho phép người dùng nối các điểm hoặc các Node (như sticky notes, shapes) lại với nhau để tạo thành sơ đồ (flowchart, mindmap). MCP Plugin cung cấp công cụ `create_connector` để thao tác trực tiếp với các đường nối này.

## 2. Ràng buộc Môi trường (FigJam Only)
API của Figma yêu cầu các thao tác liên quan tới Connector chỉ có thể chạy trong một file FigJam (`figma.editorType === "figjam"`). Nếu người dùng cố gắng gọi từ Figma Design thông thường, MCP Plugin sẽ kiểm tra và trả về lỗi: 
> "create_connector is only supported in FigJam files"

## 3. Tạo Connector (`create_connector`)

### Input Payload
```json
{
  "startNodeId": "1:1",
  "endNodeId": "2:2",
  "startPosition": { "x": 100, "y": 200 },
  "endPosition": { "x": 500, "y": 200 },
  "lineType": "ELBOW"
}
```
*Ghi chú*: Cần cung cấp ít nhất một điểm bắt đầu và một điểm kết thúc (có thể là bằng ID của Node hoặc bằng Toạ độ).

### Logic khởi tạo & Toán học

1. **Khởi tạo**: Gọi `const connector = figma.createConnector()`.
2. **Cấu hình điểm bắt đầu (`connectorStart`)**:
   - Nếu có `startNodeId`, gán `endpointNodeId` và dùng nam châm từ tính `magnet = "AUTO"` để Figma tự động chọn điểm bám dính tốt nhất trên viền của Node:
     ```typescript
     connector.connectorStart = { endpointNodeId: startNode.id, magnet: "AUTO" };
     ```
   - Nếu dùng toạ độ `startPosition`, gán thẳng toạ độ `{x, y}` lên canvas:
     ```typescript
     connector.connectorStart = { position: p.startPosition };
     ```
3. **Cấu hình điểm kết thúc (`connectorEnd`)**:
   - Tương tự như điểm bắt đầu, hỗ trợ cả `endNodeId` hoặc `endPosition`.
4. **Định hình (`connectorLineType`)**:
   - Connector có thể vẽ theo kiểu đường thẳng (`STRAIGHT`) hoặc gấp khúc (`ELBOW`). Nếu được cung cấp `lineType`, gán `connector.connectorLineType = p.lineType`.

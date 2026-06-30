# Specs: Quản lý Components & Instances cho Figma MCP Plugin

## 1. Giới thiệu
Figma sử dụng hệ thống Component và Instance để tái sử dụng UI. Mỗi Component có thể là độc lập (`COMPONENT`) hoặc nằm trong một bộ (`COMPONENT_SET`). MCP Plugin hỗ trợ khả năng tạo Component Instance từ ID hoặc Component Key, cũng như đọc/ghi các thuộc tính ghi đè (overrides).

## 2. Tạo Component Instance (`create_component_instance`)

### Input Payload
```json
{
  "componentId": "1:2",
  "componentKey": "abc123xyz...",
  "parentId": "3:4",
  "x": 100,
  "y": 200
}
```

### Logic khởi tạo
1. **Tìm kiếm base component**: 
   - Nếu cung cấp `componentId`, dùng `figma.getNodeByIdAsync`.
   - Nếu cung cấp `componentKey`, dùng `figma.importComponentByKeyAsync`.
2. **Xử lý Component Set**:
   - Nếu node tìm được là `COMPONENT_SET`, tự động chọn `defaultVariant` để làm base component.
   - Nếu không có `defaultVariant`, chọn variant đầu tiên trong mảng `children`.
3. **Tạo Instance**: Gọi `baseComponent.createInstance()`.
4. **Định vị & Gắn vào cây (Parent)**:
   - Nếu có `parentId`, gán instance vào parent đó.
   - Nếu không, gán vào `figma.currentPage`.
   - Cập nhật toạ độ `x, y` nếu có truyền. Nếu không truyền và parent là `PAGE`, tự động căn giữa viewport:
     ```typescript
     instance.x = figma.viewport.center.x - instance.width / 2;
     instance.y = figma.viewport.center.y - instance.height / 2;
     ```

## 3. Đọc Instance Overrides (`get_instance_overrides`)

### Mục đích
Đọc danh sách các Component Properties (tương đương với overrides ở panel bên phải trong Figma) đang được gán cho một instance.

### Logic
- Tìm Node bằng `nodeId`. 
- Đảm bảo type là `INSTANCE`.
- Gọi `instance.componentProperties`. Trả về mapping từ Tên Property sang object chứa `type` và `value`.

## 4. Ghi Instance Overrides (`set_instance_overrides`)

### Input Payload
```json
{
  "nodeId": "1:1",
  "properties": {
    "Size": "Large",
    "Show Icon": true
  }
}
```

### Logic
- Tìm Node bằng `nodeId` (phải là `INSTANCE`).
- Dùng `instance.setProperties(properties)` để truyền nguyên mapping object `{ [propertyName: string]: value }` vào.
- Phương pháp tiếp cận "Fail-fast": Nếu properties truyền vào không hợp lệ (sai tên, sai kiểu), API của Figma sẽ throw Error, MCP sẽ bắt lỗi và trả về cho client.

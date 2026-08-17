# Specs: Dev Mode Annotations cho Figma MCP Plugin

## 1. Giới thiệu
Dev Mode Annotations là một tính năng đặc biệt của Figma dành cho quá trình chuyển giao (Handoff) từ thiết kế sang lập trình. Người dùng (có Dev Mode seat) có thể gắn các "ghi chú" lên bề mặt thiết kế chứa các thuộc tính (properties) như kích thước, màu sắc, border radius, v.v.

MCP Plugin hỗ trợ thao tác Ghi (Gán mới / Ghi đè) và Xoá Annotations.
*(API đọc `get_annotations` đã được triển khai độc lập)*

## 2. Ràng buộc Môi trường (Paid Users / Dev Mode Seat)
- **Hiển thị trên giao diện**: Chỉ người dùng có giấy phép (license) trả phí với quyền Dev Mode mới có thể nhìn thấy Annotations trên màn hình giao diện.
- **Tầng API (Mức kỹ thuật)**: API của plugin (`node.annotations`) vẫn cho phép đọc và ghi data nội bộ vào Node ngay cả khi người dùng không có quyền hiển thị. MCP lợi dụng đặc điểm này để LLMs có thể hỗ trợ chuẩn bị sẵn tài liệu kỹ thuật trên thiết kế mà không gặp lỗi.

## 3. Ghi Annotations (`set_annotations`)

### Input Payload
```json
{
  "nodeId": "1:1",
  "annotations": [
    {
      "label": "Button Container",
      "properties": [
        { "type": "width" },
        { "type": "fills" },
        { "type": "cornerRadius" }
      ]
    }
  ]
}
```

### Logic Ghi đè
Figma API định nghĩa thuộc tính `annotations` của một Node dưới dạng `ReadonlyArray<Annotation>`. Việc thêm mới yêu cầu phải gán lại toàn bộ mảng thay vì phương thức `.push()`:
```typescript
(node as any).annotations = p.annotations;
```
*Lưu ý:* Việc gán này sẽ thay thế toàn bộ Annotations cũ đang có trên Node đó.

## 4. Xoá Annotations (`clear_annotations`)

### Mục đích
Làm sạch (clear) toàn bộ các ghi chú kỹ thuật hiện có trên một hoặc nhiều node cùng lúc.

### Input Payload
```json
{
  "nodeIds": ["1:1", "1:2"]
}
```

### Logic
Lặp qua danh sách các ID, kiểm tra node có hỗ trợ thuộc tính `annotations` hay không (`"annotations" in node`), sau đó gán mảng rỗng:
```typescript
(node as any).annotations = [];
```

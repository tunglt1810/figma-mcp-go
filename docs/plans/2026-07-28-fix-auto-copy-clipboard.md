# Implementation Plan: Triệt để sửa lỗi Auto-copy Clipboard trong Figma MCP Go

Khắc phục triệt để lỗi `"Auto-copy disabled (browser blocked it)"` khi chọn element trong Figma bằng cách chuyển giao thao tác copy sang Native OS Clipboard trên Go Server thông qua kết nối WebSocket bridge, loại bỏ hoàn toàn rào cản Transient User Activation của Chromium Iframe Security Policy 2026.

## User Review Required

> [!IMPORTANT]
> - **Go Backend Native Clipboard**: Go Server sẽ tự động ghi nội dung được chọn vào OS Clipboard (macOS `pbcopy`, Windows `clip`, Linux `xclip`/`wl-copy`) ngay khi nhận message `copy_to_clipboard` từ Figma Plugin UI qua WebSocket.
> - **Fallback UX**: Khi Plugin không kết nối với Go Server local, giao diện Plugin UI sẽ hiển thị hướng dẫn trực quan thay vì liên tục hiện banner đỏ làm phiền người dùng.

## Proposed Changes

### Component 1: Go Server (`internal/`)

#### [NEW] [clipboard.go](file:///Users/bez/Workspace/repos/bez/figma-mcp-go/internal/clipboard.go)
- Tạo module `WriteOSClipboard(text string) error` hỗ trợ ghi clipboard đa nền tảng (macOS, Windows, Linux) sử dụng native command runner không cần phụ thuộc thư viện bên ngoài.

#### [MODIFY] [types.go](file:///Users/bez/Workspace/repos/bez/figma-mcp-go/internal/types.go)
- Thêm trường `Text string `json:"text,omitempty"`` vào `BridgeResponse` để nhận nội dung copy từ plugin UI.

#### [MODIFY] [bridge.go](file:///Users/bez/Workspace/repos/bez/figma-mcp-go/internal/bridge.go)
- Trong `readLoop`, xử lý sự kiện `resp.Type == "copy_to_clipboard"`. Khi nhận message này, gọi `WriteOSClipboard(resp.Text)` và log phản hồi thành công.

---

### Component 2: Plugin Svelte UI (`plugin/src/ui/`)

#### [MODIFY] [App.svelte](file:///Users/bez/Workspace/repos/bez/figma-mcp-go/plugin/src/ui/App.svelte)
- Cập nhật logic `onselectionchange`: Khi `autoCopyEnabled` bật và chọn node, nếu WebSocket đang mở (`socket?.readyState === WebSocket.OPEN`), gửi WS message `{ type: "copy_to_clipboard", text: ids }` trực tiếp tới Go server.
- Cập nhật `copyToClipboard`: Nếu WS mở, đồng thời gửi cho Go Server để đảm bảo copy thành công ngay cả khi browser block DOM copy.
- Cải thiện Banner thông báo khi mất kết nối WS: Giải thích rõ ràng cơ chế bảo mật browser và gợi ý kết nối Go MCP Server để tự động copy.

## Verification Plan

### Automated Tests
- Chạy unit test Go backend: `go test ./internal/...`
- Kiểm tra build plugin Svelte: `cd plugin && npm run build`

### Manual Verification
- Kiểm tra chọn Node trên Canvas Figma khi kết nối Go Server: Nội dung ID tự động ghi vào OS Clipboard của hệ thống.
- Kiểm tra Paste (Cmd+V / Ctrl+V) để xác nhận ID đã được ghi đúng vào Clipboard hệ thống.

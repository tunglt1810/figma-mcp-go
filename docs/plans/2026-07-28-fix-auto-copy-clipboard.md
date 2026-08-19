# Implementation Plan: Fully Fix Auto-copy Clipboard in Figma MCP Go

Fully resolve the `"Auto-copy disabled (browser blocked it)"` error that occurs when selecting an element in Figma by moving the copy operation to the Native OS Clipboard on the Go Server through a WebSocket bridge, completely removing the Transient User Activation barrier imposed by Chromium Iframe Security Policy 2026.

## User Review Required

> [!IMPORTANT]
> - **Go Backend Native Clipboard**: The Go Server will automatically write the selected content to the OS Clipboard (macOS `pbcopy`, Windows `clip`, Linux `xclip`/`wl-copy`) as soon as it receives a `copy_to_clipboard` message from the Figma Plugin UI through WebSocket.
> - **Fallback UX**: When the Plugin is not connected to the local Go Server, the Plugin UI will show clear instructions instead of repeatedly displaying an intrusive red banner.

## Proposed Changes

### Component 1: Go Server (`internal/`)

#### [NEW] [clipboard.go](file:///Users/bez/Workspace/repos/bez/figma-mcp-go/internal/clipboard.go)
- Create a `WriteOSClipboard(text string) error` module that supports writing to the clipboard across macOS, Windows, and Linux using a native command runner without external library dependencies.

#### [MODIFY] [types.go](file:///Users/bez/Workspace/repos/bez/figma-mcp-go/internal/types.go)
- Add a `Text string` field with the JSON tag `json:"text,omitempty"` to `BridgeResponse` to receive copy content from the Plugin UI.

#### [MODIFY] [bridge.go](file:///Users/bez/Workspace/repos/bez/figma-mcp-go/internal/bridge.go)
- In `readLoop`, handle the `resp.Type == "copy_to_clipboard"` event. When this message is received, call `WriteOSClipboard(resp.Text)` and log a successful response.

---

### Component 2: Plugin Svelte UI (`plugin/src/ui/`)

#### [MODIFY] [App.svelte](file:///Users/bez/Workspace/repos/bez/figma-mcp-go/plugin/src/ui/App.svelte)
- Update `onselectionchange`: When `autoCopyEnabled` is enabled and a node is selected, if the WebSocket is open (`socket?.readyState === WebSocket.OPEN`), send the WS message `{ type: "copy_to_clipboard", text: ids }` directly to the Go Server.
- Update `copyToClipboard`: If the WS is open, also send the content to the Go Server to ensure the copy succeeds even when the browser blocks DOM clipboard access.
- Improve the WS-disconnection notification banner: clearly explain the browser security mechanism and suggest connecting the Go MCP Server for automatic copying.

## Verification Plan

### Automated Tests
- Run the Go backend unit tests: ` go test ./internal/...`
- Check the Svelte plugin build: ` cd plugin && npm run build`

### Manual Verification
- Select a node on the Figma canvas while connected to the Go Server: the node ID should be written automatically to the system OS Clipboard.
- Paste (Cmd+V / Ctrl+V) to confirm that the ID was written correctly to the system Clipboard.

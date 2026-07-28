# MCP Server Version Display on Plugin UI — Design Specification

**Date**: 2026-07-29  
**Status**: Approved  

---

## 1. Overview

This feature displays the running Go MCP Server version on the Figma Plugin UI status badge (e.g., `Connected (v0.1.0)`), using `server.json` as the Single Source of Truth for versioning, while ensuring 100% backward compatibility between old and new plugin client / server versions.

---

## 2. Design Details

### A. Single Source of Truth (Server Version)
- Use Go 1.16+ `//go:embed` feature in the Go codebase to read `server.json` (located at repo root).
- Parse `"version"` from `server.json` at runtime.
- If reading or parsing fails, fallback gracefully to `"dev"`.

### B. Client-Initiated Handshake Protocol (Backward Compatible)
- **Plugin UI (`App.svelte`)**: Upon WebSocket connection open (`onopen`), send a message:
  ```json
  { "type": "get_server_info" }
  ```
- **Go Bridge (`internal/bridge.go`)**:
  - In `readLoop()`, when a message with `Type == "get_server_info"` is received, immediately reply over WebSocket:
    ```json
    { "type": "server-info", "version": "<version>" }
    ```
- **Plugin UI Handling**:
  - In `ws.onmessage`, when a payload has `type === "server-info"`, update `serverVersion = payload.version`.
  - **Do NOT** forward `server-info` to `main.ts` (prevents `Unknown request type` errors).
  - When WS disconnects (`onclose`), reset `serverVersion = ""`.
  - In UI badge:
    - If `connected === true` and `serverVersion` is non-empty: Display `Connected (v<version>)`.
    - If `connected === true` and `serverVersion` is empty: Display `Connected`.
    - If `connected === false`: Display `Disconnected`.

---

## 3. Backward Compatibility Matrix

| Plugin UI Version | Go Server Version | Behavior |
| :--- | :--- | :--- |
| **New UI** | **New Server** | UI requests `get_server_info` → Server responds with `server-info` → Badge displays **`Connected (v0.1.0)`**. |
| **New UI** | **Old Server** | UI requests `get_server_info` → Old Server ignores empty `requestId` → `serverVersion` remains empty → Badge displays **`Connected`**. |
| **Old UI** | **New Server** | Old UI does not request `get_server_info` → Server never sends unsolicited frames → Old UI works normally with **`Connected`**. |

---

## 4. Affected Files
- `internal/version.go` [NEW]: Helper to embed `server.json` and export `GetVersion()`.
- `cmd/figma-mcp-go/main.go`: Use `internal.GetVersion()` instead of hardcoded `var version = "dev"`.
- `internal/bridge.go`: Accept `version` string in `NewBridge(version)` and handle `get_server_info` message type in `readLoop`.
- `internal/leader.go`: Pass `version` to `NewBridge(version)`.
- `plugin/src/ui/App.svelte`: Send `get_server_info` on WS open, handle `server-info` message, update status badge UI.

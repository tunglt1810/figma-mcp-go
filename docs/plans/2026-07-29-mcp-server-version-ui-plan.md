# MCP Server Version Display on Plugin UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Embed the MCP server version from `server.json` using `//go:embed` and display it in the Figma plugin UI status badge (`Connected (v0.1.1)`) via a backward-compatible WebSocket handshake (`get_server_info`).

**Architecture:** 
1. `version.go` at repository root embeds `server.json` and exports `GetVersion()`.
2. `internal/bridge.go` handles incoming `{ "type": "get_server_info" }` WebSocket frames and responds with `{ "type": "server-info", "version": b.version }`.
3. `plugin/src/ui/App.svelte` requests `get_server_info` on WebSocket open, parses `server-info`, and displays the version string in the status badge.

**Tech Stack:** Go 1.26 (std `embed`, `encoding/json`), Svelte 5, TypeScript, Vitest.

## Global Constraints

- Single Source of Truth for version: `server.json` (`"version": "0.1.1"`).
- Backward Compatibility: Old UI connecting to New Server or New UI connecting to Old Server MUST NOT crash or show error banners.
- Shell Commands: EVERY shell command line MUST start with a leading space character ` `.

---

### Task 1: Single Source of Truth Version Embed in Go

**Files:**
- Create: `version.go`
- Create: `version_test.go`
- Modify: `cmd/figma-mcp-go/main.go:18-48`
- Modify: `internal/leader.go:34-40`
- Modify: `internal/bridge.go:30-43`

**Interfaces:**
- Produces: `figmamcpgo.GetVersion() string` returning `"0.1.1"` (or `"dev"` if unparseable).
- Produces: `internal.NewBridge(version string) *Bridge` storing `b.version`.

- [ ] **Step 1: Write failing test for `version.go`**

Create `version_test.go`:
```go
package figmamcpgo

import (
	"testing"
)

func TestGetVersion(t *testing.T) {
	v := GetVersion()
	if v == "" || v == "dev" {
		t.Errorf("expected valid version from server.json, got %q", v)
	}
	if v != "0.1.1" {
		t.Errorf("expected 0.1.1, got %q", v)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: ` go test -v ./version_test.go`
Expected: FAIL (package/file not found)

- [ ] **Step 3: Implement `version.go`**

Create `version.go`:
```go
package figmamcpgo

import (
	_ "embed"
	"encoding/json"
)

//go:embed server.json
var serverJSON []byte

type serverConfig struct {
	Version string `json:"version"`
}

// GetVersion returns the version string embedded from server.json.
// Falls back to "dev" if missing or unparseable.
func GetVersion() string {
	var cfg serverConfig
	if err := json.Unmarshal(serverJSON, &cfg); err == nil && cfg.Version != "" {
		return cfg.Version
	}
	return "dev"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: ` go test -v .`
Expected: PASS (`TestGetVersion`)

- [ ] **Step 5: Pass `version` into `Bridge`**

In `internal/bridge.go`:
```go
type Bridge struct {
	mu      sync.RWMutex
	wmu     sync.Mutex
	conn    *websocket.Conn
	pending map[string]*pendingEntry
	counter atomic.Int64
	version string
}

func NewBridge(version string) *Bridge {
	return &Bridge{
		pending: make(map[string]*pendingEntry),
		version: version,
	}
}
```

In `internal/leader.go`:
```go
func NewLeader(ip string, port int, version string) *Leader {
	return &Leader{
		ip:      ip,
		port:    port,
		bridge:  NewBridge(version),
		version: version,
	}
}
```

In `cmd/figma-mcp-go/main.go`:
```go
	ver := figmamcpgo.GetVersion()
	logger.Printf("Starting figma-mcp-go %s", ver)

	node := internal.NewNode(*ip, *port, ver)
```

- [ ] **Step 6: Commit Task 1**

```bash
 git add version.go version_test.go cmd/figma-mcp-go/main.go internal/bridge.go internal/leader.go
 git commit -m "feat: embed server.json version as single source of truth in Go"
```

---

### Task 2: WebSocket `get_server_info` Handshake Protocol

**Files:**
- Modify: `internal/bridge.go:110-130`

**Interfaces:**
- Consumes: `{ "type": "get_server_info" }` incoming WebSocket frame.
- Produces: `{ "type": "server-info", "version": b.version }` response frame.

- [ ] **Step 1: Update `readLoop` in `internal/bridge.go`**

In `internal/bridge.go` inside `readLoop(conn *websocket.Conn)`:
Add handling for `resp.Type == "get_server_info"`:

```go
		if resp.Type == "get_server_info" {
			infoMsg := map[string]string{
				"type":    "server-info",
				"version": b.version,
			}
			b.wmu.Lock()
			if err := wsjson.Write(ctx, conn, infoMsg); err != nil {
				bridgeLogger.Printf("failed to write server-info: %v", err)
			}
			b.wmu.Unlock()
			continue
		}
```

- [ ] **Step 2: Verify Go build**

Run: ` go test ./...`
Expected: PASS

- [ ] **Step 3: Commit Task 2**

```bash
 git add internal/bridge.go
 git commit -m "feat: handle get_server_info in bridge readLoop"
```

---

### Task 3: Display Server Version on Figma Plugin UI

**Files:**
- Modify: `plugin/src/ui/App.svelte:20-95, 296-299`

**Interfaces:**
- Consumes: `{ "type": "server-info", "version": string }` frame from WebSocket.
- Produces: `serverVersion` state rendered in UI badge as `Connected (v0.1.1)`.

- [ ] **Step 1: Update `plugin/src/ui/App.svelte` state & WS handlers**

In `plugin/src/ui/App.svelte`:

1. Add state variable:
```typescript
  let serverVersion = "";
```

2. On WebSocket `onopen`:
```typescript
    ws.onopen = () => {
      connected = true;
      ws.send(JSON.stringify({ type: "get_server_info" }));
      parent.postMessage({ pluginMessage: { type: "ui-ready" } }, "*");
    };
```

3. On WebSocket `onclose`:
```typescript
    ws.onclose = () => {
      if (socket !== ws) return;
      connected = false;
      serverVersion = "";
      socket = null;
      activeRequests.clear();
      activeRequests = activeRequests;
      ...
    };
```

4. On WebSocket `onmessage`:
```typescript
    ws.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data);
        if (payload.type === "server-info") {
          serverVersion = payload.version ?? "";
          return;
        }
        if (payload.requestId) {
          activeRequests.add(payload.requestId);
          activeRequests = activeRequests;
        }
        parent.postMessage({ pluginMessage: { type: "server-request", payload } }, "*");
      } catch {
        // ignore malformed frames
      }
    };
```

5. Update UI badge template (around line 296):
```svelte
      <div class="badge" class:connected class:disconnected={!connected}>
        <span class="dot" class:connected></span>
        <span>{connected ? (serverVersion ? `Connected (v${serverVersion})` : "Connected") : "Disconnected"}</span>
      </div>
```

- [ ] **Step 2: Run frontend test & build**

Run: ` cd plugin && pnpm test && pnpm run build`
Expected: PASS (build outputs `dist/index.html` and `dist/code.js`)

- [ ] **Step 3: Commit Task 3**

```bash
 git add plugin/src/ui/App.svelte plugin/dist/
 git commit -m "feat: display mcp server version on plugin ui badge"
```

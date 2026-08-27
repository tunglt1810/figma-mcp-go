package internal

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// setupBridgeWithClient creates a Bridge with an active WebSocket client connected to it.
// Returns the bridge and the client-side connection (already cleaned up on t.Cleanup).
func setupBridgeWithClient(t *testing.T) (*Bridge, *websocket.Conn) {
	t.Helper()
	bridge := NewBridge("0.1.1")

	srv := httptest.NewServer(http.HandlerFunc(bridge.HandleUpgrade))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { clientConn.Close(websocket.StatusNormalClosure, "") })

	// Poll until bridge registers the server-side connection.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if bridge.IsConnected() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !bridge.IsConnected() {
		t.Fatal("bridge not connected after 500ms")
	}

	return bridge, clientConn
}

// ── NewBridge ─────────────────────────────────────────────────────────────────

func TestNewBridge(t *testing.T) {
	b := NewBridge("0.1.1")
	if b == nil {
		t.Fatal("NewBridge returned nil")
	}
	if b.IsConnected() {
		t.Error("new bridge should not be connected")
	}
}

// ── nextID ────────────────────────────────────────────────────────────────────

func TestBridgeNextID(t *testing.T) {
	b := NewBridge("0.1.1")
	id1 := b.nextID()
	id2 := b.nextID()

	if id1 == id2 {
		t.Error("consecutive IDs must be unique")
	}
	if !strings.HasPrefix(id1, "req-") {
		t.Errorf("ID %q does not have req- prefix", id1)
	}
	// Format: req-HHMMSS-N  (14 chars min: "req-000000-1")
	parts := strings.Split(id1, "-")
	if len(parts) != 3 {
		t.Errorf("ID %q has wrong format (want 3 dash-separated parts)", id1)
	}
}

// ── MarshalJSON ───────────────────────────────────────────────────────────────

func TestBridgeMarshalJSON_Disconnected(t *testing.T) {
	b := NewBridge("0.1.1")
	data, err := b.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var m map[string]any
	json.Unmarshal(data, &m)
	if m["connected"] != false {
		t.Errorf("connected = %v, want false", m["connected"])
	}
	if m["pending"] != float64(0) {
		t.Errorf("pending = %v, want 0", m["pending"])
	}
}

func TestBridgeMarshalJSON_Connected(t *testing.T) {
	b, _ := setupBridgeWithClient(t)
	data, err := b.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var m map[string]any
	json.Unmarshal(data, &m)
	if m["connected"] != true {
		t.Errorf("connected = %v, want true", m["connected"])
	}
}

// ── Close ─────────────────────────────────────────────────────────────────────

func TestBridgeClose_NoPanic(t *testing.T) {
	b := NewBridge("0.1.1")
	// Close on an unconnected bridge should not panic.
	b.Close()
}

func TestBridgeClose_DrainsPending(t *testing.T) {
	b, _ := setupBridgeWithClient(t)

	// Manually insert a pending entry so we can verify Close drains it.
	ch := make(chan BridgeResponse, 1)
	entry := &pendingEntry{ch: ch}
	entry.timer = time.AfterFunc(10*time.Second, func() {})

	b.mu.Lock()
	b.pending["test-id"] = entry
	b.mu.Unlock()

	b.Close()

	// Channel must be closed (receive returns zero value, ok=false).
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timed out waiting for channel to be closed")
	}
}

// ── Send ─────────────────────────────────────────────────────────────────────

func TestBridgeSend_NotConnected(t *testing.T) {
	b := NewBridge("0.1.1")
	_, err := b.Send(context.Background(), "get_node", []string{"1:1"}, nil)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestBridgeSend_ContextCancelled(t *testing.T) {
	b, _ := setupBridgeWithClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestBridgeSend_Success(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	// Goroutine: echo request back as a successful response.
	go func() {
		var req BridgeRequest
		if err := readJSON(ctx, clientConn, &req); err != nil {
			return
		}
		resp := BridgeResponse{
			RequestID: req.RequestID,
			Type:      req.Type,
			Data:      map[string]any{"id": "1:1", "name": "Frame 1"},
		}
		writeJSON(ctx, clientConn, resp) //nolint:errcheck
	}()

	got, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.Data == nil {
		t.Error("expected non-nil data in response")
	}
}

func TestBridgeSend_PluginError(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	go func() {
		var req BridgeRequest
		if err := readJSON(ctx, clientConn, &req); err != nil {
			return
		}
		resp := BridgeResponse{
			RequestID: req.RequestID,
			Error:     "node not found",
		}
		writeJSON(ctx, clientConn, resp) //nolint:errcheck
	}()

	got, err := b.Send(ctx, "get_node", []string{"9:9"}, nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if got.Error == "" {
		t.Error("expected error field from plugin")
	}
}

func TestBridgeSend_Timeout(t *testing.T) {
	b, _ := setupBridgeWithClient(t)
	// Don't send any response from the client — bridge should time out.
	// We manipulate the timeout via a very short context rather than waiting 30s.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
	if err == nil {
		t.Error("expected timeout error")
	}
}

// ── IsConnected ───────────────────────────────────────────────────────────────

func TestBridgeIsConnected(t *testing.T) {
	b := NewBridge("0.1.1")
	if b.IsConnected() {
		t.Error("should not be connected before any upgrade")
	}

	b2, _ := setupBridgeWithClient(t)
	if !b2.IsConnected() {
		t.Error("should be connected after upgrade")
	}
}

// The bridge skipped the Origin check entirely, and a new connection replaces
// the live one — so any page the user had open could connect to the local port,
// displace the real plugin and answer tool calls with whatever it liked.
func TestAllowedOrigin(t *testing.T) {
	allowed := []string{
		"",                      // non-browser client, sends no Origin
		"null",                  // Figma serves plugin UI in a sandboxed iframe
		"https://www.figma.com", // and a same-origin iframe in some contexts
		"https://figma.com",
		"http://localhost:5173", // plugin UI in dev
		"http://127.0.0.1:1994",
	}
	for _, origin := range allowed {
		if !allowedOrigin(origin) {
			t.Errorf("origin %q should be allowed", origin)
		}
	}

	denied := []string{
		"https://evil.com",
		"http://evil.com",
		"https://figma.com.evil.com",
		"https://notfigma.com",
	}
	for _, origin := range denied {
		if allowedOrigin(origin) {
			t.Errorf("origin %q should be denied", origin)
		}
	}
}

// A connection that dies without a close frame — laptop sleep, network drop —
// used to look alive until the next tool call timed out 30 seconds later. The
// keepalive notices instead: a client that has stopped reading never pongs.
func TestKeepalive_DropsAConnectionThatStopsAnswering(t *testing.T) {
	bridge := NewBridge("0.1.1")
	bridge.pingInterval = 20 * time.Millisecond
	bridge.pingTimeout = 60 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(bridge.HandleUpgrade))
	t.Cleanup(srv.Close)

	// A raw TCP connection speaking the handshake by hand: it never reads
	// frames, so it can never answer a ping.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { clientConn.Close(websocket.StatusNormalClosure, "") })

	waitFor(t, 500*time.Millisecond, bridge.IsConnected, "bridge to register the connection")

	// The client never calls Read, so the library never sends a pong.
	waitFor(t, 2*time.Second, func() bool { return !bridge.IsConnected() },
		"the bridge to drop the silent connection")
}

// A client that is reading normally answers pings, and the connection stays up.
func TestKeepalive_LeavesAHealthyConnectionAlone(t *testing.T) {
	bridge := NewBridge("0.1.1")
	bridge.pingInterval = 20 * time.Millisecond
	bridge.pingTimeout = 200 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(bridge.HandleUpgrade))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { clientConn.Close(websocket.StatusNormalClosure, "") })

	// Reading is what lets the library answer pings.
	ctx := t.Context()
	go func() {
		for {
			var msg map[string]any
			if err := readJSON(ctx, clientConn, &msg); err != nil {
				return
			}
		}
	}()

	waitFor(t, 500*time.Millisecond, bridge.IsConnected, "bridge to register the connection")

	time.Sleep(300 * time.Millisecond) // several ping rounds
	if !bridge.IsConnected() {
		t.Error("a connection answering pings was dropped")
	}
}

func waitFor(t *testing.T, limit time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", limit, what)
}

package cluster

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// ── handlePing ────────────────────────────────────────────────────────────────

func TestLeaderHandlePing_OK(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "v1.2.3", passthroughGuard)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	l.handlePing(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
	if body["version"] != "v1.2.3" {
		t.Errorf("version = %q, want v1.2.3", body["version"])
	}
}

func TestLeaderHandlePing_MethodNotAllowed(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "", passthroughGuard)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/ping", nil)
		w := httptest.NewRecorder()
		l.handlePing(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /ping: status = %d, want 405", method, w.Code)
		}
	}
}

// ── handleRPC ─────────────────────────────────────────────────────────────────

func TestLeaderHandleRPC_MethodNotAllowed(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "", passthroughGuard)

	req := httptest.NewRequest(http.MethodGet, "/rpc", nil)
	w := httptest.NewRecorder()
	l.handleRPC(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestLeaderHandleRPC_InvalidJSON(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "", passthroughGuard)

	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewBufferString("{bad json}"))
	w := httptest.NewRecorder()
	l.handleRPC(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var resp RPCResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == "" {
		t.Error("expected error in response body")
	}
}

// What the leader owes the guard: run it, and turn a rejection into a 400 that
// carries the reason. What the rejection means is the tool table's business,
// tested where the table lives.
func TestLeaderHandleRPC_GuardRejectionBecomes400(t *testing.T) {
	rejecting := func(string, []string, map[string]any) ([]string, map[string]any, error) {
		return nil, nil, errors.New("text is required")
	}
	l := NewLeader("127.0.0.1", 0, "", rejecting)

	body, _ := json.Marshal(RPCRequest{
		Tool:    "set_text",
		NodeIDs: []string{"1:1"},
		Params:  map[string]any{},
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()
	l.handleRPC(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var resp RPCResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error != "text is required" {
		t.Errorf("error = %q, want the guard's own message", resp.Error)
	}
}

func TestLeaderHandleRPC_BridgeNotConnected(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "", passthroughGuard)

	// get_document has no required params — passes validation, hits bridge
	body, _ := json.Marshal(RPCRequest{Tool: "get_document"})
	req := httptest.NewRequest(http.MethodPost, "/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()
	l.handleRPC(w, req)

	// Bridge returns "plugin not connected" error → 200 with error field
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp RPCResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == "" {
		t.Error("expected 'plugin not connected' error in response")
	}
}

// ── Start / Stop ──────────────────────────────────────────────────────────────

func TestLeaderStart_BindsPort(t *testing.T) {
	port := freePort(t)
	l := NewLeader("127.0.0.1", port, "", passthroughGuard)

	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(l.Stop)

	// Second leader on the same port must fail.
	l2 := NewLeader("127.0.0.1", port, "", passthroughGuard)
	if err := l2.Start(); err == nil {
		l2.Stop()
		t.Error("expected error when binding already-used port")
	}
}

func TestLeaderStop_FreesPort(t *testing.T) {
	port := freePort(t)
	l := NewLeader("127.0.0.1", port, "", passthroughGuard)

	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	l.Stop()

	// Allow OS to release the port.
	time.Sleep(20 * time.Millisecond)

	l2 := NewLeader("127.0.0.1", port, "", passthroughGuard)
	if err := l2.Start(); err != nil {
		t.Fatalf("port should be free after Stop: %v", err)
	}
	l2.Stop()
}

func TestLeaderStop_Idempotent(t *testing.T) {
	l := NewLeader("127.0.0.1", 0, "", passthroughGuard)
	// Stop on a never-started leader should not panic.
	l.Stop()
	l.Stop()
}

// ── /ping endpoint (integration via httptest.Server) ─────────────────────────

func TestLeaderPingEndpoint(t *testing.T) {
	port := freePort(t)
	l := NewLeader("127.0.0.1", port, "test-ver", passthroughGuard)
	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(l.Stop)

	f := NewFollower("http://127.0.0.1:" + itoa(port))
	if !f.Ping(t.Context()) {
		t.Error("expected ping to succeed for running leader")
	}
}

// A WebSocket has to outlive the header deadline: the handshake is an ordinary
// HTTP request, so ReadHeaderTimeout applies to it, and the long-lived socket
// that follows must not inherit anything from it.
func TestLeaderStart_WebSocketOutlivesTheHeaderTimeout(t *testing.T) {
	port := freePort(t)
	leader := NewLeader("127.0.0.1", port, "test", passthroughGuard)
	leader.readHeaderTimeout = 100 * time.Millisecond
	if err := leader.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(leader.Stop)

	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
	client, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { client.Close(websocket.StatusNormalClosure, "") })

	// Well past the header deadline.
	time.Sleep(400 * time.Millisecond)

	if !leader.GetBridge().IsConnected() {
		t.Fatal("the plugin socket was closed by an HTTP server timeout")
	}
}

func TestLeaderStart_SetsAHeaderTimeoutAndNoWriteTimeout(t *testing.T) {
	port := freePort(t)
	leader := NewLeader("127.0.0.1", port, "test", passthroughGuard)
	if err := leader.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(leader.Stop)

	if leader.server.ReadHeaderTimeout == 0 {
		t.Error("ReadHeaderTimeout must be set — an idle connection can hold a slot forever")
	}
	if leader.server.WriteTimeout != 0 {
		t.Error("WriteTimeout must stay zero — it would cap a /rpc response that is allowed to take MaxToolTimeout")
	}
	if leader.server.ReadTimeout != 0 {
		t.Error("ReadTimeout must stay zero — it would cap reading a 32 MB /rpc body")
	}
}

// /ping is the only thing a user can query when something is wrong. "ok" alone
// does not distinguish a leader with no plugin from a healthy one.
func TestLeaderPing_ReportsState(t *testing.T) {
	port := freePort(t)
	leader := NewLeader("127.0.0.1", port, "9.9.9", passthroughGuard)
	if err := leader.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(leader.Stop)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", port))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	var got map[string]any
	if err := json.UnmarshalRead(resp.Body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, key := range []string{"status", "version", "role", "connected", "pending", "uptimeSeconds"} {
		if _, ok := got[key]; !ok {
			t.Errorf("/ping is missing %q: %v", key, got)
		}
	}
	if got["connected"] != false {
		t.Errorf("want connected=false with no plugin, got %v", got["connected"])
	}
	if got["version"] != "9.9.9" {
		t.Errorf("want version 9.9.9, got %v", got["version"])
	}
}

package bridge

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"log/slog"
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
	ch := make(chan Response, 1)
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
	// No handover is in progress here, so there is nothing to wait for.
	b.connectGrace = 10 * time.Millisecond
	_, err := b.Send(context.Background(), "get_nodes_info", []string{"1:1"}, nil)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestBridgeSend_ContextCancelled(t *testing.T) {
	b, _ := setupBridgeWithClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := b.Send(ctx, "get_nodes_info", []string{"1:1"}, nil)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestBridgeSend_Success(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	// Goroutine: echo request back as a successful response.
	go func() {
		var req Request
		if err := readJSON(ctx, clientConn, &req); err != nil {
			return
		}
		resp := Response{
			RequestID: req.RequestID,
			Type:      req.Type,
			Data:      map[string]any{"id": "1:1", "name": "Frame 1"},
		}
		writeJSON(ctx, clientConn, resp) //nolint:errcheck
	}()

	got, err := b.Send(ctx, "get_nodes_info", []string{"1:1"}, nil)
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
		var req Request
		if err := readJSON(ctx, clientConn, &req); err != nil {
			return
		}
		resp := Response{
			RequestID: req.RequestID,
			Error:     "node not found",
		}
		writeJSON(ctx, clientConn, resp) //nolint:errcheck
	}()

	got, err := b.Send(ctx, "get_nodes_info", []string{"9:9"}, nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if got.Error == "" {
		t.Error("expected error field from plugin")
	}
}

// This was TestBridgeSend_Timeout, which named the bridge's own tool timer and
// tested the caller's deadline instead — a different branch, a different error,
// and green whatever the timer did. Both branches are worth pinning, so this is
// the one it actually tested, under the name of what it does.
func TestBridgeSend_CallerDeadlineEndsTheWait(t *testing.T) {
	b, _ := setupBridgeWithClient(t)
	// The client never answers, so only the deadline can end this.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := b.Send(ctx, "get_nodes_info", []string{"1:1"}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
	if n := b.Pending(); n != 0 {
		t.Errorf("pending = %d after the caller gave up, want 0", n)
	}
}

// And the branch the old name promised: the plugin takes the request and never
// answers, so the bridge's own timer for that tool fires. Nothing exercised it,
// because at 30 seconds no test could afford to wait for it.
func TestBridgeSend_TimesOutWhenThePluginNeverAnswers(t *testing.T) {
	b, _ := setupBridgeWithClient(t)
	b.toolTimeout = func(string) time.Duration { return 50 * time.Millisecond }

	start := time.Now()
	_, err := b.Send(context.Background(), "get_nodes_info", []string{"1:1"}, nil)
	if err == nil || err.Error() != "request timed out" {
		t.Fatalf("err = %v, want \"request timed out\"", err)
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("Send took %s — that is not the 50ms tool timer", took)
	}
	if n := b.Pending(); n != 0 {
		t.Errorf("pending = %d after a timeout, want 0", n)
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

// A ping that could not be completed is not proof the peer is gone. Ping goes
// through writeControl, which caps its own wait for the frame lock at 5s
// (write.go:232), so a large send still draining to a healthy plugin fails the
// ping while the plugin is fine — and the keepalive dropped the connection on
// that first failure. A plugin that is still sending us messages is
// demonstrably alive and gets a few more rounds. Only a few: the keepalive is
// also the only thing that clears a write parked on a full socket buffer.
func TestKeepalive_ForgivesAFailedPingWhileThePluginIsStillTalking(t *testing.T) {
	bridge := NewBridge("0.1.1")
	bridge.pingInterval = 100 * time.Millisecond
	bridge.pingTimeout = 150 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(bridge.HandleUpgrade))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { client.Close(websocket.StatusNormalClosure, "") })
	waitFor(t, 500*time.Millisecond, bridge.IsConnected, "the bridge to register the connection")

	// The client never reads, so a big enough frame parks on a full socket
	// buffer holding the library's frame lock — every ping from here on fails
	// on that lock, not on the peer. But the client keeps sending, so the peer
	// is plainly alive.
	big := strings.Repeat("x", 8<<20)
	go bridge.Send(context.Background(), "get_document", nil, map[string]any{"blob": big}) //nolint:errcheck

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(25 * time.Millisecond):
				if err := writeJSON(context.Background(), client, Response{Progress: 1, RequestID: "none"}); err != nil {
					return
				}
			}
		}
	}()

	// Two ping rounds in, the old code has already dropped it.
	time.Sleep(300 * time.Millisecond)
	if !bridge.IsConnected() {
		t.Fatal("the keepalive dropped a plugin that was still sending messages")
	}

	// Forgiveness is bounded, or the parked write would never be cleared.
	waitFor(t, 2*time.Second, func() bool { return !bridge.IsConnected() },
		"the keepalive to drop the connection once forgiveness ran out")
}

// The server-info reply used to be written on the read goroutine. That is the
// one goroutine that has to be inside conn.Read for the library to process
// anything the peer sends, pongs included: handleControl is only reached from
// reader (read.go:289, :368). So a reply parked behind another write stopped
// this connection being read at all — pings went unanswered and the keepalive
// dropped a plugin that was perfectly healthy, and a close frame went unnoticed.
func TestReadLoop_KeepsReadingWhileAServerInfoReplyIsParked(t *testing.T) {
	b, client := setupBridgeWithClient(t)

	// Hold the write slot, so the reply cannot go out.
	b.wslot <- struct{}{}
	t.Cleanup(func() { <-b.wslot })

	if err := writeJSON(t.Context(), client, map[string]string{"type": "get_server_info"}); err != nil {
		t.Fatalf("write get_server_info: %v", err)
	}

	// Let the reply park on the slot, then hang up. A read loop that is still
	// reading notices; one waiting behind the write does not.
	time.Sleep(100 * time.Millisecond)
	client.Close(websocket.StatusNormalClosure, "") //nolint:errcheck

	waitFor(t, 2*time.Second, func() bool { return !b.IsConnected() },
		"the read loop to notice the client hung up")
}

// The panel raises its confirm guard when the server says its listener is
// reachable from the network, so this flag is the whole of that signal.
func TestReplyServerInfo_ReportsWhetherTheListenerIsExposed(t *testing.T) {
	for _, exposed := range []bool{false, true} {
		b, client := setupBridgeWithClient(t)
		b.SetExposed(exposed)

		if err := writeJSON(t.Context(), client, map[string]string{"type": "get_server_info"}); err != nil {
			t.Fatalf("write get_server_info: %v", err)
		}

		var info struct {
			Type    string `json:"type"`
			Version string `json:"version"`
			Exposed bool   `json:"exposed"`
		}
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		if err := readJSON(ctx, client, &info); err != nil {
			cancel()
			t.Fatalf("read server-info: %v", err)
		}
		cancel()

		if info.Type != "server-info" {
			t.Fatalf("frame type = %q, want server-info", info.Type)
		}
		if info.Exposed != exposed {
			t.Errorf("exposed = %v, want %v", info.Exposed, exposed)
		}
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

// Cancelling one request used to close the whole WebSocket. Send passed the
// caller's context to conn.Write, and for the duration of a write the library
// registers context.AfterFunc(ctx, c.close) (conn.go:171, write.go:276) — so a
// cancel landing while the write was in flight dropped the socket for every
// other request too.
//
// The window is only open while the write is blocked, which on loopback means
// never for a small payload. This test forces it open: the client never reads,
// so a large enough frame fills the socket buffer and the write parks there.
func TestSend_CancellingMidWriteLeavesTheConnectionUp(t *testing.T) {
	b, _ := setupBridgeWithClient(t)

	// Big enough to overflow the socket buffers in both directions.
	big := strings.Repeat("x", 8<<20)

	ctx, cancel := context.WithCancel(context.Background())
	go b.Send(ctx, "get_document", nil, map[string]any{"blob": big}) //nolint:errcheck

	// Give the write time to park on a full buffer, then hang up on it.
	time.Sleep(200 * time.Millisecond)
	cancel()

	// Do not wait for Send to return: the parked write only unblocks once the
	// keepalive drops this deliberately deaf client, seconds later. What is
	// being measured is whether the cancel itself took the connection down, so
	// watch the connection for a window well inside the ping interval.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !b.IsConnected() {
			t.Fatal("cancelling one request closed the shared plugin connection")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Writing with a context that never cancels keeps one caller's cancel from
// closing the shared socket, but it leaves a write parked on a full socket
// buffer with no escape but the keepalive, which takes up to three ping rounds
// — about a minute with production defaults. b.wmu was a sync.Mutex, which
// consults nothing, so every other caller in the process waited out that whole
// window regardless of the deadline it arrived with.
func TestSend_HonoursTheCallersDeadlineWhileAnotherWriteIsParked(t *testing.T) {
	b, _ := setupBridgeWithClient(t)

	// The client never reads, so a big enough frame fills the socket buffers
	// and the write parks there holding the write lock.
	big := strings.Repeat("x", 8<<20)
	go b.Send(context.Background(), "get_document", nil, map[string]any{"blob": big}) //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	waited := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		b.Send(ctx, "get_nodes_info", []string{"1:1"}, nil) //nolint:errcheck
		waited <- time.Since(start)
	}()

	select {
	case took := <-waited:
		if took > time.Second {
			t.Fatalf("a Send with a 200ms deadline returned after %s", took)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a Send with a 200ms deadline was still blocked after 2s behind a parked write")
	}

	// The escape must be the caller giving up, not the socket being dropped —
	// that would be the defect TestSend_CancellingMidWriteLeavesTheConnectionUp
	// pins, reintroduced by another route.
	if !b.IsConnected() {
		t.Error("giving up on the write lock closed the shared plugin connection")
	}
}

// setupBridgeWithClient's client never calls Read, so the library on that side
// never answers a close frame — the same trick the keepalive tests use. Close
// used to sit on the handshake for the library's 5s budget (close.go:199) plus
// up to 15s in waitGoroutines (close.go:231), delaying process exit for a
// plugin that was already gone.
func TestClose_IsBoundedWhenThePeerNeverAnswers(t *testing.T) {
	b, _ := setupBridgeWithClient(t)
	b.closeGrace = 100 * time.Millisecond

	done := make(chan struct{})
	go func() {
		defer close(done)
		b.Close()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close blocked on a close handshake the peer will never answer")
	}
}

// HandleUpgrade used to close the displaced connection gracefully while holding
// b.mu. A peer still alive at TCP level but not answering — laptop asleep, Figma
// reloading its UI — makes the library spend its whole handshake budget
// (close.go:199), and with the lock held that freezes every Send, IsConnected,
// Pending and MarshalJSON in the process. Close already bounds the same
// handshake; this is the reconnect path getting the same treatment.
func TestHandleUpgrade_DoesNotHoldTheLockAcrossTheCloseHandshake(t *testing.T) {
	b := NewBridge("0.1.1")
	b.closeGrace = 100 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	// The displaced peer never calls Read, so it never answers a close frame —
	// the same trick the keepalive and Close tests use.
	first, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { first.Close(websocket.StatusNormalClosure, "") })
	waitFor(t, 500*time.Millisecond, b.IsConnected, "the bridge to register the first connection")

	// Dial returns on the 101, so HandleUpgrade is still inside its critical
	// section for the replacement when this returns.
	second, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { second.Close(websocket.StatusNormalClosure, "") })

	worst := time.Duration(0)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		start := time.Now()
		b.Pending()
		if waited := time.Since(start); waited > worst {
			worst = waited
		}
		time.Sleep(5 * time.Millisecond)
	}
	if worst > 250*time.Millisecond {
		t.Fatalf("a reader waited %s for b.mu while the replaced connection was being closed", worst)
	}
}

// After a takeover the plugin needs about 1.5s to notice and reconnect
// (RECONNECT_DELAY_MS in plugin/src/ui/App.svelte). Failing instantly through
// that window reports "plugin not connected" for a plugin that is on its way
// back.
func TestSend_WaitsBrieflyForAReconnectingPlugin(t *testing.T) {
	b := NewBridge("0.1.1")
	b.connectGrace = 2 * time.Second

	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)

	// The plugin arrives after Send has already started waiting.
	go func() {
		time.Sleep(200 * time.Millisecond)
		wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
		client, _, err := websocket.Dial(context.Background(), wsURL, nil)
		if err != nil {
			return
		}
		for {
			var req Request
			if err := readJSON(context.Background(), client, &req); err != nil {
				return
			}
			writeJSON(context.Background(), client, Response{ //nolint:errcheck
				Type:      req.Type,
				RequestID: req.RequestID,
				Data:      map[string]any{"ok": true},
			})
		}
	}()

	resp, err := b.Send(context.Background(), "get_document", nil, nil)
	if err != nil {
		t.Fatalf("Send gave up on a plugin that was reconnecting: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("plugin error: %s", resp.Error)
	}
}

// The wait must not turn a plugin that never arrives into a hang.
func TestSend_StillReportsAPluginThatNeverArrives(t *testing.T) {
	b := NewBridge("0.1.1")
	b.connectGrace = 50 * time.Millisecond

	_, err := b.Send(context.Background(), "get_document", nil, nil)
	if err == nil {
		t.Fatal("expected an error when no plugin connects")
	}
	if !strings.Contains(err.Error(), "plugin not connected") {
		t.Errorf("unexpected message: %v", err)
	}
}

// The params map holds whatever the user is designing — text content, colours,
// names. It is fine at debug, where someone asked for it. It is not fine in the
// default output.
func TestSend_DoesNotLogParamsAtInfo(t *testing.T) {
	buf := captureLogs(t, slog.LevelInfo)

	b, _ := setupBridgeWithClient(t)
	sendAndIgnore(b, "set_text", map[string]any{"text": "Quarterly revenue projection"})

	if strings.Contains(buf.String(), "Quarterly revenue projection") {
		t.Errorf("params reached the default log:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "set_text") {
		t.Errorf("the tool name should still be logged:\n%s", buf.String())
	}
}

func TestSend_LogsParamsAtDebug(t *testing.T) {
	buf := captureLogs(t, slog.LevelDebug)

	b, _ := setupBridgeWithClient(t)
	sendAndIgnore(b, "set_text", map[string]any{"text": "Quarterly revenue projection"})

	if !strings.Contains(buf.String(), "Quarterly revenue projection") {
		t.Errorf("debug should carry the params:\n%s", buf.String())
	}
}

// captureLogs points the default logger at a buffer for the length of a test.
func captureLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	restore := slog.Default()
	t.Cleanup(func() { slog.SetDefault(restore) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})))
	return &buf
}

// sendAndIgnore fires a request and returns as soon as it has been logged. The
// client never answers, so waiting for the reply would mean waiting out the
// tool's whole budget.
func sendAndIgnore(b *Bridge, tool string, params map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	b.Send(ctx, tool, []string{"1:1"}, params) //nolint:errcheck
}

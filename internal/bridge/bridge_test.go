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

// setupBridgeWithClient creates a Bridge with a WebSocket client connected to it.
// It returns the bridge and the client-side connection. t.Cleanup closes both.
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

	// Poll until the bridge registers the server-side connection.
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
	// Format: req-HHMMSS-N (at least 14 chars: "req-000000-1")
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

	// Insert a pending entry by hand, to check that Close drains it.
	ch := make(chan Response, 1)
	entry := &pendingEntry{ch: ch}
	entry.timer = time.AfterFunc(10*time.Second, func() {})

	b.mu.Lock()
	b.pending["test-id"] = entry
	b.mu.Unlock()

	b.Close()

	// The channel must be closed (receive returns the zero value, ok=false).
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

	// Goroutine: echo the request back as a successful response.
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

// This test used to be TestBridgeSend_Timeout. That name pointed at the
// bridge's own tool timer, but the test checked the caller's deadline: a
// different branch with a different error, and it passed whatever the timer
// did. Both branches are worth testing, so this one now has a name that says
// what it tests.
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
// answers, so the bridge's own timer for that tool fires. No test covered it,
// because no test could wait the real 30 seconds.
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

// The bridge used to skip the Origin check, and a new connection replaces the
// live one. So any open page could connect to the local port, push out the
// real plugin, and answer tool calls with anything it liked.
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

// A connection can die without a close frame (laptop sleep, network drop). It
// used to look alive until the next tool call timed out 30 seconds later. Now
// the keepalive notices: a client that has stopped reading never sends a pong.
func TestKeepalive_DropsAConnectionThatStopsAnswering(t *testing.T) {
	bridge := NewBridge("0.1.1")
	bridge.pingInterval = 20 * time.Millisecond
	bridge.pingTimeout = 60 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(bridge.HandleUpgrade))
	t.Cleanup(srv.Close)

	// A raw TCP connection that does the handshake by hand. It never reads
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

// A failed ping does not prove the peer is gone. Ping goes through
// writeControl, which waits at most 5s for the frame lock (write.go:232). So a
// large send that a healthy plugin is still reading makes the ping fail, and
// the keepalive used to drop the connection on that first failure. A plugin
// that is still sending us messages is clearly alive, so it gets a few more
// rounds. Only a few: the keepalive is also the only thing that clears a write
// stuck on a full socket buffer.
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

	// The client never reads, so a big enough frame gets stuck on a full socket
	// buffer while holding the library's frame lock. Every ping after that fails
	// on the lock, not because of the peer. But the client keeps sending, so the
	// peer is clearly alive.
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

	// After two ping rounds, the old code has already dropped it.
	time.Sleep(300 * time.Millisecond)
	if !bridge.IsConnected() {
		t.Fatal("the keepalive dropped a plugin that was still sending messages")
	}

	// Forgiveness has a limit, or the stuck write would never be cleared.
	waitFor(t, 2*time.Second, func() bool { return !bridge.IsConnected() },
		"the keepalive to drop the connection once forgiveness ran out")
}

// The server-info reply used to be written on the read goroutine. That is the
// one goroutine that must be inside conn.Read for the library to handle
// anything the peer sends, pongs included: handleControl is only called from
// reader (read.go:289, :368). So a reply stuck behind another write stopped
// this connection from being read at all. Pings went unanswered, the keepalive
// dropped a healthy plugin, and a close frame went unnoticed.
func TestReadLoop_KeepsReadingWhileAServerInfoReplyIsParked(t *testing.T) {
	b, client := setupBridgeWithClient(t)

	// Hold the write slot, so the reply cannot go out.
	b.wslot <- struct{}{}
	t.Cleanup(func() { <-b.wslot })

	if err := writeJSON(t.Context(), client, map[string]string{"type": "get_server_info"}); err != nil {
		t.Fatalf("write get_server_info: %v", err)
	}

	// Let the reply wait on the slot, then hang up. A read loop that is still
	// reading notices. One stuck behind the write does not.
	time.Sleep(100 * time.Millisecond)
	client.Close(websocket.StatusNormalClosure, "") //nolint:errcheck

	waitFor(t, 2*time.Second, func() bool { return !b.IsConnected() },
		"the read loop to notice the client hung up")
}

// The panel turns on its confirm guard when the server says its listener can
// be reached from the network. This flag is the only signal for that.
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
// caller's context to conn.Write, and during a write the library registers
// context.AfterFunc(ctx, c.close) (conn.go:171, write.go:276). So a cancel
// that arrived during the write dropped the socket for every other request too.
//
// This only happens while the write is blocked, which on loopback never
// happens for a small payload. This test forces it: the client never reads,
// so a large enough frame fills the socket buffer and the write gets stuck.
func TestSend_CancellingMidWriteLeavesTheConnectionUp(t *testing.T) {
	b, _ := setupBridgeWithClient(t)

	// Big enough to overflow the socket buffers in both directions.
	big := strings.Repeat("x", 8<<20)

	ctx, cancel := context.WithCancel(context.Background())
	go b.Send(ctx, "get_document", nil, map[string]any{"blob": big}) //nolint:errcheck

	// Give the write time to get stuck on a full buffer, then cancel it.
	time.Sleep(200 * time.Millisecond)
	cancel()

	// Do not wait for Send to return. The stuck write only ends when the
	// keepalive drops this client, which never reads, seconds later. The test
	// checks whether the cancel itself closed the connection, so it watches the
	// connection for a period well inside the ping interval.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !b.IsConnected() {
			t.Fatal("cancelling one request closed the shared plugin connection")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A write context that never cancels stops one caller's cancel from closing
// the shared socket. But then a write stuck on a full socket buffer can only be
// cleared by the keepalive, which takes up to three ping rounds: about a
// minute with production defaults. b.wmu was a sync.Mutex, which cannot be
// cancelled, so every other caller in the process waited that whole time,
// whatever its deadline.
func TestSend_HonoursTheCallersDeadlineWhileAnotherWriteIsParked(t *testing.T) {
	b, _ := setupBridgeWithClient(t)

	// The client never reads, so a big enough frame fills the socket buffers
	// and the write gets stuck there, holding the write lock.
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

	// The caller must get out by giving up, not by the socket being dropped. That
	// would bring back the bug TestSend_CancellingMidWriteLeavesTheConnectionUp
	// guards against, by another route.
	if !b.IsConnected() {
		t.Error("giving up on the write lock closed the shared plugin connection")
	}
}

// setupBridgeWithClient's client never calls Read, so on that side the library
// never answers a close frame. The keepalive tests use the same trick. Close
// used to wait on the handshake for the library's 5s budget (close.go:199)
// plus up to 15s in waitGoroutines (close.go:231). That delayed process exit
// for a plugin that was already gone.
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

// HandleUpgrade used to close the old connection gracefully while holding
// b.mu. A peer that is alive at the TCP level but not answering (laptop asleep,
// Figma reloading its UI) makes the library wait its whole handshake budget
// (close.go:199). With the lock held, that froze every Send, IsConnected,
// Pending and MarshalJSON in the process. Close already limits the same
// handshake. This gives the reconnect path the same fix.
func TestHandleUpgrade_DoesNotHoldTheLockAcrossTheCloseHandshake(t *testing.T) {
	b := NewBridge("0.1.1")
	b.closeGrace = 100 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(b.HandleUpgrade))
	t.Cleanup(srv.Close)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	// The old peer never calls Read, so it never answers a close frame. The
	// keepalive and Close tests use the same trick.
	first, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { first.Close(websocket.StatusNormalClosure, "") })
	waitFor(t, 500*time.Millisecond, b.IsConnected, "the bridge to register the first connection")

	// Dial returns on the 101 response, so HandleUpgrade is still inside its
	// critical section for the new connection when this returns.
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
// (RECONNECT_DELAY_MS in plugin/src/ui/App.svelte). Failing at once during
// that time reports "plugin not connected" for a plugin that is coming back.
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

// The params map holds whatever the user is designing: text content, colours,
// names. Logging it at debug level is fine, because someone asked for it. It
// must not appear in the default output.
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

// sendAndIgnore sends a request and returns as soon as it is logged. The
// client never answers, so waiting for the reply would mean waiting out the
// tool's whole budget.
func sendAndIgnore(b *Bridge, tool string, params map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	b.Send(ctx, tool, []string{"1:1"}, params) //nolint:errcheck
}

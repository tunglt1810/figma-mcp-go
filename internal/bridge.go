package internal

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

var bridgeLogger = log.New(os.Stderr, "[bridge] ", 0)

// pendingEntry holds the response channel and inactivity timer for an in-flight request.
type pendingEntry struct {
	ch    chan BridgeResponse
	timer *time.Timer
	once  sync.Once // guards channel close/send — prevents panic on concurrent timeout + response

	// timeout is this tool's budget, restored on every progress update;
	// hardDeadline is the point past which no progress update extends it.
	timeout      time.Duration
	hardDeadline time.Time
}

// nextTimeout is how long a progress update may extend this request, capped by
// the hard deadline. Zero or less means the request has run out of time.
func (e *pendingEntry) nextTimeout() time.Duration {
	remaining := time.Until(e.hardDeadline)
	if remaining < e.timeout {
		return remaining
	}
	return e.timeout
}

// Bridge manages the single WebSocket connection from the Figma plugin
// and matches responses to pending requests via request IDs.
type Bridge struct {
	mu      sync.RWMutex
	wmu     sync.Mutex // serialises concurrent WebSocket writes (coder/websocket does not support concurrent writes)
	conn    *websocket.Conn
	pending map[string]*pendingEntry
	counter atomic.Int64
	version string

	// Ping cadence, overridable in tests so they need not wait 20 seconds.
	pingInterval time.Duration
	pingTimeout  time.Duration
}

// NewBridge creates a ready-to-use Bridge.
func NewBridge(version string) *Bridge {
	return &Bridge{
		pending:      make(map[string]*pendingEntry),
		version:      version,
		pingInterval: defaultPingInterval,
		pingTimeout:  defaultPingTimeout,
	}
}

// allowedOrigin reports whether a browser at this Origin may open the bridge.
// A new connection replaces the live one, so without this any page the user had
// open could connect to the local port, displace the real plugin and answer
// tool calls itself. Browsers set Origin and scripts cannot forge it, which is
// what makes the check worth having.
//
// Figma serves plugin UI from a sandboxed iframe, whose Origin is the literal
// "null". Allowing that leaves one gap: a hostile page can sandbox an iframe of
// its own and present "null" too. It closes the ordinary case, which is a page
// simply running a script.
func allowedOrigin(origin string) bool {
	if origin == "" || origin == "null" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "figma.com" || strings.HasSuffix(host, ".figma.com") ||
		host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// HandleUpgrade upgrades an HTTP request to a WebSocket connection.
// Only one plugin connection is maintained at a time; a new connection
// replaces the old one (same behaviour as the TypeScript version).
func (b *Bridge) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	if origin := r.Header.Get("Origin"); !allowedOrigin(origin) {
		bridgeLogger.Printf("upgrade refused: origin %q is not allowed", origin)
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The check above replaces the library's: it has to accept the "null"
		// origin of a sandboxed iframe, which the library rejects outright.
		InsecureSkipVerify: true,
	})
	if err != nil {
		bridgeLogger.Printf("upgrade error: %v", err)
		return
	}

	// Raise the read limit to 100 MB — Figma documents can be large.
	// Default is 32 KiB which causes "read limited at 32769 bytes" disconnects.
	conn.SetReadLimit(100 * 1024 * 1024)

	b.mu.Lock()
	replaced := b.conn != nil
	if replaced {
		if err := b.conn.Close(websocket.StatusNormalClosure, "replaced by new connection"); err != nil {
			bridgeLogger.Printf("close previous connection error: %v", err)
		}
	}
	b.conn = conn
	b.mu.Unlock()

	if replaced {
		bridgeLogger.Printf("plugin connected (replaced previous connection) from %s", r.RemoteAddr)
	} else {
		bridgeLogger.Printf("plugin connected from %s", r.RemoteAddr)
	}
	go b.readLoop(conn)
	go b.keepalive(conn)
}

// keepalive pings the plugin on a timer. Without it a connection that died
// without a close frame — laptop asleep, network dropped — keeps looking alive,
// and the first sign of trouble is a tool call timing out much later. A missed
// pong closes the connection, so the next call fails immediately and says the
// plugin is not connected.
//
// Ping writes a control frame, which the library serialises separately from
// data messages, so this does not need the write lock and cannot block a send
// while it waits for the pong.
func (b *Bridge) keepalive(conn *websocket.Conn) {
	ticker := time.NewTicker(b.pingInterval)
	defer ticker.Stop()

	for range ticker.C {
		b.mu.RLock()
		current := b.conn
		b.mu.RUnlock()
		if current != conn {
			return // replaced or already gone
		}

		ctx, cancel := context.WithTimeout(context.Background(), b.pingTimeout)
		err := conn.Ping(ctx)
		cancel()
		if err != nil {
			bridgeLogger.Printf("keepalive: no pong, dropping connection: %v", err)
			// CloseNow, not Close: a graceful close waits for the peer's close
			// frame, and the peer not answering is exactly what got us here.
			// Dropping the socket makes readLoop return, which clears b.conn.
			conn.CloseNow() //nolint:errcheck
			return
		}
	}
}

// readLoop reads messages from the plugin and resolves pending requests.
func (b *Bridge) readLoop(conn *websocket.Conn) {
	defer func() {
		b.mu.Lock()
		if b.conn == conn {
			b.conn = nil
		}
		b.mu.Unlock()
		bridgeLogger.Printf("plugin disconnected")
	}()

	ctx := context.Background()
	for {
		var resp BridgeResponse
		if err := readJSON(ctx, conn, &resp); err != nil {
			if !errors.Is(err, context.Canceled) {
				bridgeLogger.Printf("read error: %v", err)
			}
			return
		}

		// Handle progress updates — extend timeout, do not resolve.
		if resp.Progress > 0 && resp.RequestID != "" {
			b.mu.RLock()
			entry, ok := b.pending[resp.RequestID]
			b.mu.RUnlock()
			if ok {
				// Stop before Reset to avoid the AfterFunc firing during Reset.
				entry.timer.Stop()
				if extension := entry.nextTimeout(); extension > 0 {
					entry.timer.Reset(extension)
					bridgeLogger.Printf("progress %s: %d%% %s", resp.RequestID, resp.Progress, resp.Message)
				} else {
					// Past the hard deadline; let the timer fire immediately
					// rather than letting progress hold the request open.
					entry.timer.Reset(time.Nanosecond)
					bridgeLogger.Printf("progress %s: %d%% %s (past the %s ceiling — timing out)", resp.RequestID, resp.Progress, resp.Message, maxToolTimeout)
				}
			} else {
				bridgeLogger.Printf("progress %s: %d%% %s (no pending entry — already resolved or timed out)", resp.RequestID, resp.Progress, resp.Message)
			}
			continue
		}

		if resp.Type == "get_server_info" {
			infoMsg := map[string]string{
				"type":    "server-info",
				"version": b.version,
			}
			b.wmu.Lock()
			if err := writeJSON(ctx, conn, infoMsg); err != nil {
				bridgeLogger.Printf("failed to write server-info: %v", err)
			}
			b.wmu.Unlock()
			continue
		}

		if resp.Type == "copy_to_clipboard" {
			if resp.Text != "" {
				if err := WriteOSClipboard(resp.Text); err != nil {
					bridgeLogger.Printf("failed to write OS clipboard: %v", err)
				} else {
					bridgeLogger.Printf("successfully copied to OS clipboard (%d bytes)", len(resp.Text))
				}
			}
			continue
		}

		if resp.RequestID == "" {
			bridgeLogger.Printf("received message with empty requestID — ignored")
			continue
		}

		b.mu.Lock()
		entry, ok := b.pending[resp.RequestID]
		if ok {
			delete(b.pending, resp.RequestID)
		}
		b.mu.Unlock()

		if ok {
			if resp.Error != "" {
				bridgeLogger.Printf("← %s error: %s", resp.RequestID, resp.Error)
			} else {
				bridgeLogger.Printf("← %s ok", resp.RequestID)
			}
			entry.timer.Stop()
			// Use once to prevent sending on a channel already closed by timeout.
			entry.once.Do(func() { entry.ch <- resp })
		} else {
			bridgeLogger.Printf("← %s received but no pending entry (timed out?)", resp.RequestID)
		}
	}
}

// Send sends a request to the plugin and waits for the response.
func (b *Bridge) Send(ctx context.Context, requestType string, nodeIDs []string, params map[string]any) (BridgeResponse, error) {
	b.mu.RLock()
	conn := b.conn
	b.mu.RUnlock()

	if conn == nil {
		return BridgeResponse{}, errors.New("plugin not connected")
	}

	requestID := b.nextID()
	req := BridgeRequest{
		Type:      requestType,
		RequestID: requestID,
		NodeIDs:   nodeIDs,
		Params:    params,
	}

	ch := make(chan BridgeResponse, 1)
	timeout := timeoutFor(requestType)
	entry := &pendingEntry{
		ch:           ch,
		timeout:      timeout,
		hardDeadline: time.Now().Add(maxToolTimeout),
	}

	// Register before sending to avoid a race where the response
	// arrives before we store the channel.
	entry.timer = time.AfterFunc(timeout, func() {
		bridgeLogger.Printf("→ %s %s timed out after %s", requestID, requestType, timeout)
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		// Use once to prevent closing a channel already consumed by the read goroutine.
		entry.once.Do(func() { close(ch) })
	})

	b.mu.Lock()
	b.pending[requestID] = entry
	b.mu.Unlock()

	bridgeLogger.Printf("→ %s %s nodeIDs=%v params=%v", requestID, requestType, nodeIDs, params)
	start := time.Now()

	b.wmu.Lock()
	writeErr := writeJSON(ctx, conn, req)
	b.wmu.Unlock()
	if writeErr != nil {
		entry.timer.Stop()
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		bridgeLogger.Printf("→ %s %s write error: %v", requestID, requestType, writeErr)
		return BridgeResponse{}, fmt.Errorf("send: %w", writeErr)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return BridgeResponse{}, errors.New("request timed out")
		}
		bridgeLogger.Printf("→ %s %s completed in %dms", requestID, requestType, time.Since(start).Milliseconds())
		return resp, nil
	case <-ctx.Done():
		entry.timer.Stop()
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		bridgeLogger.Printf("→ %s %s context cancelled: %v", requestID, requestType, ctx.Err())
		return BridgeResponse{}, ctx.Err()
	}
}

// Close shuts down the bridge, rejecting all pending requests.
func (b *Bridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for id, entry := range b.pending {
		entry.timer.Stop()
		entry.once.Do(func() { close(entry.ch) })
		delete(b.pending, id)
	}

	if b.conn != nil {
		if err := b.conn.Close(websocket.StatusNormalClosure, "bridge closed"); err != nil {
			bridgeLogger.Printf("close connection error: %v", err)
		}
		b.conn = nil
	}
}

// nextID generates a request ID in the format req-HHMMSS-N.
func (b *Bridge) nextID() string {
	n := b.counter.Add(1)
	now := time.Now()
	return fmt.Sprintf("req-%02d%02d%02d-%d",
		now.Hour(), now.Minute(), now.Second(), n)
}

// IsConnected reports whether the plugin is currently connected.
func (b *Bridge) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.conn != nil
}

// readJSON reads one WebSocket message and decodes it into v. It stands in for
// wsjson.Read, which is hardwired to encoding/json v1. Like wsjson, a payload
// that fails to decode closes the connection: a peer that cannot frame valid
// JSON will not do better on the next message.
func readJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		conn.Close(websocket.StatusInvalidFramePayloadData, "failed to unmarshal JSON") //nolint:errcheck
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return nil
}

// writeJSON encodes v and sends it as a single text message, standing in for
// wsjson.Write for the same reason as readJSON.
func writeJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

// MarshalJSON is used when logging — avoid printing full conn object.
func (b *Bridge) MarshalJSON() ([]byte, error) {
	b.mu.RLock()
	connected := b.conn != nil
	pending := len(b.pending)
	b.mu.RUnlock()
	return json.Marshal(map[string]any{
		"connected": connected,
		"pending":   pending,
	}, json.Deterministic(true))
}

package bridge

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// log is resolved per call rather than held in a package variable: a package
// variable is initialised before main installs the default handler, so it would
// capture the stock one and ignore the configured level.
func log() *slog.Logger { return slog.Default().With("component", "bridge") }

// pendingEntry holds the response channel and inactivity timer for an in-flight request.
type pendingEntry struct {
	ch    chan Response
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
	mu sync.RWMutex

	// wslot serialises writes — coder/websocket does not support concurrent
	// ones. It is a channel rather than a sync.Mutex because a mutex consults
	// nothing: writes go out under a context that never cancels (see Send), so
	// a write parked on a full socket buffer holds the slot until the keepalive
	// drops the peer, and a mutex would make every other caller wait out that
	// whole window whatever deadline it arrived with.
	wslot chan struct{}

	conn    *websocket.Conn
	pending map[string]*pendingEntry
	counter atomic.Int64
	version string
	// Set once at startup, before any connection is served, and only read after.
	exposed bool

	// pluginVersion is what the connected plugin last announced, "" when no
	// plugin has connected or when it is too old to announce anything.
	pluginVersion string

	// pluginHandlers is what that plugin said it can do. Empty means "it did
	// not say", which is not the same as "it can do nothing" — an older plugin
	// announces no handlers and must keep working.
	pluginHandlers map[string]bool

	// Ping cadence, overridable in tests so they need not wait 20 seconds.
	pingInterval time.Duration
	pingTimeout  time.Duration

	// toolTimeout is how long a request waits for the plugin, indirected for
	// the same reason: at 30 seconds the real budget is not something a test
	// can sit through.
	toolTimeout func(string) time.Duration

	// closeGrace bounds the WebSocket close handshake on shutdown.
	closeGrace time.Duration

	// connected is closed and replaced each time a plugin connects, so a Send
	// arriving during a leader handover can wait for the next one instead of
	// failing on a gap that closes itself.
	connected chan struct{}

	// connectGrace is how long Send waits for a plugin that may be reconnecting.
	connectGrace time.Duration

	// lastRead is when the plugin last sent us anything, in unix nanoseconds.
	// The keepalive reads it as evidence of life; see there for why a failed
	// ping alone is not evidence of death.
	lastRead atomic.Int64
}

// NewBridge creates a ready-to-use Bridge.
func NewBridge(version string) *Bridge {
	return &Bridge{
		wslot:        make(chan struct{}, 1),
		pending:      make(map[string]*pendingEntry),
		version:      version,
		pingInterval: defaultPingInterval,
		pingTimeout:  defaultPingTimeout,
		toolTimeout:  TimeoutFor,
		closeGrace:   defaultCloseGrace,
		connected:    make(chan struct{}),
		connectGrace: defaultConnectGrace,
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
		log().Warn("upgrade refused: origin not allowed", "origin", origin)
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The check above replaces the library's: it has to accept the "null"
		// origin of a sandboxed iframe, which the library rejects outright.
		InsecureSkipVerify: true,
	})
	if err != nil {
		log().Warn("upgrade failed", "err", err)
		return
	}

	// Raise the read limit to 100 MB — Figma documents can be large.
	// Default is 32 KiB which causes "read limited at 32769 bytes" disconnects.
	conn.SetReadLimit(100 * 1024 * 1024)

	// A fresh connection starts with a clean slate, rather than inheriting the
	// silence of the one it replaces.
	b.markRead()

	b.mu.Lock()
	previous := b.conn
	b.conn = conn
	// Wake anything waiting for a plugin, then arm the signal for the next wait.
	close(b.connected)
	b.connected = make(chan struct{})
	grace := b.closeGrace
	b.mu.Unlock()

	replaced := previous != nil
	if replaced {
		// Off the lock, and off this goroutine: the displaced peer may be alive
		// at TCP level and not answering — laptop asleep, Figma reloading its
		// UI — in which case the handshake runs to the library's budget. Under
		// b.mu that stalls every reader; on this goroutine it delays the new
		// connection's readLoop, which is the reconnect the user is waiting on.
		go closeBounded(previous, "replaced by new connection", grace)
	}
	log().Info("plugin connected", "remote", r.RemoteAddr, "replaced", replaced)
	go b.readLoop(conn)
	go b.keepalive(conn)
}

// keepalive pings the plugin on a timer. Without it a connection that died
// without a close frame — laptop asleep, network dropped — keeps looking alive,
// and the first sign of trouble is a tool call timing out much later. A missed
// pong closes the connection, so the next call fails immediately and says the
// plugin is not connected.
//
// The ping deliberately does not take the bridge's own write lock, and not
// because control frames bypass the data path — they do not. Ping goes through
// writeControl, which calls the same writeFrame and takes the same
// c.writeFrameMu as a data message (write.go:231, :244), so it does queue
// behind a send parked on a full socket buffer. The reason to stay off b.wmu is
// that the keepalive is the only thing that clears such a send: it is what
// notices the peer has stopped draining the socket and drops it. Waiting on the
// lock the stuck write holds would park the keepalive behind the very problem
// it exists to resolve. The library's frame lock is context-aware
// (conn.go:276) and writeControl caps the wait at 5s, so the ping still gives
// up on its own terms.
func (b *Bridge) keepalive(conn *websocket.Conn) {
	ticker := time.NewTicker(b.pingInterval)
	defer ticker.Stop()

	failures := 0
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
		if err == nil {
			failures = 0
			continue
		}

		// A ping that could not be completed is not proof the peer is gone. It
		// fails on the library's frame lock too, which a send parked on a full
		// socket buffer holds — so a plugin that is merely draining a large
		// message slowly fails the ping while being perfectly healthy. A plugin
		// that has sent us something since the last tick is talking, whatever
		// the ping says, so forgive the failure. Not indefinitely: this is also
		// the only thing that clears such a parked write.
		failures++
		if failures < keepaliveForgiveness && b.readWithin(b.pingInterval) {
			log().Warn("keepalive: ping failed but the plugin is still sending — holding on",
				"err", err, "failures", failures)
			continue
		}

		log().Warn("keepalive: no pong, dropping the connection", "err", err, "failures", failures)
		// CloseNow, not Close: a graceful close waits for the peer's close
		// frame, and the peer not answering is exactly what got us here.
		// Dropping the socket makes readLoop return, which clears b.conn.
		conn.CloseNow() //nolint:errcheck
		return
	}
}

// markRead records that the plugin has just sent us something.
func (b *Bridge) markRead() { b.lastRead.Store(time.Now().UnixNano()) }

// readWithin reports whether the plugin has sent anything in the last d.
func (b *Bridge) readWithin(d time.Duration) bool {
	return time.Since(time.Unix(0, b.lastRead.Load())) <= d
}

// lockWrite takes the write slot, giving up if ctx is done first. Giving up
// leaves the connection alone: it is the wait that is abandoned, not the write.
func (b *Bridge) lockWrite(ctx context.Context) error {
	select {
	case b.wslot <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *Bridge) unlockWrite() { <-b.wslot }

// replyServerInfo answers the plugin's get_server_info. Only the wait for the
// write slot is bounded; the write itself goes out under a context that never
// cancels, for the reason Send documents. A reply that cannot get on the wire
// within the grace is dropped rather than left to pile up behind whatever is
// holding the slot — the plugin asks again on its next connect.
func (b *Bridge) replyServerInfo(conn *websocket.Conn) {
	b.writeControlFrame(conn, "server-info", map[string]any{
		"type":    "server-info",
		"version": b.version,
		// Whether the listener is reachable from another machine. There is no
		// authentication on the socket — pairing was considered and rejected,
		// because a prompt in front of every connect costs every local user
		// something to protect the few who move the listener off loopback. So
		// the exposure is reported instead, and the panel turns its confirm
		// guard on by default when it hears this, gating the destructive tools
		// rather than the connection.
		"exposed": b.exposed,
	})
}

// SetExposed records that the listener is bound somewhere other than loopback.
func (b *Bridge) SetExposed(exposed bool) {
	b.exposed = exposed
}

// writeControlFrame sends a frame that is not a response to anything: nothing
// waits on it and nothing retries it. Only the wait for the write slot is
// bounded; the write itself goes out under a context that never cancels, for
// the reason Send documents. A frame that cannot get on the wire within the
// grace is dropped rather than left to pile up behind whatever holds the slot.
func (b *Bridge) writeControlFrame(conn *websocket.Conn, what string, frame any) {
	ctx, cancel := context.WithTimeout(context.Background(), serverInfoGrace)
	defer cancel()
	if err := b.lockWrite(ctx); err != nil {
		log().Warn("gave up queueing a control frame", "frame", what, "err", err)
		return
	}
	defer b.unlockWrite()

	if err := writeJSON(context.Background(), conn, frame); err != nil {
		log().Warn("failed to write a control frame", "frame", what, "err", err)
	}
}

// cancelRequest tells the plugin to stop work it is still doing for a request
// nobody is waiting for any more.
//
// Without this the plugin runs a long scan to completion after the caller has
// walked away, holding the single WebSocket against the next request. The frame
// is advisory: a handler that never checks simply finishes, and its response is
// dropped as "a request that is already gone".
func (b *Bridge) cancelRequest(requestID string) {
	b.mu.RLock()
	conn := b.conn
	b.mu.RUnlock()
	if conn == nil {
		return
	}
	go b.writeControlFrame(conn, "cancel_request", map[string]string{
		"type":      "cancel_request",
		"requestId": requestID,
	})
}

// readLoop reads messages from the plugin and resolves pending requests.
func (b *Bridge) readLoop(conn *websocket.Conn) {
	defer func() {
		b.mu.Lock()
		if b.conn == conn {
			b.conn = nil
		}
		b.mu.Unlock()
		log().Info("plugin disconnected")
	}()

	ctx := context.Background()
	for {
		var resp Response
		if err := readJSON(ctx, conn, &resp); err != nil {
			if !errors.Is(err, context.Canceled) {
				log().Warn("read error", "err", err)
			}
			return
		}
		b.markRead()

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
					log().Debug("progress", "id", resp.RequestID, "percent", resp.Progress, "message", resp.Message)
				} else {
					// Past the hard deadline; let the timer fire immediately
					// rather than letting progress hold the request open.
					entry.timer.Reset(time.Nanosecond)
					log().Warn("progress past the ceiling — timing out", "id", resp.RequestID, "percent", resp.Progress, "message", resp.Message, "ceiling", MaxToolTimeout)
				}
			} else {
				log().Debug("progress for a request that is already gone", "id", resp.RequestID, "percent", resp.Progress, "message", resp.Message)
			}
			continue
		}

		if resp.Type == "get_server_info" {
			// On its own goroutine. This one has to get back into conn.Read:
			// the library processes what the peer sends only from there
			// (handleControl is reached from reader, read.go:289, :368), so a
			// reply parked behind another write would stop pongs being seen and
			// the keepalive would drop a plugin that is perfectly healthy.
			go b.replyServerInfo(conn)
			continue
		}

		if resp.Type == "plugin-info" {
			// The plugin announces itself on connect. Log the skew here as well
			// as showing it in the panel: a user filing a bug sends the server
			// log, and may never have opened the panel to see the banner.
			b.setPluginInfo(resp.Version, resp.Handlers)
			if msg := VersionSkewMessage(resp.Version, b.version); msg != "" {
				log().Warn("version mismatch — " + msg)
			} else {
				log().Info("plugin connected", "pluginVersion", resp.Version, "serverVersion", b.version)
			}
			continue
		}

		if resp.Type == "copy_to_clipboard" {
			if resp.Text != "" {
				if err := WriteOSClipboard(resp.Text); err != nil {
					log().Warn("failed to write the OS clipboard", "err", err)
				} else {
					log().Info("copied to the OS clipboard", "bytes", len(resp.Text))
				}
			}
			continue
		}

		if resp.RequestID == "" {
			log().Warn("message with an empty requestId — ignored")
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
				log().Info("response", "id", resp.RequestID, "err", resp.Error)
			} else {
				log().Info("response", "id", resp.RequestID, "ok", true)
			}
			entry.timer.Stop()
			// Use once to prevent sending on a channel already closed by timeout.
			entry.once.Do(func() { entry.ch <- resp })
		} else {
			log().Warn("response for a request that is already gone", "id", resp.RequestID)
		}
	}
}

// Send sends a request to the plugin and waits for the response.
func (b *Bridge) Send(ctx context.Context, requestType string, nodeIDs []string, params map[string]any) (Response, error) {
	b.mu.RLock()
	conn := b.conn
	arrived := b.connected
	grace := b.connectGrace
	b.mu.RUnlock()

	if conn == nil {
		// A leader handover leaves a gap: the new leader holds the port but the
		// plugin has not noticed yet and reconnects about 1.5s later. Wait it
		// out rather than reporting a plugin that is on its way back as absent.
		select {
		case <-arrived:
			b.mu.RLock()
			conn = b.conn
			b.mu.RUnlock()
		case <-time.After(grace):
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
	}
	if conn == nil {
		return Response{}, errors.New("plugin not connected")
	}

	// Checked after the connection wait, so a plugin that reconnects mid-call
	// has had its chance to announce before its capabilities are consulted.
	if msg := b.checkPluginSupports(requestType); msg != "" {
		log().Warn("tool refused by the plugin's declared capabilities", "tool", requestType, "err", msg)
		return Response{}, errors.New(msg)
	}

	requestID := b.nextID()
	req := Request{
		Type:      requestType,
		RequestID: requestID,
		NodeIDs:   nodeIDs,
		Params:    params,
	}

	log().Info("request", "id", requestID, "tool", requestType, "nodes", len(nodeIDs), "paramBytes", paramSize(params))
	log().Debug("request params", "id", requestID, "params", params)
	start := time.Now()

	// Queue for the wire before registering. A request that spends its whole
	// budget behind someone else's write used to time out as if the plugin had
	// gone quiet, leaving a pending entry for a message that never reached the
	// socket; now the timer starts when this request owns the wire. Registration
	// still happens before the write, which is the ordering that matters — the
	// response can arrive before writeJSON returns.
	if err := b.lockWrite(ctx); err != nil {
		log().Info("request gave up queueing for the connection", "id", requestID, "tool", requestType, "err", err)
		return Response{}, err
	}

	ch := make(chan Response, 1)
	timeout := b.toolTimeout(requestType)
	entry := &pendingEntry{
		ch:           ch,
		timeout:      timeout,
		hardDeadline: time.Now().Add(MaxToolTimeout),
	}
	entry.timer = time.AfterFunc(timeout, func() {
		log().Warn("request timed out", "id", requestID, "tool", requestType, "after", timeout)
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		// Use once to prevent closing a channel already consumed by the read goroutine.
		entry.once.Do(func() { close(ch) })
		b.cancelRequest(requestID)
	})

	b.mu.Lock()
	b.pending[requestID] = entry
	b.mu.Unlock()

	// A context that never cancels, deliberately. For the duration of a write
	// the library registers context.AfterFunc(ctx, c.close) (conn.go:171,
	// write.go:276), so a context that can be cancelled — the caller's, or one
	// carrying a write deadline — takes the shared connection down with it when
	// it fires. A blocked write is instead resolved by the keepalive, which
	// drops a peer that has stopped answering and so unblocks the write with an
	// error. Callers waiting behind it are covered by lockWrite above, which
	// does honour their contexts; the caller's context also governs the wait
	// below.
	writeErr := writeJSON(context.Background(), conn, req)
	b.unlockWrite()
	if writeErr != nil {
		entry.timer.Stop()
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		log().Warn("write error", "id", requestID, "tool", requestType, "err", writeErr)
		return Response{}, fmt.Errorf("send: %w", writeErr)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return Response{}, errors.New("request timed out")
		}
		log().Info("request completed", "id", requestID, "tool", requestType, "ms", time.Since(start).Milliseconds())
		return resp, nil
	case <-ctx.Done():
		entry.timer.Stop()
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		log().Info("request cancelled by the caller", "id", requestID, "tool", requestType, "err", ctx.Err())
		b.cancelRequest(requestID)
		return Response{}, ctx.Err()
	}
}

// closeBounded closes conn gracefully but returns after grace at the latest.
// The close frame is still sent in the normal case; a peer that has gone away
// no longer holds the caller for the library's handshake budget — 5s for the
// peer's reply (close.go:199) plus 15s for its goroutines (close.go:231). The
// goroutine finishes on its own and the library drops the socket regardless.
func closeBounded(conn *websocket.Conn, reason string, grace time.Duration) {
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		if err := conn.Close(websocket.StatusNormalClosure, reason); err != nil {
			log().Warn("closing the connection", "err", err, "reason", reason)
		}
	}()

	select {
	case <-closed:
	case <-time.After(grace):
		log().Warn("close handshake did not finish — dropping the socket", "grace", grace, "reason", reason)
	}
}

// Close shuts down the bridge, rejecting all pending requests.
func (b *Bridge) Close() {
	b.mu.Lock()
	for id, entry := range b.pending {
		entry.timer.Stop()
		entry.once.Do(func() { close(entry.ch) })
		delete(b.pending, id)
	}
	conn := b.conn
	b.conn = nil
	grace := b.closeGrace
	b.mu.Unlock()

	if conn == nil {
		return
	}
	closeBounded(conn, "bridge closed", grace)
}

// paramSize is how big a params map is on the wire, for a log line that says
// something about the payload without quoting the user's design back at them.
func paramSize(params map[string]any) int {
	if params == nil {
		return 0
	}
	b, err := json.Marshal(params)
	if err != nil {
		return -1
	}
	return len(b)
}

// nextID generates a request ID in the format req-HHMMSS-N.
func (b *Bridge) nextID() string {
	n := b.counter.Add(1)
	now := time.Now()
	return fmt.Sprintf("req-%02d%02d%02d-%d",
		now.Hour(), now.Minute(), now.Second(), n)
}

// Pending is how many requests are in flight, for the health endpoint.
func (b *Bridge) Pending() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.pending)
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

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

// log looks up the logger on each call. A package variable would be set before
// main installs the default handler, so it would keep the stock handler and
// ignore the configured level.
func log() *slog.Logger { return slog.Default().With("component", "bridge") }

// pendingEntry holds the response channel and inactivity timer for an in-flight request.
type pendingEntry struct {
	ch    chan Response
	timer *time.Timer
	once  sync.Once // guards close and send, so a timeout racing a response cannot panic

	// timeout is this tool's time budget. Each progress update resets it.
	// hardDeadline is the limit: no progress update extends past it.
	timeout      time.Duration
	hardDeadline time.Time
}

// nextTimeout is how far a progress update may extend this request. It never
// goes past the hard deadline. Zero or less means the request is out of time.
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

	// wslot lets one write through at a time, because coder/websocket does not
	// support concurrent writes. It is a channel, not a sync.Mutex, so a waiter
	// can give up when its context ends. Writes use a context that never
	// cancels (see Send). A write stuck on a full socket buffer holds the slot
	// until the keepalive drops the peer. With a mutex, every other caller
	// would wait that whole time, whatever its deadline.
	wslot chan struct{}

	conn    *websocket.Conn
	pending map[string]*pendingEntry
	counter atomic.Int64
	version string
	// Set once at startup, before any connection is served. Only read after that.
	exposed bool

	// pluginVersion is the version the connected plugin last announced. It is
	// "" when no plugin has connected, or when the plugin is too old to announce.
	pluginVersion string

	// pluginHandlers lists what that plugin says it can do. Empty means "it did
	// not say", not "it can do nothing". An older plugin announces no handlers
	// and must keep working.
	pluginHandlers map[string]bool

	// Ping timing. Tests override it so they do not wait 20 seconds.
	pingInterval time.Duration
	pingTimeout  time.Duration

	// toolTimeout is how long a request waits for the plugin. It is a function
	// for the same reason: tests cannot wait out the real 30 seconds.
	toolTimeout func(string) time.Duration

	// closeGrace bounds the WebSocket close handshake on shutdown.
	closeGrace time.Duration

	// connected is closed and replaced each time a plugin connects. A Send that
	// arrives during a leader handover waits on it for the plugin to come back,
	// instead of failing during a short gap.
	connected chan struct{}

	// connectGrace is how long Send waits for a plugin that may be reconnecting.
	connectGrace time.Duration

	// lastRead is when the plugin last sent us anything, in unix nanoseconds.
	// The keepalive treats it as proof the plugin is alive. See keepalive for
	// why one failed ping does not prove the plugin is gone.
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

// allowedOrigin reports whether a browser page at this Origin may open the
// bridge. A new connection replaces the live one. Without this check, any open
// page could connect to the local port, push out the real plugin, and answer
// tool calls itself. Browsers set Origin and scripts cannot fake it, so the
// check is worth having.
//
// Figma serves the plugin UI from a sandboxed iframe, whose Origin is the
// literal "null". Allowing "null" leaves one gap: a hostile page can create its
// own sandboxed iframe and send "null" too. The check still blocks the common
// case, a page that just runs a script.
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
// Only one plugin connection is kept at a time. A new connection replaces the
// old one, as in the TypeScript version.
func (b *Bridge) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	if origin := r.Header.Get("Origin"); !allowedOrigin(origin) {
		log().Warn("upgrade refused: origin not allowed", "origin", origin)
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The check above replaces the library's own check. It must accept the
		// "null" origin of a sandboxed iframe, and the library rejects it.
		InsecureSkipVerify: true,
	})
	if err != nil {
		log().Warn("upgrade failed", "err", err)
		return
	}

	// Raise the read limit to 100 MB, because Figma documents can be large.
	// The 32 KiB default causes "read limited at 32769 bytes" disconnects.
	conn.SetReadLimit(100 * 1024 * 1024)

	// A new connection starts fresh. It does not inherit the silence of the
	// connection it replaces.
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
		// Close outside the lock and on another goroutine. The old peer may be
		// alive at the TCP level but not answering (laptop asleep, Figma
		// reloading its UI). Then the handshake waits for the library's full
		// time budget. Under b.mu that would block every reader. On this
		// goroutine it would delay the new connection's readLoop, which is the
		// reconnect the user is waiting for.
		go closeBounded(previous, "replaced by new connection", grace)
	}
	log().Info("plugin connected", "remote", r.RemoteAddr, "replaced", replaced)
	go b.readLoop(conn)
	go b.keepalive(conn)
}

// keepalive pings the plugin on a timer. A connection can die without a close
// frame (laptop asleep, network dropped). Without pings it keeps looking alive,
// and the first sign of trouble is a tool call that times out much later. A
// missed pong closes the connection, so the next call fails at once and says
// the plugin is not connected.
//
// The ping does not take the bridge's own write slot, on purpose. Control
// frames do not skip the data path: Ping goes through writeControl, which calls
// the same writeFrame and takes the same c.writeFrameMu as a data message
// (write.go:231, :244). So a ping does queue behind a send stuck on a full
// socket buffer. The keepalive stays off the write slot because it is the only
// thing that clears such a send: it notices the peer has stopped reading and
// drops it. If it waited on the slot the stuck write holds, it would be stuck
// behind the very problem it exists to fix. The library's frame lock honours
// the context (conn.go:276) and writeControl caps the wait at 5s, so the ping
// still gives up by itself.
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

		// A failed ping does not prove the peer is gone. The ping also fails
		// while waiting on the library's frame lock, which a send stuck on a
		// full socket buffer holds. So a healthy plugin that is slowly reading a
		// large message can fail the ping. If the plugin has sent us something
		// since the last tick, it is alive, whatever the ping says, so forgive
		// the failure. Only a few times, though: this is also the only thing
		// that clears such a stuck write.
		failures++
		if failures < keepaliveForgiveness && b.readWithin(b.pingInterval) {
			log().Warn("keepalive: ping failed but the plugin is still sending — holding on",
				"err", err, "failures", failures)
			continue
		}

		log().Warn("keepalive: no pong, dropping the connection", "err", err, "failures", failures)
		// CloseNow, not Close. A graceful close waits for the peer's close
		// frame, and the peer is not answering. Dropping the socket makes
		// readLoop return, which clears b.conn.
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

// lockWrite takes the write slot, or gives up if ctx ends first. Giving up
// leaves the connection alone: only the wait is dropped, not a write.
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
// write slot has a time limit. The write itself uses a context that never
// cancels, for the reason given in Send. If the reply cannot be sent within the
// grace period, it is dropped instead of piling up behind the slot holder. The
// plugin asks again on its next connect.
func (b *Bridge) replyServerInfo(conn *websocket.Conn) {
	b.writeControlFrame(conn, "server-info", map[string]any{
		"type":    "server-info",
		"version": b.version,
		// Whether another machine can reach the listener. The socket has no
		// authentication. Pairing was considered and rejected: a prompt on
		// every connect would bother every local user to protect the few who
		// move the listener off loopback. So the server reports the exposure
		// instead. When the panel sees it, it turns on its confirm guard by
		// default, which guards the destructive tools, not the connection.
		"exposed": b.exposed,
	})
}

// SetExposed records that the listener is bound somewhere other than loopback.
func (b *Bridge) SetExposed(exposed bool) {
	b.exposed = exposed
}

// writeControlFrame sends a frame that answers no request. Nothing waits on it
// and nothing retries it. Only the wait for the write slot has a time limit.
// The write itself uses a context that never cancels, for the reason given in
// Send. If the frame cannot be sent within the grace period, it is dropped
// instead of piling up behind the slot holder.
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

// cancelRequest tells the plugin to stop work for a request that nobody is
// waiting for anymore.
//
// Without it, the plugin runs a long scan to the end after the caller has left,
// and holds the single WebSocket while the next request waits. The frame is
// only a hint. A handler that never checks it just finishes, and its response
// is dropped as "a request that is already gone".
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

		// A progress update extends the timeout. It does not complete the request.
		if resp.Progress > 0 && resp.RequestID != "" {
			b.mu.RLock()
			entry, ok := b.pending[resp.RequestID]
			b.mu.RUnlock()
			if ok {
				// Stop before Reset so the AfterFunc cannot fire during Reset.
				entry.timer.Stop()
				if extension := entry.nextTimeout(); extension > 0 {
					entry.timer.Reset(extension)
					log().Debug("progress", "id", resp.RequestID, "percent", resp.Progress, "message", resp.Message)
				} else {
					// Past the hard deadline. Fire the timer now instead of
					// letting progress keep the request open.
					entry.timer.Reset(time.Nanosecond)
					log().Warn("progress past the ceiling — timing out", "id", resp.RequestID, "percent", resp.Progress, "message", resp.Message, "ceiling", MaxToolTimeout)
				}
			} else {
				log().Debug("progress for a request that is already gone", "id", resp.RequestID, "percent", resp.Progress, "message", resp.Message)
			}
			continue
		}

		if resp.Type == "get_server_info" {
			// Reply on another goroutine, because this one must get back to
			// conn.Read. The library only handles what the peer sends from there
			// (handleControl is called from reader, read.go:289, :368). If the
			// reply waited here behind another write, pongs would go unseen and
			// the keepalive would drop a healthy plugin.
			go b.replyServerInfo(conn)
			continue
		}

		if resp.Type == "plugin-info" {
			// The plugin announces itself on connect. Log a version mismatch here
			// as well as in the panel. A user filing a bug sends the server log,
			// and may never have opened the panel to see the banner.
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
			// once stops a send on a channel the timeout already closed.
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
		// A leader handover leaves a gap. The new leader holds the port, but the
		// plugin has not noticed yet and reconnects about 1.5s later. Wait for
		// it instead of reporting a returning plugin as missing.
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

	// Check after the connection wait, so a plugin that reconnects during the
	// call can announce itself before we look at its capabilities.
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

	// Wait for the write slot before registering. A request used to spend its
	// whole budget behind another write and then time out as if the plugin had
	// gone quiet, leaving a pending entry for a message never sent. Now the
	// timer starts once this request owns the socket. Registration still happens
	// before the write, and that order matters: the response can arrive before
	// writeJSON returns.
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
		// once stops a close on a channel the read goroutine already used.
		entry.once.Do(func() { close(ch) })
		b.cancelRequest(requestID)
	})

	b.mu.Lock()
	b.pending[requestID] = entry
	b.mu.Unlock()

	// Use a context that never cancels, on purpose. During a write the library
	// registers context.AfterFunc(ctx, c.close) (conn.go:171, write.go:276).
	// So when a cancellable context fires (the caller's, or one with a write
	// deadline), it closes the shared connection. Instead, the keepalive
	// handles a blocked write: it drops a peer that stopped answering, which
	// ends the write with an error. Callers waiting behind it are handled by
	// lockWrite above, which does honour their contexts. The caller's context
	// also controls the wait below.
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
		// checkPluginSupports lets through a plugin too old to announce its
		// handlers, so a tool it lacks shows up here instead.
		resp.Error = b.explainUnknownRequest(requestType, resp.Error)
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

// closeBounded closes conn gracefully, but returns after grace at the latest.
// The close frame is still sent in the normal case. A peer that has gone away
// no longer holds the caller for the library's full handshake budget: 5s for
// the peer's reply (close.go:199) plus 15s for its goroutines (close.go:231).
// The goroutine finishes by itself, and the library drops the socket anyway.
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

// paramSize is the size of a params map on the wire. The log shows it to
// describe the payload without printing the user's design.
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

// readJSON reads one WebSocket message and decodes it into v. It replaces
// wsjson.Read, which only works with encoding/json v1. Like wsjson, it closes
// the connection when a payload fails to decode: a peer that sends invalid JSON
// will not do better on the next message.
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

// writeJSON encodes v and sends it as one text message. It replaces
// wsjson.Write for the same reason as readJSON.
func writeJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

// MarshalJSON is used for logging, so the full conn object is not printed.

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

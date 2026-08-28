package cluster

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/tunglt1810/figma-mcp-go/internal/bridge"
)

func leaderLog() *slog.Logger { return slog.Default().With("component", "leader") }

// Guard checks and normalizes an incoming call. The leader holds one so /rpc
// applies the same rules as a local tool call without this package having to
// know what the rules are.
type Guard func(tool string, nodeIDs []string, params map[string]any) ([]string, map[string]any, error)

// Leader owns the WebSocket bridge to the Figma plugin and exposes
// HTTP endpoints for health checks and follower RPC proxying.
//
// Endpoints:
//
//	/ws   — WebSocket upgrade for the Figma plugin
//	/ping — Health check (GET)
//	/rpc  — JSON RPC for follower tool calls (POST)
type Leader struct {
	ip      string
	port    int
	b       *bridge.Bridge
	server  *http.Server
	version string
	guard   Guard

	// readHeaderTimeout bounds how long a client may take to send its request
	// headers. Overridable so tests need not wait seconds.
	readHeaderTimeout time.Duration

	started time.Time
}

// NewLeader creates a Leader. Call Start() to bind the ip:port.
func NewLeader(ip string, port int, version string, guard Guard) *Leader {
	return &Leader{
		ip:                ip,
		port:              port,
		b:                 bridge.NewBridge(version),
		version:           version,
		guard:             guard,
		readHeaderTimeout: 5 * time.Second,
		started:           time.Now(),
	}
}

// GetBridge returns the underlying Bridge so Node can use it directly.
func (l *Leader) GetBridge() *bridge.Bridge {
	return l.b
}

// Start binds the port and begins serving. Returns an error immediately
// if the port is already in use (EADDRINUSE → caller detects another leader).
func (l *Leader) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", l.ip, l.port))
	if err != nil {
		return err // includes EADDRINUSE
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", l.handlePing)
	mux.HandleFunc("/rpc", l.handleRPC)
	mux.HandleFunc("/ws", l.handleWS)

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: l.readHeaderTimeout,
		IdleTimeout:       60 * time.Second,
		// No ReadTimeout or WriteTimeout, because both are wrong for what this
		// server carries. A WriteTimeout would cap /rpc, where a
		// batch_execute_pipeline response legitimately takes up to
		// MaxToolTimeout; a ReadTimeout would cap reading a 32 MB body. The
		// WebSocket is not the reason — net/http clears the deadline itself
		// when a handler hijacks (server.go, hijackLocked). ReadHeaderTimeout
		// is safe on every path: net/http restores the read deadline to the
		// zero time after the headers when ReadTimeout is zero.
	}
	l.server = srv

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			leaderLog().Error("serve error", "err", err)
		}
	}()

	leaderLog().Info("listening", "ip", l.ip, "port", l.port)
	return nil
}

// Stop shuts down the HTTP server and closes the bridge.
func (l *Leader) Stop() {
	if l.server != nil {
		if err := l.server.Shutdown(context.Background()); err != nil {
			leaderLog().Warn("server shutdown", "err", err)
		}
		l.server = nil
	}
	l.b.Close()
}

// handlePing responds to health checks from followers.
func (l *Leader) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// role is the literal LEADER: only a leader serves this endpoint.
	err := json.MarshalWrite(w, map[string]any{
		"status":        "ok",
		"version":       l.version,
		"role":          "LEADER",
		"connected":     l.b.IsConnected(),
		"pending":       l.b.Pending(),
		"uptimeSeconds": int(time.Since(l.started).Seconds()),
	}, json.Deterministic(true))
	if err != nil {
		leaderLog().Warn("encode ping response", "err", err)
	}
}

// handleWS upgrades the connection to WebSocket for the Figma plugin.
func (l *Leader) handleWS(w http.ResponseWriter, r *http.Request) {
	l.b.HandleUpgrade(w, r)
}

// handleRPC handles JSON RPC calls from follower processes.
func (l *Leader) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// A tool call carries params, not a file. 32 MB is generous and stops an
	// unbounded read.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<20))
	if err != nil {
		l.sendJSON(w, http.StatusBadRequest, RPCResponse{Error: "failed to read body"})
		return
	}

	var req RPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		l.sendJSON(w, http.StatusBadRequest, RPCResponse{Error: "invalid JSON"})
		return
	}

	leaderLog().Info("rpc", "tool", req.Tool, "nodes", len(req.NodeIDs), "remote", r.RemoteAddr)

	nodeIDs, params, checkErr := l.guard(req.Tool, req.NodeIDs, req.Params)
	if checkErr != nil {
		leaderLog().Warn("rpc rejected", "tool", req.Tool, "err", checkErr)
		l.sendJSON(w, http.StatusBadRequest, RPCResponse{Error: checkErr.Error()})
		return
	}

	resp, err := l.b.Send(r.Context(), req.Tool, nodeIDs, params)
	if err != nil {
		leaderLog().Warn("rpc bridge error", "tool", req.Tool, "err", err)
		l.sendJSON(w, http.StatusOK, RPCResponse{Error: err.Error()})
		return
	}

	if resp.Error != "" {
		leaderLog().Warn("rpc plugin error", "tool", req.Tool, "err", resp.Error)
		l.sendJSON(w, http.StatusOK, RPCResponse{Error: resp.Error})
		return
	}

	l.sendJSON(w, http.StatusOK, RPCResponse{Data: resp.Data})
}

func (l *Leader) sendJSON(w http.ResponseWriter, status int, body RPCResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.MarshalWrite(w, body); err != nil {
		leaderLog().Warn("encode response", "err", err)
	}
}

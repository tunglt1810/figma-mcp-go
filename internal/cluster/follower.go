package cluster

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/tunglt1810/figma-mcp-go/internal/bridge"
)

func followerLog() *slog.Logger { return slog.Default().With("component", "follower") }

// Follower proxies MCP tool calls to the leader via HTTP /rpc.
type Follower struct {
	leaderURL string
	client    *http.Client
}

// NewFollower creates a Follower pointed at the given leader base URL.
func NewFollower(leaderURL string) *Follower {
	return &Follower{
		leaderURL: leaderURL,
		// No client-wide Timeout: one number cannot serve tools whose budgets
		// differ. Send sets a per-request deadline from the shared table.
		client: &http.Client{},
	}
}

// Send proxies a tool call to the leader.
func (f *Follower) Send(ctx context.Context, tool string, nodeIDs []string, params map[string]any) (bridge.Response, error) {
	followerLog().Info("proxying", "tool", tool, "nodes", len(nodeIDs), "leader", f.leaderURL)
	followerLog().Debug("proxy params", "tool", tool, "params", params)
	start := time.Now()

	// Outlast the leader's own timeout so its error reaches the caller instead
	// of a transport deadline that says nothing about what failed.
	ctx, cancel := context.WithTimeout(ctx, bridge.FollowerTimeoutFor(tool))
	defer cancel()

	rpcReq := RPCRequest{
		Tool:    tool,
		NodeIDs: nodeIDs,
		Params:  params,
	}

	body, err := json.Marshal(rpcReq)
	if err != nil {
		return bridge.Response{}, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.leaderURL+"/rpc", bytes.NewReader(body))
	if err != nil {
		return bridge.Response{}, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		followerLog().Warn("proxy rpc error", "tool", tool, "err", err)
		return bridge.Response{}, fmt.Errorf("rpc call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return bridge.Response{}, fmt.Errorf("read response: %w", err)
	}

	var rpcResp RPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return bridge.Response{}, fmt.Errorf("unmarshal: %w", err)
	}

	if rpcResp.Error != "" {
		followerLog().Warn("proxy error from the leader", "tool", tool, "ms", time.Since(start).Milliseconds(), "err", rpcResp.Error)
		return bridge.Response{Error: rpcResp.Error}, nil
	}

	followerLog().Info("proxied", "tool", tool, "ms", time.Since(start).Milliseconds())
	return bridge.Response{
		Type: tool,
		Data: rpcResp.Data,
	}, nil
}

// Ping checks if the leader is alive. Returns true if healthy.
func (f *Follower) Ping(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.leaderURL+"/ping", nil)
	if err != nil {
		followerLog().Warn("ping request", "err", err)
		return false
	}

	resp, err := f.client.Do(req)
	if err != nil {
		followerLog().Warn("ping failed", "leader", f.leaderURL, "err", err)
		return false
	}
	resp.Body.Close()
	ok := resp.StatusCode == http.StatusOK
	followerLog().Debug("ping", "leader", f.leaderURL, "status", resp.StatusCode, "healthy", ok)
	return ok
}

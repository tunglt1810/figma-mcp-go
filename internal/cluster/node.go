package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/tunglt1810/figma-mcp-go/internal/bridge"
)

func nodeLog() *slog.Logger { return slog.Default().With("component", "node") }

// sender is anything that can carry a tool call to the plugin — the Leader's
// Bridge (direct WebSocket) or a Follower (HTTP proxy to the leader).
type sender interface {
	Send(ctx context.Context, tool string, nodeIDs []string, params map[string]any) (bridge.Response, error)
}

// Node dynamically routes MCP tool calls to either the Leader bridge
// or the Follower HTTP proxy, depending on the current role.
type Node struct {
	mu       sync.RWMutex
	role     Role
	ip       string
	port     int
	leader   *Leader
	follower sender
	version  string
	guard    Guard
}

// NewNode creates a Node in the Unknown role.
func NewNode(ip string, port int, version string, guard Guard) *Node {
	return &Node{
		ip:       ip,
		port:     port,
		role:     RoleUnknown,
		version:  version,
		guard:    guard,
		follower: NewFollower(fmt.Sprintf("http://%s:%d", ip, port)),
	}
}

// Role returns the current role.
func (n *Node) Role() Role {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.role
}

// RoleName returns a human-readable role string.
func (n *Node) RoleName() string {
	switch n.Role() {
	case RoleLeader:
		return "LEADER"
	case RoleFollower:
		return "FOLLOWER"
	default:
		return "UNKNOWN"
	}
}

// Send routes a tool call to the plugin: the leader writes to its own bridge,
// a follower proxies to the leader over HTTP. Arguments arrive already
// normalized and checked — the tool layer does that for a local call, and the
// leader's /rpc guard does it for one that came from another process.
//
// A plugin-reported error and a transport error become the same thing here.
// Every caller already treated them identically; keeping them apart only
// duplicated the branch.
func (n *Node) Send(ctx context.Context, tool string, nodeIDs []string, params map[string]any) (any, error) {
	n.mu.RLock()
	role := n.role
	leader := n.leader
	follower := n.follower
	n.mu.RUnlock()

	nodeLog().Info("routing", "tool", tool, "role", n.RoleName(), "nodes", len(nodeIDs))

	var (
		resp bridge.Response
		err  error
	)
	switch {
	case role == RoleLeader && leader != nil:
		resp, err = leader.GetBridge().Send(ctx, tool, nodeIDs, params)
	case role == RoleFollower:
		resp, err = follower.Send(ctx, tool, nodeIDs, params)
	default:
		// The election has not settled. Say so: proxying to a port nobody holds
		// only produces "connection refused", which describes the symptom and
		// not the situation.
		return nil, errors.New("no leader yet — the server is still electing one, retry in a moment")
	}
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return resp.Data, nil
}

// BecomeLeader attempts to bind the port and transition to Leader role.
// Returns an error if the port is already in use.
func (n *Node) BecomeLeader() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.role == RoleLeader {
		return nil
	}

	leader := NewLeader(n.ip, n.port, n.version, n.guard)
	if err := leader.Start(); err != nil {
		return err
	}

	n.leader = leader
	n.role = RoleLeader
	nodeLog().Info("became LEADER")
	return nil
}

// BecomeFollower transitions to Follower role, stopping the leader if running.
func (n *Node) BecomeFollower() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.role == RoleFollower {
		return
	}

	if n.leader != nil {
		n.leader.Stop()
		n.leader = nil
	}

	n.role = RoleFollower
	nodeLog().Info("became FOLLOWER")
}

// Stop shuts down the node regardless of role.
func (n *Node) Stop() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.leader != nil {
		n.leader.Stop()
		n.leader = nil
	}
	n.role = RoleUnknown
}

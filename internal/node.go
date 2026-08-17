package internal

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
)

var nodeLogger = log.New(os.Stderr, "[node] ", 0)

// sender is anything that can carry a tool call to the plugin — the Leader's
// Bridge (direct WebSocket) or a Follower (HTTP proxy to the leader).
type sender interface {
	Send(ctx context.Context, tool string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error)
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
}

// nodeIDParams are the parameter names that carry a Figma node ID and so need
// the same hyphen→colon normalization as the nodeIDs slice.
// nodeIDParams name the arguments that carry a single node ID. They are
// matched at any depth: a pipeline step nests a whole parameter set of its own
// under steps[].params, and a reaction nests a destination below that again.
var nodeIDParams = map[string]bool{
	"nodeId": true, "parentId": true, "pageId": true, "componentId": true,
	"startNodeId": true, "endNodeId": true, "destinationId": true,
}

// normalizeArgs returns copies of the arguments with node IDs normalized.
// Copies, not in-place edits: the caller's slice and map belong to the caller.
func normalizeArgs(nodeIDs []string, params map[string]interface{}) ([]string, map[string]interface{}) {
	var ids []string
	if nodeIDs != nil {
		ids = make([]string, len(nodeIDs))
		for i, id := range nodeIDs {
			ids[i] = NormalizeNodeID(id)
		}
	}

	var p map[string]interface{}
	if params != nil {
		p, _ = normalizeValue(params).(map[string]interface{})
	}

	return ids, p
}

// normalizeValue rebuilds v with every node ID it can find normalized. Maps and
// slices are rebuilt rather than edited, so nothing the caller passed in
// changes underfoot.
func normalizeValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			switch {
			case nodeIDParams[k]:
				if s, ok := val.(string); ok {
					out[k] = NormalizeNodeID(s)
					continue
				}
				out[k] = normalizeValue(val)
			case k == "nodeIds":
				out[k] = normalizeIDList(val)
			default:
				out[k] = normalizeValue(val)
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, item := range t {
			out[i] = normalizeValue(item)
		}
		return out
	default:
		return v
	}
}

// normalizeIDList handles a "nodeIds" value, which is a list of ids rather than
// a nested structure.
func normalizeIDList(v interface{}) interface{} {
	list, ok := v.([]interface{})
	if !ok {
		return normalizeValue(v)
	}
	out := make([]interface{}, len(list))
	for i, item := range list {
		if s, ok := item.(string); ok {
			out[i] = NormalizeNodeID(s)
			continue
		}
		out[i] = normalizeValue(item)
	}
	return out
}

// NewNode creates a Node in the Unknown role.
func NewNode(ip string, port int, version string) *Node {
	return &Node{
		ip:       ip,
		port:     port,
		role:     RoleUnknown,
		version:  version,
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

// Send validates a request and routes it to the appropriate backend.
//
// Validation lives here rather than only in the leader's /rpc handler so that
// it applies to every tool call. A leader process talks to its own Bridge
// directly and never crosses /rpc, so validation placed there alone would be
// skipped for the common single-client setup.
func (n *Node) Send(ctx context.Context, tool string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error) {
	// Normalize first: the hyphen format LLMs emit must be accepted, not
	// rejected by the validation that exists to tolerate it.
	nodeIDs, params = normalizeArgs(nodeIDs, params)

	if msg := ValidateRPC(tool, nodeIDs, params); msg != "" {
		nodeLogger.Printf("tool=%s rejected: %s", tool, msg)
		return BridgeResponse{Error: msg}, nil
	}

	n.mu.RLock()
	role := n.role
	leader := n.leader
	follower := n.follower
	n.mu.RUnlock()

	nodeLogger.Printf("tool=%s role=%s nodeIDs=%v", tool, n.RoleName(), nodeIDs)

	if role == RoleLeader && leader != nil {
		return leader.GetBridge().Send(ctx, tool, nodeIDs, params)
	}
	return follower.Send(ctx, tool, nodeIDs, params)
}

// BecomeLeader attempts to bind the port and transition to Leader role.
// Returns an error if the port is already in use.
func (n *Node) BecomeLeader() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.role == RoleLeader {
		return nil
	}

	leader := NewLeader(n.ip, n.port, n.version)
	if err := leader.Start(); err != nil {
		return err
	}

	n.leader = leader
	n.role = RoleLeader
	nodeLogger.Printf("became LEADER")
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
	nodeLogger.Printf("became FOLLOWER")
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

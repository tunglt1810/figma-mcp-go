package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── RoleName ─────────────────────────────────────────────────────────────────

func TestNodeRoleName(t *testing.T) {
	cases := []struct {
		role Role
		want string
	}{
		{RoleUnknown, "UNKNOWN"},
		{RoleLeader, "LEADER"},
		{RoleFollower, "FOLLOWER"},
	}
	for _, c := range cases {
		n := &Node{role: c.role}
		if got := n.RoleName(); got != c.want {
			t.Errorf("RoleName(%v) = %q, want %q", c.role, got, c.want)
		}
	}
}

// ── NewNode ───────────────────────────────────────────────────────────────────

func TestNewNode_StartsUnknown(t *testing.T) {
	n := NewNode("127.0.0.1", 19940, "test")
	if n.Role() != RoleUnknown {
		t.Errorf("new node role = %v, want UNKNOWN", n.Role())
	}
}

// ── BecomeLeader ─────────────────────────────────────────────────────────────

func TestNodeBecomeLeader(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test")
	t.Cleanup(n.Stop)

	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("BecomeLeader: %v", err)
	}
	if n.Role() != RoleLeader {
		t.Errorf("role = %v, want LEADER", n.Role())
	}
}

func TestNodeBecomeLeader_PortTaken(t *testing.T) {
	port := freePort(t)

	n1 := NewNode("127.0.0.1", port, "test")
	if err := n1.BecomeLeader(); err != nil {
		t.Fatalf("first BecomeLeader: %v", err)
	}
	t.Cleanup(n1.Stop)

	n2 := NewNode("127.0.0.1", port, "test")
	if err := n2.BecomeLeader(); err == nil {
		n2.Stop()
		t.Error("expected error when port is already taken")
	}
}

func TestNodeBecomeLeader_Idempotent(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test")
	t.Cleanup(n.Stop)

	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("first BecomeLeader: %v", err)
	}
	// Calling again on the same node should be a no-op.
	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("second BecomeLeader: %v", err)
	}
}

// ── BecomeFollower ────────────────────────────────────────────────────────────

func TestNodeBecomeFollower(t *testing.T) {
	n := NewNode("127.0.0.1", 19940, "test")
	n.BecomeFollower()
	if n.Role() != RoleFollower {
		t.Errorf("role = %v, want FOLLOWER", n.Role())
	}
}

func TestNodeBecomeFollower_Idempotent(t *testing.T) {
	n := NewNode("127.0.0.1", 19940, "test")
	n.BecomeFollower()
	n.BecomeFollower() // should not panic
	if n.Role() != RoleFollower {
		t.Errorf("role = %v, want FOLLOWER", n.Role())
	}
}

func TestNodeBecomeFollower_FromLeader(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test")

	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("BecomeLeader: %v", err)
	}
	n.BecomeFollower()
	if n.Role() != RoleFollower {
		t.Errorf("role = %v, want FOLLOWER after BecomeFollower", n.Role())
	}

	// Give the OS a moment to fully release the port after Shutdown.
	time.Sleep(20 * time.Millisecond)

	// Port should be free now — a new leader can bind it.
	n2 := NewNode("127.0.0.1", port, "test")
	if err := n2.BecomeLeader(); err != nil {
		t.Fatalf("new node could not bind freed port: %v", err)
	}
	n2.Stop()
}

// ── Stop ─────────────────────────────────────────────────────────────────────

func TestNodeStop_ResetsRole(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test")

	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("BecomeLeader: %v", err)
	}
	n.Stop()
	if n.Role() != RoleUnknown {
		t.Errorf("role after Stop = %v, want UNKNOWN", n.Role())
	}
}

func TestNodeStop_Idempotent(t *testing.T) {
	n := NewNode("127.0.0.1", 19940, "test")
	n.Stop()
	n.Stop() // should not panic
}

// ── Send: ID normalisation ────────────────────────────────────────────────────

// TestNodeSend_NormalizesIDs verifies that hyphen-format node IDs are converted
// to colon format before being forwarded to the backend.
func TestNodeSend_NormalizesIDs(t *testing.T) {
	var capturedReq RPCRequest

	// Fake leader that records what the follower sends.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(RPCResponse{Data: "ok"})
	}))
	t.Cleanup(srv.Close)

	// Build a follower node pointed at the fake server.
	n := &Node{
		role:     RoleFollower,
		follower: NewFollower(srv.URL),
	}

	params := map[string]any{
		"nodeId":   "100-200", // hyphen format
		"parentId": "300-400", // hyphen format
	}
	n.Send(context.Background(), "clone_node", []string{"1-1", "2-2"}, params) //nolint:errcheck

	// nodeIDs should be normalised.
	for _, id := range capturedReq.NodeIDs {
		if id == "1-1" || id == "2-2" {
			t.Errorf("nodeID %q was not normalised to colon format", id)
		}
	}

	// Params nodeId/parentId should be normalised.
	if nodeID, _ := capturedReq.Params["nodeId"].(string); nodeID == "100-200" {
		t.Error("params.nodeId was not normalised")
	}
	if parentID, _ := capturedReq.Params["parentId"].(string); parentID == "300-400" {
		t.Error("params.parentId was not normalised")
	}
}

// ── Validation on the primary path ───────────────────────────────────────────
//
// ValidateRPC used to be reachable only from the leader's /rpc handler, i.e.
// only for follower processes. The first process to start is the leader, so for
// most users the validation never ran and bad arguments went straight to Figma.
// These tests pin validation to Node.Send, which every tool call goes through.

// fakeSender records what it was asked to send and never touches the network.
type fakeSender struct {
	calls []struct {
		tool    string
		nodeIDs []string
		params  map[string]interface{}
	}
	resp BridgeResponse
	err  error
}

func (f *fakeSender) Send(_ context.Context, tool string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error) {
	f.calls = append(f.calls, struct {
		tool    string
		nodeIDs []string
		params  map[string]interface{}
	}{tool, nodeIDs, params})
	return f.resp, f.err
}

func newNodeWithSender(s sender) *Node {
	n := NewNode("127.0.0.1", 19940, "test")
	n.follower = s
	return n
}

func TestNodeSend_RejectsInvalidArgsBeforeReachingPlugin(t *testing.T) {
	cases := []struct {
		name    string
		tool    string
		nodeIDs []string
		params  map[string]interface{}
		wantMsg string
	}{
		{"opacity out of range", "set_node_properties", []string{"1:1"}, map[string]interface{}{"opacity": 5.0}, "opacity must be at most 1"},
		{"invalid blend mode", "set_node_properties", []string{"1:1"}, map[string]interface{}{"blendMode": "NEON"}, "blendMode must be one of"},
		{"missing node id", "get_node", nil, nil, "nodeId is required"},
		{"bad node id format", "rename_node", []string{"nope"}, map[string]interface{}{"name": "x"}, "colon format"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeSender{}
			n := newNodeWithSender(fake)

			resp, err := n.Send(context.Background(), c.tool, c.nodeIDs, c.params)
			if err != nil {
				t.Fatalf("Send returned a Go error: %v", err)
			}
			if resp.Error == "" {
				t.Fatalf("expected a validation error for %s, got none", c.tool)
			}
			if !strings.Contains(resp.Error, c.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", resp.Error, c.wantMsg)
			}
			if len(fake.calls) != 0 {
				t.Errorf("invalid request reached the plugin: %+v", fake.calls)
			}
		})
	}
}

func TestNodeSend_PassesValidArgsThrough(t *testing.T) {
	fake := &fakeSender{resp: BridgeResponse{Data: map[string]any{"ok": true}}}
	n := newNodeWithSender(fake)

	if _, err := n.Send(context.Background(), "set_node_properties", []string{"1:1"}, map[string]interface{}{"opacity": 0.5}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 call to the sender, got %d", len(fake.calls))
	}
	if fake.calls[0].tool != "set_node_properties" {
		t.Errorf("tool = %q, want set_node_properties", fake.calls[0].tool)
	}
}

// Node IDs must be normalised before validation, otherwise the hyphen format
// LLMs emit would be rejected by the very check meant to tolerate it.
func TestNodeSend_NormalizesBeforeValidating(t *testing.T) {
	fake := &fakeSender{}
	n := newNodeWithSender(fake)

	resp, err := n.Send(context.Background(), "get_node", []string{"4029-12345"}, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("hyphen-format id was rejected: %s", resp.Error)
	}
	if len(fake.calls) != 1 || fake.calls[0].nodeIDs[0] != "4029:12345" {
		t.Errorf("expected normalized id to reach the sender, got %+v", fake.calls)
	}
}

// Node.Send must not mutate the caller's slice or map (P2-12).
func TestNodeSend_DoesNotMutateCallerArgs(t *testing.T) {
	fake := &fakeSender{}
	n := newNodeWithSender(fake)

	nodeIDs := []string{"4029-12345"}
	params := map[string]interface{}{"nodeId": "4029-12345"}

	if _, err := n.Send(context.Background(), "get_node", nodeIDs, params); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if nodeIDs[0] != "4029-12345" {
		t.Errorf("caller slice was mutated: %v", nodeIDs)
	}
	if params["nodeId"] != "4029-12345" {
		t.Errorf("caller map was mutated: %v", params)
	}
}

// A pipeline step carries a whole parameter set of its own, one level below
// anything the top-level pass reached — so hyphen IDs inside a pipeline went
// to the plugin unconverted and the step failed to find its node.
func TestNodeSend_NormalizesIDsInsidePipelineSteps(t *testing.T) {
	fake := &fakeSender{}
	n := newNodeWithSender(fake)

	steps := []interface{}{
		map[string]interface{}{
			"action": "clone_node",
			"params": map[string]interface{}{
				"nodeId":   "100-200",
				"parentId": "300-400",
			},
		},
		map[string]interface{}{
			"action": "delete_nodes",
			"params": map[string]interface{}{
				"nodeIds": []interface{}{"1-1", "2-2"},
			},
		},
	}
	if _, err := n.Send(context.Background(), "batch_execute_pipeline", nil, map[string]interface{}{"steps": steps}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fake.calls))
	}

	sent, _ := fake.calls[0].params["steps"].([]interface{})
	if len(sent) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(sent))
	}

	first, _ := sent[0].(map[string]interface{})["params"].(map[string]interface{})
	if first["nodeId"] != "100:200" {
		t.Errorf("steps[0].params.nodeId = %v, want 100:200", first["nodeId"])
	}
	if first["parentId"] != "300:400" {
		t.Errorf("steps[0].params.parentId = %v, want 300:400", first["parentId"])
	}

	second, _ := sent[1].(map[string]interface{})["params"].(map[string]interface{})
	ids, _ := second["nodeIds"].([]interface{})
	if len(ids) != 2 || ids[0] != "1:1" || ids[1] != "2:2" {
		t.Errorf("steps[1].params.nodeIds = %v, want [1:1 2:2]", ids)
	}
}

// Normalizing must still copy rather than edit: the nested maps belong to the
// caller too.
func TestNodeSend_DoesNotMutateNestedCallerArgs(t *testing.T) {
	fake := &fakeSender{}
	n := newNodeWithSender(fake)

	inner := map[string]interface{}{"nodeId": "100-200"}
	steps := []interface{}{map[string]interface{}{"action": "clone_node", "params": inner}}
	if _, err := n.Send(context.Background(), "batch_execute_pipeline", nil, map[string]interface{}{"steps": steps}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner["nodeId"] != "100-200" {
		t.Errorf("caller's nested map was mutated: nodeId = %v", inner["nodeId"])
	}
}

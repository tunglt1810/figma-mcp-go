package cluster

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"time"

	"github.com/tunglt1810/figma-mcp-go/internal/bridge"
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
	n := NewNode("127.0.0.1", 19940, "test", passthroughGuard)
	if n.Role() != RoleUnknown {
		t.Errorf("new node role = %v, want UNKNOWN", n.Role())
	}
}

// ── BecomeLeader ─────────────────────────────────────────────────────────────

func TestNodeBecomeLeader(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test", passthroughGuard)
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

	n1 := NewNode("127.0.0.1", port, "test", passthroughGuard)
	if err := n1.BecomeLeader(); err != nil {
		t.Fatalf("first BecomeLeader: %v", err)
	}
	t.Cleanup(n1.Stop)

	n2 := NewNode("127.0.0.1", port, "test", passthroughGuard)
	if err := n2.BecomeLeader(); err == nil {
		n2.Stop()
		t.Error("expected error when port is already taken")
	}
}

func TestNodeBecomeLeader_Idempotent(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test", passthroughGuard)
	t.Cleanup(n.Stop)

	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("first BecomeLeader: %v", err)
	}
	// Calling it again on the same node should do nothing.
	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("second BecomeLeader: %v", err)
	}
}

// ── BecomeFollower ────────────────────────────────────────────────────────────

func TestNodeBecomeFollower(t *testing.T) {
	n := NewNode("127.0.0.1", 19940, "test", passthroughGuard)
	n.BecomeFollower()
	if n.Role() != RoleFollower {
		t.Errorf("role = %v, want FOLLOWER", n.Role())
	}
}

func TestNodeBecomeFollower_Idempotent(t *testing.T) {
	n := NewNode("127.0.0.1", 19940, "test", passthroughGuard)
	n.BecomeFollower()
	n.BecomeFollower() // should not panic
	if n.Role() != RoleFollower {
		t.Errorf("role = %v, want FOLLOWER", n.Role())
	}
}

func TestNodeBecomeFollower_FromLeader(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test", passthroughGuard)

	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("BecomeLeader: %v", err)
	}
	n.BecomeFollower()
	if n.Role() != RoleFollower {
		t.Errorf("role = %v, want FOLLOWER after BecomeFollower", n.Role())
	}

	// Give the OS a moment to fully release the port after Shutdown.
	time.Sleep(20 * time.Millisecond)

	// The port should be free now, so a new leader can bind it.
	n2 := NewNode("127.0.0.1", port, "test", passthroughGuard)
	if err := n2.BecomeLeader(); err != nil {
		t.Fatalf("new node could not bind freed port: %v", err)
	}
	n2.Stop()
}

// ── Stop ─────────────────────────────────────────────────────────────────────

func TestNodeStop_ResetsRole(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test", passthroughGuard)

	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("BecomeLeader: %v", err)
	}
	n.Stop()
	if n.Role() != RoleUnknown {
		t.Errorf("role after Stop = %v, want UNKNOWN", n.Role())
	}
}

func TestNodeStop_Idempotent(t *testing.T) {
	n := NewNode("127.0.0.1", 19940, "test", passthroughGuard)
	n.Stop()
	n.Stop() // should not panic
}

// ── Send: ID normalisation ────────────────────────────────────────────────────

// A follower posts the call to the leader's /rpc as is. Checking already
// happened earlier, so any rewrite by the node here would be a second, silent
// change on the way out.
func TestNodeSend_ProxiesToTheLeaderVerbatim(t *testing.T) {
	var capturedReq RPCRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.UnmarshalRead(r.Body, &capturedReq) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		json.MarshalWrite(w, RPCResponse{Data: "ok"}) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)

	n := &Node{
		role:     RoleFollower,
		follower: NewFollower(srv.URL),
	}

	params := map[string]any{"nodeId": "100:200", "parentId": "300:400"}
	if _, err := n.Send(context.Background(), "clone_node", []string{"1:1", "2:2"}, params); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if capturedReq.Tool != "clone_node" {
		t.Errorf("tool = %q, want clone_node", capturedReq.Tool)
	}
	if len(capturedReq.NodeIDs) != 2 || capturedReq.NodeIDs[0] != "1:1" || capturedReq.NodeIDs[1] != "2:2" {
		t.Errorf("nodeIDs = %v, want [1:1 2:2]", capturedReq.NodeIDs)
	}
	if capturedReq.Params["nodeId"] != "100:200" || capturedReq.Params["parentId"] != "300:400" {
		t.Errorf("params = %v, want them carried through unchanged", capturedReq.Params)
	}
}

// ── Routing ──────────────────────────────────────────────────────────────────

// fakeBackend stands in for a Follower or a Bridge behind the internal
// sender interface, which uses bridge.Response.
type fakeBackend struct {
	calls []fakeCall
	resp  bridge.Response
	err   error
}

func (f *fakeBackend) Send(_ context.Context, tool string, nodeIDs []string, params map[string]any) (bridge.Response, error) {
	f.calls = append(f.calls, fakeCall{tool, nodeIDs, params})
	return f.resp, f.err
}

// newNodeWithSender builds a follower node with its proxy replaced. Swapping
// the follower backend only matters once the node is a follower.
func newNodeWithSender(s sender) *Node {
	n := NewNode("127.0.0.1", 19940, "test", passthroughGuard)
	n.follower = s
	n.role = RoleFollower
	return n
}

// Send passes the arguments on unchanged. Normalizing and checking now happen
// earlier, so a node that rewrote them would be doing it twice.
func TestNodeSend_PassesArgumentsThrough(t *testing.T) {
	backend := &fakeBackend{resp: bridge.Response{Data: map[string]any{"ok": true}}}
	n := newNodeWithSender(backend)

	params := map[string]any{"opacity": 0.5}
	if _, err := n.Send(context.Background(), "set_node_properties", []string{"1:1"}, params); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(backend.calls) != 1 {
		t.Fatalf("expected 1 call to the backend, got %d", len(backend.calls))
	}
	got := backend.calls[0]
	if got.tool != "set_node_properties" {
		t.Errorf("tool = %q, want set_node_properties", got.tool)
	}
	if len(got.nodeIDs) != 1 || got.nodeIDs[0] != "1:1" {
		t.Errorf("nodeIDs = %v, want [1:1]", got.nodeIDs)
	}
	if got.params["opacity"] != 0.5 {
		t.Errorf("params = %v, want opacity 0.5", got.params)
	}
}

// To a caller, a plugin error and a transport error mean the same thing: the
// call did not happen. Send returns one error type, so the tool layer needs
// one branch instead of two that did the same thing.
func TestNodeSend_TurnsAPluginErrorIntoAnError(t *testing.T) {
	backend := &fakeBackend{resp: bridge.Response{Error: "node not found"}}
	n := newNodeWithSender(backend)

	data, err := n.Send(context.Background(), "get_nodes_info", []string{"1:1"}, nil)
	if err == nil {
		t.Fatal("expected a plugin error to surface as an error")
	}
	if !strings.Contains(err.Error(), "node not found") {
		t.Errorf("unexpected message: %v", err)
	}
	if data != nil {
		t.Errorf("expected no data alongside an error, got %v", data)
	}
}

func TestNodeSend_ReturnsTheDataOnly(t *testing.T) {
	backend := &fakeBackend{resp: bridge.Response{Data: map[string]any{"ok": true}}}
	n := newNodeWithSender(backend)

	data, err := n.Send(context.Background(), "get_nodes_info", []string{"1:1"}, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	got, ok := data.(map[string]any)
	if !ok || got["ok"] != true {
		t.Errorf("want the plugin's data unwrapped, got %#v", data)
	}
}

// An Unknown role means the election has not settled. Falling through to the
// follower branch posts to a port nobody is listening on. The user then sees
// "connection refused", which says nothing about the real problem.
func TestNodeSend_UnknownRoleReportsTheRole(t *testing.T) {
	backend := &fakeBackend{}
	n := NewNode("127.0.0.1", 19940, "test", passthroughGuard)
	n.follower = backend

	if n.Role() != RoleUnknown {
		t.Fatalf("expected RoleUnknown to start, got %s", n.RoleName())
	}

	_, err := n.Send(context.Background(), "get_document", nil, nil)
	if err == nil {
		t.Fatal("expected an error while the role is unknown")
	}
	if !strings.Contains(err.Error(), "no leader") {
		t.Errorf("unexpected message: %v", err)
	}
	if len(backend.calls) != 0 {
		t.Errorf("Unknown role still proxied to the leader: %v", backend.calls)
	}
}

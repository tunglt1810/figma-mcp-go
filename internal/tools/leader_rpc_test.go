package tools

import (
	"encoding/json/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/tunglt1810/figma-mcp-go/internal/cluster"
)

// freePort finds an available TCP port on 127.0.0.1.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// /rpc is where another process's input arrives, so it checks arguments itself
// rather than trusting the follower that sent them. It normalizes too: a
// hyphen-format node ID posted here reaches the bridge in colon format.
func TestLeaderRPC_NormalizesAndValidates(t *testing.T) {
	port := freePort(t)
	leader := cluster.NewLeader("127.0.0.1", port, "test", Check)
	if err := leader.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(leader.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)

	// Invalid arguments are rejected with 400 and never reach the bridge.
	body := `{"tool":"set_node_properties","nodeIds":["1:1"],"params":{"opacity":5}}`
	resp, err := http.Post(base+"/rpc", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400 for invalid opacity, got %d", resp.StatusCode)
	}

	// A hyphen-format ID is accepted; with no plugin connected the call fails
	// at the bridge, which is proof it got past the check.
	body = `{"tool":"set_text","nodeIds":["4029-12345"],"params":{"text":"hi"}}`
	resp2, err := http.Post(base+"/rpc", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("want 200 for a normalizable node ID, got %d", resp2.StatusCode)
	}
	var rpcResp cluster.RPCResponse
	if err := json.UnmarshalRead(resp2.Body, &rpcResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(rpcResp.Error, "plugin not connected") {
		t.Errorf("want a bridge error, got %q", rpcResp.Error)
	}
}

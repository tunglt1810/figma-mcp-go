package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// The golden schema snapshot pins what clients see, but says nothing about what
// the plugin receives. Migrating a hand-written handler to a toolSpec can keep
// the schema byte-identical while changing the wire call — get_annotations sends
// its node id inside params, and moving it to the nodeIDs field would silently
// stop scoping the query. These tests pin the wire shape.

func newWireTestServer(t *testing.T) (*server.MCPServer, *fakeSender) {
	t.Helper()
	fake := &fakeSender{}
	node := newNodeWithSender(fake)
	s := server.NewMCPServer("test", "0.0.1")
	RegisterTools(s, node)
	return s, fake
}

func callWire(t *testing.T, s *server.MCPServer, tool string, args map[string]any) {
	t.Helper()
	argsJSON, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	msg := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
		tool, argsJSON,
	)
	if resp := s.HandleMessage(context.Background(), []byte(msg)); resp == nil {
		t.Fatalf("HandleMessage returned nil for %s", tool)
	}
}

func TestToolWireShape(t *testing.T) {
	cases := []struct {
		tool        string
		args        map[string]any
		wantNodeIDs []string
		wantParams  map[string]interface{}
	}{
		{
			tool: "get_styles", args: map[string]any{},
			wantNodeIDs: nil, wantParams: map[string]interface{}{},
		},
		{
			// The node id belongs in params here; the plugin reads
			// request.params.nodeId and ignores request.nodeIds.
			tool: "get_annotations", args: map[string]any{"nodeId": "4029:12345"},
			wantNodeIDs: nil, wantParams: map[string]interface{}{"nodeId": "4029:12345"},
		},
		{
			tool: "get_annotations", args: map[string]any{},
			wantNodeIDs: nil, wantParams: map[string]interface{}{},
		},
		{
			tool: "export_tokens", args: map[string]any{"format": "css"},
			wantNodeIDs: nil, wantParams: map[string]interface{}{"format": "css"},
		},
		{
			// An omitted argument must stay omitted so the plugin's own default
			// applies, rather than being sent as a zero value.
			tool: "export_tokens", args: map[string]any{},
			wantNodeIDs: nil, wantParams: map[string]interface{}{},
		},
	}

	for _, c := range cases {
		t.Run(c.tool+"/"+fmt.Sprint(len(c.args)), func(t *testing.T) {
			s, fake := newWireTestServer(t)
			callWire(t, s, c.tool, c.args)

			if len(fake.calls) != 1 {
				t.Fatalf("expected 1 wire call, got %d", len(fake.calls))
			}
			got := fake.calls[0]
			if got.tool != c.tool {
				t.Errorf("wire tool = %q, want %q", got.tool, c.tool)
			}
			if !reflect.DeepEqual(got.nodeIDs, c.wantNodeIDs) {
				t.Errorf("nodeIDs = %#v, want %#v", got.nodeIDs, c.wantNodeIDs)
			}
			if !reflect.DeepEqual(got.params, c.wantParams) {
				t.Errorf("params = %#v, want %#v", got.params, c.wantParams)
			}
		})
	}
}

// Every table-declared tool must be registered, and vice versa, so a spec
// cannot be added to the registry without reaching clients.
func TestSpecRegistry_MatchesRegisteredTools(t *testing.T) {
	registered := map[string]bool{}
	for _, tool := range listTools(t).Result.Tools {
		registered[tool.Name] = true
	}
	for name := range specRegistry {
		if !registered[name] {
			t.Errorf("spec %q is in the registry but not registered with the server", name)
		}
	}
}

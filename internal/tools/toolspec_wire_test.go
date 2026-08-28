package tools

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// The golden schema snapshot pins what clients see, but says nothing about what
// the plugin receives. Migrating a hand-written handler to a toolSpec can keep
// the schema byte-identical while changing the wire call — get_annotations sends
// its node id inside params, and moving it to the nodeIDs field would silently
// stop scoping the query. These tests pin the wire shape.

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
		name        string
		tool        string
		args        map[string]any
		wantNodeIDs []string
		wantParams  map[string]any
	}{
		{
			name: "no arguments",
			tool: "get_styles", args: map[string]any{},
			wantNodeIDs: nil, wantParams: map[string]any{},
		},
		{
			// The node id belongs in params here; the plugin reads
			// request.params.nodeId and ignores request.nodeIds.
			name: "node id stays in params",
			tool: "get_annotations", args: map[string]any{"nodeId": "4029:12345"},
			wantNodeIDs: nil, wantParams: map[string]any{"nodeId": "4029:12345"},
		},
		{
			name: "no node id",
			tool: "get_annotations", args: map[string]any{},
			wantNodeIDs: nil, wantParams: map[string]any{},
		},
		{
			name: "format forwarded",
			tool: "export_tokens", args: map[string]any{"format": "css"},
			wantNodeIDs: nil, wantParams: map[string]any{"format": "css"},
		},
		{
			// An omitted argument must stay omitted so the plugin's own default
			// applies, rather than being sent as a zero value.
			name: "omitted argument stays omitted",
			tool: "export_tokens", args: map[string]any{},
			wantNodeIDs: nil, wantParams: map[string]any{},
		},
		{
			// The plugin reads request.nodeIds[0] as the search root, so the
			// nodeId argument must travel in the nodeIDs field, not in params.
			name:        "scoped to a subtree",
			tool:        "find_replace_text",
			args:        map[string]any{"nodeId": "4029:12345", "find": "a", "replace": "b"},
			wantNodeIDs: []string{"4029:12345"},
			wantParams:  map[string]any{"find": "a", "replace": "b"},
		},
		{
			name: "whole page",
			tool: "find_replace_text", args: map[string]any{"find": "a", "replace": ""},
			wantNodeIDs: nil,
			// An empty replacement deletes matches, so it must reach the plugin
			// rather than being dropped as an absent argument.
			wantParams: map[string]any{"find": "a", "replace": ""},
		},
		{
			name: "empty text clears the node",
			tool: "set_text", args: map[string]any{"nodeId": "1:1", "text": ""},
			wantNodeIDs: []string{"1:1"}, wantParams: map[string]any{"text": ""},
		},
		{
			name:        "empty replacement strips the found text",
			tool:        "batch_rename_nodes",
			args:        map[string]any{"nodeIds": []any{"1:1"}, "find": "old", "replace": ""},
			wantNodeIDs: []string{"1:1"},
			wantParams:  map[string]any{"find": "old", "replace": ""},
		},
		{
			// The old handler forwarded every argument it was given, node id
			// included; only declared parameters should reach the plugin now.
			name:        "node id is not repeated in params",
			tool:        "set_auto_layout",
			args:        map[string]any{"nodeId": "1:1", "layoutMode": "VERTICAL", "itemSpacing": 8},
			wantNodeIDs: []string{"1:1"},
			wantParams:  map[string]any{"layoutMode": "VERTICAL", "itemSpacing": float64(8)},
		},
		{
			// An empty array means "remove them all" and is not the same as
			// omitting the argument.
			name:        "empty indices removes every reaction",
			tool:        "remove_reactions",
			args:        map[string]any{"nodeId": "1:1", "indices": []any{}},
			wantNodeIDs: []string{"1:1"},
			wantParams:  map[string]any{"indices": []any{}},
		},
		{
			// Empty clears every effect on the node, so the array has to be
			// forwarded rather than treated as an absent argument.
			name:        "empty effects clears them",
			tool:        "set_effects",
			args:        map[string]any{"nodeId": "1:1", "effects": []any{}},
			wantNodeIDs: []string{"1:1"},
			wantParams:  map[string]any{"effects": []any{}},
		},
		{
			// This tool takes no node ids at all; both ends live in params.
			name:        "connector endpoints stay in params",
			tool:        "create_connector",
			args:        map[string]any{"startNodeId": "1:1", "endNodeId": "2:2"},
			wantNodeIDs: nil,
			wantParams:  map[string]any{"startNodeId": "1:1", "endNodeId": "2:2"},
		},
		{
			name:        "false is a value, not an omission",
			tool:        "set_node_properties",
			args:        map[string]any{"nodeIds": []any{"1:1"}, "visible": false},
			wantNodeIDs: []string{"1:1"},
			wantParams:  map[string]any{"visible": false},
		},
	}

	for _, c := range cases {
		t.Run(c.tool+"/"+c.name, func(t *testing.T) {
			s, fake := newTestServer(t)
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

// Every tool is now table-declared, so the registry and the registered set are
// the same set. A hand-written registration would show up here.
func TestSpecRegistry_CoversEveryTool(t *testing.T) {
	for _, tool := range listTools(t).Result.Tools {
		if _, ok := specRegistry[tool.Name]; !ok {
			t.Errorf("tool %q is registered but has no spec", tool.Name)
		}
	}
}

// Checking belongs to the handler, not to whatever happens to be behind it.
// Registered against a bare Sender — no Node, so nothing downstream validates
// anything — a bad call must still be rejected before it reaches the sender.
// That holds for a plain forwarding tool and for one that runs its own Go code.
func TestRegisterTools_ChecksWithoutANodeBehindIt(t *testing.T) {
	cases := []struct {
		name    string
		tool    string
		args    map[string]any
		wantMsg string
	}{
		{"plain forwarder", "set_node_properties",
			map[string]any{"nodeIds": []any{"1:1"}, "opacity": 5.0}, "opacity must be at most 1"},
		{"tool with its own Go code", "export_frames_to_pdf",
			map[string]any{"nodeIds": []any{"not-a-node-id"}, "outputPath": "out.pdf"}, "colon format"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeSender{}
			s := server.NewMCPServer("test", "0.0.1")
			RegisterTools(s, fake)

			res := callToolResult(t, s, tc.tool, tc.args)

			if !res.IsError {
				t.Fatalf("expected %s to be rejected", tc.tool)
			}
			if !strings.Contains(res.Text, tc.wantMsg) {
				t.Errorf("want a message containing %q, got %q", tc.wantMsg, res.Text)
			}
			if len(fake.calls) != 0 {
				t.Errorf("rejected call still reached the sender: %v", fake.calls)
			}
		})
	}
}

// save_screenshots calls get_screenshot once per item with params it builds
// itself, so checking the arguments it was invoked with says nothing about what
// reaches the plugin. Before this was noticed, an unrecognised per-item format
// went straight through: on a path whose extension implies nothing, the
// format-conflict guard does not fire either.
//
// The invariant is about Sender calls, not entry points.
func TestCustomHandler_InnerCallIsCheckedUnderItsOwnName(t *testing.T) {
	s, fake := newTestServer(t)

	callTool(t, s, "save_screenshots", map[string]any{
		"items": []any{
			map[string]any{"nodeId": "1:2", "outputPath": "out/shot.bin", "format": "GIF"},
		},
	})

	for _, c := range fake.calls {
		if msg := ValidateRPC(c.tool, c.nodeIDs, c.params); msg != "" {
			t.Errorf("a call reached the sender that its own spec rejects: tool=%s params=%v: %s",
				c.tool, c.params, msg)
		}
	}
}

// The wrapping must not break the calls that are fine.
func TestCustomHandler_ValidInnerCallStillGoesThrough(t *testing.T) {
	s, fake := newTestServer(t)

	callTool(t, s, "save_screenshots", map[string]any{
		"items": []any{
			map[string]any{"nodeId": "1:2", "outputPath": "out/shot.png"},
		},
	})

	if len(fake.calls) != 1 {
		t.Fatalf("expected the inner get_screenshot call, got %d calls", len(fake.calls))
	}
	if fake.calls[0].tool != "get_screenshot" {
		t.Errorf("inner tool = %q, want get_screenshot", fake.calls[0].tool)
	}
	if fake.calls[0].params["format"] != "PNG" {
		t.Errorf("inner params = %v, want format PNG", fake.calls[0].params)
	}
}

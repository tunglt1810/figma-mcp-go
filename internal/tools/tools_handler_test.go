package tools

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// fakeCall is one recorded call.
type fakeCall struct {
	tool    string
	nodeIDs []string
	params  map[string]any
}

// fakeSender records what the tool layer asked for and never touches the
// network.
type fakeSender struct {
	calls []fakeCall
	data  any
	err   error
}

func (f *fakeSender) Send(_ context.Context, tool string, nodeIDs []string, params map[string]any) (any, error) {
	f.calls = append(f.calls, fakeCall{tool, nodeIDs, params})
	return f.data, f.err
}

// newTestServer returns an MCPServer with every tool registered against a fake
// sender. No Node, no HTTP: a tool test has no business dialling anything.
func newTestServer(t *testing.T) (*server.MCPServer, *fakeSender) {
	t.Helper()
	s := server.NewMCPServer("test", "0.0.1")
	fake := &fakeSender{}
	RegisterTools(s, fake)
	return s, fake
}

// callTool dispatches a tool call through the server's full HandleMessage path.
// Against the fake sender every call succeeds at the MCP level; these tests are
// about the handler reaching the sender at all, not about what comes back.
func callTool(t *testing.T, s *server.MCPServer, name string, args map[string]any) {
	t.Helper()
	argsJSON, _ := json.Marshal(args)
	msg := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
		name, argsJSON,
	)
	resp := s.HandleMessage(context.Background(), []byte(msg))
	if resp == nil {
		t.Errorf("HandleMessage returned nil for tool %q", name)
	}
}

// toolResult is the part of a tools/call response a test asserts on.
type toolResult struct {
	IsError bool
	Text    string
}

// callToolResult dispatches a tool call and returns the parsed result, so a
// test can assert on the message a rejected call produces. It parses into a
// local struct rather than mcp.CallToolResult, whose Content is an interface
// that will not unmarshal.
func callToolResult(t *testing.T, s *server.MCPServer, name string, args map[string]any) toolResult {
	t.Helper()
	argsJSON, _ := json.Marshal(args)
	msg := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
		name, argsJSON,
	)
	raw := s.HandleMessage(context.Background(), []byte(msg))
	if raw == nil {
		t.Fatalf("HandleMessage returned nil for tool %q", name)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal tools/call response: %v", err)
	}
	var envelope struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		t.Fatalf("unmarshal tools/call response: %v", err)
	}
	var texts []string
	for _, c := range envelope.Result.Content {
		texts = append(texts, c.Text)
	}
	return toolResult{IsError: envelope.Result.IsError, Text: strings.Join(texts, "\n")}
}

// ── Registration smoke tests ──────────────────────────────────────────────────

func TestRegisterTools_Smoke(t *testing.T) {
	s := server.NewMCPServer("test", "0.0.1")
	RegisterTools(s, &fakeSender{})
}

// ── makeHandler ───────────────────────────────────────────────────────────────

func TestMakeHandler_SenderError(t *testing.T) {
	handler := makeHandler(&fakeSender{err: errors.New("plugin not connected")}, "get_document", nil, nil)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when the sender fails")
	}
}

// ── Read – no-param tools (all use makeHandler) ───────────────────────────────

func TestHandlers_NoParamReadTools(t *testing.T) {
	s, _ := newTestServer(t)
	noParamTools := []string{
		"get_document", "get_metadata", "get_selection",
		"get_viewport", "get_fonts", "get_styles", "get_variable_defs",
		"get_local_components", "get_annotations",
	}
	for _, name := range noParamTools {
		callTool(t, s, name, nil)
	}
}

// ── Read – param tools ────────────────────────────────────────────────────────

func TestHandlers_GetNodesInfo(t *testing.T) {
	s, _ := newTestServer(t)
	// One node and several: this absorbed get_node, so the single-node call has
	// to keep working through the plural argument.
	callTool(t, s, "get_nodes_info", map[string]any{"nodeIds": []any{"1:1"}})
	callTool(t, s, "get_nodes_info", map[string]any{"nodeIds": []string{"1:1", "2:2"}})
}

func TestHandlers_GetDesignContext(t *testing.T) {
	s, _ := newTestServer(t)
	// with all optional params
	callTool(t, s, "get_design_context", map[string]any{
		"depth": float64(2), "detail": "compact", "dedupe_components": true,
	})
	// with no params (defaults)
	callTool(t, s, "get_design_context", nil)
	// depth = 0 should be ignored (not passed through)
	callTool(t, s, "get_design_context", map[string]any{"depth": float64(0)})
}

func TestHandlers_SearchNodes(t *testing.T) {
	s, _ := newTestServer(t)
	// all optional params present
	callTool(t, s, "search_nodes", map[string]any{
		"query":  "button",
		"nodeId": "1:1",
		"types":  []any{"TEXT", "FRAME"},
		"limit":  float64(25),
	})
	// minimal (query only)
	callTool(t, s, "search_nodes", map[string]any{"query": "icon"})
	// the two scans it absorbed
	callTool(t, s, "search_nodes", map[string]any{
		"nodeId": "1:1", "types": []any{"FRAME", "COMPONENT"}, "includeHidden": false,
	})
	callTool(t, s, "search_nodes", map[string]any{
		"nodeId": "1:1", "types": []any{"TEXT"}, "includeText": true,
	})
}

func TestHandlers_GetReactions(t *testing.T) {
	s, _ := newTestServer(t)
	callTool(t, s, "get_reactions", map[string]any{"nodeId": "1:1"})
}

// ── Read – export tools ───────────────────────────────────────────────────────

func TestHandlers_GetScreenshot(t *testing.T) {
	s, _ := newTestServer(t)
	// with format + scale
	callTool(t, s, "get_screenshot", map[string]any{
		"nodeIds": []any{"1:1"},
		"format":  "PNG",
		"scale":   float64(2),
	})
	// no params (exports current selection)
	callTool(t, s, "get_screenshot", nil)
}

// TestHandlers_SaveScreenshots exercises executeSaveScreenshots +
// saveScreenshotItem. The fake sender returns no export data, so each item ends
// up an error inside the result JSON rather than a panic.
func TestHandlers_SaveScreenshots(t *testing.T) {
	s, _ := newTestServer(t)

	// single item – reaches saveScreenshotItem → node.Send fails → error result
	callTool(t, s, "save_screenshots", map[string]any{
		"items": []any{
			map[string]any{"nodeId": "1:1", "outputPath": "out/screen.png"},
		},
	})

	// multiple items with default format + scale
	callTool(t, s, "save_screenshots", map[string]any{
		"format": "SVG",
		"scale":  float64(1),
		"items": []any{
			map[string]any{"nodeId": "1:1", "outputPath": "out/a.svg"},
			map[string]any{"nodeId": "2:2", "outputPath": "out/b.svg", "format": "PNG"},
		},
	})

	// item with explicit per-item format + scale
	callTool(t, s, "save_screenshots", map[string]any{
		"items": []any{
			map[string]any{"nodeId": "3:3", "outputPath": "out/c.jpg", "format": "JPG", "scale": float64(2)},
		},
	})
}

// ── Write – create tools ──────────────────────────────────────────────────────

func TestHandlers_WriteCreateTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "create_frame", map[string]any{
		"width": float64(100), "height": float64(100), "name": "Card",
		"layoutMode": "VERTICAL", "parentId": "1:1",
	})
	callTool(t, s, "create_frame", map[string]any{}) // minimal

	callTool(t, s, "create_rectangle", map[string]any{"fillColor": "#FF5733", "cornerRadius": float64(8)})
	callTool(t, s, "create_rectangle", map[string]any{})

	callTool(t, s, "create_ellipse", map[string]any{"width": float64(50), "height": float64(50), "startAngle": float64(0), "endAngle": float64(180), "innerRadiusRatio": float64(0.5)})
	callTool(t, s, "create_ellipse", map[string]any{})

	callTool(t, s, "create_star", map[string]any{"pointCount": float64(5), "outerRadius": float64(50), "innerRadius": float64(20), "fillColor": "#FFD700"})
	callTool(t, s, "create_star", map[string]any{})

	callTool(t, s, "create_polygon", map[string]any{"pointCount": float64(6), "radius": float64(40), "fillColor": "#FF00FF"})
	callTool(t, s, "create_polygon", map[string]any{})

	callTool(t, s, "create_line", map[string]any{"length": float64(100), "rotation": float64(45), "strokeColor": "#000000", "strokeWeight": float64(2)})
	callTool(t, s, "create_line", map[string]any{})

	callTool(t, s, "create_text", map[string]any{
		"text": "Hello", "fontSize": float64(16), "fontFamily": "Inter", "fontStyle": "Bold",
		"fillColor": "#000000", "name": "Label",
	})

	// import_image with optional params
	callTool(t, s, "import_image", map[string]any{
		"imageData": "abc123", "x": float64(10), "y": float64(20),
		"width": float64(200), "height": float64(150),
		"name": "Hero", "scaleMode": "FILL", "parentId": "1:1",
	})
	// import_image minimal
	callTool(t, s, "import_image", map[string]any{"imageData": "abc123"})
}

// ── Write – modify tools ──────────────────────────────────────────────────────

func TestHandlers_WriteModifyTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "set_text", map[string]any{"nodeId": "1:1", "text": "Updated"})

	callTool(t, s, "set_fills", map[string]any{
		"nodeId": "1:1", "color": "#FF0000", "opacity": float64(0.8), "mode": "replace",
	})
	callTool(t, s, "set_fills", map[string]any{"nodeId": "1:1", "color": "#00FF00"}) // minimal

	callTool(t, s, "set_strokes", map[string]any{
		"nodeId": "1:1", "color": "#000000", "strokeWeight": float64(2), "mode": "append",
	})
	callTool(t, s, "set_strokes", map[string]any{"nodeId": "1:1", "color": "#000000"}) // minimal

	callTool(t, s, "move_nodes", map[string]any{"nodeIds": []any{"1:1"}, "x": float64(10), "y": float64(20)})
	callTool(t, s, "move_nodes", map[string]any{"nodeIds": []any{"1:1"}, "x": float64(5)}) // y omitted

	callTool(t, s, "resize_nodes", map[string]any{"nodeIds": []any{"1:1"}, "width": float64(300), "height": float64(200)})
	callTool(t, s, "resize_nodes", map[string]any{"nodeIds": []any{"1:1"}, "height": float64(100)}) // width omitted

	callTool(t, s, "rename_node", map[string]any{"nodeId": "1:1", "name": "New Name"})

	callTool(t, s, "clone_node", map[string]any{"nodeId": "1:1", "x": float64(50), "y": float64(50), "parentId": "2:2"})
	callTool(t, s, "clone_node", map[string]any{"nodeId": "1:1"}) // minimal

	callTool(t, s, "set_auto_layout", map[string]any{"nodeIds": []any{"1:1"}, "layoutMode": "HORIZONTAL"})
	callTool(t, s, "set_auto_layout", map[string]any{"nodeIds": []any{"1:1", "2:2"}, "layoutSizingHorizontal": "FILL"})

	callTool(t, s, "delete_nodes", map[string]any{"nodeIds": []any{"1:1", "2:2"}})
}

// ── Write – style tools ───────────────────────────────────────────────────────

func TestHandlers_WriteStyleTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "create_paint_style", map[string]any{"name": "Brand/Primary", "color": "#FF5733", "description": "Main brand color"})
	callTool(t, s, "create_text_style", map[string]any{"name": "Heading/H1"})
	callTool(t, s, "create_effect_style", map[string]any{"name": "Elevation/1", "type": "DROP_SHADOW"})
	callTool(t, s, "create_grid_style", map[string]any{"name": "Layout/12col", "pattern": "COLUMNS", "alignment": "STRETCH"})

	callTool(t, s, "update_paint_style", map[string]any{"styleId": "S:abc", "color": "#00FF00"})
	callTool(t, s, "update_paint_style", map[string]any{"styleId": "S:abc", "name": "Renamed"})

	callTool(t, s, "delete_style", map[string]any{"styleId": "S:abc"})
}

// ── Write – variable tools ────────────────────────────────────────────────────

func TestHandlers_WriteVariableTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "create_variable_collection", map[string]any{"name": "Brand", "initialModeName": "Light"})
	callTool(t, s, "add_variable_mode", map[string]any{"collectionId": "c1", "modeName": "Dark"})
	callTool(t, s, "create_variable", map[string]any{"name": "primary", "collectionId": "c1", "type": "COLOR"})
	callTool(t, s, "set_variable_value", map[string]any{"variableId": "v1", "modeId": "m1", "value": "#fff"})
	callTool(t, s, "delete_variable", map[string]any{"variableId": "v1"})
	callTool(t, s, "delete_variable", map[string]any{"collectionId": "c1"})
}

// ── Write – component tools ───────────────────────────────────────────────────

func TestHandlers_WriteComponentTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "swap_component", map[string]any{"nodeId": "1:1", "componentId": "2:2"})
	callTool(t, s, "detach_instance", map[string]any{"nodeIds": []any{"1:1", "2:2"}})
}

// ── Write – linked tools (apply_style_to_node, bind_variable_to_node) ─────────

func TestHandlers_LinkedTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "apply_style_to_node", map[string]any{"nodeId": "1:1", "styleId": "S:abc", "target": "fill"})
	callTool(t, s, "apply_style_to_node", map[string]any{"nodeId": "1:1", "styleId": "S:abc"}) // no target

	callTool(t, s, "bind_variable_to_node", map[string]any{"nodeId": "1:1", "variableId": "v1", "field": "fills"})
}

func TestHandlers_NodeControlTools(t *testing.T) {
	s, _ := newTestServer(t)

	// set_visible — show
	callTool(t, s, "set_visible", map[string]any{"nodeIds": []any{"1:1"}, "visible": true})
	// set_visible — hide
	callTool(t, s, "set_visible", map[string]any{"nodeIds": []any{"1:1", "2:2"}, "visible": false})

	// lock_nodes
	callTool(t, s, "lock_nodes", map[string]any{"nodeIds": []any{"1:1"}})
	callTool(t, s, "lock_nodes", map[string]any{"nodeIds": []any{"1:1", "2:2"}})

	// unlock_nodes
	callTool(t, s, "unlock_nodes", map[string]any{"nodeIds": []any{"1:1"}})

	// rotate_nodes
	callTool(t, s, "rotate_nodes", map[string]any{"nodeIds": []any{"1:1"}, "rotation": float64(45)})
	callTool(t, s, "rotate_nodes", map[string]any{"nodeIds": []any{"1:1"}, "rotation": float64(-90)})

	// reorder_nodes
	callTool(t, s, "reorder_nodes", map[string]any{"nodeIds": []any{"1:1"}, "order": "bringToFront"})
	callTool(t, s, "reorder_nodes", map[string]any{"nodeIds": []any{"1:1"}, "order": "sendToBack"})
	callTool(t, s, "reorder_nodes", map[string]any{"nodeIds": []any{"1:1"}, "order": "bringForward"})
	callTool(t, s, "reorder_nodes", map[string]any{"nodeIds": []any{"1:1"}, "order": "sendBackward"})

	// set_blend_mode
	callTool(t, s, "set_blend_mode", map[string]any{"nodeIds": []any{"1:1"}, "blendMode": "MULTIPLY"})
	callTool(t, s, "set_blend_mode", map[string]any{"nodeIds": []any{"1:1", "2:2"}, "blendMode": "SCREEN"})

	// set_constraints
	callTool(t, s, "set_constraints", map[string]any{"nodeIds": []any{"1:1"}, "horizontal": "STRETCH"})
	callTool(t, s, "set_constraints", map[string]any{"nodeIds": []any{"1:1"}, "vertical": "CENTER"})
	callTool(t, s, "set_constraints", map[string]any{"nodeIds": []any{"1:1"}, "horizontal": "MIN", "vertical": "MAX"})
}

// ── Write – page management tools ───────────────────────────────────

func TestHandlers_PageManagementTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "manage_page", map[string]any{"action": "add", "name": "Flows"})
	callTool(t, s, "manage_page", map[string]any{"action": "add"}) // minimal
	callTool(t, s, "manage_page", map[string]any{"action": "add", "name": "Sprint 1", "index": float64(0)})

	callTool(t, s, "manage_page", map[string]any{"action": "delete", "pageId": "0:2"})
	callTool(t, s, "manage_page", map[string]any{"action": "delete", "pageName": "Flows"})

	callTool(t, s, "manage_page", map[string]any{"action": "rename", "pageId": "0:2", "newName": "Sprint 1"})
	callTool(t, s, "manage_page", map[string]any{"action": "rename", "pageName": "Flows", "newName": "User Flows"})

	callTool(t, s, "manage_page", map[string]any{"action": "navigate", "pageId": "0:2"})
	callTool(t, s, "manage_page", map[string]any{"action": "navigate", "pageName": "Flows"})
}

func TestHandlers_ReparentBatchRenameTextReplaceEffectsSection(t *testing.T) {
	s, _ := newTestServer(t)

	// reparent_nodes
	callTool(t, s, "reparent_nodes", map[string]any{"nodeIds": []any{"1:1"}, "parentId": "2:2"})
	callTool(t, s, "reparent_nodes", map[string]any{"nodeIds": []any{"1:1", "3:3"}, "parentId": "2:2"})

	// batch_rename_nodes — find/replace
	callTool(t, s, "batch_rename_nodes", map[string]any{
		"nodeIds": []any{"1:1", "2:2"}, "find": "Button", "replace": "Btn",
	})
	// batch_rename_nodes — prefix/suffix
	callTool(t, s, "batch_rename_nodes", map[string]any{
		"nodeIds": []any{"1:1"}, "prefix": "UI/", "suffix": "_v2",
	})
	// batch_rename_nodes — regex
	callTool(t, s, "batch_rename_nodes", map[string]any{
		"nodeIds": []any{"1:1"}, "find": "\\d+", "replace": "N", "useRegex": true,
	})

	// find_replace_text — across page
	callTool(t, s, "find_replace_text", map[string]any{"find": "Old", "replace": "New"})
	// find_replace_text — scoped to node
	callTool(t, s, "find_replace_text", map[string]any{"find": "x", "replace": "y", "nodeId": "1:1"})
	// find_replace_text — regex
	callTool(t, s, "find_replace_text", map[string]any{
		"find": "\\$\\d+", "replace": "$0", "useRegex": true,
	})

	// set_effects — drop shadow
	callTool(t, s, "set_effects", map[string]any{
		"nodeId":  "1:1",
		"effects": []any{map[string]any{"type": "DROP_SHADOW", "radius": float64(8), "color": "#000000", "opacity": float64(0.3)}},
	})
	// set_effects — layer blur
	callTool(t, s, "set_effects", map[string]any{
		"nodeId":  "1:1",
		"effects": []any{map[string]any{"type": "LAYER_BLUR", "radius": float64(4)}},
	})
	// set_effects — clear
	callTool(t, s, "set_effects", map[string]any{"nodeId": "1:1", "effects": []any{}})

	// create_section
	callTool(t, s, "create_section", map[string]any{"name": "Sprint 1", "x": float64(0), "y": float64(0)})
	callTool(t, s, "create_section", map[string]any{}) // minimal
	callTool(t, s, "create_section", map[string]any{"width": float64(1200), "height": float64(900)})
}

// TestToolCall_InvalidArgsRejected proves validation is reachable from the MCP
// entry point, not just from Node.Send. Before validation moved into Node.Send
// a leader process forwarded these straight to Figma.
func TestToolCall_InvalidArgsRejected(t *testing.T) {
	s, _ := newTestServer(t)

	cases := []struct {
		tool    string
		args    map[string]any
		wantMsg string
	}{
		{"set_node_properties", map[string]any{"nodeIds": []any{"1:1"}, "opacity": 5.0}, "opacity must be at most 1"},
		{"set_node_properties", map[string]any{"nodeIds": []any{"1:1"}, "blendMode": "NEON"}, "blendMode must be one of"},
		{"set_node_properties", map[string]any{"nodeIds": []any{"1:1"}, "order": "sideways"}, "order must be"},
		{"resize_nodes", map[string]any{"nodeIds": []any{"nope"}, "width": 10.0}, "colon format"},
		{"search_nodes", map[string]any{"query": ""}, "at least one of query or types is required"},
	}

	for _, c := range cases {
		t.Run(c.tool+"/"+c.wantMsg, func(t *testing.T) {
			argsJSON, _ := json.Marshal(c.args)
			msg := fmt.Sprintf(
				`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
				c.tool, argsJSON,
			)
			raw := s.HandleMessage(context.Background(), []byte(msg))
			b, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("marshal response: %v", err)
			}
			if !strings.Contains(string(b), c.wantMsg) {
				t.Errorf("response for %s did not carry the validation error %q:\n%s", c.tool, c.wantMsg, b)
			}
		})
	}
}

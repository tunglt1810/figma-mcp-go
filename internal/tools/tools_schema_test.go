package tools

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"sort"
	"testing"
)

// toolsListResponse mirrors the subset of the MCP tools/list JSON-RPC response
// that we need to inspect for schema correctness.
type toolsListResponse struct {
	Result struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Properties map[string]propertySchema `json:"properties"`
			} `json:"inputSchema"`
		} `json:"tools"`
	} `json:"result"`
}

type propertySchema struct {
	Type  string         `json:"type"`
	Items jsontext.Value `json:"items"`
}

// listTools calls tools/list through the server's HandleMessage path and returns
// the parsed response.
func listTools(t *testing.T) toolsListResponse {
	t.Helper()
	s, _ := newTestServer(t)
	msg := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	raw := s.HandleMessage(context.Background(), []byte(msg))
	if raw == nil {
		t.Fatal("HandleMessage returned nil for tools/list")
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal tools/list response: %v", err)
	}
	var resp toolsListResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal tools/list response: %v", err)
	}
	return resp
}

// TestToolSchemas_ArrayItemsHaveType ensures every array-typed parameter across
// all registered tools declares an items.type.  Missing items (or items without
// a type) is the exact class of bug that causes GitHub Copilot MCP validation to
// fail (see commit af0325c).
func TestToolSchemas_ArrayItemsHaveType(t *testing.T) {
	resp := listTools(t)

	if len(resp.Result.Tools) == 0 {
		t.Fatal("tools/list returned no tools — registration may have failed")
	}

	type violation struct {
		tool, param, reason string
	}
	var violations []violation

	for _, tool := range resp.Result.Tools {
		for param, prop := range tool.InputSchema.Properties {
			if prop.Type != "array" || tool.Name == "set_annotations" {
				continue
			}

			if len(prop.Items) == 0 || string(prop.Items) == "null" {
				violations = append(violations, violation{
					tool:   tool.Name,
					param:  param,
					reason: "items is missing",
				})
				continue
			}

			var items map[string]any
			if err := json.Unmarshal(prop.Items, &items); err != nil {
				violations = append(violations, violation{
					tool:   tool.Name,
					param:  param,
					reason: fmt.Sprintf("items is not a valid JSON object: %v", err),
				})
				continue
			}

			if _, ok := items["type"]; !ok {
				violations = append(violations, violation{
					tool:   tool.Name,
					param:  param,
					reason: "items.type is missing",
				})
			}
		}
	}

	for _, v := range violations {
		t.Errorf("tool %q param %q: %s", v.tool, v.param, v.reason)
	}
}

// expectedTools is the exact set of tools the server advertises, sorted.
// Changing the tool surface is a breaking change for every MCP client, so it
// must be a deliberate edit here rather than a silently drifting count.
var expectedTools = []string{
	"add_variable_mode",
	"apply_style_to_node",
	"batch_execute_pipeline",
	"batch_rename_nodes",
	"bind_variable_to_node",
	"boolean_operation",
	"clone_node",
	"combine_as_variants",
	"create_component",
	"create_component_instance",
	"create_connector",
	"create_node",
	"create_style",
	"create_text",
	"create_variable",
	"create_variable_collection",
	"create_vector",
	"delete_nodes",
	"delete_style",
	"delete_variable",
	"detach_instance",
	"export_frames_to_pdf",
	"export_tokens",
	"find_replace_text",
	"flatten_nodes",
	"get_annotations",
	"get_document",
	"get_fonts",
	"get_image_bytes",
	"get_instance_overrides",
	"get_local_components",
	"get_metadata",
	"get_nodes_info",
	"get_reactions",
	"get_screenshot",
	"get_selection",
	"get_styles",
	"get_variable_defs",
	"get_viewport",
	"group_nodes",
	"import_image",
	"manage_component_properties",
	"manage_page",
	"manage_plugin_data",
	"outline_stroke",
	"reparent_nodes",
	"save_screenshots",
	"save_version_checkpoint",
	"search_nodes",
	"set_annotations",
	"set_layout_grids",
	"set_node_properties",
	"set_paint",
	"set_auto_layout",
	"set_codegen_result",
	"set_effects",
	"set_export_settings",
	"set_instance_overrides",
	"set_reactions",
	"set_selection",
	"set_text",
	"set_text_ranges",
	"set_variable_value",
	"swap_component",
	"ungroup_nodes",
	"update_paint_style",
}

// TestToolSchemas_ExpectedToolSet pins the advertised tool names. A count alone
// hides a rename or a swap; comparing names reports exactly what moved.
func TestToolSchemas_ExpectedToolSet(t *testing.T) {
	resp := listTools(t)

	got := make([]string, 0, len(resp.Result.Tools))
	for _, tool := range resp.Result.Tools {
		got = append(got, tool.Name)
	}
	sort.Strings(got)

	want := make(map[string]bool, len(expectedTools))
	for _, name := range expectedTools {
		want[name] = true
	}
	have := make(map[string]bool, len(got))
	for _, name := range got {
		have[name] = true
	}

	for _, name := range expectedTools {
		if !have[name] {
			t.Errorf("tool %q is expected but not registered", name)
		}
	}
	for _, name := range got {
		if !want[name] {
			t.Errorf("tool %q is registered but not in expectedTools — add it there if intended", name)
		}
	}
	if len(got) != len(expectedTools) {
		t.Errorf("registered %d tools, expected %d", len(got), len(expectedTools))
	}
}

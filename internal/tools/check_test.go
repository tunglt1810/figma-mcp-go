package tools

import (
	"reflect"
	"strings"
	"testing"
)

// Check is the one place where both entry points (an MCP tool call and a
// follower's /rpc post) agree on what a valid call is. Normalization must come
// first: the hyphen format LLMs produce must be accepted, not rejected by the
// validation that exists to accept it.

func TestCheck_NormalizesBeforeValidating(t *testing.T) {
	ids, params, err := Check("set_text", []string{"4029-12345"}, map[string]any{"text": "hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "4029:12345" {
		t.Errorf("node ID not normalised: %v", ids)
	}
	if params["text"] != "hi" {
		t.Errorf("params mangled: %v", params)
	}
}

func TestCheck_RejectsInvalidArguments(t *testing.T) {
	_, _, err := Check("set_node_properties", []string{"1:1"}, map[string]any{"opacity": 5.0})
	if err == nil {
		t.Fatal("expected an error for opacity 5.0")
	}
	if !strings.Contains(err.Error(), "opacity must be at most 1") {
		t.Errorf("unexpected message: %v", err)
	}
}

// A name with no spec has no rules, so letting it through would send
// unchecked arguments to whatever plugin handler has that name.
func TestCheck_RefusesUnknownTool(t *testing.T) {
	for _, tool := range []string{"not_a_tool", "create_frame", "set_fills"} {
		if _, _, err := Check(tool, nil, nil); err == nil {
			t.Errorf("Check(%q) = nil error, want refusal", tool)
		}
	}
}

// export_screenshots reaches the plugin as get_screenshot. A follower sends
// that name to the leader's /rpc, so both guards must know it.
func TestCheck_AcceptsTheInternalScreenshotCall(t *testing.T) {
	params := map[string]any{"format": "PNG", "scale": 1.0}
	if _, _, err := Check("get_screenshot", []string{"1:2"}, params); err != nil {
		t.Errorf("Check: %v", err)
	}
	if _, _, err := CheckReadOnly("get_screenshot", []string{"1:2"}, params); err != nil {
		t.Errorf("CheckReadOnly: %v", err)
	}
}

func TestCheck_DoesNotMutateCallerArguments(t *testing.T) {
	ids := []string{"4029-12345"}
	if _, _, err := Check("set_text", ids, map[string]any{"text": "hi"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ids[0] != "4029-12345" {
		t.Error("Check mutated the caller's slice")
	}
}

// Node IDs also appear inside params, not only in the dedicated field.
func TestCheck_NormalizesIDsInParams(t *testing.T) {
	_, params, err := Check("clone_node", []string{"1-1"}, map[string]any{
		"nodeId":   "100-200",
		"parentId": "300-400",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["nodeId"] != "100:200" {
		t.Errorf("params.nodeId = %v, want 100:200", params["nodeId"])
	}
	if params["parentId"] != "300:400" {
		t.Errorf("params.parentId = %v, want 300:400", params["parentId"])
	}
}

// A pipeline step has its own parameter set, one level deeper than the
// top-level pass reached. So hyphen IDs inside a pipeline reached the plugin
// unconverted, and the step could not find its node.
func TestCheck_NormalizesIDsInsidePipelineSteps(t *testing.T) {
	steps := []any{
		map[string]any{
			"action": "clone_node",
			"params": map[string]any{
				"nodeId":   "100-200",
				"parentId": "300-400",
			},
		},
		map[string]any{
			"action": "delete_nodes",
			"params": map[string]any{
				"nodeIds": []any{"1-1", "2-2"},
			},
		},
	}

	_, params, err := Check("batch_execute_pipeline", nil, map[string]any{"steps": steps})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sent, _ := params["steps"].([]any)
	if len(sent) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(sent))
	}

	first, _ := sent[0].(map[string]any)["params"].(map[string]any)
	if first["nodeId"] != "100:200" {
		t.Errorf("steps[0].params.nodeId = %v, want 100:200", first["nodeId"])
	}
	if first["parentId"] != "300:400" {
		t.Errorf("steps[0].params.parentId = %v, want 300:400", first["parentId"])
	}

	second, _ := sent[1].(map[string]any)["params"].(map[string]any)
	ids, _ := second["nodeIds"].([]any)
	if len(ids) != 2 || ids[0] != "1:1" || ids[1] != "2:2" {
		t.Errorf("steps[1].params.nodeIds = %v, want [1:1 2:2]", ids)
	}
}

// Normalizing must copy, not edit: the nested maps belong to the caller too
// (P2-12).
func TestCheck_DoesNotMutateNestedCallerArgs(t *testing.T) {
	inner := map[string]any{"nodeId": "100-200"}
	steps := []any{map[string]any{"action": "clone_node", "params": inner}}

	if _, _, err := Check("batch_execute_pipeline", nil, map[string]any{"steps": steps}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner["nodeId"] != "100-200" {
		t.Errorf("caller's nested map was mutated: nodeId = %v", inner["nodeId"])
	}
}

// Read-only mode must fail closed: a name with no spec could still reach a
// plugin write handler, such as the internal create_frame.
func TestCheckReadOnly_RejectsUnknownAndWriteTools(t *testing.T) {
	for _, tool := range []string{"create_frame", "set_fills", "batch_execute_pipeline"} {
		if _, _, err := CheckReadOnly(tool, nil, nil); err == nil {
			t.Errorf("CheckReadOnly(%q) = nil error, want rejection", tool)
		}
	}
	if _, _, err := CheckReadOnly("get_selection", nil, nil); err != nil {
		t.Errorf("CheckReadOnly(get_selection) = %v, want nil", err)
	}
}

// A step reaches the plugin as the same direct call would: null, empty and
// undeclared arguments are gone. A $variable stays as written, because the
// plugin replaces it at run time.
func TestCheck_PipelineStepSendsWhatADirectCallSends(t *testing.T) {
	move := map[string]any{"id": "move", "action": "set_node_properties",
		"export_vars": map[string]any{"id": "$moved"},
		"params": map[string]any{"nodeId": "1-2", "x": nil, "y": 10.0, "blendMode": "",
			"note": "x", "rotation": "$angle"}}
	remove := map[string]any{"action": "delete_nodes",
		"params": map[string]any{"nodeIds": []any{"$frame"}}}

	_, params, err := Check(pipelineTool, nil, map[string]any{"steps": []any{move, remove}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	steps := params["steps"].([]any)
	want := []any{
		map[string]any{"id": "move", "action": "set_node_properties",
			"export_vars": map[string]any{"id": "$moved"},
			"params":      map[string]any{"nodeId": "1:2", "y": 10.0, "rotation": "$angle"}},
		map[string]any{"action": "delete_nodes",
			"params": map[string]any{"nodeIds": []any{"$frame"}}},
	}
	if !reflect.DeepEqual(steps, want) {
		t.Errorf("steps = %#v\nwant    %#v", steps, want)
	}

	// A follower's call is checked twice: by its own handler, then by the
	// leader's /rpc guard. The second check must not change the steps again.
	if _, again, err := Check(pipelineTool, nil, params); err != nil || !reflect.DeepEqual(again, params) {
		t.Errorf("a second Check gave %#v, %v", again, err)
	}
	if _, kept := move["params"].(map[string]any)["x"]; !kept {
		t.Error("Check changed the caller's step")
	}
}

// A step's params go to the plugin under the names the plugin reads, and
// validateStep reads them under those names too. A write tool with a Wire name
// would need a rename for each step. None has one, so steps are not renamed.
func TestPipelineSteps_NeedNoWireRename(t *testing.T) {
	for _, spec := range allSpecs() {
		if spec.ReadOnly || spec.Custom != nil || spec.Name == pipelineTool {
			continue
		}
		for _, p := range spec.Params {
			if p.Wire != "" {
				t.Errorf("%s.%s has Wire %q. Rename it in the pipeline steps too", spec.Name, p.Name, p.Wire)
			}
		}
	}
}

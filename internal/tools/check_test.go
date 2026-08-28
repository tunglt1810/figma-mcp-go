package tools

import (
	"strings"
	"testing"
)

// Check is the one place both entry points — an MCP tool call and a follower's
// /rpc post — agree on what a valid call looks like. Normalisation has to come
// first: the hyphen format LLMs emit must be accepted, not rejected by the very
// validation that exists to tolerate it.

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

func TestCheck_UnknownToolIsNotRejected(t *testing.T) {
	// A tool with no spec has no rules to break. ValidateRPC has always
	// returned "" for one, and Check must not turn that into an error.
	if _, _, err := Check("not_a_tool", nil, nil); err != nil {
		t.Errorf("unknown tool should pass through, got %v", err)
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

// Node IDs travel inside params too, not only in the dedicated field.
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

// A pipeline step carries a whole parameter set of its own, one level below
// anything the top-level pass reached — so hyphen IDs inside a pipeline went
// to the plugin unconverted and the step failed to find its node.
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

// Normalizing must copy rather than edit: the nested maps belong to the caller
// too (P2-12).
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

package tools

import (
	"testing"
)

// autoLayoutParamNames controls which arguments create_node accepts for a
// FRAME. If a parameter is added to autoLayoutParams but missed here, the
// caller sees no clear error. It is rejected as "not allowed for this shape",
// which looks as if the argument does not exist.
func TestAutoLayoutParamNamesCoverTheSpecs(t *testing.T) {
	listed := make(map[string]bool, len(autoLayoutParamNames))
	for _, name := range autoLayoutParamNames {
		listed[name] = true
	}

	for _, spec := range autoLayoutParams() {
		if !listed[spec.Name] {
			t.Errorf("autoLayoutParams has %q but autoLayoutParamNames does not — create_node would reject it for a FRAME", spec.Name)
		}
		delete(listed, spec.Name)
	}

	for name := range listed {
		t.Errorf("autoLayoutParamNames has %q with no matching entry in autoLayoutParams", name)
	}
}

func TestSetAutoLayout_AcceptsLayoutSizing(t *testing.T) {
	for _, value := range []string{"FIXED", "HUG", "FILL"} {
		params := map[string]any{"layoutSizingHorizontal": value}
		if msg := ValidateRPC("set_auto_layout", []string{"1:2"}, params); msg != "" {
			t.Errorf("layoutSizingHorizontal=%s rejected: %s", value, msg)
		}
	}
	params := map[string]any{"layoutSizingHorizontal": "STRETCH"}
	if msg := ValidateRPC("set_auto_layout", []string{"1:2"}, params); msg == "" {
		t.Error("expected an unknown layoutSizing value to be rejected")
	}
}

// A min/max constraint is cleared by passing null, so null must survive both
// the argument extraction and the validation after it.
func TestSetAutoLayout_NullClearsAConstraint(t *testing.T) {
	spec, ok := specRegistry["set_auto_layout"]
	if !ok {
		t.Fatal("set_auto_layout spec not found")
	}

	_, params := specArgs(spec, map[string]any{
		"nodeIds":  []any{"1:2"},
		"maxWidth": nil,
	})
	value, present := params["maxWidth"]
	if !present {
		t.Fatal("an explicit null was dropped — the plugin cannot tell it from an absent argument")
	}
	if value != nil {
		t.Errorf("maxWidth = %v, want nil", value)
	}

	if msg := ValidateRPC("set_auto_layout", []string{"1:2"}, params); msg != "" {
		t.Errorf("a null constraint was rejected: %s", msg)
	}
}

// Nullable only covers null. A value of the wrong type is still an error.
func TestSetAutoLayout_RejectsANonNumericConstraint(t *testing.T) {
	params := map[string]any{"maxWidth": "wide"}
	if msg := ValidateRPC("set_auto_layout", []string{"1:2"}, params); msg == "" {
		t.Error("expected a string maxWidth to be rejected")
	}
}

// A parameter that is not Nullable keeps the old behaviour: null counts as absent.
func TestSpecArgs_DropsNullForANonNullableParam(t *testing.T) {
	spec, ok := specRegistry["set_auto_layout"]
	if !ok {
		t.Fatal("set_auto_layout spec not found")
	}
	_, params := specArgs(spec, map[string]any{
		"nodeIds":     []any{"1:2"},
		"itemSpacing": nil,
	})
	if _, present := params["itemSpacing"]; present {
		t.Error("a null on a non-nullable parameter should be dropped, not forwarded")
	}
}

// set_selection takes no nodes only when it clears the selection, which needs
// select on. With select off, the call would have nothing left to do.
func TestSetSelection_RequiresNodesWhenNotSelecting(t *testing.T) {
	if msg := ValidateRPC("set_selection", nil, map[string]any{"select": false}); msg == "" {
		t.Error("expected select:false with no nodes to be rejected")
	}
	if msg := ValidateRPC("set_selection", nil, nil); msg != "" {
		t.Errorf("clearing the selection should be allowed, got: %s", msg)
	}
	if msg := ValidateRPC("set_selection", []string{"1:2"}, map[string]any{"select": false, "zoom": true}); msg != "" {
		t.Errorf("focus without selecting should be allowed, got: %s", msg)
	}
}

// set_layout_sizing applied the same nine sizing arguments to several nodes.
// set_auto_layout took it over by accepting nodeIds, so setting one sizing on
// a row of siblings must still be a single call.
func TestSetAutoLayout_AcceptsSeveralNodes(t *testing.T) {
	params := map[string]any{"layoutSizingHorizontal": "FILL"}
	if msg := ValidateRPC("set_auto_layout", []string{"1:2", "1:3"}, params); msg != "" {
		t.Errorf("FILL across two nodes was rejected: %s", msg)
	}
	if msg := ValidateRPC("set_auto_layout", nil, params); msg == "" {
		t.Error("expected a call with no node ids to be rejected")
	}
}

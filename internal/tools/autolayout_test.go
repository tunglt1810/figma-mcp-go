package tools

import (
	"testing"
)

// autoLayoutParamNames gates which arguments create_node accepts for a FRAME.
// A parameter added to autoLayoutParams but missed here is not an error the
// caller can see — it is rejected as "not allowed for this shape", which reads
// as if the argument does not exist.
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

// A min/max constraint is cleared by passing null, so null has to survive both
// the argument extraction and the validation that follows it.
func TestSetAutoLayout_NullClearsAConstraint(t *testing.T) {
	spec, ok := specRegistry["set_auto_layout"]
	if !ok {
		t.Fatal("set_auto_layout spec not found")
	}

	_, params := specArgs(spec, map[string]any{
		"nodeId":   "1:2",
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

// Nullable is about null specifically; a wrong-typed value is still an error.
func TestSetAutoLayout_RejectsANonNumericConstraint(t *testing.T) {
	params := map[string]any{"maxWidth": "wide"}
	if msg := ValidateRPC("set_auto_layout", []string{"1:2"}, params); msg == "" {
		t.Error("expected a string maxWidth to be rejected")
	}
}

// A parameter that is not Nullable keeps the old behaviour: null reads as absent.
func TestSpecArgs_DropsNullForANonNullableParam(t *testing.T) {
	spec, ok := specRegistry["set_auto_layout"]
	if !ok {
		t.Fatal("set_auto_layout spec not found")
	}
	_, params := specArgs(spec, map[string]any{
		"nodeId":      "1:2",
		"itemSpacing": nil,
	})
	if _, present := params["itemSpacing"]; present {
		t.Error("a null on a non-nullable parameter should be dropped, not forwarded")
	}
}

// set_selection takes no nodes only when it is clearing the selection, which
// needs select on. With select off there would be nothing left for the call to do.
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

// set_layout_sizing derives its parameters from autoLayoutParams by name. A
// name with no matching entry there would reach the plugin with no schema
// behind it: no enum, no bounds, no description.
func TestLayoutSizingParamsComeFromAutoLayout(t *testing.T) {
	got := layoutSizingParams()
	if len(got) != len(layoutSizingParamNames) {
		t.Fatalf("layoutSizingParams returned %d specs for %d names — a name has no entry in autoLayoutParams", len(got), len(layoutSizingParamNames))
	}
	for i, name := range layoutSizingParamNames {
		if got[i].Name != name {
			t.Errorf("layoutSizingParams()[%d] = %q, want %q", i, got[i].Name, name)
		}
	}
}

// The tool is about how a node sizes itself, not about the layout it gives its
// own children. Accepting layoutMode or itemSpacing here would silently do
// nothing useful across a set of siblings.
func TestSetLayoutSizing_RejectsFrameLayoutParams(t *testing.T) {
	for _, name := range []string{"layoutMode", "itemSpacing", "paddingTop", "clipsContent"} {
		params := map[string]any{name: "HORIZONTAL"}
		if msg := ValidateRPC("set_layout_sizing", []string{"1:2"}, params); msg == "" {
			t.Errorf("expected %s to be rejected by set_layout_sizing", name)
		}
	}
}

func TestSetLayoutSizing_RequiresAtLeastOneProperty(t *testing.T) {
	if msg := ValidateRPC("set_layout_sizing", []string{"1:2"}, nil); msg == "" {
		t.Error("expected a call with no properties to be rejected")
	}
	if msg := ValidateRPC("set_layout_sizing", []string{"1:2", "1:3"}, map[string]any{"layoutSizingHorizontal": "FILL"}); msg != "" {
		t.Errorf("FILL across two nodes was rejected: %s", msg)
	}
}

// null clears a constraint here for the same reason it does on set_auto_layout.
func TestSetLayoutSizing_NullClearsAConstraint(t *testing.T) {
	spec, ok := specRegistry["set_layout_sizing"]
	if !ok {
		t.Fatal("set_layout_sizing spec not found")
	}
	_, params := specArgs(spec, map[string]any{
		"nodeIds":  []any{"1:2"},
		"maxWidth": nil,
	})
	if value, present := params["maxWidth"]; !present || value != nil {
		t.Errorf("maxWidth = %v (present %v), want an explicit nil", value, present)
	}
}

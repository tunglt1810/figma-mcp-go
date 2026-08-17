package internal

import (
	"strings"
	"testing"
)

// ── ValidNodeID ──────────────────────────────────────────────────────────────

func TestValidNodeID(t *testing.T) {
	valid := []string{
		"4029:12345",
		"0:1",
		"1:1",
		"I44:9;44:3",
		"I2167:9091;186:1579;186:1745",
	}
	for _, id := range valid {
		if !ValidNodeID(id) {
			t.Errorf("expected %q to be valid", id)
		}
	}

	invalid := []string{
		"",
		"4029-12345",
		"4029:12345:6789",
		"abc:def",
		"4029:",
		":12345",
		"4029",
	}
	for _, id := range invalid {
		if ValidNodeID(id) {
			t.Errorf("expected %q to be invalid", id)
		}
	}
}

// ── NormalizeNodeID ───────────────────────────────────────────────────────────

func TestNormalizeNodeID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"4029-12345", "4029:12345"},
		{"4029:12345", "4029:12345"},       // already valid, no-op
		{"not-a-node-id", "not-a-node-id"}, // hyphen but not a node ID
		{"", ""},
	}
	for _, c := range cases {
		got := NormalizeNodeID(c.input)
		if got != c.want {
			t.Errorf("NormalizeNodeID(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ── ValidateRPC ───────────────────────────────────────────────────────────────

func TestValidateRPC_GetNode(t *testing.T) {
	// missing nodeId
	if msg := ValidateRPC("get_node", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// hyphen format
	if msg := ValidateRPC("get_node", []string{"4029-12345"}, nil); msg == "" {
		t.Error("expected error for hyphen nodeId")
	}
	// valid
	if msg := ValidateRPC("get_node", []string{"4029:12345"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_GetNodesInfo(t *testing.T) {
	if msg := ValidateRPC("get_nodes_info", nil, nil); msg == "" {
		t.Error("expected error for empty nodeIds")
	}
	if msg := ValidateRPC("get_nodes_info", []string{"bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("get_nodes_info", []string{"1:1", "2:2"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_GetScreenshot(t *testing.T) {
	// invalid format
	msg := ValidateRPC("get_screenshot", []string{"1:1"}, map[string]interface{}{"format": "GIF"})
	if msg == "" {
		t.Error("expected error for invalid format")
	}
	// valid formats
	for _, f := range []string{"PNG", "SVG", "JPG", "PDF"} {
		msg := ValidateRPC("get_screenshot", []string{"1:1"}, map[string]interface{}{"format": f})
		if msg != "" {
			t.Errorf("unexpected error for format %s: %s", f, msg)
		}
	}
}

func TestValidateRPC_SaveScreenshots(t *testing.T) {
	// missing items
	if msg := ValidateRPC("save_screenshots", nil, nil); msg == "" {
		t.Error("expected error for missing items")
	}
	// empty items array
	msg := ValidateRPC("save_screenshots", nil, map[string]interface{}{
		"items": []interface{}{},
	})
	if msg == "" {
		t.Error("expected error for empty items")
	}
	// invalid nodeId in item
	msg = ValidateRPC("save_screenshots", nil, map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"nodeId": "bad", "outputPath": "out.png"},
		},
	})
	if msg == "" {
		t.Error("expected error for bad nodeId in item")
	}
	// missing outputPath
	msg = ValidateRPC("save_screenshots", nil, map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"nodeId": "1:1"},
		},
	})
	if msg == "" {
		t.Error("expected error for missing outputPath")
	}
	// valid
	msg = ValidateRPC("save_screenshots", nil, map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"nodeId": "1:1", "outputPath": "out.png"},
		},
	})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_GetDesignContext(t *testing.T) {
	// negative depth
	msg := ValidateRPC("get_design_context", nil, map[string]interface{}{"depth": float64(-1)})
	if msg == "" {
		t.Error("expected error for negative depth")
	}
	// invalid detail
	msg = ValidateRPC("get_design_context", nil, map[string]interface{}{"detail": "huge"})
	if msg == "" {
		t.Error("expected error for invalid detail")
	}
	// valid detail values
	for _, d := range []string{"minimal", "compact", "full"} {
		msg := ValidateRPC("get_design_context", nil, map[string]interface{}{"detail": d})
		if msg != "" {
			t.Errorf("unexpected error for detail %s: %s", d, msg)
		}
	}
}

func TestValidateRPC_SearchNodes(t *testing.T) {
	// missing query
	if msg := ValidateRPC("search_nodes", nil, nil); msg == "" {
		t.Error("expected error for missing query")
	}
	// invalid nodeId
	msg := ValidateRPC("search_nodes", nil, map[string]interface{}{
		"query":  "button",
		"nodeId": "bad",
	})
	if msg == "" {
		t.Error("expected error for bad nodeId")
	}
	// non-positive limit
	msg = ValidateRPC("search_nodes", nil, map[string]interface{}{
		"query": "button",
		"limit": float64(0),
	})
	if msg == "" {
		t.Error("expected error for zero limit")
	}
	// valid
	msg = ValidateRPC("search_nodes", nil, map[string]interface{}{"query": "button"})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_CreateFrame(t *testing.T) {
	// zero width
	msg := ValidateRPC("create_frame", nil, map[string]interface{}{"width": float64(0)})
	if msg == "" {
		t.Error("expected error for zero width")
	}
	// invalid layoutMode
	msg = ValidateRPC("create_frame", nil, map[string]interface{}{"layoutMode": "DIAGONAL"})
	if msg == "" {
		t.Error("expected error for invalid layoutMode")
	}
	// valid
	msg = ValidateRPC("create_frame", nil, map[string]interface{}{
		"width": float64(100), "height": float64(100), "layoutMode": "VERTICAL",
	})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetText(t *testing.T) {
	// missing nodeId
	if msg := ValidateRPC("set_text", nil, map[string]interface{}{"text": "hello"}); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// missing text
	if msg := ValidateRPC("set_text", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing text")
	}
	// valid
	msg := ValidateRPC("set_text", []string{"1:1"}, map[string]interface{}{"text": "hello"})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetFills(t *testing.T) {
	// missing color
	if msg := ValidateRPC("set_fills", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing color")
	}
	// invalid mode
	msg := ValidateRPC("set_fills", []string{"1:1"}, map[string]interface{}{
		"color": "#ff0000", "mode": "overwrite",
	})
	if msg == "" {
		t.Error("expected error for invalid mode")
	}
	// valid modes
	for _, mode := range []string{"replace", "append"} {
		msg := ValidateRPC("set_fills", []string{"1:1"}, map[string]interface{}{
			"color": "#ff0000", "mode": mode,
		})
		if msg != "" {
			t.Errorf("unexpected error for mode %s: %s", mode, msg)
		}
	}
}

func TestValidateRPC_MoveNodes(t *testing.T) {
	// no x or y
	msg := ValidateRPC("move_nodes", []string{"1:1"}, nil)
	if msg == "" {
		t.Error("expected error when neither x nor y provided")
	}
	// valid with just x
	msg = ValidateRPC("move_nodes", []string{"1:1"}, map[string]interface{}{"x": float64(10)})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_CreateVariable(t *testing.T) {
	// invalid type
	msg := ValidateRPC("create_variable", nil, map[string]interface{}{
		"name": "myVar", "collectionId": "abc", "type": "NUMBER",
	})
	if msg == "" {
		t.Error("expected error for invalid variable type")
	}
	// valid types
	for _, vt := range []string{"COLOR", "FLOAT", "STRING", "BOOLEAN"} {
		msg := ValidateRPC("create_variable", nil, map[string]interface{}{
			"name": "myVar", "collectionId": "abc", "type": vt,
		})
		if msg != "" {
			t.Errorf("unexpected error for type %s: %s", vt, msg)
		}
	}
}

func TestValidateRPC_DeleteVariable(t *testing.T) {
	// neither variableId nor collectionId
	if msg := ValidateRPC("delete_variable", nil, nil); msg == "" {
		t.Error("expected error when neither id provided")
	}
	// variableId only — valid
	msg := ValidateRPC("delete_variable", nil, map[string]interface{}{"variableId": "abc"})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SwapComponent(t *testing.T) {
	// invalid componentId format
	msg := ValidateRPC("swap_component", []string{"1:1"}, map[string]interface{}{
		"componentId": "bad-format",
	})
	if msg == "" {
		t.Error("expected error for hyphen componentId")
	}
	// valid
	msg = ValidateRPC("swap_component", []string{"1:1"}, map[string]interface{}{
		"componentId": "2:2",
	})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_UnknownTool(t *testing.T) {
	// unknown tools pass through with no error
	msg := ValidateRPC("unknown_tool", nil, nil)
	if msg != "" {
		t.Errorf("expected no error for unknown tool, got: %s", msg)
	}
}

func TestValidateRPC_GetReactions(t *testing.T) {
	if msg := ValidateRPC("get_reactions", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("get_reactions", []string{"bad-id"}, nil); msg == "" {
		t.Error("expected error for hyphen nodeId")
	}
	if msg := ValidateRPC("get_reactions", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_ScanTextNodes(t *testing.T) {
	if msg := ValidateRPC("scan_text_nodes", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("scan_text_nodes", nil, map[string]interface{}{"nodeId": "bad"}); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("scan_text_nodes", nil, map[string]interface{}{"nodeId": "1:1"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_ScanNodesByTypes(t *testing.T) {
	if msg := ValidateRPC("scan_nodes_by_types", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// missing types
	msg := ValidateRPC("scan_nodes_by_types", nil, map[string]interface{}{"nodeId": "1:1"})
	if msg == "" {
		t.Error("expected error for missing types")
	}
	// valid
	msg = ValidateRPC("scan_nodes_by_types", nil, map[string]interface{}{
		"nodeId": "1:1",
		"types":  []interface{}{"FRAME"},
	})
	if msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetAutoLayout(t *testing.T) {
	if msg := ValidateRPC("set_auto_layout", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("set_auto_layout", []string{"bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("set_auto_layout", []string{"1:1"}, map[string]interface{}{"layoutMode": "DIAGONAL"}); msg == "" {
		t.Error("expected error for invalid layoutMode")
	}
	if msg := ValidateRPC("set_auto_layout", []string{"1:1"}, map[string]interface{}{"layoutMode": "HORIZONTAL"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_CreateRectangleEllipse(t *testing.T) {
	for _, tool := range []string{"create_rectangle", "create_ellipse"} {
		if msg := ValidateRPC(tool, nil, map[string]interface{}{"width": float64(-1)}); msg == "" {
			t.Errorf("%s: expected error for negative width", tool)
		}
		if msg := ValidateRPC(tool, nil, map[string]interface{}{"height": float64(0)}); msg == "" {
			t.Errorf("%s: expected error for zero height", tool)
		}
		if msg := ValidateRPC(tool, nil, map[string]interface{}{"parentId": "bad-id"}); msg == "" {
			t.Errorf("%s: expected error for invalid parentId", tool)
		}
		if msg := ValidateRPC(tool, nil, map[string]interface{}{"width": float64(50), "parentId": "1:1"}); msg != "" {
			t.Errorf("%s unexpected error: %s", tool, msg)
		}
	}
}

func TestValidateRPC_CreateStarPolygonLine(t *testing.T) {
	if msg := ValidateRPC("create_star", nil, map[string]interface{}{"outerRadius": float64(-1)}); msg == "" {
		t.Error("create_star: expected error for negative outerRadius")
	}
	if msg := ValidateRPC("create_star", nil, map[string]interface{}{"innerRadius": float64(0)}); msg == "" {
		t.Error("create_star: expected error for zero innerRadius")
	}
	if msg := ValidateRPC("create_star", nil, map[string]interface{}{"pointCount": float64(2)}); msg == "" {
		t.Error("create_star: expected error for pointCount < 3")
	}
	if msg := ValidateRPC("create_star", nil, map[string]interface{}{"parentId": "bad-id"}); msg == "" {
		t.Error("create_star: expected error for invalid parentId")
	}

	if msg := ValidateRPC("create_polygon", nil, map[string]interface{}{"radius": float64(-1)}); msg == "" {
		t.Error("create_polygon: expected error for negative radius")
	}
	if msg := ValidateRPC("create_polygon", nil, map[string]interface{}{"pointCount": float64(2)}); msg == "" {
		t.Error("create_polygon: expected error for pointCount < 3")
	}

	if msg := ValidateRPC("create_line", nil, map[string]interface{}{"length": float64(-1)}); msg == "" {
		t.Error("create_line: expected error for negative length")
	}
}

func TestValidateRPC_CreateText(t *testing.T) {
	if msg := ValidateRPC("create_text", nil, nil); msg == "" {
		t.Error("expected error for missing text")
	}
	if msg := ValidateRPC("create_text", nil, map[string]interface{}{"text": "hi", "parentId": "bad"}); msg == "" {
		t.Error("expected error for invalid parentId")
	}
	if msg := ValidateRPC("create_text", nil, map[string]interface{}{"text": "hi"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetStrokes(t *testing.T) {
	if msg := ValidateRPC("set_strokes", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("set_strokes", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing color")
	}
	if msg := ValidateRPC("set_strokes", []string{"1:1"}, map[string]interface{}{"color": "#000", "mode": "bad"}); msg == "" {
		t.Error("expected error for invalid mode")
	}
	for _, mode := range []string{"replace", "append"} {
		if msg := ValidateRPC("set_strokes", []string{"1:1"}, map[string]interface{}{"color": "#000", "mode": mode}); msg != "" {
			t.Errorf("unexpected error for mode %s: %s", mode, msg)
		}
	}
}

func TestValidateRPC_SetGradientFills(t *testing.T) {
	if msg := ValidateRPC("set_gradient_fills", nil, nil); msg == "" {
		t.Error("expected error for missing params")
	}
	if msg := ValidateRPC("set_gradient_fills", []string{"1:1"}, map[string]interface{}{"type": "GRADIENT_RADIAL"}); msg == "" {
		t.Error("expected error for missing stops and geometry")
	}

	validArgs := map[string]interface{}{
		"type": "GRADIENT_RADIAL",
		"stops": []interface{}{
			map[string]interface{}{"position": 0, "color": "#ff0000"},
			map[string]interface{}{"position": 1, "color": "#00ff00"},
		},
		"geometry": map[string]interface{}{
			"center":   map[string]interface{}{"percentX": 50, "percentY": 50},
			"radius":   map[string]interface{}{"percentX": 50, "percentY": 50},
			"rotation": 0,
		},
	}
	if msg := ValidateRPC("set_gradient_fills", []string{"1:1"}, validArgs); msg != "" {
		t.Errorf("unexpected error for valid arguments: %s", msg)
	}

	// A stop's color is a level deeper than a paramSpec reaches, but it ends up
	// in the same hexToRgb call and used to become NaN just as quietly.
	badStops := map[string]interface{}{
		"type": "GRADIENT_LINEAR",
		"stops": []interface{}{
			map[string]interface{}{"position": 0, "color": "red"},
		},
		"geometry": map[string]interface{}{},
	}
	if msg := ValidateRPC("set_gradient_fills", []string{"1:1"}, badStops); msg == "" {
		t.Error("expected error for a stop color that is not hex")
	}
}

func TestValidateRPC_ResizeNodes(t *testing.T) {
	if msg := ValidateRPC("resize_nodes", nil, nil); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	if msg := ValidateRPC("resize_nodes", []string{"bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("resize_nodes", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error when neither width nor height provided")
	}
	if msg := ValidateRPC("resize_nodes", []string{"1:1"}, map[string]interface{}{"width": float64(200)}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_DeleteNodes(t *testing.T) {
	if msg := ValidateRPC("delete_nodes", nil, nil); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	if msg := ValidateRPC("delete_nodes", []string{"bad-id"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("delete_nodes", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_RenameNode(t *testing.T) {
	if msg := ValidateRPC("rename_node", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("rename_node", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing name")
	}
	if msg := ValidateRPC("rename_node", []string{"1:1"}, map[string]interface{}{"name": "Frame 1"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_CloneNode(t *testing.T) {
	if msg := ValidateRPC("clone_node", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("clone_node", []string{"1:1"}, map[string]interface{}{"parentId": "bad"}); msg == "" {
		t.Error("expected error for invalid parentId")
	}
	if msg := ValidateRPC("clone_node", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_ImportImage(t *testing.T) {
	if msg := ValidateRPC("import_image", nil, nil); msg == "" {
		t.Error("expected error for missing imageData")
	}
	if msg := ValidateRPC("import_image", nil, map[string]interface{}{"imageData": "b64", "scaleMode": "STRETCH"}); msg == "" {
		t.Error("expected error for invalid scaleMode")
	}
	if msg := ValidateRPC("import_image", nil, map[string]interface{}{"imageData": "b64", "parentId": "bad"}); msg == "" {
		t.Error("expected error for invalid parentId")
	}
	for _, sm := range []string{"FILL", "FIT", "CROP", "TILE"} {
		if msg := ValidateRPC("import_image", nil, map[string]interface{}{"imageData": "b64", "scaleMode": sm}); msg != "" {
			t.Errorf("unexpected error for scaleMode %s: %s", sm, msg)
		}
	}
}

func TestValidateRPC_UpdatePaintStyle(t *testing.T) {
	if msg := ValidateRPC("update_paint_style", nil, nil); msg == "" {
		t.Error("expected error for missing styleId")
	}
	if msg := ValidateRPC("update_paint_style", nil, map[string]interface{}{"styleId": "S:abc"}); msg == "" {
		t.Error("expected error when no fields to update")
	}
	if msg := ValidateRPC("update_paint_style", nil, map[string]interface{}{"styleId": "S:abc", "color": "#fff"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	if msg := ValidateRPC("update_paint_style", nil, map[string]interface{}{"styleId": "S:abc", "description": "desc"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_DeleteStyle(t *testing.T) {
	if msg := ValidateRPC("delete_style", nil, nil); msg == "" {
		t.Error("expected error for missing styleId")
	}
	if msg := ValidateRPC("delete_style", nil, map[string]interface{}{"styleId": "S:abc"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_CreateVariableCollection(t *testing.T) {
	if msg := ValidateRPC("create_variable_collection", nil, nil); msg == "" {
		t.Error("expected error for missing name")
	}
	if msg := ValidateRPC("create_variable_collection", nil, map[string]interface{}{"name": "Brand"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_AddVariableMode(t *testing.T) {
	if msg := ValidateRPC("add_variable_mode", nil, nil); msg == "" {
		t.Error("expected error for missing collectionId")
	}
	if msg := ValidateRPC("add_variable_mode", nil, map[string]interface{}{"collectionId": "c1"}); msg == "" {
		t.Error("expected error for missing modeName")
	}
	if msg := ValidateRPC("add_variable_mode", nil, map[string]interface{}{"collectionId": "c1", "modeName": "Dark"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetVariableValue(t *testing.T) {
	if msg := ValidateRPC("set_variable_value", nil, nil); msg == "" {
		t.Error("expected error for missing variableId")
	}
	if msg := ValidateRPC("set_variable_value", nil, map[string]interface{}{"variableId": "v1"}); msg == "" {
		t.Error("expected error for missing modeId")
	}
	if msg := ValidateRPC("set_variable_value", nil, map[string]interface{}{"variableId": "v1", "modeId": "m1"}); msg == "" {
		t.Error("expected error for missing value")
	}
	if msg := ValidateRPC("set_variable_value", nil, map[string]interface{}{"variableId": "v1", "modeId": "m1", "value": "#fff"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_ApplyStyleToNode(t *testing.T) {
	if msg := ValidateRPC("apply_style_to_node", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("apply_style_to_node", []string{"bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("apply_style_to_node", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing styleId")
	}
	if msg := ValidateRPC("apply_style_to_node", []string{"1:1"}, map[string]interface{}{"styleId": "S:abc", "target": "shadow"}); msg == "" {
		t.Error("expected error for invalid target")
	}
	for _, target := range []string{"fill", "stroke"} {
		if msg := ValidateRPC("apply_style_to_node", []string{"1:1"}, map[string]interface{}{"styleId": "S:abc", "target": target}); msg != "" {
			t.Errorf("unexpected error for target %s: %s", target, msg)
		}
	}
}

func TestValidateRPC_BindVariableToNode(t *testing.T) {
	if msg := ValidateRPC("bind_variable_to_node", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("bind_variable_to_node", []string{"bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("bind_variable_to_node", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing variableId")
	}
	if msg := ValidateRPC("bind_variable_to_node", []string{"1:1"}, map[string]interface{}{"variableId": "v1"}); msg == "" {
		t.Error("expected error for missing field")
	}
	if msg := ValidateRPC("bind_variable_to_node", []string{"1:1"}, map[string]interface{}{"variableId": "v1", "field": "fill"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_DetachInstance(t *testing.T) {
	if msg := ValidateRPC("detach_instance", nil, nil); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	if msg := ValidateRPC("detach_instance", []string{"bad-id"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("detach_instance", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetCornerRadius(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("set_corner_radius", nil, map[string]interface{}{"cornerRadius": float64(8)}); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	// invalid nodeId
	if msg := ValidateRPC("set_corner_radius", []string{"bad"}, map[string]interface{}{"cornerRadius": float64(8)}); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	// no radius param provided
	if msg := ValidateRPC("set_corner_radius", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error when no radius param provided")
	}
	// uniform cornerRadius
	if msg := ValidateRPC("set_corner_radius", []string{"1:1"}, map[string]interface{}{"cornerRadius": float64(8)}); msg != "" {
		t.Errorf("unexpected error for cornerRadius: %s", msg)
	}
	// per-corner individually
	for _, param := range []string{"topLeftRadius", "topRightRadius", "bottomLeftRadius", "bottomRightRadius"} {
		if msg := ValidateRPC("set_corner_radius", []string{"1:1"}, map[string]interface{}{param: float64(4)}); msg != "" {
			t.Errorf("unexpected error for %s: %s", param, msg)
		}
	}
	// mixed per-corner
	if msg := ValidateRPC("set_corner_radius", []string{"1:1"}, map[string]interface{}{
		"topLeftRadius": float64(8), "topRightRadius": float64(0),
		"bottomLeftRadius": float64(8), "bottomRightRadius": float64(0),
	}); msg != "" {
		t.Errorf("unexpected error for per-corner radii: %s", msg)
	}
}

func TestValidateRPC_GroupNodes(t *testing.T) {
	// fewer than 2 nodes
	if msg := ValidateRPC("group_nodes", nil, nil); msg == "" {
		t.Error("expected error for empty nodeIds")
	}
	if msg := ValidateRPC("group_nodes", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for single nodeId")
	}
	// invalid nodeId
	if msg := ValidateRPC("group_nodes", []string{"1:1", "bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	// valid
	if msg := ValidateRPC("group_nodes", []string{"1:1", "2:2"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	if msg := ValidateRPC("group_nodes", []string{"1:1", "2:2", "3:3"}, nil); msg != "" {
		t.Errorf("unexpected error for 3 nodeIds: %s", msg)
	}
}

func TestValidateRPC_UngroupNodes(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("ungroup_nodes", nil, nil); msg == "" {
		t.Error("expected error for empty nodeIds")
	}
	// invalid nodeId
	if msg := ValidateRPC("ungroup_nodes", []string{"bad-id"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	// valid single
	if msg := ValidateRPC("ungroup_nodes", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid multiple
	if msg := ValidateRPC("ungroup_nodes", []string{"1:1", "2:2"}, nil); msg != "" {
		t.Errorf("unexpected error for multiple nodeIds: %s", msg)
	}
}

func TestValidateRPC_NavigateToPage(t *testing.T) {
	// neither pageId nor pageName
	if msg := ValidateRPC("navigate_to_page", nil, nil); msg == "" {
		t.Error("expected error when neither pageId nor pageName provided")
	}
	if msg := ValidateRPC("navigate_to_page", nil, map[string]interface{}{}); msg == "" {
		t.Error("expected error for empty params")
	}
	// pageId provided
	if msg := ValidateRPC("navigate_to_page", nil, map[string]interface{}{"pageId": "0:1"}); msg != "" {
		t.Errorf("unexpected error for pageId: %s", msg)
	}
	// pageName provided
	if msg := ValidateRPC("navigate_to_page", nil, map[string]interface{}{"pageName": "Design"}); msg != "" {
		t.Errorf("unexpected error for pageName: %s", msg)
	}
	// both provided — also valid
	if msg := ValidateRPC("navigate_to_page", nil, map[string]interface{}{"pageId": "0:1", "pageName": "Design"}); msg != "" {
		t.Errorf("unexpected error when both provided: %s", msg)
	}
}

func TestValidateRPC_CreateComponent(t *testing.T) {
	// missing nodeId
	if msg := ValidateRPC("create_component", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("create_component", []string{""}, nil); msg == "" {
		t.Error("expected error for empty nodeId")
	}
	// invalid nodeId format
	if msg := ValidateRPC("create_component", []string{"bad-id"}, nil); msg == "" {
		t.Error("expected error for hyphen nodeId")
	}
	// valid
	if msg := ValidateRPC("create_component", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	if msg := ValidateRPC("create_component", []string{"1:1"}, map[string]interface{}{"name": "MyComponent"}); msg != "" {
		t.Errorf("unexpected error with name: %s", msg)
	}
}

func TestValidateRPC_ExportTokens(t *testing.T) {
	// no params — valid (defaults to json)
	if msg := ValidateRPC("export_tokens", nil, nil); msg != "" {
		t.Errorf("unexpected error for no params: %s", msg)
	}
	// valid formats
	for _, f := range []string{"json", "css"} {
		if msg := ValidateRPC("export_tokens", nil, map[string]interface{}{"format": f}); msg != "" {
			t.Errorf("unexpected error for format %s: %s", f, msg)
		}
	}
	// invalid format
	if msg := ValidateRPC("export_tokens", nil, map[string]interface{}{"format": "yaml"}); msg == "" {
		t.Error("expected error for invalid format")
	}
	if msg := ValidateRPC("export_tokens", nil, map[string]interface{}{"format": "style-dictionary"}); msg == "" {
		t.Error("expected error for unsupported format")
	}
}

func TestValidateAutoLayoutParams_InvalidValues(t *testing.T) {
	cases := []struct {
		param string
		value string
	}{
		{"primaryAxisAlignItems", "LEFT"},
		{"counterAxisAlignItems", "TOP"},
		{"primaryAxisSizingMode", "SHRINK"},
		{"counterAxisSizingMode", "SHRINK"},
		{"layoutWrap", "FLEX_WRAP"},
	}
	for _, c := range cases {
		msg := ValidateRPC("create_frame", nil, map[string]interface{}{c.param: c.value})
		if msg == "" {
			t.Errorf("expected error for invalid %s=%q", c.param, c.value)
		}
	}

	// All valid auto-layout params together
	msg := ValidateRPC("create_frame", nil, map[string]interface{}{
		"primaryAxisAlignItems": "CENTER",
		"counterAxisAlignItems": "BASELINE",
		"primaryAxisSizingMode": "AUTO",
		"counterAxisSizingMode": "FIXED",
		"layoutWrap":            "WRAP",
	})
	if msg != "" {
		t.Errorf("unexpected error for valid auto-layout params: %s", msg)
	}
}

// ── set_reactions ─────────────────────────────────────────────────────────────

func TestValidateRPC_SetReactions(t *testing.T) {
	validReaction := map[string]interface{}{
		"trigger": map[string]interface{}{"type": "ON_CLICK"},
		"action": map[string]interface{}{
			"type":          "NODE",
			"destinationId": "1:3",
			"navigation":    "NAVIGATE",
		},
	}

	// missing nodeId
	if msg := ValidateRPC("set_reactions", nil, map[string]interface{}{"reactions": []interface{}{}}); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// bad nodeId format
	if msg := ValidateRPC("set_reactions", []string{"1-2"}, map[string]interface{}{"reactions": []interface{}{}}); msg == "" {
		t.Error("expected error for bad nodeId format")
	}
	// missing reactions
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{}); msg == "" {
		t.Error("expected error for missing reactions")
	}
	// reactions not an array
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{"reactions": "not-array"}); msg == "" {
		t.Error("expected error for non-array reactions")
	}
	// bad mode
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{},
		"mode":      "overwrite",
	}); msg == "" {
		t.Error("expected error for bad mode")
	}
	// valid mode replace
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{validReaction},
		"mode":      "replace",
	}); msg != "" {
		t.Errorf("unexpected error for mode=replace: %s", msg)
	}
	// valid mode append
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{validReaction},
		"mode":      "append",
	}); msg != "" {
		t.Errorf("unexpected error for mode=append: %s", msg)
	}
	// invalid trigger type
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "INVALID_TRIGGER"},
				"action":  map[string]interface{}{"type": "BACK"},
			},
		},
	}); msg == "" {
		t.Error("expected error for invalid trigger type")
	}
	// AFTER_TIMEOUT missing timeout
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "AFTER_TIMEOUT"},
				"action":  map[string]interface{}{"type": "BACK"},
			},
		},
	}); msg == "" {
		t.Error("expected error for AFTER_TIMEOUT without timeout")
	}
	// AFTER_TIMEOUT with valid timeout
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "AFTER_TIMEOUT", "timeout": float64(3000)},
				"action":  map[string]interface{}{"type": "BACK"},
			},
		},
	}); msg != "" {
		t.Errorf("unexpected error for valid AFTER_TIMEOUT: %s", msg)
	}
	// invalid action type
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "ON_CLICK"},
				"action":  map[string]interface{}{"type": "INVALID_ACTION"},
			},
		},
	}); msg == "" {
		t.Error("expected error for invalid action type")
	}
	// NODE missing navigation field
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "ON_CLICK"},
				"action":  map[string]interface{}{"type": "NODE", "destinationId": "1:3"},
			},
		},
	}); msg == "" {
		t.Error("expected error for NODE without navigation")
	}
	// URL missing url
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{
			map[string]interface{}{
				"trigger": map[string]interface{}{"type": "ON_CLICK"},
				"action":  map[string]interface{}{"type": "URL"},
			},
		},
	}); msg == "" {
		t.Error("expected error for URL without url")
	}
	// empty reactions array is valid (clear all)
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{},
	}); msg != "" {
		t.Errorf("unexpected error for empty reactions: %s", msg)
	}
	// valid full reaction
	if msg := ValidateRPC("set_reactions", []string{"1:2"}, map[string]interface{}{
		"reactions": []interface{}{validReaction},
	}); msg != "" {
		t.Errorf("unexpected error for valid reaction: %s", msg)
	}
}

// ── remove_reactions ──────────────────────────────────────────────────────────

func TestValidateRPC_RemoveReactions(t *testing.T) {
	// missing nodeId
	if msg := ValidateRPC("remove_reactions", nil, map[string]interface{}{}); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// bad nodeId format
	if msg := ValidateRPC("remove_reactions", []string{"1-2"}, map[string]interface{}{}); msg == "" {
		t.Error("expected error for bad nodeId format")
	}
	// non-number in indices
	if msg := ValidateRPC("remove_reactions", []string{"1:2"}, map[string]interface{}{
		"indices": []interface{}{"zero"},
	}); msg == "" {
		t.Error("expected error for non-number index")
	}
	// valid with no indices (remove all)
	if msg := ValidateRPC("remove_reactions", []string{"1:2"}, map[string]interface{}{}); msg != "" {
		t.Errorf("unexpected error for remove all: %s", msg)
	}
	// valid with numeric indices
	if msg := ValidateRPC("remove_reactions", []string{"1:2"}, map[string]interface{}{
		"indices": []interface{}{float64(0), float64(2)},
	}); msg != "" {
		t.Errorf("unexpected error for valid indices: %s", msg)
	}
}

// ── set_visible ─────────────────────────────────────────────────────

func TestValidateRPC_ReparentNodes(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("reparent_nodes", nil, map[string]interface{}{"parentId": "2:2"}); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	// missing parentId
	if msg := ValidateRPC("reparent_nodes", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing parentId")
	}
	// invalid parentId
	if msg := ValidateRPC("reparent_nodes", []string{"1:1"}, map[string]interface{}{"parentId": "bad"}); msg == "" {
		t.Error("expected error for invalid parentId")
	}
	// valid
	if msg := ValidateRPC("reparent_nodes", []string{"1:1"}, map[string]interface{}{"parentId": "2:2"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

// ── batch_rename_nodes ──────────────────────────────────────────────

func TestValidateRPC_BatchRenameNodes(t *testing.T) {
	// missing nodeIds
	if msg := ValidateRPC("batch_rename_nodes", nil, map[string]interface{}{"prefix": "x"}); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	// no operation provided
	if msg := ValidateRPC("batch_rename_nodes", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for no rename operation")
	}
	// find without replace
	if msg := ValidateRPC("batch_rename_nodes", []string{"1:1"}, map[string]interface{}{"find": "x"}); msg == "" {
		t.Error("expected error for find without replace")
	}
	// valid prefix only
	if msg := ValidateRPC("batch_rename_nodes", []string{"1:1"}, map[string]interface{}{"prefix": "UI/"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid find+replace
	if msg := ValidateRPC("batch_rename_nodes", []string{"1:1"}, map[string]interface{}{"find": "Btn", "replace": "Button"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

// ── find_replace_text ───────────────────────────────────────────────

func TestValidateRPC_FindReplaceText(t *testing.T) {
	// missing find
	if msg := ValidateRPC("find_replace_text", nil, map[string]interface{}{"replace": "x"}); msg == "" {
		t.Error("expected error for missing find")
	}
	// missing replace
	if msg := ValidateRPC("find_replace_text", nil, map[string]interface{}{"find": "x"}); msg == "" {
		t.Error("expected error for missing replace")
	}
	// valid minimal
	if msg := ValidateRPC("find_replace_text", nil, map[string]interface{}{"find": "x", "replace": "y"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid with empty replace (delete matches)
	if msg := ValidateRPC("find_replace_text", nil, map[string]interface{}{"find": "x", "replace": ""}); msg != "" {
		t.Errorf("unexpected error for empty replace: %s", msg)
	}
}

// ── Page management ─────────────────────────────────────────────────

func TestValidateRPC_AddPage(t *testing.T) {
	// valid with no params
	if msg := ValidateRPC("add_page", nil, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// negative index
	if msg := ValidateRPC("add_page", nil, map[string]interface{}{"index": float64(-1)}); msg == "" {
		t.Error("expected error for negative index")
	}
	// valid with name
	if msg := ValidateRPC("add_page", nil, map[string]interface{}{"name": "Flows"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_DeletePage(t *testing.T) {
	// missing both pageId and pageName
	if msg := ValidateRPC("delete_page", nil, nil); msg == "" {
		t.Error("expected error for missing page identifier")
	}
	// valid with pageId
	if msg := ValidateRPC("delete_page", nil, map[string]interface{}{"pageId": "0:2"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid with pageName
	if msg := ValidateRPC("delete_page", nil, map[string]interface{}{"pageName": "Flows"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_RenamePage(t *testing.T) {
	// missing page identifier
	if msg := ValidateRPC("rename_page", nil, map[string]interface{}{"newName": "X"}); msg == "" {
		t.Error("expected error for missing page identifier")
	}
	// missing newName
	if msg := ValidateRPC("rename_page", nil, map[string]interface{}{"pageId": "0:2"}); msg == "" {
		t.Error("expected error for missing newName")
	}
	// valid
	if msg := ValidateRPC("rename_page", nil, map[string]interface{}{"pageId": "0:2", "newName": "Sprint 1"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetEffects(t *testing.T) {
	// missing nodeId
	if msg := ValidateRPC("set_effects", nil, map[string]interface{}{"effects": []interface{}{}}); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	// missing effects
	if msg := ValidateRPC("set_effects", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing effects")
	}
	// effects not an array
	if msg := ValidateRPC("set_effects", []string{"1:1"}, map[string]interface{}{"effects": "shadow"}); msg == "" {
		t.Error("expected error for non-array effects")
	}
	// invalid effect type
	if msg := ValidateRPC("set_effects", []string{"1:1"}, map[string]interface{}{
		"effects": []interface{}{map[string]interface{}{"type": "GLOW"}},
	}); msg == "" {
		t.Error("expected error for invalid effect type")
	}
	// valid empty effects (clear all)
	if msg := ValidateRPC("set_effects", []string{"1:1"}, map[string]interface{}{"effects": []interface{}{}}); msg != "" {
		t.Errorf("unexpected error for empty effects: %s", msg)
	}
	// valid drop shadow
	if msg := ValidateRPC("set_effects", []string{"1:1"}, map[string]interface{}{
		"effects": []interface{}{map[string]interface{}{"type": "DROP_SHADOW"}},
	}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid layer blur
	if msg := ValidateRPC("set_effects", []string{"1:1"}, map[string]interface{}{
		"effects": []interface{}{map[string]interface{}{"type": "LAYER_BLUR", "radius": float64(4)}},
	}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_CreateSection(t *testing.T) {
	// valid with no params
	if msg := ValidateRPC("create_section", nil, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// valid with name
	if msg := ValidateRPC("create_section", nil, map[string]interface{}{"name": "Sprint 1"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	// invalid width
	if msg := ValidateRPC("create_section", nil, map[string]interface{}{"width": float64(-10)}); msg == "" {
		t.Error("expected error for negative width")
	}
	// invalid height
	if msg := ValidateRPC("create_section", nil, map[string]interface{}{"height": float64(0)}); msg == "" {
		t.Error("expected error for zero height")
	}
}

func TestValidateRPC_CreateComponentInstance(t *testing.T) {
	if msg := ValidateRPC("create_component_instance", nil, nil); msg == "" {
		t.Error("expected error for missing componentId/Key")
	}
	if msg := ValidateRPC("create_component_instance", nil, map[string]interface{}{"componentId": "1:1", "parentId": "bad"}); msg == "" {
		t.Error("expected error for invalid parentId")
	}
	if msg := ValidateRPC("create_component_instance", nil, map[string]interface{}{"componentId": "1:1"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
	if msg := ValidateRPC("create_component_instance", nil, map[string]interface{}{"componentKey": "abc"}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_GetInstanceOverrides(t *testing.T) {
	if msg := ValidateRPC("get_instance_overrides", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("get_instance_overrides", []string{"bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("get_instance_overrides", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetInstanceOverrides(t *testing.T) {
	if msg := ValidateRPC("set_instance_overrides", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("set_instance_overrides", []string{"bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeId")
	}
	if msg := ValidateRPC("set_instance_overrides", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing properties")
	}
	if msg := ValidateRPC("set_instance_overrides", []string{"1:1"}, map[string]interface{}{"properties": "not-map"}); msg == "" {
		t.Error("expected error for non-map properties")
	}
	if msg := ValidateRPC("set_instance_overrides", []string{"1:1"}, map[string]interface{}{"properties": map[string]interface{}{"Size": "Small"}}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_CreateConnector(t *testing.T) {
	if msg := ValidateRPC("create_connector", nil, nil); msg == "" {
		t.Error("expected error for missing endpoints")
	}
	if msg := ValidateRPC("create_connector", nil, map[string]interface{}{"startNodeId": "bad"}); msg == "" {
		t.Error("expected error for invalid startNodeId")
	}
	if msg := ValidateRPC("create_connector", nil, map[string]interface{}{"endNodeId": "bad"}); msg == "" {
		t.Error("expected error for invalid endNodeId")
	}
	if msg := ValidateRPC("create_connector", nil, map[string]interface{}{"lineType": "CURVED"}); msg == "" {
		t.Error("expected error for invalid lineType")
	}
	if msg := ValidateRPC("create_connector", nil, map[string]interface{}{
		"startNodeId": "1:1",
		"endNodeId":   "2:2",
		"lineType":    "ELBOW",
	}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetAnnotations(t *testing.T) {
	if msg := ValidateRPC("set_annotations", nil, nil); msg == "" {
		t.Error("expected error for missing nodeId")
	}
	if msg := ValidateRPC("set_annotations", []string{"1:1"}, nil); msg == "" {
		t.Error("expected error for missing annotations array")
	}
	if msg := ValidateRPC("set_annotations", []string{"1:1"}, map[string]interface{}{"annotations": "not-array"}); msg == "" {
		t.Error("expected error for non-array annotations")
	}
	if msg := ValidateRPC("set_annotations", []string{"1:1"}, map[string]interface{}{
		"annotations": []interface{}{map[string]interface{}{"label": "Btn"}},
	}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_ClearAnnotations(t *testing.T) {
	if msg := ValidateRPC("clear_annotations", nil, nil); msg == "" {
		t.Error("expected error for missing nodeIds")
	}
	if msg := ValidateRPC("clear_annotations", []string{"bad"}, nil); msg == "" {
		t.Error("expected error for invalid nodeIds")
	}
	if msg := ValidateRPC("clear_annotations", []string{"1:1"}, nil); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_BatchExecutePipeline(t *testing.T) {
	step := map[string]interface{}{
		"id":          "step_1",
		"action":      "create_frame",
		"params":      map[string]interface{}{"name": "Header", "width": 100.0, "height": 100.0},
		"export_vars": map[string]interface{}{"id": "$header_id"},
	}

	if msg := ValidateRPC("batch_execute_pipeline", nil, map[string]interface{}{}); msg == "" {
		t.Error("expected error for missing steps")
	}
	if msg := ValidateRPC("batch_execute_pipeline", nil, map[string]interface{}{
		"steps": []interface{}{"create_frame"},
	}); msg == "" {
		t.Error("expected error for a step that is not an object")
	}
	if msg := ValidateRPC("batch_execute_pipeline", nil, map[string]interface{}{
		"stop_on_error": true,
		"steps":         []interface{}{step},
	}); msg != "" {
		t.Errorf("unexpected error: %s", msg)
	}
}

func TestValidateRPC_SetNodeProperties(t *testing.T) {
	valid := []string{"1:1"}

	cases := []struct {
		name    string
		nodeIDs []string
		params  map[string]interface{}
		wantMsg string // "" means the request must be accepted
	}{
		{"no nodeIds", nil, map[string]interface{}{"opacity": 0.5}, "nodeIds is required"},
		{"bad nodeId", []string{"nope"}, map[string]interface{}{"opacity": 0.5}, "colon format"},
		{"no properties", valid, map[string]interface{}{}, "at least one of"},
		{"opacity too high", valid, map[string]interface{}{"opacity": 5.0}, "opacity must be at most 1"},
		{"opacity negative", valid, map[string]interface{}{"opacity": -0.1}, "opacity must be at least 0"},
		{"invalid blend mode", valid, map[string]interface{}{"blendMode": "NEON"}, "blendMode must be one of"},
		{"invalid order", valid, map[string]interface{}{"order": "sideways"}, "order must be"},
		{"invalid constraint axis", valid, map[string]interface{}{
			"constraints": map[string]interface{}{"horizontal": "MIDDLE"},
		}, "horizontal must be"},

		{"opacity at 0", valid, map[string]interface{}{"opacity": 0.0}, ""},
		{"opacity at 1", valid, map[string]interface{}{"opacity": 1.0}, ""},
		{"visible false only", valid, map[string]interface{}{"visible": false}, ""},
		{"locked false only", valid, map[string]interface{}{"locked": false}, ""},
		{"every property", valid, map[string]interface{}{
			"visible": true, "locked": false, "opacity": 0.5, "rotation": 45.0,
			"blendMode": "MULTIPLY", "order": "bringToFront",
			"constraints": map[string]interface{}{"horizontal": "STRETCH", "vertical": "MIN"},
		}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := ValidateRPC("set_node_properties", c.nodeIDs, c.params)
			if c.wantMsg == "" {
				if msg != "" {
					t.Errorf("expected the request to be accepted, got %q", msg)
				}
				return
			}
			if msg == "" {
				t.Fatalf("expected an error containing %q, got none", c.wantMsg)
			}
			if !strings.Contains(msg, c.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", msg, c.wantMsg)
			}
		})
	}
}

func TestValidateRPC_HexColor(t *testing.T) {
	// The plugin used to turn an unreadable color into NaN channels and paint a
	// broken fill silently. These are rejected before the round-trip now.
	bad := []string{"red", "rgb(255,0,0)", "#ff", "#12345", "#gggggg"}
	for _, color := range bad {
		if msg := ValidateRPC("set_fills", []string{"1:1"}, map[string]interface{}{"color": color}); msg == "" {
			t.Errorf("expected %q to be rejected", color)
		}
	}

	// Shorthand is real CSS and the plugin expands it, so it must pass here.
	good := []string{"#f00", "#f00a", "#ff0000", "#ff0000aa", "ff0000"}
	for _, color := range good {
		if msg := ValidateRPC("set_fills", []string{"1:1"}, map[string]interface{}{"color": color}); msg != "" {
			t.Errorf("unexpected error for %q: %s", color, msg)
		}
	}

	// Every tool that takes a color gets the same check.
	cases := []struct {
		tool   string
		params map[string]interface{}
	}{
		{"set_strokes", map[string]interface{}{"color": "nope"}},
		{"create_style", map[string]interface{}{"type": "PAINT", "name": "Brand", "color": "nope"}},
		{"create_rectangle", map[string]interface{}{"fillColor": "nope"}},
		{"create_line", map[string]interface{}{"strokeColor": "nope"}},
	}
	for _, c := range cases {
		if msg := ValidateRPC(c.tool, []string{"1:1"}, c.params); msg == "" {
			t.Errorf("%s: expected the bad color to be rejected", c.tool)
		}
	}
}

// create_style merged four tools behind a `type` discriminator. The risk that
// buys is the model reaching for an argument that belongs to a different kind
// of style, so those are rejected rather than dropped.
func TestValidateRPC_CreateStyle(t *testing.T) {
	cases := []struct {
		name    string
		params  map[string]interface{}
		wantMsg string // "" means the request must be accepted
	}{
		{"paint", map[string]interface{}{"type": "PAINT", "name": "Brand", "color": "#ff0000"}, ""},
		{"text", map[string]interface{}{"type": "TEXT", "name": "H1", "fontSize": 32.0}, ""},
		{"effect", map[string]interface{}{"type": "EFFECT", "name": "Card", "effectType": "DROP_SHADOW", "radius": 8.0}, ""},
		{"grid", map[string]interface{}{"type": "GRID", "name": "Desktop", "pattern": "COLUMNS", "count": 12.0}, ""},
		{"description is common", map[string]interface{}{"type": "TEXT", "name": "H1", "description": "big"}, ""},

		{"missing type", map[string]interface{}{"name": "Brand"}, "type is required"},
		{"unknown type", map[string]interface{}{"type": "SHADOW", "name": "Brand"}, "type must be one of"},
		{"missing name", map[string]interface{}{"type": "PAINT", "color": "#ff0000"}, "name is required"},
		{"paint without color", map[string]interface{}{"type": "PAINT", "name": "Brand"}, "color is required when type is PAINT"},

		{"text argument on a paint style", map[string]interface{}{
			"type": "PAINT", "name": "Brand", "color": "#ff0000", "fontSize": 32.0,
		}, "fontSize does not apply when type is PAINT"},
		{"grid argument on a text style", map[string]interface{}{
			"type": "TEXT", "name": "H1", "pattern": "COLUMNS",
		}, "pattern does not apply when type is TEXT"},
		{"effect argument on a grid style", map[string]interface{}{
			"type": "GRID", "name": "Desktop", "spread": 4.0,
		}, "spread does not apply when type is GRID"},
		{"colour is shared, spacing is not", map[string]interface{}{
			"type": "GRID", "name": "Desktop", "color": "#ff0000", "letterSpacingValue": 2.0,
		}, "letterSpacingValue does not apply when type is GRID"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := ValidateRPC("create_style", nil, c.params)
			if c.wantMsg == "" {
				if msg != "" {
					t.Errorf("expected the request to be accepted, got %q", msg)
				}
				return
			}
			if !strings.Contains(msg, c.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", msg, c.wantMsg)
			}
		})
	}
}

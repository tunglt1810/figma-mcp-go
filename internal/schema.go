package internal

import (
	"fmt"
	"regexp"
	"strings"
)

// nodeIDPattern matches Figma node IDs:
//
//	simple:   "4029:12345"
//	compound: "I2167:9091;186:1579;186:1745" (instances/variants)
var nodeIDPattern = regexp.MustCompile(`^I?\d+:\d+(;\d+:\d+)*$`)

// NormalizeNodeID converts hyphen-format node IDs (LLM output artifact) to colon format.
// "4029-12345" → "4029:12345". No-ops for already-valid or unrecognized strings.
func NormalizeNodeID(s string) string {
	if strings.Contains(s, "-") && !strings.Contains(s, ":") {
		normalized := strings.ReplaceAll(s, "-", ":")
		if nodeIDPattern.MatchString(normalized) {
			return normalized
		}
	}
	return s
}

// ValidNodeID reports whether s is a valid Figma node ID.
func ValidNodeID(s string) bool {
	return nodeIDPattern.MatchString(s)
}

// ValidateRPC validates an incoming RPC request against the tool's expected
// input shape. Returns an error string on failure, empty string if valid.
func ValidateRPC(tool string, nodeIDs []string, params map[string]interface{}) string {
	// Table-declared tools carry their own rules; derive the checks from the
	// spec rather than repeating the argument list here.
	if spec, ok := specRegistry[tool]; ok {
		return validateSpec(spec, nodeIDs, params)
	}

	switch tool {
	case "export_frames_to_pdf":
		if len(nodeIDs) == 0 {
			return "nodeIds is required and must not be empty"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	case "get_screenshot":
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		if format, ok := params["format"].(string); ok {
			if !validExportFormat(format) {
				return fmt.Sprintf("format must be PNG, SVG, JPG, or PDF, got: %s", format)
			}
		}

	case "save_screenshots":
		items, ok := params["items"]
		if !ok {
			return "items is required"
		}
		itemList, ok := items.([]interface{})
		if !ok || len(itemList) == 0 {
			return "items must be a non-empty array"
		}
		for i, item := range itemList {
			m, ok := item.(map[string]interface{})
			if !ok {
				return fmt.Sprintf("items[%d] must be an object", i)
			}
			nodeID, _ := m["nodeId"].(string)
			if !ValidNodeID(nodeID) {
				return fmt.Sprintf("items[%d].nodeId must use colon format e.g. 4029:12345", i)
			}
			outputPath, _ := m["outputPath"].(string)
			if outputPath == "" {
				return fmt.Sprintf("items[%d].outputPath is required", i)
			}
		}

	case "group_nodes":
		if len(nodeIDs) < 2 {
			return "nodeIds must contain at least 2 nodes to group"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	case "ungroup_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required and must not be empty"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	case "navigate_to_page":
		pageID, _ := params["pageId"].(string)
		pageName, _ := params["pageName"].(string)
		if pageID == "" && pageName == "" {
			return "pageId or pageName is required"
		}

	case "create_component_instance":
		componentID, _ := params["componentId"].(string)
		componentKey, _ := params["componentKey"].(string)
		if componentID == "" && componentKey == "" {
			return "componentId or componentKey is required"
		}
		if componentID != "" && !ValidNodeID(componentID) {
			return fmt.Sprintf("componentId must use colon format e.g. 4029:12345, got: %s", componentID)
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "set_instance_overrides":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		props, ok := params["properties"]
		if !ok {
			return "properties is required"
		}
		if _, ok := props.(map[string]interface{}); !ok {
			return "properties must be an object/map"
		}

	case "create_paint_style":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if color, _ := params["color"].(string); color == "" {
			return "color is required (hex string e.g. #FF5733)"
		}

	case "create_text_style":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if td, ok := params["textDecoration"].(string); ok && td != "" {
			switch td {
			case "NONE", "UNDERLINE", "STRIKETHROUGH":
			default:
				return fmt.Sprintf("textDecoration must be NONE, UNDERLINE, or STRIKETHROUGH, got: %s", td)
			}
		}
		if unit, ok := params["lineHeightUnit"].(string); ok && unit != "" {
			switch unit {
			case "PIXELS", "PERCENT":
			default:
				return fmt.Sprintf("lineHeightUnit must be PIXELS or PERCENT, got: %s", unit)
			}
		}
		if unit, ok := params["letterSpacingUnit"].(string); ok && unit != "" {
			switch unit {
			case "PIXELS", "PERCENT":
			default:
				return fmt.Sprintf("letterSpacingUnit must be PIXELS or PERCENT, got: %s", unit)
			}
		}

	case "create_effect_style":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if t, ok := params["type"].(string); ok && t != "" {
			switch t {
			case "DROP_SHADOW", "INNER_SHADOW", "LAYER_BLUR", "BACKGROUND_BLUR":
			default:
				return fmt.Sprintf("type must be DROP_SHADOW, INNER_SHADOW, LAYER_BLUR, or BACKGROUND_BLUR, got: %s", t)
			}
		}

	case "create_grid_style":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if p, ok := params["pattern"].(string); ok && p != "" {
			switch p {
			case "GRID", "COLUMNS", "ROWS":
			default:
				return fmt.Sprintf("pattern must be GRID, COLUMNS, or ROWS, got: %s", p)
			}
		}
		if a, ok := params["alignment"].(string); ok && a != "" {
			switch a {
			case "STRETCH", "CENTER", "MIN", "MAX":
			default:
				return fmt.Sprintf("alignment must be STRETCH, CENTER, MIN, or MAX, got: %s", a)
			}
		}

	case "update_paint_style":
		if styleId, _ := params["styleId"].(string); styleId == "" {
			return "styleId is required"
		}
		_, hasName := params["name"]
		_, hasColor := params["color"]
		_, hasDesc := params["description"]
		if !hasName && !hasColor && !hasDesc {
			return "at least one of name, color, or description is required"
		}

	case "delete_style":
		if styleId, _ := params["styleId"].(string); styleId == "" {
			return "styleId is required"
		}

	// ── Variable tools ───────────────────────────────────────────────────────

	// ── Linked tools ─────────────────────────────────────────────────────────

	case "apply_style_to_node":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if styleId, _ := params["styleId"].(string); styleId == "" {
			return "styleId is required"
		}
		if target, ok := params["target"].(string); ok && target != "" {
			switch target {
			case "fill", "stroke":
			default:
				return fmt.Sprintf("target must be fill or stroke, got: %s", target)
			}
		}

	case "bind_variable_to_node":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if variableId, _ := params["variableId"].(string); variableId == "" {
			return "variableId is required"
		}
		if field, _ := params["field"].(string); field == "" {
			return "field is required"
		}

	case "swap_component":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if componentId, _ := params["componentId"].(string); componentId == "" {
			return "componentId is required"
		}
		if cid, _ := params["componentId"].(string); cid != "" && !ValidNodeID(cid) {
			return fmt.Sprintf("componentId must use colon format e.g. 4029:12345, got: %s", cid)
		}

	case "detach_instance":
		if len(nodeIDs) == 0 {
			return "nodeIds is required and must not be empty"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	// ── Prototype tools ──────────────────────────────────────────────────────

	case "set_effects":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		effects, ok := params["effects"]
		if !ok {
			return "effects array is required"
		}
		effectList, ok := effects.([]interface{})
		if !ok {
			return "effects must be an array"
		}
		for i, e := range effectList {
			em, ok := e.(map[string]interface{})
			if !ok {
				return fmt.Sprintf("effects[%d] must be an object", i)
			}
			t, _ := em["type"].(string)
			switch t {
			case "DROP_SHADOW", "INNER_SHADOW", "LAYER_BLUR", "BACKGROUND_BLUR":
			default:
				return fmt.Sprintf("effects[%d].type must be DROP_SHADOW, INNER_SHADOW, LAYER_BLUR, or BACKGROUND_BLUR, got: %s", i, t)
			}
		}

	case "create_connector":
		startNodeId, _ := params["startNodeId"].(string)
		endNodeId, _ := params["endNodeId"].(string)
		if startNodeId == "" && endNodeId == "" {
			_, hasStartPos := params["startPosition"]
			_, hasEndPos := params["endPosition"]
			if !hasStartPos && !hasEndPos {
				return "at least one of startNodeId, endNodeId, startPosition, or endPosition is required"
			}
		}
		if startNodeId != "" && !ValidNodeID(startNodeId) {
			return fmt.Sprintf("startNodeId must use colon format e.g. 4029:12345, got: %s", startNodeId)
		}
		if endNodeId != "" && !ValidNodeID(endNodeId) {
			return fmt.Sprintf("endNodeId must use colon format e.g. 4029:12345, got: %s", endNodeId)
		}
		if lineType, ok := params["lineType"].(string); ok && lineType != "" {
			if lineType != "STRAIGHT" && lineType != "ELBOW" {
				return fmt.Sprintf("lineType must be STRAIGHT or ELBOW, got: %s", lineType)
			}
		}

	case "set_annotations":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if anns, ok := params["annotations"]; ok {
			if _, isSlice := anns.([]interface{}); !isSlice {
				return "annotations must be an array"
			}
		} else {
			return "annotations array is required"
		}

	case "clear_annotations":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
	}

	return ""
}

var validTriggerTypes = map[string]bool{
	"ON_CLICK": true, "ON_HOVER": true, "ON_PRESS": true, "ON_DRAG": true,
	"AFTER_TIMEOUT": true, "MOUSE_ENTER": true, "MOUSE_LEAVE": true,
	"MOUSE_UP": true, "MOUSE_DOWN": true,
}

var validActionTypes = map[string]bool{
	// Current Figma plugin API action types (plugin-api >= 1.0.0)
	"NODE": true, "BACK": true, "CLOSE": true, "URL": true,
	"CONDITIONAL": true, "SET_VARIABLE": true, "SET_VARIABLE_MODE": true,
	"UPDATE_MEDIA_RUNTIME": true,
}

func validateReaction(idx int, r map[string]any) string {
	if trigger, ok := r["trigger"].(map[string]any); ok {
		if msg := validateTriggerType(idx, trigger); msg != "" {
			return msg
		}
	}
	if action, ok := r["action"].(map[string]any); ok {
		if msg := validateActionType(idx, action); msg != "" {
			return msg
		}
	}
	return ""
}

func validateTriggerType(idx int, trigger map[string]any) string {
	t, _ := trigger["type"].(string)
	if t != "" && !validTriggerTypes[t] {
		return fmt.Sprintf("reactions[%d].trigger.type is invalid: %s", idx, t)
	}
	if t == "AFTER_TIMEOUT" {
		if _, ok := trigger["timeout"].(float64); !ok {
			return fmt.Sprintf("reactions[%d].trigger.timeout is required for AFTER_TIMEOUT and must be a number (milliseconds)", idx)
		}
	}
	return ""
}

func validateActionType(idx int, action map[string]any) string {
	t, _ := action["type"].(string)
	if t != "" && !validActionTypes[t] {
		return fmt.Sprintf("reactions[%d].action.type is invalid: %s", idx, t)
	}
	switch t {
	case "NODE":
		if nav, _ := action["navigation"].(string); nav == "" {
			return fmt.Sprintf("reactions[%d].action.navigation is required for NODE (e.g. NAVIGATE, OVERLAY, SCROLL_TO, SWAP, CHANGE_TO)", idx)
		}
	case "URL":
		if url, _ := action["url"].(string); url == "" {
			return fmt.Sprintf("reactions[%d].action.url is required for URL", idx)
		}
	}
	return ""
}

func validateAutoLayoutParams(params map[string]interface{}) string {
	if lm, ok := params["layoutMode"].(string); ok && lm != "" {
		switch lm {
		case "HORIZONTAL", "VERTICAL", "NONE":
		default:
			return fmt.Sprintf("layoutMode must be HORIZONTAL, VERTICAL, or NONE, got: %s", lm)
		}
	}
	if v, ok := params["primaryAxisAlignItems"].(string); ok && v != "" {
		switch v {
		case "MIN", "CENTER", "MAX", "SPACE_BETWEEN":
		default:
			return fmt.Sprintf("primaryAxisAlignItems must be MIN, CENTER, MAX, or SPACE_BETWEEN, got: %s", v)
		}
	}
	if v, ok := params["counterAxisAlignItems"].(string); ok && v != "" {
		switch v {
		case "MIN", "CENTER", "MAX", "BASELINE":
		default:
			return fmt.Sprintf("counterAxisAlignItems must be MIN, CENTER, MAX, or BASELINE, got: %s", v)
		}
	}
	if v, ok := params["primaryAxisSizingMode"].(string); ok && v != "" {
		switch v {
		case "FIXED", "AUTO":
		default:
			return fmt.Sprintf("primaryAxisSizingMode must be FIXED or AUTO, got: %s", v)
		}
	}
	if v, ok := params["counterAxisSizingMode"].(string); ok && v != "" {
		switch v {
		case "FIXED", "AUTO":
		default:
			return fmt.Sprintf("counterAxisSizingMode must be FIXED or AUTO, got: %s", v)
		}
	}
	if v, ok := params["layoutWrap"].(string); ok && v != "" {
		switch v {
		case "NO_WRAP", "WRAP":
		default:
			return fmt.Sprintf("layoutWrap must be NO_WRAP or WRAP, got: %s", v)
		}
	}
	return ""
}

// blendModeNames are the Figma blend modes, in the order the API documents them.
var blendModeNames = []string{
	"NORMAL", "MULTIPLY", "SCREEN", "OVERLAY",
	"DARKEN", "LIGHTEN", "COLOR_DODGE", "COLOR_BURN",
	"HARD_LIGHT", "SOFT_LIGHT", "DIFFERENCE", "EXCLUSION",
	"HUE", "SATURATION", "COLOR", "LUMINOSITY",
	"PASS_THROUGH",
}

// validateConstraintAxes checks the horizontal/vertical values of a constraints
// object. Shared by set_constraints (flat params) and set_node_properties
// (nested under "constraints"), which is why it takes a plain map.
func validateConstraintAxes(c map[string]interface{}) string {
	for _, axis := range []string{"horizontal", "vertical"} {
		v, ok := c[axis].(string)
		if !ok || v == "" {
			continue
		}
		switch v {
		case "MIN", "MAX", "CENTER", "STRETCH", "SCALE":
		default:
			return fmt.Sprintf("%s must be MIN, MAX, CENTER, STRETCH, or SCALE, got: %s", axis, v)
		}
	}
	return ""
}

func validExportFormat(f string) bool {
	switch f {
	case "PNG", "SVG", "JPG", "PDF":
		return true
	}
	return false
}

type BatchPipelineStep struct {
	ID         string                 `json:"id"`
	Action     string                 `json:"action"`
	Params     map[string]interface{} `json:"params"`
	ExportVars map[string]string      `json:"export_vars,omitempty"`
}

type BatchPipelineRequest struct {
	StopOnError bool                `json:"stop_on_error,omitempty"`
	Steps       []BatchPipelineStep `json:"steps"`
}

type BatchPipelineResponse struct {
	Success        bool                     `json:"success"`
	CompletedSteps int                      `json:"completed_steps"`
	Exports        map[string]interface{}   `json:"exports,omitempty"`
	Results        []map[string]interface{} `json:"results,omitempty"`
	FailedStep     map[string]interface{}   `json:"failed_step,omitempty"`
	Rollback       bool                     `json:"rollback_executed,omitempty"`
}

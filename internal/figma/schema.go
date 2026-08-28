package figma

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

// hexColorPattern matches #RGB, #RGBA, #RRGGBB and #RRGGBBAA, with the leading
// # optional. It mirrors what the plugin's hexToRgb accepts.
var hexColorPattern = regexp.MustCompile(`^#?([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

// ValidHexColor reports whether s is a hex color the plugin can read. Anything
// else — a color name, an rgb() call, a truncated hex — used to reach Figma as
// NaN channels and paint a broken fill without reporting an error.
func ValidHexColor(s string) bool {
	return hexColorPattern.MatchString(s)
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

func ValidateReaction(idx int, r map[string]any) string {
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

// BlendModeNames are the Figma blend modes, in the order the API documents them.
var BlendModeNames = []string{
	"NORMAL", "MULTIPLY", "SCREEN", "OVERLAY",
	"DARKEN", "LIGHTEN", "COLOR_DODGE", "COLOR_BURN",
	"HARD_LIGHT", "SOFT_LIGHT", "DIFFERENCE", "EXCLUSION",
	"HUE", "SATURATION", "COLOR", "LUMINOSITY",
	"PASS_THROUGH",
}

// ValidateConstraintAxes checks the horizontal/vertical values of a constraints
// object, which set_node_properties carries nested under "constraints".
func ValidateConstraintAxes(c map[string]any) string {
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

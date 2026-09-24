package tools

import (
	"github.com/tunglt1810/figma-mcp-go/internal/figma"

	"fmt"
	"strings"
)

// fillModeParam is the shared replace/append switch on the paint tools.
func fillModeParam(desc string) paramSpec {
	return paramSpec{Name: "mode", Kind: kindString, Enum: []string{"replace", "append"}, Desc: desc}
}

// nodePropertyKeys are the properties set_node_properties understands. They are
// all optional and independent; at least one must be supplied.
var nodePropertyKeys = []string{
	"visible", "locked", "opacity", "rotation", "blendMode", "constraints", "order",
	"isMask", "maskType",
	"x", "y", "width", "height",
	"cornerRadius", "topLeftRadius", "topRightRadius", "bottomLeftRadius", "bottomRightRadius",
	"strokeWeight", "strokeAlign", "strokeCap", "strokeJoin", "strokeMiterLimit", "dashPattern",
}

// paintVariants say which arguments belong to which kind of paint. set_fills,
// set_gradient_fills and set_strokes became one tool; without this the
// arguments of the other kinds would be accepted and silently dropped.
var paintVariants = map[string]variantSpec{
	"SOLID":           {Allowed: []string{"color", "opacity"}, Required: []string{"color"}},
	"GRADIENT_LINEAR": {Allowed: []string{"stops", "geometry", "opacity"}, Required: []string{"stops", "geometry"}},
	"GRADIENT_RADIAL": {Allowed: []string{"stops", "geometry", "opacity"}, Required: []string{"stops", "geometry"}},
}

var validNodeOrders = []string{"bringToFront", "sendToBack", "bringForward", "sendBackward"}

var writeModifySpecs = []toolSpec{
	{
		Name:       "set_text",
		Desc:       "Change a TEXT node's text and whole-node settings (resize, truncation, alignment, spacing). To style part of the text use set_text_ranges.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "TEXT node ID",
		Params: []paramSpec{
			// An empty string is a legitimate value here: it clears the node.
			{Name: "text", Kind: kindString, AllowEmpty: true, Desc: "New text"},
			{Name: "textAutoResize", Kind: kindString,
				Enum: []string{"NONE", "WIDTH_AND_HEIGHT", "HEIGHT", "TRUNCATE"},
				Desc: "NONE (fixed), HEIGHT (grow down), WIDTH_AND_HEIGHT (hug), or TRUNCATE"},
			{Name: "textTruncation", Kind: kindString, Enum: []string{"DISABLED", "ENDING"},
				Desc: "ENDING adds … on overflow"},
			{Name: "maxLines", Kind: kindNumber, Min: floatPtr(1), Nullable: true,
				Desc: "Max lines (needs ENDING); null removes"},
			{Name: "paragraphSpacing", Kind: kindNumber, Min: floatPtr(0),
				Desc: "Space between paragraphs"},
			{Name: "paragraphIndent", Kind: kindNumber, Min: floatPtr(0),
				Desc: "First line indent"},
			{Name: "textAlignHorizontal", Kind: kindString,
				Enum: []string{"LEFT", "CENTER", "RIGHT", "JUSTIFIED"},
				Desc: "Horizontal align"},
			{Name: "textAlignVertical", Kind: kindString, Enum: []string{"TOP", "CENTER", "BOTTOM"},
				Desc: "Vertical align"},
		},
		Validate: requireAnyOf(
			"at least one of text, textAutoResize, textTruncation, maxLines, paragraphSpacing, paragraphIndent, textAlignHorizontal, or textAlignVertical is required",
			"text", "textAutoResize", "textTruncation", "maxLines", "paragraphSpacing",
			"paragraphIndent", "textAlignHorizontal", "textAlignVertical"),
	},
	{
		Name:       "set_text_ranges",
		Desc:       "Style parts of a TEXT node (bold word, color, link, list). Each range covers characters [start, end) of the current text. Later ranges win where they overlap. Omitted properties stay the same.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "TEXT node ID",
		Params: []paramSpec{
			{Name: "ranges", Kind: kindObjectArray, Required: true,
				Desc: "Ranges to style; start and end required",
				ItemSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"start":             map[string]any{"type": "number", "description": "Start index, from 0"},
						"end":               map[string]any{"type": "number", "description": "End index (not included)"},
						"fontFamily":        map[string]any{"type": "string", "description": "Font family e.g. 'Inter'"},
						"fontStyle":         map[string]any{"type": "string", "description": "Font style e.g. 'Bold'"},
						"fontSize":          map[string]any{"type": "number", "description": "Font size"},
						"color":             map[string]any{"type": "string", "description": "Color hex"},
						"opacity":           map[string]any{"type": "number", "description": "Color opacity 0-1"},
						"textDecoration":    map[string]any{"type": "string", "enum": []string{"NONE", "UNDERLINE", "STRIKETHROUGH"}, "description": "Decoration"},
						"textCase":          map[string]any{"type": "string", "enum": []string{"ORIGINAL", "UPPER", "LOWER", "TITLE"}, "description": "Case"},
						"letterSpacing":     map[string]any{"type": "number", "description": "Letter spacing"},
						"letterSpacingUnit": map[string]any{"type": "string", "enum": []string{"PIXELS", "PERCENT"}, "description": "Default PIXELS"},
						"lineHeight":        map[string]any{"description": "Number or 'AUTO'"},
						"lineHeightUnit":    map[string]any{"type": "string", "enum": []string{"PIXELS", "PERCENT"}, "description": "Default PIXELS"},
						"listType":          map[string]any{"type": "string", "enum": []string{"NONE", "ORDERED", "UNORDERED"}, "description": "Numbered or bulleted list"},
						"indentation":       map[string]any{"type": "number", "description": "List indent level"},
						"hyperlink":         map[string]any{"description": "Link URL; null removes"},
					},
					"required": []string{"start", "end"},
				}},
		},
	},
	{
		Name: "set_paint",
		Desc: "Set a node's fill or stroke. SOLID: color, opacity. GRADIENT_LINEAR/GRADIENT_RADIAL: stops, geometry, opacity (fill only).",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID",
		Params: []paramSpec{
			{Name: "type", Kind: kindString, Required: true, Enum: variantKinds(paintVariants),
				Desc: "SOLID, GRADIENT_LINEAR, or GRADIENT_RADIAL"},
			{Name: "target", Kind: kindString, Enum: []string{"fill", "stroke"},
				Desc: "fill (default) or stroke"},
			{Name: "color", Kind: kindString, IsHexColor: true,
				Desc: "SOLID: hex #RRGGBB or #RRGGBBAA"},
			{Name: "opacity", Kind: kindNumber,
				Desc: "Opacity 0-1 (default 1), multiplied with hex alpha"},
			{Name: "stops", Kind: kindAny,
				Desc: "GRADIENT: [{position: 0-1, color: hex}]"},
			{Name: "geometry", Kind: kindAny,
				Desc: "GRADIENT: in percentX/percentY. Linear: start, end, angle. Radial: center, radius, rotation."},
			{Name: "strokeWeight", Kind: kindNumber,
				Desc: "Stroke width (default 1), stroke only"},
			fillModeParam("replace (default) or append on top"),
		},
		Validate: func(nodeIDs []string, params map[string]any) string {
			if msg := requireVariant("type", paintVariants, "target", "mode", "strokeWeight")(nodeIDs, params); msg != "" {
				return msg
			}
			kind, _ := params["type"].(string)
			target, _ := params["target"].(string)
			if kind != "SOLID" && target == "stroke" {
				return "gradients can only target fill, not stroke"
			}
			if _, ok := params["strokeWeight"]; ok && target != "stroke" {
				return "strokeWeight applies only when target is stroke"
			}
			// The stops carry colors of their own, one level down from anything
			// a paramSpec can reach.
			if kind != "SOLID" {
				stops, _ := params["stops"].([]any)
				for i, raw := range stops {
					stop, ok := raw.(map[string]any)
					if !ok {
						return fmt.Sprintf("stops[%d] must be an object", i)
					}
					if color, _ := stop["color"].(string); !figma.ValidHexColor(color) {
						return fmt.Sprintf("stops[%d].color must be a hex color e.g. #FF5733, got: %s", i, color)
					}
				}
			}
			return ""
		},
	},
	{
		Name:       "clone_node",
		Desc:       "Copy a node, optionally to a new position or parent.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Source node ID",
		Params: []paramSpec{
			{Name: "x", Kind: kindNumber, Desc: "X of the copy"},
			{Name: "y", Kind: kindNumber, Desc: "Y of the copy"},
			parentIDParam("Parent (default: same as source)"),
		},
	},
	{
		Name:       "set_layout_grids",
		Desc:       "Set layout grids (columns, rows, or square grid) on frames. [] removes them.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Frame, component, or section IDs",
		Params: []paramSpec{
			{Name: "grids", Kind: kindObjectArray, Required: true, AllowEmpty: true,
				Desc: "Grids; [] removes all",
				ItemSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern":     map[string]any{"type": "string", "enum": []string{"COLUMNS", "ROWS", "GRID"}, "description": "Default GRID"},
						"count":       map[string]any{"type": "number", "description": "Count (default 12)"},
						"gutterSize":  map[string]any{"type": "number", "description": "Gutter (default 16)"},
						"offset":      map[string]any{"type": "number", "description": "Margin (default 0)"},
						"alignment":   map[string]any{"type": "string", "enum": []string{"MIN", "MAX", "CENTER", "STRETCH"}, "description": "Default STRETCH"},
						"sectionSize": map[string]any{"type": "number", "description": "GRID cell size (default 8)"},
						"color":       map[string]any{"type": "string", "description": "Hex (default #FF0000)"},
						"opacity":     map[string]any{"type": "number", "description": "Default 0.1"},
						"visible":     map[string]any{"type": "boolean", "description": "Default true"},
					},
				}},
			{Name: "mode", Kind: kindString, Enum: []string{"replace", "append"},
				Desc: "replace (default) or append"},
		},
	},
	{
		Name: "set_auto_layout",
		Desc: "Set auto layout (flexbox) on frames, components or instances: direction, padding, gap, align, and sizing (HUG/FILL via layoutSizing*). " +
			"layoutPositioning, layoutAlign and layoutGrow set how a node sits in its parent. Each node reports its own result.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Frame, component, or instance IDs",
		Params:     autoLayoutParams(),
	},
	{
		Name: "set_node_properties",
		Desc: "Set any node properties: position, size, radius, visibility, lock, opacity, rotation, blend, constraints, z-order, mask, stroke. " +
			"Pass only what to change. Each node reports what was applied.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs",
		Params: []paramSpec{
			{Name: "x", Kind: kindNumber,
				Desc: "Absolute X (not an offset)"},
			{Name: "y", Kind: kindNumber,
				Desc: "Absolute Y (not an offset)"},
			{Name: "width", Kind: kindNumber, Desc: "Width"},
			{Name: "height", Kind: kindNumber, Desc: "Height"},
			{Name: "cornerRadius", Kind: kindNumber, Desc: "Radius for all corners"},
			{Name: "topLeftRadius", Kind: kindNumber, Desc: "Top left radius"},
			{Name: "topRightRadius", Kind: kindNumber, Desc: "Top right radius"},
			{Name: "bottomLeftRadius", Kind: kindNumber, Desc: "Bottom left radius"},
			{Name: "bottomRightRadius", Kind: kindNumber, Desc: "Bottom right radius"},
			{Name: "visible", Kind: kindBool, Desc: "Show or hide"},
			{Name: "locked", Kind: kindBool, Desc: "Lock or unlock"},
			{Name: "opacity", Kind: kindNumber, Min: floatPtr(0), Max: floatPtr(1),
				Desc: "Opacity 0-1"},
			{Name: "rotation", Kind: kindNumber, Desc: "Rotation in degrees"},
			{Name: "blendMode", Kind: kindString, Enum: figma.BlendModeNames,
				Desc: "e.g. NORMAL, MULTIPLY, SCREEN"},
			{Name: "constraints", Kind: kindObject,
				Desc: "{horizontal, vertical}: MIN, MAX, CENTER, STRETCH, or SCALE"},
			{Name: "order", Kind: kindString, Enum: validNodeOrders,
				Desc: "Layer order: bringToFront, sendToBack, bringForward, sendBackward"},
			{Name: "isMask", Kind: kindBool,
				Desc: "Use as mask for the layers above it"},
			{Name: "maskType", Kind: kindString, Enum: []string{"ALPHA", "VECTOR", "LUMINANCE"},
				Desc: "ALPHA, VECTOR, or LUMINANCE"},
			{Name: "strokeWeight", Kind: kindNumber, Min: floatPtr(0),
				Desc: "Stroke width"},
			{Name: "strokeAlign", Kind: kindString, Enum: []string{"INSIDE", "OUTSIDE", "CENTER"},
				Desc: "Stroke position"},
			{Name: "strokeCap", Kind: kindString, Enum: figma.StrokeCapNames,
				Desc: "Line ends: NONE, ROUND, SQUARE, ARROW_LINES, ARROW_EQUILATERAL"},
			{Name: "strokeJoin", Kind: kindString, Enum: []string{"MITER", "BEVEL", "ROUND"},
				Desc: "Corner style"},
			{Name: "strokeMiterLimit", Kind: kindNumber, Min: floatPtr(1),
				Desc: "Miter limit (default 4)"},
			{Name: "dashPattern", Kind: kindNumberArray,
				Desc: "Dash and gap e.g. [4, 2]; [] = solid"},
		},
		Validate: func(_ []string, params map[string]any) string {
			supplied := false
			for _, key := range nodePropertyKeys {
				if _, ok := params[key]; ok {
					supplied = true
					break
				}
			}
			if !supplied {
				return "at least one of " + strings.Join(nodePropertyKeys, ", ") + " is required"
			}
			if c, ok := params["constraints"].(map[string]any); ok {
				return figma.ValidateConstraintAxes(c)
			}
			return ""
		},
	},
	{
		Name:       "delete_nodes",
		Desc:       "Delete nodes. Cannot be undone from here.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs to delete",
	},
	{
		Name:       "reparent_nodes",
		Desc:       "Move nodes into another frame, group, or section.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs to move",
		Params: []paramSpec{
			{Name: "parentId", Kind: kindString, Required: true, IsNodeID: true,
				Desc: "New parent ID"},
		},
	},
	{
		Name: "batch_rename_nodes",
		Desc: "Rename nodes: set `name`, or use find/replace (regex allowed) or prefix/suffix. `name` cannot mix with the others.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs",
		Params: []paramSpec{
			{Name: "name", Kind: kindString,
				Desc: "New name for all nodes; slashes group e.g. 'Icons/Arrow'"},
			{Name: "find", Kind: kindString,
				Desc: "Text or regex to find in names"},
			{Name: "replace", Kind: kindString, AllowEmpty: true,
				Desc: "Replacement; required with find"},
			{Name: "useRegex", Kind: kindBool, Desc: "find is a regex (default false)"},
			{Name: "regexFlags", Kind: kindString, Desc: "Regex flags (default 'g')"},
			{Name: "prefix", Kind: kindString, Desc: "Add to start of name"},
			{Name: "suffix", Kind: kindString, Desc: "Add to end of name"},
		},
		Validate: func(_ []string, params map[string]any) string {
			_, hasName := params["name"]
			_, hasFind := params["find"]
			_, hasReplace := params["replace"]
			_, hasPrefix := params["prefix"]
			_, hasSuffix := params["suffix"]
			if !hasName && !hasFind && !hasReplace && !hasPrefix && !hasSuffix {
				return "at least one of name, find/replace, prefix, or suffix is required"
			}
			// Silently picking one is exactly the failure this validation exists
			// to prevent: the caller would get a name it did not ask for.
			if hasName && (hasFind || hasReplace || hasPrefix || hasSuffix) {
				return "name sets the name outright and cannot be combined with find/replace, prefix, or suffix"
			}
			if hasFind && !hasReplace {
				return "replace is required when find is provided"
			}
			return ""
		},
	},
	{
		Name:       "find_replace_text",
		Desc:       "Find and replace text in TEXT nodes, on the current page or inside nodeId.",
		NodeIDs:    nodeIDsSingle,
		NodeIDDesc: "Search inside this node (default: current page)",
		Params: []paramSpec{
			{Name: "find", Kind: kindString, Required: true,
				Desc: "Text or regex to find"},
			{Name: "replace", Kind: kindString, Required: true, AllowEmpty: true,
				Desc: "Replacement; \"\" deletes"},
			{Name: "useRegex", Kind: kindBool, Desc: "find is a regex (default false)"},
			{Name: "regexFlags", Kind: kindString, Desc: "Regex flags (default 'g')"},
		},
	},
}

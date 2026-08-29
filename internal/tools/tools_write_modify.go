package tools

import (
	"github.com/tunglt1810/figma-mcp-go/internal/figma"

	"fmt"
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
		Desc:       "Update the text content of an existing TEXT node, and the settings that apply to the node as a whole — wrapping, truncation, alignment, and paragraph spacing. For styling that varies across the text (a bold word, a link, a bulleted list), use set_text_ranges.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "TEXT node ID in colon format e.g. '4029:12345'",
		Params: []paramSpec{
			// An empty string is a legitimate value here: it clears the node.
			{Name: "text", Kind: kindString, AllowEmpty: true, Desc: "New text content"},
			{Name: "textAutoResize", Kind: kindString,
				Enum: []string{"NONE", "WIDTH_AND_HEIGHT", "HEIGHT", "TRUNCATE"},
				Desc: "How the box sizes to its text: NONE (fixed), HEIGHT (grow down), WIDTH_AND_HEIGHT (hug), or TRUNCATE"},
			{Name: "textTruncation", Kind: kindString, Enum: []string{"DISABLED", "ENDING"},
				Desc: "ENDING adds an ellipsis when the text overflows"},
			{Name: "maxLines", Kind: kindNumber, Min: floatPtr(1), Nullable: true,
				Desc: "Cap the text at this many lines (needs textTruncation ENDING); pass null to remove the cap"},
			{Name: "paragraphSpacing", Kind: kindNumber, Min: floatPtr(0),
				Desc: "Space between paragraphs in pixels"},
			{Name: "paragraphIndent", Kind: kindNumber, Min: floatPtr(0),
				Desc: "First-line indent in pixels"},
			{Name: "textAlignHorizontal", Kind: kindString,
				Enum: []string{"LEFT", "CENTER", "RIGHT", "JUSTIFIED"},
				Desc: "Horizontal text alignment"},
			{Name: "textAlignVertical", Kind: kindString, Enum: []string{"TOP", "CENTER", "BOTTOM"},
				Desc: "Vertical text alignment within the box"},
		},
		Validate: requireAnyOf(
			"at least one of text, textAutoResize, textTruncation, maxLines, paragraphSpacing, paragraphIndent, textAlignHorizontal, or textAlignVertical is required",
			"text", "textAutoResize", "textTruncation", "maxLines", "paragraphSpacing",
			"paragraphIndent", "textAlignHorizontal", "textAlignVertical"),
	},
	{
		Name:       "set_text_ranges",
		Desc:       "Style parts of a TEXT node independently: a bold word, a coloured phrase, a hyperlink, a bulleted list. Each range is a half-open character span [start, end) over the node's existing text, so call set_text first to put the text there. Ranges may overlap; they are applied in text order, so a later one wins where they meet. Omit a property to leave it as it is.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "TEXT node ID in colon format e.g. '4029:12345'",
		Params: []paramSpec{
			{Name: "ranges", Kind: kindObjectArray, Required: true,
				Desc: "Character ranges to style. Each needs start and end; every other property is optional.",
				ItemSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"start":             map[string]any{"type": "number", "description": "First character index, from 0"},
						"end":               map[string]any{"type": "number", "description": "One past the last character index"},
						"fontFamily":        map[string]any{"type": "string", "description": "Font family e.g. 'Inter'. With fontStyle omitted, the range keeps its current style."},
						"fontStyle":         map[string]any{"type": "string", "description": "Font style e.g. 'Bold', 'Italic'. With fontFamily omitted, the range keeps its current family."},
						"fontSize":          map[string]any{"type": "number", "description": "Font size in pixels"},
						"color":             map[string]any{"type": "string", "description": "Text colour as hex e.g. '#FF5733'"},
						"opacity":           map[string]any{"type": "number", "description": "Colour opacity from 0 to 1"},
						"textDecoration":    map[string]any{"type": "string", "enum": []string{"NONE", "UNDERLINE", "STRIKETHROUGH"}, "description": "Underline or strikethrough"},
						"textCase":          map[string]any{"type": "string", "enum": []string{"ORIGINAL", "UPPER", "LOWER", "TITLE"}, "description": "Letter casing"},
						"letterSpacing":     map[string]any{"type": "number", "description": "Letter spacing"},
						"letterSpacingUnit": map[string]any{"type": "string", "enum": []string{"PIXELS", "PERCENT"}, "description": "Unit for letterSpacing (default PIXELS)"},
						"lineHeight":        map[string]any{"description": "Line height as a number, or the string 'AUTO'"},
						"lineHeightUnit":    map[string]any{"type": "string", "enum": []string{"PIXELS", "PERCENT"}, "description": "Unit for a numeric lineHeight (default PIXELS)"},
						"listType":          map[string]any{"type": "string", "enum": []string{"NONE", "ORDERED", "UNORDERED"}, "description": "Turn the range into a numbered or bulleted list"},
						"indentation":       map[string]any{"type": "number", "description": "List indent level"},
						"hyperlink":         map[string]any{"description": "URL to link the range to; pass null to remove an existing link"},
					},
					"required": []string{"start", "end"},
				}},
		},
	},
	{
		Name: "set_paint",
		Desc: "Paint a node's fill or stroke. `type` selects the kind of paint and each takes its own arguments — " +
			"SOLID: color, opacity. " +
			"GRADIENT_LINEAR / GRADIENT_RADIAL: stops, geometry, opacity. " +
			"`target` chooses fill (default) or stroke; gradients can only target fill. " +
			"An argument belonging to a different kind is rejected rather than ignored.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID in colon format e.g. '4029:12345'",
		Params: []paramSpec{
			{Name: "type", Kind: kindString, Required: true, Enum: variantKinds(paintVariants),
				Desc: "Kind of paint: SOLID, GRADIENT_LINEAR, or GRADIENT_RADIAL"},
			{Name: "target", Kind: kindString, Enum: []string{"fill", "stroke"},
				Desc: "What to paint: 'fill' (default) or 'stroke'"},
			{Name: "color", Kind: kindString, IsHexColor: true,
				Desc: "SOLID: color as hex — #RRGGBB e.g. #FF5733, or #RRGGBBAA e.g. #FF573380 for 50% alpha (required)"},
			{Name: "opacity", Kind: kindNumber,
				Desc: "Paint opacity 0–1 (default 1). For SOLID it combines multiplicatively with any alpha in the color hex; for gradients it scales the whole gradient."},
			{Name: "stops", Kind: kindAny,
				Desc: "GRADIENT: array of color stops e.g. [{position: 0, color: '#ff0000'}, {position: 1, color: '#00ff00'}] (required)"},
			{Name: "geometry", Kind: kindAny,
				Desc: "GRADIENT: coordinates in percentX/Y — start, end, angle for linear; center, radius, rotation for radial (required)"},
			{Name: "strokeWeight", Kind: kindNumber,
				Desc: "Stroke weight in pixels (default 1). Only when target is stroke."},
			fillModeParam("'replace' (default) overwrites the existing paints; 'append' stacks this one on top"),
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
		Name:       "move_nodes",
		Desc:       "Move one or more nodes to an absolute canvas position. The same x/y is applied to every node independently (not a relative offset from current position).",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs in colon format e.g. ['4029:12345']",
		Params: []paramSpec{
			{Name: "x", Kind: kindNumber, Desc: "Target X position"},
			{Name: "y", Kind: kindNumber, Desc: "Target Y position"},
		},
		Validate: requireAnyOf("at least one of x or y is required", "x", "y"),
	},
	{
		Name:       "resize_nodes",
		Desc:       "Resize one or more nodes. The same width/height is applied to every node in the list independently. Provide width, height, or both.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs in colon format e.g. ['4029:12345']",
		Params: []paramSpec{
			{Name: "width", Kind: kindNumber, Desc: "New width in pixels"},
			{Name: "height", Kind: kindNumber, Desc: "New height in pixels"},
		},
		Validate: requireAnyOf("at least one of width or height is required", "width", "height"),
	},
	{
		Name:       "rename_node",
		Desc:       "Rename a single node by ID. Returns the updated node with its new name. Use batch_rename_nodes to rename multiple nodes at once or to apply find/replace patterns across many nodes.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID in colon format e.g. '4029:12345'",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Required: true,
				Desc: "New name for the node. Figma supports slash-separated path notation e.g. 'Icons/Arrow/Left' to organise nodes in component panels."},
		},
	},
	{
		Name:       "clone_node",
		Desc:       "Clone an existing node, optionally repositioning it or placing it in a new parent.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Source node ID in colon format e.g. '4029:12345'",
		Params: []paramSpec{
			{Name: "x", Kind: kindNumber, Desc: "X position of the clone"},
			{Name: "y", Kind: kindNumber, Desc: "Y position of the clone"},
			parentIDParam("Parent node ID for the clone. Defaults to same parent as source."),
		},
	},
	{
		Name:       "set_corner_radius",
		Desc:       "Set corner radius on one or more nodes. Provide a uniform cornerRadius or individual per-corner values.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs in colon format e.g. ['4029:12345']",
		Params: []paramSpec{
			{Name: "cornerRadius", Kind: kindNumber, Desc: "Uniform corner radius applied to all corners"},
			{Name: "topLeftRadius", Kind: kindNumber, Desc: "Top-left corner radius"},
			{Name: "topRightRadius", Kind: kindNumber, Desc: "Top-right corner radius"},
			{Name: "bottomLeftRadius", Kind: kindNumber, Desc: "Bottom-left corner radius"},
			{Name: "bottomRightRadius", Kind: kindNumber, Desc: "Bottom-right corner radius"},
		},
		Validate: requireAnyOf(
			"at least one of cornerRadius, topLeftRadius, topRightRadius, bottomLeftRadius, or bottomRightRadius is required",
			"cornerRadius", "topLeftRadius", "topRightRadius", "bottomLeftRadius", "bottomRightRadius"),
	},
	{
		Name:       "set_layout_grids",
		Desc:       "Set the layout grids drawn over a frame — columns, rows, or a square grid. These are the guides a layout is built against, distinct from a saved grid style. Pass an empty grids array to remove the grids a frame already has.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Frame, component, or section node IDs in colon format e.g. ['4029:12345']",
		Params: []paramSpec{
			{Name: "grids", Kind: kindObjectArray, Required: true, AllowEmpty: true,
				Desc: "Grids to draw. Empty removes every grid on the node.",
				ItemSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern":     map[string]any{"type": "string", "enum": []string{"COLUMNS", "ROWS", "GRID"}, "description": "Grid kind (default GRID)"},
						"count":       map[string]any{"type": "number", "description": "COLUMNS/ROWS: how many (default 12)"},
						"gutterSize":  map[string]any{"type": "number", "description": "COLUMNS/ROWS: gap between them (default 16)"},
						"offset":      map[string]any{"type": "number", "description": "COLUMNS/ROWS: margin from the edge (default 0)"},
						"alignment":   map[string]any{"type": "string", "enum": []string{"MIN", "MAX", "CENTER", "STRETCH"}, "description": "COLUMNS/ROWS: how they sit in the frame (default STRETCH)"},
						"sectionSize": map[string]any{"type": "number", "description": "GRID: square size in pixels (default 8)"},
						"color":       map[string]any{"type": "string", "description": "GRID: overlay colour as hex (default #FF0000)"},
						"opacity":     map[string]any{"type": "number", "description": "GRID: overlay opacity (default 0.1)"},
						"visible":     map[string]any{"type": "boolean", "description": "Whether the grid is shown (default true)"},
					},
				}},
			{Name: "mode", Kind: kindString, Enum: []string{"replace", "append"},
				Desc: "replace swaps the node's grids for these (default); append adds them"},
		},
	},
	{
		Name:       "set_auto_layout",
		Desc:       "Set or update auto-layout (flex) properties on an existing frame, component, component set, or instance. Covers the frame's own layout (direction, padding, gap, alignment) and how it sizes itself — use layoutSizingHorizontal/layoutSizingVertical for HUG and FILL, with minWidth/maxWidth/minHeight/maxHeight to bound them. layoutPositioning, layoutAlign, and layoutGrow describe how this node sits inside its parent's auto layout instead.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Frame, component, component set, or instance node ID in colon format e.g. '4029:12345'",
		Params:     autoLayoutParams(),
	},
	{
		Name: "set_layout_sizing",
		Desc: "Set how nodes size themselves inside auto layout, across several nodes at once. " +
			"layoutSizingHorizontal/layoutSizingVertical give FIXED, HUG, or FILL; minWidth/maxWidth/minHeight/maxHeight bound them; " +
			"layoutAlign, layoutGrow, and layoutPositioning describe how the node sits in its PARENT's auto layout. " +
			"set_auto_layout does the same for one node alongside the frame's own layout — use this one when the same sizing goes on a row of siblings.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs in colon format e.g. ['4029:12345', '4029:67890']",
		Params:     layoutSizingParams(),
		Validate: requireAnyOf(
			"at least one of layoutSizingHorizontal, layoutSizingVertical, minWidth, maxWidth, minHeight, maxHeight, layoutAlign, layoutGrow, or layoutPositioning is required",
			layoutSizingParamNames...,
		),
	},
	{
		Name:       "set_node_properties",
		Desc:       "Set one or more display properties on nodes in a single call: visibility, lock state, opacity, rotation, blend mode, constraints, z-order, and masking. Every property is optional and independent — supply only the ones you want to change. Each node reports which properties were applied; a property the node type does not support is reported against that property alone, leaving the others applied.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs in colon format e.g. ['4029:12345']",
		Params: []paramSpec{
			{Name: "visible", Kind: kindBool, Desc: "Show (true) or hide (false) the nodes"},
			{Name: "locked", Kind: kindBool, Desc: "Lock (true) or unlock (false) the nodes against accidental edits"},
			{Name: "opacity", Kind: kindNumber, Min: floatPtr(0), Max: floatPtr(1),
				Desc: "Opacity from 0 (transparent) to 1 (opaque)"},
			{Name: "rotation", Kind: kindNumber, Desc: "Absolute rotation in degrees"},
			{Name: "blendMode", Kind: kindString, Enum: figma.BlendModeNames,
				Desc: "Blend mode e.g. NORMAL, MULTIPLY, SCREEN, OVERLAY, LUMINOSITY"},
			{Name: "constraints", Kind: kindObject,
				Desc: "Responsive constraints {horizontal, vertical}, each MIN, MAX, CENTER, STRETCH, or SCALE. Axes you omit keep their current value."},
			{Name: "order", Kind: kindString, Enum: validNodeOrders,
				Desc: "Change z-order: bringToFront, sendToBack, bringForward, or sendBackward"},
			{Name: "isMask", Kind: kindBool,
				Desc: "Turn the node into a mask for its later siblings (true) or back into an ordinary layer (false)"},
			{Name: "maskType", Kind: kindString, Enum: []string{"ALPHA", "VECTOR", "LUMINANCE"},
				Desc: "How the mask is read: ALPHA uses opacity, VECTOR the outline, LUMINANCE the brightness"},
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
				return "at least one of visible, locked, opacity, rotation, blendMode, constraints, order, isMask, or maskType is required"
			}
			if c, ok := params["constraints"].(map[string]any); ok {
				return figma.ValidateConstraintAxes(c)
			}
			return ""
		},
	},
	{
		Name:       "delete_nodes",
		Desc:       "Delete one or more nodes. This cannot be undone via MCP — use with care.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs to delete in colon format e.g. ['4029:12345']",
	},
	{
		Name:       "reparent_nodes",
		Desc:       "Move one or more nodes to a different parent frame, group, or section.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs to move in colon format e.g. ['4029:12345']",
		Params: []paramSpec{
			{Name: "parentId", Kind: kindString, Required: true, IsNodeID: true,
				Desc: "Target parent node ID in colon format e.g. '4029:99'"},
		},
	},
	{
		Name:       "batch_rename_nodes",
		Desc:       "Rename multiple nodes using find/replace, regex substitution, or prefix/suffix addition.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs in colon format e.g. ['4029:12345']",
		Params: []paramSpec{
			{Name: "find", Kind: kindString,
				Desc: "String (or regex pattern when useRegex=true) to search for in the node name"},
			{Name: "replace", Kind: kindString, AllowEmpty: true,
				Desc: "Replacement string. Required when find is provided."},
			{Name: "useRegex", Kind: kindBool, Desc: "Treat find as a regular expression (default false)"},
			{Name: "regexFlags", Kind: kindString, Desc: "Regex flags e.g. 'gi' (default 'g'). Only used when useRegex=true."},
			{Name: "prefix", Kind: kindString, Desc: "String to prepend to the node name"},
			{Name: "suffix", Kind: kindString, Desc: "String to append to the node name"},
		},
		Validate: func(_ []string, params map[string]any) string {
			_, hasFind := params["find"]
			_, hasReplace := params["replace"]
			_, hasPrefix := params["prefix"]
			_, hasSuffix := params["suffix"]
			if !hasFind && !hasReplace && !hasPrefix && !hasSuffix {
				return "at least one of find/replace, prefix, or suffix is required"
			}
			if hasFind && !hasReplace {
				return "replace is required when find is provided"
			}
			return ""
		},
	},
	{
		Name:       "find_replace_text",
		Desc:       "Find and replace text content across all TEXT nodes in a subtree. Searches the entire current page if no nodeId is given.",
		NodeIDs:    nodeIDsSingle,
		NodeIDDesc: "Root node ID to scope the search. Defaults to the entire current page.",
		Params: []paramSpec{
			{Name: "find", Kind: kindString, Required: true,
				Desc: "Text string (or regex pattern when useRegex=true) to search for"},
			{Name: "replace", Kind: kindString, Required: true, AllowEmpty: true,
				Desc: "Replacement string (use empty string to delete matches)"},
			{Name: "useRegex", Kind: kindBool, Desc: "Treat find as a regular expression (default false)"},
			{Name: "regexFlags", Kind: kindString, Desc: "Regex flags e.g. 'gi' (default 'g'). Only used when useRegex=true."},
		},
	},
}

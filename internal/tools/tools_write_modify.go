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
		Desc:       "Update the text content of an existing TEXT node.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "TEXT node ID in colon format e.g. '4029:12345'",
		Params: []paramSpec{
			// An empty string is a legitimate value here: it clears the node.
			{Name: "text", Kind: kindString, Required: true, AllowEmpty: true, Desc: "New text content"},
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
		Name:       "set_auto_layout",
		Desc:       "Set or update auto-layout (flex) properties on an existing frame.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Frame node ID in colon format e.g. '4029:12345'",
		Params:     autoLayoutParams(),
	},
	{
		Name:       "set_node_properties",
		Desc:       "Set one or more display properties on nodes in a single call: visibility, lock state, opacity, rotation, blend mode, constraints, and z-order. Every property is optional and independent — supply only the ones you want to change. Each node reports which properties were applied; a property the node type does not support is reported against that property alone, leaving the others applied.",
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
				return "at least one of visible, locked, opacity, rotation, blendMode, constraints, or order is required"
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

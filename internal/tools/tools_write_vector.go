package tools

// Vector and boolean geometry: the tools an icon needs.
//
// Each of these uses up the shapes it takes and returns one new node. So
// batch_execute_pipeline's rollback cannot undo them: the log can remove a
// node it created, but cannot bring back shapes Figma has already merged. The
// tool descriptions say this, and point to save_version_checkpoint.

var writeVectorSpecs = []toolSpec{
	{
		Name:       "boolean_operation",
		Desc:       "Combine 2+ shapes (same parent): UNION, SUBTRACT (cut the rest from the first), INTERSECT, or EXCLUDE. Pipeline rollback cannot undo it.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Shape IDs. Order matters for SUBTRACT.",
		Params: []paramSpec{
			{Name: "operation", Kind: kindString, Required: true,
				Enum: []string{"UNION", "SUBTRACT", "INTERSECT", "EXCLUDE"},
				Desc: "Operation"},
			{Name: "name", Kind: kindString, Desc: "Result name"},
		},
	},
	{
		Name:       "flatten_nodes",
		Desc:       "Merge nodes (same parent) into one vector. Pipeline rollback cannot undo it.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Desc: "Result name"},
		},
	},
	{
		Name:       "outline_stroke",
		Desc:       "Turn each node's stroke into a filled shape. Nodes without a stroke go in `skipped`.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs",
	},
	{
		Name: "create_vector",
		Desc: "Create a vector from SVG markup (e.g. an icon). Several paths become a frame of vectors.",

		Params: append([]paramSpec{
			{Name: "svg", Kind: kindString, Required: true,
				Desc: "SVG markup"},
		}, append(positionParams(),
			paramSpec{Name: "width", Kind: kindNumber, Positive: true, Desc: "Width"},
			paramSpec{Name: "height", Kind: kindNumber, Positive: true, Desc: "Height"},
			paramSpec{Name: "fillColor", Kind: kindString, IsHexColor: true,
				Desc: "Fill color hex"},
			paramSpec{Name: "parentId", Kind: kindString, IsNodeID: true,
				Desc: "Parent ID (default: current page)"},
		)...),
		Validate: func(_ []string, params map[string]any) string {
			// Figma's resize takes both dimensions. One alone would do nothing,
			// which looks like the tool ignoring the argument.
			_, hasWidth := params["width"]
			_, hasHeight := params["height"]
			if hasWidth != hasHeight {
				return "width and height must be given together"
			}
			return ""
		},
	},
}

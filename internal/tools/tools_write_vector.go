package tools

// Vector and boolean geometry — the tools an icon needs.
//
// Every one of these consumes the shapes it takes and returns one new node, so
// batch_execute_pipeline's rollback cannot reverse them: the log can remove a
// node it created, not put back shapes Figma has already merged. The
// descriptions say so, and point at save_version_checkpoint.

var writeVectorSpecs = []toolSpec{
	{
		Name:       "boolean_operation",
		Desc:       "Combine two or more shapes into one: UNION merges them, SUBTRACT cuts the later shapes out of the first, INTERSECT keeps the overlap, EXCLUDE keeps everything but the overlap. Node order is meaningful for SUBTRACT and EXCLUDE. All nodes must share a parent. This consumes the shapes and cannot be rolled back by a pipeline — call save_version_checkpoint first if the shapes matter.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Shape node IDs in colon format e.g. ['4029:12345', '4029:12346']; for SUBTRACT and EXCLUDE the first is the shape the others are cut from",
		Params: []paramSpec{
			{Name: "operation", Kind: kindString, Required: true,
				Enum: []string{"UNION", "SUBTRACT", "INTERSECT", "EXCLUDE"},
				Desc: "Which boolean operation to apply"},
			{Name: "name", Kind: kindString, Desc: "Name for the resulting node"},
		},
	},
	{
		Name:       "flatten_nodes",
		Desc:       "Flatten nodes into a single vector, merging their geometry and discarding the layer structure. Use it to simplify a finished icon. All nodes must share a parent. This consumes the nodes and cannot be rolled back by a pipeline.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs to flatten in colon format e.g. ['4029:12345']",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Desc: "Name for the resulting vector"},
		},
	},
	{
		Name:       "outline_stroke",
		Desc:       "Convert each node's stroke into a filled vector, so the outline can be edited as a shape. Nodes with no visible stroke are reported under `skipped` rather than failing the call.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs whose strokes to outline, in colon format e.g. ['4029:12345']",
	},
	{
		Name: "create_vector",
		Desc: "Create a vector node from SVG markup — the way to bring in an icon. An SVG with a single path becomes the vector itself; one with several becomes a frame containing them. Returns the created node ID and bounds.",
		Params: append([]paramSpec{
			{Name: "svg", Kind: kindString, Required: true,
				Desc: "SVG markup e.g. '<svg viewBox=\"0 0 24 24\"><path d=\"M12 2 L22 22 L2 22 Z\"/></svg>'"},
		}, append(positionParams(),
			paramSpec{Name: "width", Kind: kindNumber, Positive: true, Desc: "Resize the result to this width"},
			paramSpec{Name: "height", Kind: kindNumber, Positive: true, Desc: "Resize the result to this height"},
			paramSpec{Name: "fillColor", Kind: kindString, IsHexColor: true,
				Desc: "Override the fill colour as hex e.g. '#FF5733'"},
			paramSpec{Name: "parentId", Kind: kindString, IsNodeID: true,
				Desc: "Parent node ID to insert into, colon format e.g. '4029:99' (default: current page)"},
		)...),
		Validate: func(_ []string, params map[string]any) string {
			// Figma's resize takes both dimensions; one alone would silently do
			// nothing, which reads as the tool ignoring the argument.
			_, hasWidth := params["width"]
			_, hasHeight := params["height"]
			if hasWidth != hasHeight {
				return "width and height must be given together"
			}
			return ""
		},
	},
}

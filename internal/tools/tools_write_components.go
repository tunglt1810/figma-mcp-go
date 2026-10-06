package tools

var writeComponentSpecs = []toolSpec{
	{
		Name:       "group_nodes",
		Desc:       "Group 2+ nodes that share a parent.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		MinNodeIDs: 2,
		NodeIDDesc: "Node IDs (2+)",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Desc: "Group name"},
		},
	},
	{
		Name:       "ungroup_nodes",
		Desc:       "Ungroup GROUP nodes. The children move to the parent.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "GROUP node IDs",
	},
	{
		Name:       "swap_component",
		Desc:       "Swap an instance to another component, keeping position and size.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "INSTANCE node ID",
		Params: []paramSpec{
			{Name: "componentId", Kind: kindString, IsNodeID: true,
				Desc: "Local component or set ID (preferred)"},
			{Name: "componentKey", Kind: kindString,
				Desc: "Library component or set key"},
		},
		Validate: requireAnyOf("componentId or componentKey is required", "componentId", "componentKey"),
	},
	{
		Name:       "detach_instance",
		Desc:       "Detach instances into plain frames (same look, no component link).",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "INSTANCE node IDs",
	},
	{
		Name: "create_component_instance",
		Desc: "Create an instance of a local (componentId) or library (componentKey) component. A component set uses its default variant.",
		Params: []paramSpec{
			{Name: "componentId", Kind: kindString, IsNodeID: true,
				Desc: "Local component or set ID (preferred)"},
			{Name: "componentKey", Kind: kindString,
				Desc: "Library component or set key"},
			parentIDParam("Parent ID (default: current page)"),
			{Name: "x", Kind: kindNumber, Desc: "X (default: center of view)"},
			{Name: "y", Kind: kindNumber, Desc: "Y"},
		},
		Validate: func(nodeIDs []string, params map[string]any) string {
			if msg := requireAnyOf("componentId or componentKey is required", "componentId", "componentKey")(nodeIDs, params); msg != "" {
				return msg
			}
			// The plugin sets a position only when it has both values. One
			// alone used to be dropped without a word.
			_, hasX := params["x"]
			_, hasY := params["y"]
			if hasX != hasY {
				return "set x and y together, or neither"
			}
			return ""
		},
	},
	{
		Name: "set_instance_overrides",
		Desc: "Set an instance's component properties. The tool fails on unknown names or wrong types.",

		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "INSTANCE node ID",
		Params: []paramSpec{
			{Name: "properties", Kind: kindObject, Required: true,
				Desc: "Name to value e.g. {\"Size\": \"Large\", \"Show Icon\": true}"},
		},
	},
	{
		Name: "create_connector",
		Desc: "Create a connector line. This tool operates only in a FigJam file.",
		Params: []paramSpec{
			{Name: "startNodeId", Kind: kindString, IsNodeID: true,
				Desc: "Start node ID"},
			{Name: "endNodeId", Kind: kindString, IsNodeID: true,
				Desc: "End node ID"},
			{Name: "startPosition", Kind: kindObject, Desc: "Start {x, y}"},
			{Name: "endPosition", Kind: kindObject, Desc: "End {x, y}"},
			{Name: "lineType", Kind: kindString, Enum: []string{"STRAIGHT", "ELBOW"},
				Desc: "STRAIGHT or ELBOW"},
		},
		Validate: requireAnyOf(
			"at least one of startNodeId, endNodeId, startPosition, or endPosition is required",
			"startNodeId", "endNodeId", "startPosition", "endPosition"),
	},
	{
		Name:       "set_annotations",
		Desc:       "Set Dev Mode annotations on nodes ([] clears them). The tool needs a paid Dev Mode seat.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs",
		Params: []paramSpec{
			{Name: "annotations", Kind: kindArray, Required: true, AllowEmpty: true,
				Desc: "e.g. [{\"label\": \"Main Button\"}]. [] clears them."},
		},
	},
}

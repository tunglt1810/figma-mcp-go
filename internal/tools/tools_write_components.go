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
		Desc:       "Ungroup GROUP nodes; children move to the parent.",
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
			{Name: "componentId", Kind: kindString, Required: true, IsNodeID: true,
				Desc: "New COMPONENT ID"},
		},
	},
	{
		Name:       "detach_instance",
		Desc:       "Detach instances into plain frames. Looks the same, no link to the component.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "INSTANCE node IDs",
	},
	{
		Name: "create_component_instance",
		Desc: "Create an instance of a local component (componentId) or library component (componentKey). A component set uses its default variant.",
		Params: []paramSpec{
			{Name: "componentId", Kind: kindString, IsNodeID: true,
				Desc: "Local component or set ID (preferred)"},
			{Name: "componentKey", Kind: kindString,
				Desc: "Library component key"},
			parentIDParam("Parent ID (default: current page)"),
			{Name: "x", Kind: kindNumber, Desc: "X (default: center of view)"},
			{Name: "y", Kind: kindNumber, Desc: "Y"},
		},
		Validate: requireAnyOf("componentId or componentKey is required", "componentId", "componentKey"),
	},
	{
		Name:       "set_instance_overrides",
		Desc:       "Set component properties (variant, boolean, text) on an instance. Fails on unknown names or wrong types.",
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
		Desc: "Create a connector line. FigJam only.",
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
		Desc:       "Set Dev Mode annotations on nodes ([] clears). Needs a paid Dev Mode seat.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs",
		Params: []paramSpec{
			{Name: "annotations", Kind: kindArray, Required: true, AllowEmpty: true,
				Desc: "e.g. [{\"label\": \"Main Button\"}]; [] clears"},
		},
	},
}

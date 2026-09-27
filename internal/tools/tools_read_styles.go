package tools

var readStyleSpecs = []toolSpec{
	{
		Name: "get_styles",
		Desc: "Get local styles (paint, text, effect, grid) with ID, name, type, values. Variables: get_variable_defs.",
	},
	{
		Name: "get_variable_defs",
		Desc: "Get local variables (design tokens): collections, modes, values.",
	},
	{
		Name: "get_local_components",
		Desc: "Get all components in the file.",
	},
	{
		Name: "get_annotations",
		Desc: "Get Dev Mode annotations on the page, or on one node and its children.",

		// The plugin reads this from params, not from the nodeIDs field.
		Params: []paramSpec{
			{Name: "nodeId", Kind: kindString, IsNodeID: true,
				Desc: "Only this node and its children"},
		},
	},
	{
		Name: "export_tokens",
		Desc: "Export variables and paint styles as JSON or CSS variables.",
		Params: []paramSpec{
			{Name: "format", Kind: kindString, Enum: []string{"json", "css"},
				Desc: "json (default) or css"},
		},
	},
}

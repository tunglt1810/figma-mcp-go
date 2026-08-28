package tools

var readStyleSpecs = []toolSpec{
	{
		Name: "get_styles",
		Desc: "Get all local styles in the document (paint, text, effect, and grid). Returns each style's ID, name, type, and properties. Use the style ID with apply_style_to_node or update_paint_style. For design tokens (variables), use get_variable_defs instead.",
	},
	{
		Name: "get_variable_defs",
		Desc: "Get all local variable definitions: collections, modes, and values. Variables are Figma's design token system.",
	},
	{
		Name: "get_local_components",
		Desc: "Get all components defined in the current Figma file.",
	},
	{
		Name: "get_annotations",
		Desc: "Get dev-mode annotations in the current document or scoped to a specific node. Returns annotation objects with label text, measurement type, and the ID of the annotated node. Omit nodeId to retrieve all annotations on the current page.",
		// The plugin reads this from params, not from the nodeIDs field.
		Params: []paramSpec{
			{Name: "nodeId", Kind: kindString, IsNodeID: true,
				Desc: "Optional — scope results to annotations on this node and its descendants, colon format e.g. '4029:12345'"},
		},
	},
	{
		Name: "export_tokens",
		Desc: "Export all design tokens (variables and paint styles) as JSON or CSS custom properties. Ideal for bridging Figma variables into your codebase.",
		Params: []paramSpec{
			{Name: "format", Kind: kindString, Enum: []string{"json", "css"},
				Desc: "Output format: json (default) or css"},
		},
	},
}

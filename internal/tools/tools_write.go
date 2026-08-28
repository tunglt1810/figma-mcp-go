package tools

var batchPipelineSpec = toolSpec{
	Name: "batch_execute_pipeline",
	Desc: "Execute a batch pipeline of mutation steps in Figma, passing values between steps via $variables. " +
		"On failure with stop_on_error, rollback removes nodes the pipeline created and restores properties it changed on existing nodes " +
		"(position, size, fills, strokes, opacity, visibility, name, text, blend mode, constraints). " +
		"Rollback CANNOT undo deletions (delete_nodes, delete_page), structural changes (group/ungroup, detach_instance, reparent), " +
		"or steps that target a node by name instead of id.",
	Params: []paramSpec{
		{Name: "stop_on_error", Kind: kindBool,
			Desc: "Whether to stop execution and rollback on error (default true)"},
		{Name: "steps", Kind: kindObjectArray, Required: true,
			Desc: "Array of pipeline steps to execute in sequence. Each step is {id, action, params, export_vars?}.",
			ItemSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":          map[string]any{"type": "string", "description": "Step identifier, referenced by $variables in later steps"},
					"action":      map[string]any{"type": "string", "description": "Tool name to run e.g. create_frame"},
					"params":      map[string]any{"type": "object", "description": "Arguments for the action"},
					"export_vars": map[string]any{"type": "object", "description": "Map of variable name to a field of this step's result"},
				},
				"required": []string{"action"},
			}},
	},
}

package tools

var batchPipelineSpec = toolSpec{
	Name: "batch_execute_pipeline",
	Desc: "Run several write steps in order. A step can use an earlier step's result via $variables. " +
		"With stop_on_error, a failure undoes created nodes and changed properties. " +
		"It cannot undo deletes, group/ungroup, detach, reparent, or steps that target nodes by name.",
	Params: []paramSpec{
		{Name: "stop_on_error", Kind: kindBool,
			Desc: "Stop and undo on error (default true)"},
		{Name: "steps", Kind: kindObjectArray, Required: true,
			Desc: "Steps in order: {id, action, params, export_vars?}",
			ItemSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":          map[string]any{"type": "string", "description": "Step ID, used by $variables in later steps"},
					"action":      map[string]any{"type": "string", "description": "Tool name to run e.g. create_frame"},
					"params":      map[string]any{"type": "object", "description": "Arguments for the action"},
					"export_vars": map[string]any{"type": "object", "description": "Variable name to result field"},
				},
				"required": []string{"action"},
			}},
	},
}

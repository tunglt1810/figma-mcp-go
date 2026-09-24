package tools

// File-level operations.

var writeDocumentSpecs = []toolSpec{
	{
		Name:       "set_codegen_result",
		Desc:       "Save code on a node to show in Dev Mode's Code panel for the whole team. Code on a component shows for all its instances. [] removes it.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID",
		Params: []paramSpec{
			{Name: "blocks", Kind: kindObjectArray, Required: true, AllowEmpty: true,
				Desc: "One tab per block",
				ItemSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title":    map[string]any{"type": "string", "description": "Tab title (default 'Code')"},
						"language": map[string]any{"type": "string", "description": "e.g. TYPESCRIPT, HTML, CSS, SWIFT, KOTLIN (default PLAINTEXT)"},
						"code":     map[string]any{"type": "string", "description": "Code"},
					},
					"required": []string{"code"},
				}},
		},
	},
	{
		Name:       "manage_plugin_data",
		Desc:       "Store string key/values on a node, saved in the file (e.g. which source file a component maps to). Use JSON for complex values.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID",
		Params: []paramSpec{
			{Name: "action", Kind: kindString, Required: true,
				Enum: []string{"get", "set", "delete", "keys"},
				Desc: "get, set, delete, or keys (list)"},
			{Name: "key", Kind: kindString,
				Desc: "Key (not for keys)"},
			{Name: "value", Kind: kindString, AllowEmpty: true,
				Desc: "Value, for set"},
			{Name: "namespace", Kind: kindString,
				Desc: "Default 'figma-mcp-go'"},
		},
		Validate: func(_ []string, params map[string]any) string {
			action, _ := params["action"].(string)
			if action == "keys" {
				return ""
			}
			if _, ok := params["key"]; !ok && action != "" {
				return "key is required unless action is keys"
			}
			if action == "set" {
				if _, ok := params["value"]; !ok {
					return "value is required when action is set"
				}
			}
			return ""
		},
	},
	{
		Name: "save_version_checkpoint",
		Desc: "Save a named version in the file history. Use before big or risky changes. Design files only.",
		Params: []paramSpec{
			{Name: "title", Kind: kindString, Required: true,
				Desc: "Version name"},
			{Name: "description", Kind: kindString,
				Desc: "Note"},
		},
	},
}

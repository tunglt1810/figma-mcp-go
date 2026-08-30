package tools

// File-level operations.

var writeDocumentSpecs = []toolSpec{
	{
		Name:       "set_codegen_result",
		Desc:       "Attach generated code to a node so it appears in Figma's Dev Mode Code panel. This is how code you write with the repository in front of you reaches the designers: it is stored in the file, so every teammate's Dev Mode shows it, not just this machine. Dev Mode looks at the node itself, then at the component an instance came from, then at its ancestors — so putting the code on a component covers every instance of it. Pass an empty blocks array to remove what is stored.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID to attach the code to, colon format e.g. '4029:12345'. A COMPONENT or COMPONENT_SET covers all its instances.",
		Params: []paramSpec{
			{Name: "blocks", Kind: kindObjectArray, Required: true, AllowEmpty: true,
				Desc: "Code blocks to show, one tab each. Empty removes the stored code.",
				ItemSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title":    map[string]any{"type": "string", "description": "Tab title e.g. 'Button.tsx' (default 'Code')"},
						"language": map[string]any{"type": "string", "description": "Syntax highlighting: TYPESCRIPT, JAVASCRIPT, HTML, CSS, JSON, GRAPHQL, PYTHON, GO, SQL, SWIFT, KOTLIN, RUBY, CPP, RUST, BASH, or PLAINTEXT. Anything else falls back to PLAINTEXT."},
						"code":     map[string]any{"type": "string", "description": "The code itself"},
					},
					"required": []string{"code"},
				}},
		},
	},
	{
		Name:       "manage_plugin_data",
		Desc:       "Read and write your own metadata on a node, stored in the Figma file itself. Use it to remember what a later session cannot re-derive — which source file a component maps to, which nodes were generated and from what, a token binding. Values are strings; encode anything richer as JSON. Stored as shared plugin data, so it travels with the file and any teammate can read it.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID in colon format e.g. '4029:12345'",
		Params: []paramSpec{
			{Name: "action", Kind: kindString, Required: true,
				Enum: []string{"get", "set", "delete", "keys"},
				Desc: "get reads one key, set writes one, delete removes one, keys lists what is stored"},
			{Name: "key", Kind: kindString,
				Desc: "Which entry to read, write, or remove. Required for every action but keys."},
			{Name: "value", Kind: kindString, AllowEmpty: true,
				Desc: "The string to store. Required when action is set."},
			{Name: "namespace", Kind: kindString,
				Desc: "Namespace to read or write under (default 'figma-mcp-go'). Use another tool's namespace to read what it stored."},
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
		Desc: "Save a named version in the Figma file's version history. Use this before a large or risky change — batch_execute_pipeline's rollback lives only in the running plugin, so a named version is the only way back once the session ends. Figma design files only; FigJam and Slides have no version history.",
		Params: []paramSpec{
			{Name: "title", Kind: kindString, Required: true,
				Desc: "Name for this version, as it appears in Figma's version history e.g. 'Before the nav redesign'"},
			{Name: "description", Kind: kindString,
				Desc: "Optional longer note about what is about to change"},
		},
	},
}

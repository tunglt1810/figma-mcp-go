package internal

import "github.com/mark3labs/mcp-go/server"

var writeVariableSpecs = []toolSpec{
	{
		Name: "create_variable_collection",
		Desc: "Create a new local variable collection with an optional initial mode name. " +
			"NOTE — Figma free plan limits each collection to 1 mode. If you need Light/Dark (or any multi-mode) " +
			"theming and the user is on the free plan, do NOT try to call add_variable_mode; instead use the " +
			"name-prefix workaround: create all variables in a single collection and prefix each variable name " +
			"with its mode, e.g. 'light/color-bg' and 'dark/color-bg'. Inform the user of this limitation.",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Required: true, Desc: "Collection name"},
			{Name: "initialModeName", Kind: kindString, Desc: "Name for the initial mode (default 'Mode 1')"},
		},
	},
	{
		Name: "add_variable_mode",
		Desc: "Add a new mode to an existing variable collection (e.g. Light/Dark, Desktop/Mobile). " +
			"IMPORTANT — Figma free plan only allows 1 mode per collection; calling this tool on a free-plan " +
			"account will return the error 'Limited to 1 modes only'. If that error occurs, stop retrying and " +
			"switch to the name-prefix workaround: keep the single default mode and create variables prefixed " +
			"by mode, e.g. 'light/color-bg' and 'dark/color-bg' in the same collection. Tell the user that " +
			"native multi-mode variables require a paid Figma plan (Professional or above).",
		Params: []paramSpec{
			{Name: "collectionId", Kind: kindString, Required: true, Desc: "Variable collection ID"},
			{Name: "modeName", Kind: kindString, Required: true, Desc: "Name for the new mode"},
		},
	},
	{
		Name: "create_variable",
		Desc: "Create a new variable (design token) inside an existing collection. Returns the new variable's ID. Use get_variable_defs to find collection IDs, set_variable_value to set values per mode, and bind_variable_to_node to apply the variable to a node property.",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Required: true,
				Desc: "Variable name — use slash notation to group e.g. 'Color/Primary', 'Spacing/MD'"},
			{Name: "collectionId", Kind: kindString, Required: true,
				Desc: "ID of the variable collection to add this variable to (from get_variable_defs)"},
			{Name: "type", Kind: kindString, Required: true, Enum: []string{"COLOR", "FLOAT", "STRING", "BOOLEAN"},
				Desc: "Variable type: COLOR (hex color), FLOAT (numeric dimension/spacing), STRING (text), or BOOLEAN (true/false toggle)"},
			{Name: "value", Kind: kindString,
				Desc: "Initial value for the first mode. COLOR: hex e.g. #FF5733. FLOAT: number e.g. 16. STRING: text. BOOLEAN: true or false."},
		},
	},
	{
		Name: "set_variable_value",
		Desc: "Set a variable's value for a specific mode.",
		Params: []paramSpec{
			{Name: "variableId", Kind: kindString, Required: true, Desc: "Variable ID"},
			{Name: "modeId", Kind: kindString, Required: true, Desc: "Mode ID within the collection"},
			{Name: "value", Kind: kindString, Required: true,
				Desc: "Value to set. COLOR: hex e.g. #FF5733. FLOAT: number e.g. 16. STRING: text. BOOLEAN: true or false."},
		},
	},
	{
		Name: "delete_variable",
		Desc: "Delete a single variable (provide variableId) or an entire collection and all its variables (provide collectionId). Provide exactly one of the two — not both.",
		Params: []paramSpec{
			{Name: "variableId", Kind: kindString, Desc: "Variable ID to delete"},
			{Name: "collectionId", Kind: kindString, Desc: "Collection ID to delete (removes all variables in the collection)"},
		},
		Validate: func(_ []string, params map[string]interface{}) string {
			variableID, _ := params["variableId"].(string)
			collectionID, _ := params["collectionId"].(string)
			if variableID == "" && collectionID == "" {
				return "variableId or collectionId is required"
			}
			return ""
		},
	},
}

func registerWriteVariableTools(s *server.MCPServer, node *Node) {
	registerSpecs(s, node, writeVariableSpecs)
}

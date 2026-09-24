package tools

// variableVariants say which arguments belong to which variable action. Six
// tools became one, and their arguments overlap without meaning the same thing
// — collectionId names the parent when creating and the target when deleting —
// so an argument from the wrong action is rejected rather than dropped.
var variableVariants = map[string]variantSpec{
	"create_collection": {Allowed: []string{"name", "initialModeName"}, Required: []string{"name"}},
	"add_mode":          {Allowed: []string{"collectionId", "modeName"}, Required: []string{"collectionId", "modeName"}},
	"create":            {Allowed: []string{"name", "collectionId", "type", "value"}, Required: []string{"name", "collectionId", "type"}},
	"set_value":         {Allowed: []string{"variableId", "modeId", "value"}, Required: []string{"variableId", "modeId", "value"}},
	"delete":            {Allowed: []string{"variableId", "collectionId"}},
	"bind":              {Allowed: []string{"variableId", "field"}, Required: []string{"variableId", "field"}},
}

var writeVariableSpecs = []toolSpec{
	{
		Name: "manage_variable",
		Desc: "Manage variables (design tokens). Arguments per `action`: " +
			"create_collection: name, initialModeName. " +
			"add_mode: collectionId, modeName. " +
			"create: name, collectionId, type, value. " +
			"set_value: variableId, modeId, value. " +
			"delete: variableId, or collectionId (deletes all its variables). " +
			"bind: nodeId, variableId, field (link a node property to the variable). Get IDs from get_variable_defs. " +
			"Free plan allows 1 mode per collection: if add_mode fails with 'Limited to 1 modes only', do not retry; " +
			"put the mode in the name instead (e.g. 'light/bg', 'dark/bg') and tell the user multi-mode needs a paid plan.",
		NodeIDs:    nodeIDsSingle,
		NodeIDDesc: "bind: the node",
		Params: []paramSpec{
			{Name: "action", Kind: kindString, Required: true, Enum: variantKinds(variableVariants),
				Desc: "create_collection, add_mode, create, set_value, delete, or bind"},
			{Name: "name", Kind: kindString,
				Desc: "Collection or variable name; slashes group e.g. 'Color/Primary'"},
			{Name: "initialModeName", Kind: kindString,
				Desc: "First mode name (default 'Mode 1')"},
			{Name: "collectionId", Kind: kindString,
				Desc: "Collection ID"},
			{Name: "modeName", Kind: kindString, Desc: "New mode name e.g. 'Dark'"},
			{Name: "type", Kind: kindString, Enum: []string{"COLOR", "FLOAT", "STRING", "BOOLEAN"},
				Desc: "COLOR, FLOAT, STRING, or BOOLEAN"},
			{Name: "value", Kind: kindString,
				Desc: "COLOR: hex. FLOAT: number. STRING: text. BOOLEAN: true/false. For create, sets the first mode."},
			{Name: "variableId", Kind: kindString,
				Desc: "Variable ID"},
			{Name: "modeId", Kind: kindString, Desc: "Mode ID"},
			{Name: "field", Kind: kindString,
				Desc: "Property to bind. COLOR: fillColor, strokeColor. BOOLEAN: visible. " +
					"FLOAT: opacity, rotation, width, height, cornerRadius, topLeftRadius, topRightRadius, bottomLeftRadius, bottomRightRadius, strokeWeight, itemSpacing, paddingTop, paddingRight, paddingBottom, paddingLeft."},
		},
		Validate: func(nodeIDs []string, params map[string]any) string {
			if msg := requireVariant("action", variableVariants)(nodeIDs, params); msg != "" {
				return msg
			}
			switch action, _ := params["action"].(string); action {
			case "delete":
				// Either target is enough, but a delete with neither would be a
				// call that names nothing to remove.
				variableID, _ := params["variableId"].(string)
				collectionID, _ := params["collectionId"].(string)
				if variableID == "" && collectionID == "" {
					return "variableId or collectionId is required when action is delete"
				}
			case "bind":
				// The node travels in its own field, so requireVariant — which
				// only sees params — cannot ask for it.
				if len(nodeIDs) == 0 {
					return "nodeId is required when action is bind"
				}
			}
			return ""
		},
	},
}

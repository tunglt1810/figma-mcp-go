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
		Desc: "Create, change, delete and apply variables — Figma's design tokens. `action` selects what, and each takes its own arguments — " +
			"create_collection: name, initialModeName. " +
			"add_mode: collectionId, modeName. " +
			"create: name, collectionId, type, value. " +
			"set_value: variableId, modeId, value. " +
			"delete: variableId, or collectionId to remove a whole collection and every variable in it. " +
			"bind: nodeId, variableId, field — points a node property at the variable so its value drives the property. " +
			"An argument belonging to a different action is rejected rather than ignored. Use get_variable_defs to find collection, mode and variable IDs. " +
			"NOTE — the Figma free plan limits each collection to 1 mode, so add_mode fails there with 'Limited to 1 modes only'. " +
			"Do not retry: keep the single default mode and prefix each variable name with its mode instead, e.g. 'light/color-bg' and 'dark/color-bg' in one collection. " +
			"Tell the user that native multi-mode variables need a paid plan (Professional or above).",
		NodeIDs:    nodeIDsSingle,
		NodeIDDesc: "bind: the node whose property the variable should drive, in colon format e.g. '4029:12345'",
		Params: []paramSpec{
			{Name: "action", Kind: kindString, Required: true, Enum: variantKinds(variableVariants),
				Desc: "What to do: create_collection, add_mode, create, set_value, delete, or bind"},
			{Name: "name", Kind: kindString,
				Desc: "create_collection: the collection's name. create: the variable's name — use slash notation to group e.g. 'Color/Primary', 'Spacing/MD'."},
			{Name: "initialModeName", Kind: kindString,
				Desc: "create_collection: name for the initial mode (default 'Mode 1')"},
			{Name: "collectionId", Kind: kindString,
				Desc: "add_mode and create: the collection to work in. delete: the collection to remove, along with every variable in it. From get_variable_defs."},
			{Name: "modeName", Kind: kindString, Desc: "add_mode: name for the new mode e.g. 'Dark'"},
			{Name: "type", Kind: kindString, Enum: []string{"COLOR", "FLOAT", "STRING", "BOOLEAN"},
				Desc: "create: COLOR (hex color), FLOAT (numeric dimension/spacing), STRING (text), or BOOLEAN (true/false toggle)"},
			{Name: "value", Kind: kindString,
				Desc: "create: initial value for the first mode. set_value: the value for the given mode. COLOR: hex e.g. #FF5733. FLOAT: number e.g. 16. STRING: text. BOOLEAN: true or false."},
			{Name: "variableId", Kind: kindString,
				Desc: "set_value, delete and bind: the variable, from get_variable_defs"},
			{Name: "modeId", Kind: kindString, Desc: "set_value: which mode of the collection to set"},
			{Name: "field", Kind: kindString,
				Desc: "bind: the property to drive. COLOR variables: fillColor, strokeColor. BOOLEAN: visible. " +
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

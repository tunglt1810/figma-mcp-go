package tools

// Component properties and variants — the half of the component surface that
// was missing. The plugin could read a design system and make a component, but
// not declare what that component exposes, so it could never build one.

var writeComponentPropertySpecs = []toolSpec{
	{
		Name:       "combine_as_variants",
		Desc:       "Combine 2+ components (same parent) into a component set of variants. Then add VARIANT properties with manage_component_properties.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "COMPONENT node IDs",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Desc: "Set name"},
		},
	},
	{
		Name:       "manage_component_properties",
		Desc:       "Add, edit, delete, or bind a component's properties. bind links a layer to a property: BOOLEAN shows/hides it, TEXT sets its text, INSTANCE_SWAP swaps it. Names work without the '#1:2' suffix.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "COMPONENT or COMPONENT_SET node ID",
		Params: []paramSpec{
			{Name: "action", Kind: kindString, Required: true,
				Enum: []string{"add", "edit", "delete", "bind"},
				Desc: "add, edit, delete, or bind"},
			{Name: "name", Kind: kindString,
				Desc: "Name (add), or new name (edit)"},
			{Name: "type", Kind: kindString, Enum: []string{"BOOLEAN", "TEXT", "INSTANCE_SWAP", "VARIANT"},
				Desc: "Type, for add. VARIANT needs a COMPONENT_SET."},
			{Name: "defaultValue", Kind: kindAny,
				Desc: "Default, for add: boolean, string, or component ID (INSTANCE_SWAP)"},
			{Name: "property", Kind: kindString,
				Desc: "Property to change, e.g. 'Size' or 'Size#1:2'"},
			{Name: "targetNodeId", Kind: kindString, IsNodeID: true,
				Desc: "bind: layer inside the component"},
			{Name: "preferredValues", Kind: kindObjectArray,
				Desc: "INSTANCE_SWAP: components shown first",
				ItemSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"type": map[string]any{"type": "string", "enum": []string{"COMPONENT", "COMPONENT_SET"}},
						"key":  map[string]any{"type": "string", "description": "Component key"},
					},
					"required": []string{"type", "key"},
				}},
		},
		Validate: func(_ []string, params map[string]any) string {
			action, _ := params["action"].(string)
			switch action {
			case "add":
				if _, ok := params["name"]; !ok {
					return "name is required when action is add"
				}
				if _, ok := params["type"]; !ok {
					return "type is required when action is add"
				}
				if _, ok := params["defaultValue"]; !ok {
					return "defaultValue is required when action is add"
				}
			case "edit":
				if _, ok := params["property"]; !ok {
					return "property is required when action is edit"
				}
				_, hasName := params["name"]
				_, hasDefault := params["defaultValue"]
				_, hasPreferred := params["preferredValues"]
				if !hasName && !hasDefault && !hasPreferred {
					return "edit needs at least one of name, defaultValue, or preferredValues"
				}
			case "delete":
				if _, ok := params["property"]; !ok {
					return "property is required when action is delete"
				}
			case "bind":
				if _, ok := params["property"]; !ok {
					return "property is required when action is bind"
				}
				if _, ok := params["targetNodeId"]; !ok {
					return "targetNodeId is required when action is bind"
				}
			}
			return ""
		},
	},
}

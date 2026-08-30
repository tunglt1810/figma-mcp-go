package tools

// Component properties and variants — the half of the component surface that
// was missing. The plugin could read a design system and make a component, but
// not declare what that component exposes, so it could never build one.

var writeComponentPropertySpecs = []toolSpec{
	{
		Name:       "combine_as_variants",
		Desc:       "Combine two or more COMPONENT nodes into a single COMPONENT_SET, so they become variants of one component. The components must already share a parent. Add VARIANT properties afterwards with manage_component_properties to say what distinguishes them.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "COMPONENT node IDs in colon format e.g. ['4029:12345', '4029:12346']",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Desc: "Name for the resulting component set"},
		},
	},
	{
		Name:       "manage_component_properties",
		Desc:       "Define what a component exposes to its instances. `add` declares a property, `edit` renames it or changes its default, `delete` removes it, and `bind` points one of the component's own layers at a property so setting it actually does something — a BOOLEAN drives the layer's visibility, a TEXT its characters, an INSTANCE_SWAP its component. Properties may be named without the `#1:2` suffix Figma appends; the current id is resolved for you and returned. Use set_instance_overrides to set the values on an instance.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "COMPONENT or COMPONENT_SET node ID in colon format e.g. '4029:12345'",
		Params: []paramSpec{
			{Name: "action", Kind: kindString, Required: true,
				Enum: []string{"add", "edit", "delete", "bind"},
				Desc: "What to do: add, edit, delete, or bind"},
			{Name: "name", Kind: kindString,
				Desc: "Property name. Required for add; for edit it renames the property."},
			{Name: "type", Kind: kindString, Enum: []string{"BOOLEAN", "TEXT", "INSTANCE_SWAP", "VARIANT"},
				Desc: "Property kind, required for add. VARIANT belongs to a COMPONENT_SET and is what distinguishes its members; the others belong to either."},
			{Name: "defaultValue", Kind: kindAny,
				Desc: "Default value, required for add: a boolean for BOOLEAN, a string for TEXT and VARIANT, a component ID for INSTANCE_SWAP"},
			{Name: "property", Kind: kindString,
				Desc: "Which property to edit, delete, or bind — its name e.g. 'Size', or its full id e.g. 'Size#1:2'"},
			{Name: "targetNodeId", Kind: kindString, IsNodeID: true,
				Desc: "For bind: the layer inside the component that the property should drive, colon format e.g. '4029:99'"},
			{Name: "preferredValues", Kind: kindObjectArray,
				Desc: "For an INSTANCE_SWAP property: the components offered first in the picker",
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

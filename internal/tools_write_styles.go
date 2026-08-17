package internal

import (
	"fmt"

	"github.com/mark3labs/mcp-go/server"
)

var effectTypes = []string{"DROP_SHADOW", "INNER_SHADOW", "LAYER_BLUR", "BACKGROUND_BLUR"}

// styleDescriptionParam is the optional blurb shown in Figma's style panel.
func styleDescriptionParam(desc string) paramSpec {
	return paramSpec{Name: "description", Kind: kindString, Desc: desc}
}

var writeStyleSpecs = []toolSpec{
	{
		Name: "create_paint_style",
		Desc: "Create a new local paint style with a solid fill color.",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Required: true, Desc: "Style name e.g. 'Brand/Primary'"},
			{Name: "color", Kind: kindString, IsHexColor: true, Required: true, Desc: "Fill color as hex e.g. #FF5733"},
			styleDescriptionParam("Optional style description"),
		},
	},
	{
		Name: "create_text_style",
		Desc: "Create a new local text style (typography preset). Returns the new style's ID. Apply it to nodes with apply_style_to_node. Use get_styles to list existing text styles.",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Required: true,
				Desc: "Style name — use slash notation to organise into groups e.g. 'Heading/H1', 'Body/Regular'"},
			{Name: "fontSize", Kind: kindNumber, Desc: "Font size in pixels (default 16)"},
			{Name: "fontFamily", Kind: kindString,
				Desc: "Font family name e.g. 'Inter', 'Roboto' (default Inter). Must be installed in Figma."},
			{Name: "fontStyle", Kind: kindString,
				Desc: "Font style variant e.g. 'Regular', 'Bold', 'Medium', 'SemiBold' (default Regular)"},
			{Name: "textDecoration", Kind: kindString, Enum: []string{"NONE", "UNDERLINE", "STRIKETHROUGH"},
				Desc: "Text decoration: NONE (default), UNDERLINE, or STRIKETHROUGH"},
			{Name: "lineHeightValue", Kind: kindNumber, Desc: "Line height value (unit set by lineHeightUnit)"},
			{Name: "lineHeightUnit", Kind: kindString, Enum: []string{"PIXELS", "PERCENT"},
				Desc: "Line height unit: PIXELS (default) or PERCENT"},
			{Name: "letterSpacingValue", Kind: kindNumber, Desc: "Letter spacing value (unit set by letterSpacingUnit)"},
			{Name: "letterSpacingUnit", Kind: kindString, Enum: []string{"PIXELS", "PERCENT"},
				Desc: "Letter spacing unit: PIXELS (default) or PERCENT"},
			styleDescriptionParam("Optional human-readable description shown in the Figma style panel"),
		},
	},
	{
		Name: "create_effect_style",
		Desc: "Create a new local effect style (drop shadow, inner shadow, or blur).",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Required: true, Desc: "Style name e.g. 'Shadow/Card'"},
			{Name: "type", Kind: kindString, Enum: effectTypes,
				Desc: "Effect type: DROP_SHADOW (default), INNER_SHADOW, LAYER_BLUR, or BACKGROUND_BLUR"},
			{Name: "color", Kind: kindString, IsHexColor: true, Desc: "Shadow color as hex e.g. #000000 (default #000000, shadows only)"},
			{Name: "opacity", Kind: kindNumber, Desc: "Shadow color opacity 0–1 (default 0.25, shadows only)"},
			{Name: "radius", Kind: kindNumber, Desc: "Blur radius in pixels (default 8 for shadows, 4 for blurs)"},
			{Name: "offsetX", Kind: kindNumber, Desc: "Shadow X offset in pixels (default 0, shadows only)"},
			{Name: "offsetY", Kind: kindNumber, Desc: "Shadow Y offset in pixels (default 4, shadows only)"},
			{Name: "spread", Kind: kindNumber, Desc: "Shadow spread in pixels (default 0, shadows only)"},
			styleDescriptionParam("Optional style description"),
		},
	},
	{
		Name: "create_grid_style",
		Desc: "Create a new local layout grid style.",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Required: true, Desc: "Style name e.g. 'Grid/Desktop'"},
			{Name: "pattern", Kind: kindString, Enum: []string{"GRID", "COLUMNS", "ROWS"},
				Desc: "Grid pattern: GRID (default), COLUMNS, or ROWS"},
			{Name: "count", Kind: kindNumber, Desc: "Number of columns or rows (COLUMNS/ROWS only, default 12)"},
			{Name: "gutterSize", Kind: kindNumber, Desc: "Gutter size in pixels (COLUMNS/ROWS only, default 16)"},
			{Name: "offset", Kind: kindNumber, Desc: "Margin/offset in pixels (COLUMNS/ROWS only, default 0)"},
			{Name: "alignment", Kind: kindString, Enum: []string{"STRETCH", "CENTER", "MIN", "MAX"},
				Desc: "Alignment: STRETCH (default), CENTER, MIN, or MAX (COLUMNS/ROWS only)"},
			{Name: "sectionSize", Kind: kindNumber, Desc: "Grid cell size in pixels (GRID only, default 8)"},
			{Name: "color", Kind: kindString, IsHexColor: true, Desc: "Grid line color as hex e.g. #FF0000 (GRID only, default #FF0000)"},
			{Name: "opacity", Kind: kindNumber, Desc: "Grid line opacity 0–1 (GRID only, default 0.1)"},
			styleDescriptionParam("Optional style description"),
		},
	},
	{
		Name: "update_paint_style",
		Desc: "Update an existing paint style's name, color, or description. Only paint styles support in-place updates — to modify text, effect, or grid styles, use delete_style and recreate them.",
		Params: []paramSpec{
			{Name: "styleId", Kind: kindString, Required: true, Desc: "Paint style ID"},
			{Name: "name", Kind: kindString, Desc: "New style name"},
			{Name: "color", Kind: kindString, IsHexColor: true, Desc: "New fill color as hex e.g. #FF5733"},
			styleDescriptionParam("New style description"),
		},
		Validate: requireAnyOf("at least one of name, color, or description is required",
			"name", "color", "description"),
	},
	{
		Name: "delete_style",
		Desc: "Delete a style (paint, text, effect, or grid) by its ID.",
		Params: []paramSpec{
			{Name: "styleId", Kind: kindString, Required: true, Desc: "Style ID to delete"},
		},
	},
	{
		Name:       "apply_style_to_node",
		Desc:       "Apply an existing local style (paint, text, effect, or grid) to a node, linking the node to that style.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Target node ID in colon format e.g. 4029:12345",
		Params: []paramSpec{
			{Name: "styleId", Kind: kindString, Required: true, Desc: "Style ID to apply (from get_styles)"},
			{Name: "target", Kind: kindString, Enum: []string{"fill", "stroke"},
				Desc: "For paint styles only — apply to 'fill' (default) or 'stroke'"},
		},
	},
	{
		Name:       "set_effects",
		Desc:       "Apply one or more effects (drop shadow, inner shadow, layer blur, background blur) directly to a node. Replaces all existing effects. Pass an empty array to clear all effects.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Target node ID in colon format e.g. 4029:12345",
		Params: []paramSpec{
			{Name: "effects", Kind: kindObjectArray, Required: true,
				Desc: "Array of effect objects. Each has: type (DROP_SHADOW | INNER_SHADOW | LAYER_BLUR | BACKGROUND_BLUR), radius, color (hex, shadows only), opacity (0–1, shadows only), offsetX, offsetY (shadows only), spread (shadows only), visible (default true)"},
		},
		Validate: func(_ []string, params map[string]interface{}) string {
			effects, _ := params["effects"].([]interface{})
			for i, e := range effects {
				em, _ := e.(map[string]interface{})
				t, _ := em["type"].(string)
				if !containsString(effectTypes, t) {
					return fmt.Sprintf("effects[%d].type must be DROP_SHADOW, INNER_SHADOW, LAYER_BLUR, or BACKGROUND_BLUR, got: %s", i, t)
				}
			}
			return ""
		},
	},
	{
		Name:       "bind_variable_to_node",
		Desc:       "Bind a local variable to a node property so the property is driven by the variable's value. COLOR variables: use fillColor or strokeColor. BOOLEAN variables: use visible. FLOAT variables: use opacity, rotation, width, height, cornerRadius, topLeftRadius, topRightRadius, bottomLeftRadius, bottomRightRadius, strokeWeight, itemSpacing, paddingTop, paddingRight, paddingBottom, paddingLeft.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Target node ID in colon format e.g. 4029:12345",
		Params: []paramSpec{
			{Name: "variableId", Kind: kindString, Required: true, Desc: "Variable ID to bind (from get_variable_defs)"},
			{Name: "field", Kind: kindString, Required: true,
				Desc: "Property to bind: fillColor | strokeColor | visible | opacity | rotation | width | height | cornerRadius | topLeftRadius | topRightRadius | bottomLeftRadius | bottomRightRadius | strokeWeight | itemSpacing | paddingTop | paddingRight | paddingBottom | paddingLeft"},
		},
	},
}

func registerWriteStyleTools(s *server.MCPServer, node *Node) {
	registerSpecs(s, node, writeStyleSpecs)
}

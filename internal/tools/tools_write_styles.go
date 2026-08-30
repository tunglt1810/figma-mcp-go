package tools

import (
	"fmt"
	"strings"
)

// effectTypes are the kinds create_style can build a reusable effect style from.
var effectTypes = []string{"DROP_SHADOW", "INNER_SHADOW", "LAYER_BLUR", "BACKGROUND_BLUR"}

// nodeEffectTypes covers Figma's whole Effect union apart from SHADER, which needs a
// shader imported by id before it can be applied and so cannot come from parameters.
// get_nodes_info reports all of these, so set_effects has to accept them back.
var nodeEffectTypes = []string{
	"DROP_SHADOW", "INNER_SHADOW", "LAYER_BLUR", "BACKGROUND_BLUR",
	"NOISE", "TEXTURE", "GLASS",
}

// styleDescriptionParam is the optional blurb shown in Figma's style panel.
func styleDescriptionParam(desc string) paramSpec {
	return paramSpec{Name: "description", Kind: kindString, Desc: desc}
}

// styleVariants say which arguments belong to which kind of style. Four
// create_*_style tools became one; without this the arguments of the other
// three would be accepted and silently dropped.
var styleVariants = map[string]variantSpec{
	"PAINT": {Allowed: []string{"color"}, Required: []string{"color"}},
	"TEXT": {Allowed: []string{
		"fontSize", "fontFamily", "fontStyle", "textDecoration",
		"lineHeightValue", "lineHeightUnit", "letterSpacingValue", "letterSpacingUnit",
	}},
	"EFFECT": {Allowed: []string{"effectType", "color", "opacity", "radius", "offsetX", "offsetY", "spread"}},
	"GRID": {Allowed: []string{
		"pattern", "count", "gutterSize", "offset", "alignment", "sectionSize", "color", "opacity",
	}},
}

var writeStyleSpecs = []toolSpec{
	{
		Name: "create_style",
		Desc: "Create a local style. `type` selects what kind, and each kind takes its own arguments — " +
			"PAINT: color. " +
			"TEXT: fontSize, fontFamily, fontStyle, textDecoration, lineHeightValue/Unit, letterSpacingValue/Unit. " +
			"EFFECT: effectType, color, opacity, radius, offsetX, offsetY, spread. " +
			"GRID: pattern, count, gutterSize, offset, alignment, sectionSize, color, opacity. " +
			"An argument belonging to a different kind is rejected rather than ignored. " +
			"Returns the new style's ID; apply it with apply_style_to_node.",
		Params: []paramSpec{
			{Name: "type", Kind: kindString, Required: true, Enum: variantKinds(styleVariants),
				Desc: "Kind of style: PAINT, TEXT, EFFECT, or GRID"},
			{Name: "name", Kind: kindString, Required: true,
				Desc: "Style name — use slash notation to organise into groups e.g. 'Brand/Primary', 'Heading/H1'"},
			styleDescriptionParam("Optional description shown in the Figma style panel"),

			{Name: "color", Kind: kindString, IsHexColor: true,
				Desc: "PAINT: fill color as hex e.g. #FF5733 (required). EFFECT: shadow color (default #000000). GRID: grid line color (default #FF0000)"},
			{Name: "opacity", Kind: kindNumber,
				Desc: "EFFECT: shadow color opacity 0–1 (default 0.25). GRID: grid line opacity 0–1 (default 0.1)"},

			{Name: "fontSize", Kind: kindNumber, Desc: "TEXT: font size in pixels (default 16)"},
			{Name: "fontFamily", Kind: kindString,
				Desc: "TEXT: font family name e.g. 'Inter', 'Roboto' (default Inter). Must be installed in Figma."},
			{Name: "fontStyle", Kind: kindString,
				Desc: "TEXT: style variant e.g. 'Regular', 'Bold', 'Medium', 'SemiBold' (default Regular)"},
			{Name: "textDecoration", Kind: kindString, Enum: []string{"NONE", "UNDERLINE", "STRIKETHROUGH"},
				Desc: "TEXT: NONE (default), UNDERLINE, or STRIKETHROUGH"},
			{Name: "lineHeightValue", Kind: kindNumber, Desc: "TEXT: line height value (unit set by lineHeightUnit)"},
			{Name: "lineHeightUnit", Kind: kindString, Enum: []string{"PIXELS", "PERCENT"},
				Desc: "TEXT: line height unit — PIXELS (default) or PERCENT"},
			{Name: "letterSpacingValue", Kind: kindNumber, Desc: "TEXT: letter spacing value (unit set by letterSpacingUnit)"},
			{Name: "letterSpacingUnit", Kind: kindString, Enum: []string{"PIXELS", "PERCENT"},
				Desc: "TEXT: letter spacing unit — PIXELS (default) or PERCENT"},

			{Name: "effectType", Kind: kindString, Enum: effectTypes,
				Desc: "EFFECT: DROP_SHADOW (default), INNER_SHADOW, LAYER_BLUR, or BACKGROUND_BLUR"},
			{Name: "radius", Kind: kindNumber, Desc: "EFFECT: blur radius in pixels (default 8 for shadows, 4 for blurs)"},
			{Name: "offsetX", Kind: kindNumber, Desc: "EFFECT: shadow X offset in pixels (default 0)"},
			{Name: "offsetY", Kind: kindNumber, Desc: "EFFECT: shadow Y offset in pixels (default 4)"},
			{Name: "spread", Kind: kindNumber, Desc: "EFFECT: shadow spread in pixels (default 0)"},

			{Name: "pattern", Kind: kindString, Enum: []string{"GRID", "COLUMNS", "ROWS"},
				Desc: "GRID: pattern — GRID (default), COLUMNS, or ROWS"},
			{Name: "count", Kind: kindNumber, Desc: "GRID: number of columns or rows (COLUMNS/ROWS only, default 12)"},
			{Name: "gutterSize", Kind: kindNumber, Desc: "GRID: gutter size in pixels (COLUMNS/ROWS only, default 16)"},
			{Name: "offset", Kind: kindNumber, Desc: "GRID: margin in pixels (COLUMNS/ROWS only, default 0)"},
			{Name: "alignment", Kind: kindString, Enum: []string{"STRETCH", "CENTER", "MIN", "MAX"},
				Desc: "GRID: STRETCH (default), CENTER, MIN, or MAX (COLUMNS/ROWS only)"},
			{Name: "sectionSize", Kind: kindNumber, Desc: "GRID: cell size in pixels (GRID pattern only, default 8)"},
		},
		Validate: requireVariant("type", styleVariants, "name", "description"),
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
		Desc:       "Apply one or more effects directly to a node. Replaces all existing effects. Pass an empty array to clear all effects. The shape matches what get_nodes_info reports under styles.effects, so effects can be read off one node and written to another unchanged.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Target node ID in colon format e.g. 4029:12345",
		Params: []paramSpec{
			{Name: "effects", Kind: kindObjectArray, Required: true,
				Desc: "Array of effect objects, each keyed by `type`. " +
					"DROP_SHADOW / INNER_SHADOW: color (hex), opacity (0–1 colour alpha), offsetX, offsetY, radius, spread, blendMode; showShadowBehindNode on drop shadows. " +
					"LAYER_BLUR / BACKGROUND_BLUR: radius; blurType PROGRESSIVE adds startRadius, startOffset, endOffset as {x, y}. " +
					"NOISE: noiseType (MONOTONE default | DUOTONE | MULTITONE), color, opacity, blendMode, noiseSize, density; secondaryColor for DUOTONE, noiseOpacity for MULTITONE. " +
					"TEXTURE: noiseSize, radius, clipToShape. " +
					"GLASS: radius, depth, lightIntensity, lightAngle, refraction, dispersion. " +
					"visible defaults to true on every type."},
		},
		Validate: func(_ []string, params map[string]any) string {
			effects, _ := params["effects"].([]any)
			for i, e := range effects {
				em, _ := e.(map[string]any)
				t, _ := em["type"].(string)
				if !containsString(nodeEffectTypes, t) {
					return fmt.Sprintf("effects[%d].type must be one of %s, got: %s",
						i, strings.Join(nodeEffectTypes, ", "), t)
				}
			}
			return ""
		},
	},
}

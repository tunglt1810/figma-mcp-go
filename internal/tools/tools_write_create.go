package tools

import "github.com/tunglt1810/figma-mcp-go/internal/figma"

// parentIDParam is the shared "where does this node go" argument.
func parentIDParam(desc string) paramSpec {
	return paramSpec{Name: "parentId", Kind: kindString, IsNodeID: true, Desc: desc}
}

var defaultParentDesc = "Parent node ID (default: current page)"

// positionParams are the x/y arguments every create tool shares.
func positionParams() []paramSpec {
	return []paramSpec{
		{Name: "x", Kind: kindNumber, Desc: "X (default 0)"},
		{Name: "y", Kind: kindNumber, Desc: "Y (default 0)"},
	}
}

// autoLayoutParams describe a frame's auto-layout (flex) configuration.
func autoLayoutParams() []paramSpec {
	return []paramSpec{
		{Name: "layoutMode", Kind: kindString, Enum: []string{"HORIZONTAL", "VERTICAL", "NONE"},
			Desc: "Auto layout: HORIZONTAL, VERTICAL, or NONE"},
		{Name: "paddingTop", Kind: kindNumber, Desc: "Top padding"},
		{Name: "paddingRight", Kind: kindNumber, Desc: "Right padding"},
		{Name: "paddingBottom", Kind: kindNumber, Desc: "Bottom padding"},
		{Name: "paddingLeft", Kind: kindNumber, Desc: "Left padding"},
		{Name: "itemSpacing", Kind: kindNumber, Desc: "Gap between children"},
		{Name: "primaryAxisAlignItems", Kind: kindString, Enum: []string{"MIN", "CENTER", "MAX", "SPACE_BETWEEN"},
			Desc: "Main axis align: MIN, CENTER, MAX, SPACE_BETWEEN"},
		{Name: "counterAxisAlignItems", Kind: kindString, Enum: []string{"MIN", "CENTER", "MAX", "BASELINE"},
			Desc: "Cross axis align: MIN, CENTER, MAX, BASELINE"},
		{Name: "primaryAxisSizingMode", Kind: kindString, Enum: []string{"FIXED", "AUTO"},
			Desc: "Main axis size: FIXED or AUTO (hug)"},
		{Name: "counterAxisSizingMode", Kind: kindString, Enum: []string{"FIXED", "AUTO"},
			Desc: "Cross axis size: FIXED or AUTO (hug)"},
		{Name: "layoutWrap", Kind: kindString, Enum: []string{"NO_WRAP", "WRAP"},
			Desc: "NO_WRAP or WRAP"},
		{Name: "counterAxisSpacing", Kind: kindNumber,
			Desc: "Gap between wrapped rows (with WRAP)"},
		{Name: "layoutSizingHorizontal", Kind: kindString, Enum: layoutSizingValues,
			Desc: "Width mode: FIXED, HUG (fit content), or FILL (fill parent). Prefer over *AxisSizingMode. HUG needs auto layout on this node; FILL on its parent."},
		{Name: "layoutSizingVertical", Kind: kindString, Enum: layoutSizingValues,
			Desc: "Height mode: FIXED, HUG, or FILL. Same rules as layoutSizingHorizontal."},
		{Name: "minWidth", Kind: kindNumber, Min: floatPtr(0), Nullable: true,
			Desc: "Min width; null clears"},
		{Name: "maxWidth", Kind: kindNumber, Min: floatPtr(0), Nullable: true,
			Desc: "Max width; null clears"},
		{Name: "minHeight", Kind: kindNumber, Min: floatPtr(0), Nullable: true,
			Desc: "Min height; null clears"},
		{Name: "maxHeight", Kind: kindNumber, Min: floatPtr(0), Nullable: true,
			Desc: "Max height; null clears"},
		{Name: "layoutPositioning", Kind: kindString, Enum: []string{"AUTO", "ABSOLUTE"},
			Desc: "In the parent's auto layout: AUTO (in flow) or ABSOLUTE (free, at x/y)"},
		{Name: "layoutAlign", Kind: kindString, Enum: []string{"MIN", "CENTER", "MAX", "STRETCH", "INHERIT"},
			Desc: "This node's cross axis align in the parent's auto layout"},
		{Name: "layoutGrow", Kind: kindNumber, Min: floatPtr(0), Max: floatPtr(1),
			Desc: "1 = grow to fill the parent's main axis, 0 = keep size"},
		{Name: "itemReverseZIndex", Kind: kindBool,
			Desc: "Put first child on top"},
		{Name: "strokesIncludedInLayout", Kind: kindBool,
			Desc: "Include strokes in layout size"},
		{Name: "clipsContent", Kind: kindBool,
			Desc: "Clip content"},
	}
}

var layoutSizingValues = []string{"FIXED", "HUG", "FILL"}

// nodeVariants say which arguments belong to which shape. Seven create_* tools
// became one, and these shapes genuinely differ — a star takes pointCount, a
// line takes length — so an argument from the wrong shape is an error rather
// than something dropped on the way to Figma.
//
// parentId is deliberately absent from SECTION: the handler does not read it,
// and accepting it would be exactly the silent no-op this guards against.
var nodeVariants = map[string]variantSpec{
	"FRAME":     {Allowed: append([]string{"width", "height", "fillColor", "parentId"}, autoLayoutParamNames...)},
	"RECTANGLE": {Allowed: []string{"width", "height", "fillColor", "cornerRadius", "parentId"}},
	"ELLIPSE":   {Allowed: []string{"width", "height", "fillColor", "startAngle", "endAngle", "innerRadiusRatio", "parentId"}},
	"STAR":      {Allowed: []string{"pointCount", "outerRadius", "innerRadius", "fillColor", "cornerRadius", "parentId"}},
	"POLYGON":   {Allowed: []string{"pointCount", "radius", "fillColor", "cornerRadius", "parentId"}},
	"LINE":      {Allowed: []string{"length", "rotation", "strokeColor", "strokeWeight", "parentId"}},
	"SECTION":   {Allowed: []string{"width", "height"}},
}

// Kept in step with autoLayoutParams by TestAutoLayoutParamNamesCoverTheSpecs —
// a name missing here is silently rejected as "not allowed for this shape".
var autoLayoutParamNames = []string{
	"layoutMode", "paddingTop", "paddingRight", "paddingBottom", "paddingLeft",
	"itemSpacing", "primaryAxisAlignItems", "counterAxisAlignItems",
	"primaryAxisSizingMode", "counterAxisSizingMode", "layoutWrap", "counterAxisSpacing",
	"layoutSizingHorizontal", "layoutSizingVertical",
	"minWidth", "maxWidth", "minHeight", "maxHeight",
	"layoutPositioning", "layoutAlign", "layoutGrow",
	"itemReverseZIndex", "strokesIncludedInLayout", "clipsContent",
}

var writeCreateSpecs = []toolSpec{
	{
		Name: "create_node",
		Desc: "Create a shape. Arguments per `type`: " +
			"FRAME: width, height, fillColor, auto layout args. " +
			"RECTANGLE: width, height, fillColor, cornerRadius. " +
			"ELLIPSE: width, height, fillColor, startAngle, endAngle, innerRadiusRatio. " +
			"STAR: pointCount, outerRadius, innerRadius, fillColor, cornerRadius. " +
			"POLYGON: pointCount, radius, fillColor, cornerRadius. " +
			"LINE: length, rotation, strokeColor, strokeWeight. " +
			"SECTION: width, height (no parent). " +
			"All: x, y, name. Wrong args for the type are rejected. For text use create_text.",
		Params: append([]paramSpec{
			{Name: "type", Kind: kindString, Required: true, Enum: variantKinds(nodeVariants),
				Desc: "FRAME, RECTANGLE, ELLIPSE, STAR, POLYGON, LINE, or SECTION"},
			{Name: "name", Kind: kindString, Desc: "Layer name"},
			{Name: "x", Kind: kindNumber, Desc: "X (default 0)"},
			{Name: "y", Kind: kindNumber, Desc: "Y (default 0)"},
			{Name: "width", Kind: kindNumber, Positive: true,
				Desc: "Width (default 100)"},
			{Name: "height", Kind: kindNumber, Positive: true,
				Desc: "Height (default 100)"},
			{Name: "fillColor", Kind: kindString, IsHexColor: true,
				Desc: "Fill hex e.g. #FF5733"},
			{Name: "cornerRadius", Kind: kindNumber,
				Desc: "Corner radius"},
			{Name: "startAngle", Kind: kindNumber,
				Desc: "Arc start, radians (default 0)"},
			{Name: "endAngle", Kind: kindNumber,
				Desc: "Arc end, radians (default full circle)"},
			{Name: "innerRadiusRatio", Kind: kindNumber, Min: floatPtr(0), Max: floatPtr(1),
				Desc: "Hole size 0-1, for rings (default 0)"},
			{Name: "pointCount", Kind: kindNumber, Min: floatPtr(3),
				Desc: "STAR points (default 5) or POLYGON sides (default 3)"},
			{Name: "outerRadius", Kind: kindNumber, Positive: true,
				Desc: "STAR outer radius (default 50)"},
			{Name: "innerRadius", Kind: kindNumber, Positive: true,
				Desc: "STAR inner radius (default 0.38 × outer)"},
			{Name: "radius", Kind: kindNumber, Positive: true,
				Desc: "POLYGON radius (default 50)"},
			{Name: "length", Kind: kindNumber, Positive: true,
				Desc: "LINE length (default 100)"},
			{Name: "rotation", Kind: kindNumber, Desc: "LINE rotation in degrees"},
			{Name: "strokeColor", Kind: kindString, IsHexColor: true,
				Desc: "LINE color hex"},
			{Name: "strokeWeight", Kind: kindNumber, Desc: "LINE width (default 1)"},
			parentIDParam(defaultParentDesc),
		}, autoLayoutParams()...),
		Validate: requireVariant("type", nodeVariants, "name", "x", "y"),
	},
	{
		Name: "create_text",
		Desc: "Create a text node. Loads the font for you. To edit existing text use set_text.",
		Params: append([]paramSpec{
			{Name: "text", Kind: kindString, Required: true, Desc: "Text"},
		}, append(positionParams(),
			paramSpec{Name: "fontSize", Kind: kindNumber, Desc: "Font size (default 14)"},
			paramSpec{Name: "fontFamily", Kind: kindString, Desc: "Font family e.g. 'Roboto' (default Inter)"},
			paramSpec{Name: "fontStyle", Kind: kindString, Desc: "Font style e.g. 'Bold' (default Regular). Must exist for the family."},
			paramSpec{Name: "fillColor", Kind: kindString, IsHexColor: true, Desc: "Text color hex (default black)"},
			paramSpec{Name: "name", Kind: kindString, Desc: "Layer name (default: the text)"},
			parentIDParam(defaultParentDesc),
		)...),
	},
	{
		Name: "import_image",
		Desc: "Add an image from a URL (preferred) or base64. Creates a rectangle sized to the image, or fills an existing node if nodeId is set.",
		Params: append([]paramSpec{
			{Name: "imageUrl", Kind: kindString, Desc: "Image URL"},
			{Name: "imageData", Kind: kindString, Desc: "Base64 PNG or JPG, if no URL"},
			{Name: "nodeId", Kind: kindString, IsNodeID: true,
				Desc: "Fill this node with the image instead of making a rectangle"},
		}, append(positionParams(),
			paramSpec{Name: "width", Kind: kindNumber, Positive: true,
				Desc: "Width (default: image size, max 1000)"},
			paramSpec{Name: "height", Kind: kindNumber, Positive: true,
				Desc: "Height (default: image size, max 1000)"},
			paramSpec{Name: "name", Kind: kindString, Desc: "Node name"},
			paramSpec{Name: "scaleMode", Kind: kindString, Enum: []string{"FILL", "FIT", "CROP", "TILE"},
				Desc: "FILL (default), FIT, CROP, or TILE"},
			paramSpec{Name: "mode", Kind: kindString, Enum: []string{"replace", "append"},
				Desc: "With nodeId: replace fills (default) or append"},
			paramSpec{Name: "crop", Kind: kindObject,
				Desc: "Part of the image to show, {x, y, width, height} each 0-1. Sets scaleMode CROP.",
				ObjectSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"x":      map[string]any{"type": "number", "description": "Left, 0-1"},
						"y":      map[string]any{"type": "number", "description": "Top, 0-1"},
						"width":  map[string]any{"type": "number", "description": "0-1"},
						"height": map[string]any{"type": "number", "description": "0-1"},
					},
				}},
			paramSpec{Name: "filters", Kind: kindObject,
				Desc: "Adjustments, each -1 to 1 (0 = none)",
				ObjectSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"exposure":    map[string]any{"type": "number"},
						"contrast":    map[string]any{"type": "number"},
						"saturation":  map[string]any{"type": "number"},
						"temperature": map[string]any{"type": "number", "description": "-1 cool to 1 warm"},
						"tint":        map[string]any{"type": "number"},
						"highlights":  map[string]any{"type": "number"},
						"shadows":     map[string]any{"type": "number"},
					},
				}},
			parentIDParam(defaultParentDesc),
		)...),
		Validate: func(_ []string, params map[string]any) string {
			_, hasURL := params["imageUrl"]
			_, hasData := params["imageData"]
			if !hasURL && !hasData {
				return "imageUrl or imageData is required"
			}
			if hasURL && hasData {
				return "pass imageUrl or imageData, not both"
			}
			if crop, ok := params["crop"].(map[string]any); ok {
				if msg := figma.ValidateImageCrop(crop); msg != "" {
					return msg
				}
				// CROP is what makes the transform mean anything; any other
				// scale mode would take the crop and silently ignore it.
				if mode, ok := params["scaleMode"].(string); ok && mode != "CROP" {
					return "crop needs scaleMode CROP, or scaleMode left out"
				}
			}
			if filters, ok := params["filters"].(map[string]any); ok {
				if msg := figma.ValidateImageFilters(filters); msg != "" {
					return msg
				}
			}
			return ""
		},
	},
	{
		Name:       "create_component",
		Desc:       "Turn a FRAME into a COMPONENT, in place.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "FRAME node ID to convert",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Desc: "Component name (default: frame name)"},
		},
	},
}

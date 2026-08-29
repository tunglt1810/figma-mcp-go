package tools

// parentIDParam is the shared "where does this node go" argument.
func parentIDParam(desc string) paramSpec {
	return paramSpec{Name: "parentId", Kind: kindString, IsNodeID: true, Desc: desc}
}

var defaultParentDesc = "Parent node ID in colon format. Defaults to current page."

// positionParams are the x/y arguments every create tool shares.
func positionParams() []paramSpec {
	return []paramSpec{
		{Name: "x", Kind: kindNumber, Desc: "X position (default 0)"},
		{Name: "y", Kind: kindNumber, Desc: "Y position (default 0)"},
	}
}

// autoLayoutParams describe a frame's auto-layout (flex) configuration.
func autoLayoutParams() []paramSpec {
	return []paramSpec{
		{Name: "layoutMode", Kind: kindString, Enum: []string{"HORIZONTAL", "VERTICAL", "NONE"},
			Desc: "Auto-layout direction: HORIZONTAL, VERTICAL, or NONE"},
		{Name: "paddingTop", Kind: kindNumber, Desc: "Auto-layout top padding"},
		{Name: "paddingRight", Kind: kindNumber, Desc: "Auto-layout right padding"},
		{Name: "paddingBottom", Kind: kindNumber, Desc: "Auto-layout bottom padding"},
		{Name: "paddingLeft", Kind: kindNumber, Desc: "Auto-layout left padding"},
		{Name: "itemSpacing", Kind: kindNumber, Desc: "Auto-layout gap between children"},
		{Name: "primaryAxisAlignItems", Kind: kindString, Enum: []string{"MIN", "CENTER", "MAX", "SPACE_BETWEEN"},
			Desc: "Main-axis alignment: MIN, CENTER, MAX, or SPACE_BETWEEN"},
		{Name: "counterAxisAlignItems", Kind: kindString, Enum: []string{"MIN", "CENTER", "MAX", "BASELINE"},
			Desc: "Cross-axis alignment: MIN, CENTER, MAX, or BASELINE"},
		{Name: "primaryAxisSizingMode", Kind: kindString, Enum: []string{"FIXED", "AUTO"},
			Desc: "Main-axis sizing: FIXED or AUTO (hug)"},
		{Name: "counterAxisSizingMode", Kind: kindString, Enum: []string{"FIXED", "AUTO"},
			Desc: "Cross-axis sizing: FIXED or AUTO (hug)"},
		{Name: "layoutWrap", Kind: kindString, Enum: []string{"NO_WRAP", "WRAP"},
			Desc: "Wrap behaviour: NO_WRAP or WRAP"},
		{Name: "counterAxisSpacing", Kind: kindNumber,
			Desc: "Gap between wrapped rows/columns (only when layoutWrap is WRAP)"},
		{Name: "layoutSizingHorizontal", Kind: kindString, Enum: layoutSizingValues,
			Desc: "Horizontal sizing: FIXED, HUG (shrink to contents), or FILL (fill the parent). Prefer this over counterAxisSizingMode/primaryAxisSizingMode — it is the only way to express 'fill container'. HUG needs auto-layout on this node; FILL needs auto-layout on its parent."},
		{Name: "layoutSizingVertical", Kind: kindString, Enum: layoutSizingValues,
			Desc: "Vertical sizing: FIXED, HUG (shrink to contents), or FILL (fill the parent). Same requirements as layoutSizingHorizontal."},
		{Name: "minWidth", Kind: kindNumber, Min: floatPtr(0), Nullable: true,
			Desc: "Minimum width in pixels; pass null to clear it"},
		{Name: "maxWidth", Kind: kindNumber, Min: floatPtr(0), Nullable: true,
			Desc: "Maximum width in pixels; pass null to clear it"},
		{Name: "minHeight", Kind: kindNumber, Min: floatPtr(0), Nullable: true,
			Desc: "Minimum height in pixels; pass null to clear it"},
		{Name: "maxHeight", Kind: kindNumber, Min: floatPtr(0), Nullable: true,
			Desc: "Maximum height in pixels; pass null to clear it"},
		{Name: "layoutPositioning", Kind: kindString, Enum: []string{"AUTO", "ABSOLUTE"},
			Desc: "How this node sits in its PARENT's auto layout: AUTO to flow with its siblings, ABSOLUTE to float free of them at its own x/y"},
		{Name: "layoutAlign", Kind: kindString, Enum: []string{"MIN", "CENTER", "MAX", "STRETCH", "INHERIT"},
			Desc: "Cross-axis alignment of this node within its PARENT's auto layout"},
		{Name: "layoutGrow", Kind: kindNumber, Min: floatPtr(0), Max: floatPtr(1),
			Desc: "Whether this node grows along its PARENT's main axis: 0 to keep its size, 1 to fill the free space"},
		{Name: "itemReverseZIndex", Kind: kindBool,
			Desc: "Stack children last-on-top instead of first-on-top"},
		{Name: "strokesIncludedInLayout", Kind: kindBool,
			Desc: "Count this node's stroke width as part of the layout size"},
		{Name: "clipsContent", Kind: kindBool,
			Desc: "Clip children to the frame's bounds"},
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
		Desc: "Create a shape on the current page or inside a parent. `type` selects which, and each takes its own arguments — " +
			"FRAME: width, height, fillColor, and the auto-layout arguments. " +
			"RECTANGLE: width, height, fillColor, cornerRadius. " +
			"ELLIPSE: width, height, fillColor, startAngle, endAngle, innerRadiusRatio (the last three make arcs and rings). " +
			"STAR: pointCount, outerRadius, innerRadius, fillColor, cornerRadius. " +
			"POLYGON: pointCount, radius, fillColor, cornerRadius. " +
			"LINE: length, rotation, strokeColor, strokeWeight. " +
			"SECTION: width, height — and no parent, sections live on the page. " +
			"x, y and name apply to all of them. An argument belonging to a different shape is rejected rather than ignored. " +
			"Use create_text for text and create_component_instance for components.",
		Params: append([]paramSpec{
			{Name: "type", Kind: kindString, Required: true, Enum: variantKinds(nodeVariants),
				Desc: "Shape to create: FRAME, RECTANGLE, ELLIPSE, STAR, POLYGON, LINE, or SECTION"},
			{Name: "name", Kind: kindString, Desc: "Node name shown in the layers panel"},
			{Name: "x", Kind: kindNumber, Desc: "X position (default 0)"},
			{Name: "y", Kind: kindNumber, Desc: "Y position (default 0)"},
			{Name: "width", Kind: kindNumber, Positive: true,
				Desc: "Width in pixels (default 100). FRAME, RECTANGLE, ELLIPSE, SECTION."},
			{Name: "height", Kind: kindNumber, Positive: true,
				Desc: "Height in pixels (default 100). FRAME, RECTANGLE, ELLIPSE, SECTION."},
			{Name: "fillColor", Kind: kindString, IsHexColor: true,
				Desc: "Fill color as hex e.g. #FF5733. Every shape but LINE and SECTION."},
			{Name: "cornerRadius", Kind: kindNumber,
				Desc: "Corner radius in pixels. RECTANGLE, STAR, POLYGON."},
			{Name: "startAngle", Kind: kindNumber,
				Desc: "ELLIPSE: arc start angle in radians (default 0)"},
			{Name: "endAngle", Kind: kindNumber,
				Desc: "ELLIPSE: arc end angle in radians (default a full turn)"},
			{Name: "innerRadiusRatio", Kind: kindNumber, Min: floatPtr(0), Max: floatPtr(1),
				Desc: "ELLIPSE: inner radius as a fraction of the outer, for rings and donuts (default 0)"},
			{Name: "pointCount", Kind: kindNumber, Min: floatPtr(3),
				Desc: "STAR: number of points (default 5). POLYGON: number of sides (default 3)."},
			{Name: "outerRadius", Kind: kindNumber, Positive: true,
				Desc: "STAR: outer radius in pixels (default 50)"},
			{Name: "innerRadius", Kind: kindNumber, Positive: true,
				Desc: "STAR: inner radius in pixels (default 0.3819 of the outer)"},
			{Name: "radius", Kind: kindNumber, Positive: true,
				Desc: "POLYGON: radius in pixels (default 50)"},
			{Name: "length", Kind: kindNumber, Positive: true,
				Desc: "LINE: length in pixels (default 100)"},
			{Name: "rotation", Kind: kindNumber, Desc: "LINE: rotation in degrees (default 0)"},
			{Name: "strokeColor", Kind: kindString, IsHexColor: true,
				Desc: "LINE: stroke color as hex e.g. #000000"},
			{Name: "strokeWeight", Kind: kindNumber, Desc: "LINE: stroke weight in pixels (default 1)"},
			parentIDParam(defaultParentDesc + " Not accepted for SECTION."),
		}, autoLayoutParams()...),
		Validate: requireVariant("type", nodeVariants, "name", "x", "y"),
	},
	{
		Name: "create_text",
		Desc: "Create a new text node on the current page or inside a parent node. The font is loaded automatically before insertion. Returns the created node ID and bounds. Use set_text to update the content of an existing text node.",
		Params: append([]paramSpec{
			{Name: "text", Kind: kindString, Required: true, Desc: "Text content to display"},
		}, append(positionParams(),
			paramSpec{Name: "fontSize", Kind: kindNumber, Desc: "Font size in pixels (default 14)"},
			paramSpec{Name: "fontFamily", Kind: kindString, Desc: "Font family name e.g. 'Inter', 'Roboto', 'SF Pro Display' (default Inter). Must be a font installed in Figma."},
			paramSpec{Name: "fontStyle", Kind: kindString, Desc: "Font style variant e.g. 'Regular', 'Bold', 'Italic', 'Medium', 'SemiBold' (default Regular). Must match an available style for the chosen fontFamily."},
			paramSpec{Name: "fillColor", Kind: kindString, IsHexColor: true, Desc: "Text color as hex e.g. #000000 (default black)"},
			paramSpec{Name: "name", Kind: kindString, Desc: "Node name shown in the layers panel (defaults to the text content)"},
			parentIDParam(defaultParentDesc),
		)...),
	},
	{
		Name: "import_image",
		Desc: "Place an image in Figma, from a URL or from base64 data. Prefer imageUrl: base64 pushes the whole file through the plugin connection, which is the expensive half of placing an image. By default the image lands as a new rectangle sized to the image itself; pass nodeId to paint it onto a node that already exists instead — an avatar or a hero slot usually does.",
		Params: append([]paramSpec{
			{Name: "imageUrl", Kind: kindString, Desc: "URL of the image to fetch. Preferred over imageData."},
			{Name: "imageData", Kind: kindString, Desc: "Base64-encoded image data (PNG or JPG), when there is no URL to fetch"},
			{Name: "nodeId", Kind: kindString, IsNodeID: true,
				Desc: "Paint the image onto this existing node instead of creating a rectangle, colon format e.g. '4029:12345'"},
		}, append(positionParams(),
			paramSpec{Name: "width", Kind: kindNumber, Positive: true,
				Desc: "Width in pixels. Omit to use the image's own size, scaled to fit 1000px."},
			paramSpec{Name: "height", Kind: kindNumber, Positive: true,
				Desc: "Height in pixels. Omit to use the image's own size, scaled to fit 1000px."},
			paramSpec{Name: "name", Kind: kindString, Desc: "Node name"},
			paramSpec{Name: "scaleMode", Kind: kindString, Enum: []string{"FILL", "FIT", "CROP", "TILE"},
				Desc: "Image scale mode: FILL (default), FIT, CROP, or TILE"},
			paramSpec{Name: "mode", Kind: kindString, Enum: []string{"replace", "append"},
				Desc: "With nodeId: replace the node's fills (default) or append the image to them"},
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
			return ""
		},
	},
	{
		Name:       "create_component",
		Desc:       "Convert an existing FRAME node into a reusable COMPONENT. The frame is replaced in place by the new component.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "FRAME node ID to convert, in colon format e.g. '4029:12345'",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Desc: "Optional name for the component. Defaults to the frame's current name."},
		},
	},
}

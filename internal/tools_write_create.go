package internal

import "github.com/mark3labs/mcp-go/server"

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

// boxParams are position plus a positive width and height.
func boxParams() []paramSpec {
	return append(positionParams(),
		paramSpec{Name: "width", Kind: kindNumber, Positive: true, Desc: "Width in pixels (default 100)"},
		paramSpec{Name: "height", Kind: kindNumber, Positive: true, Desc: "Height in pixels (default 100)"},
	)
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
	}
}

func frameParams() []paramSpec {
	params := boxParams()
	params = append(params,
		paramSpec{Name: "name", Kind: kindString, Desc: "Frame name"},
		paramSpec{Name: "fillColor", Kind: kindString, IsHexColor: true, Desc: "Fill color as hex e.g. #FFFFFF"},
	)
	params = append(params, autoLayoutParams()...)
	return append(params, parentIDParam(defaultParentDesc))
}

var writeCreateSpecs = []toolSpec{
	{
		Name:   "create_frame",
		Desc:   "Create a new frame on the current page or inside a parent node.",
		Params: frameParams(),
	},
	{
		Name: "create_rectangle",
		Desc: "Create a new rectangle on the current page or inside a parent node.",
		Params: append(boxParams(),
			paramSpec{Name: "name", Kind: kindString, Desc: "Rectangle name"},
			paramSpec{Name: "fillColor", Kind: kindString, IsHexColor: true, Desc: "Fill color as hex e.g. #FF5733"},
			paramSpec{Name: "cornerRadius", Kind: kindNumber, Desc: "Corner radius in pixels"},
			parentIDParam(defaultParentDesc),
		),
	},
	{
		Name: "create_ellipse",
		Desc: "Create a new ellipse (circle/oval) on the current page or inside a parent node.",
		Params: append(boxParams(),
			paramSpec{Name: "name", Kind: kindString, Desc: "Ellipse name"},
			paramSpec{Name: "fillColor", Kind: kindString, IsHexColor: true, Desc: "Fill color as hex e.g. #3B82F6"},
			paramSpec{Name: "startAngle", Kind: kindNumber, Desc: "Start angle for arcs in radians (default 0)"},
			paramSpec{Name: "endAngle", Kind: kindNumber, Desc: "End angle for arcs in radians (default 0)"},
			paramSpec{Name: "innerRadiusRatio", Kind: kindNumber, Desc: "Inner radius ratio for rings/donuts (default 0)"},
			parentIDParam(defaultParentDesc),
		),
	},
	{
		Name: "create_star",
		Desc: "Create a new star shape.",
		Params: append(positionParams(),
			paramSpec{Name: "pointCount", Kind: kindNumber, Min: floatPtr(3), Desc: "Number of points (default 5)"},
			paramSpec{Name: "outerRadius", Kind: kindNumber, Positive: true, Desc: "Outer radius in pixels (default 50)"},
			paramSpec{Name: "innerRadius", Kind: kindNumber, Positive: true, Desc: "Inner radius in pixels (default calculated based on 0.3819 ratio)"},
			paramSpec{Name: "fillColor", Kind: kindString, IsHexColor: true, Desc: "Fill color as hex e.g. #FF5733"},
			paramSpec{Name: "cornerRadius", Kind: kindNumber, Desc: "Corner radius in pixels"},
			parentIDParam("Parent node ID in colon format."),
		),
	},
	{
		Name: "create_polygon",
		Desc: "Create a new polygon shape.",
		Params: append(positionParams(),
			paramSpec{Name: "pointCount", Kind: kindNumber, Min: floatPtr(3), Desc: "Number of sides (default 3)"},
			paramSpec{Name: "radius", Kind: kindNumber, Positive: true, Desc: "Radius in pixels (default 50)"},
			paramSpec{Name: "fillColor", Kind: kindString, IsHexColor: true, Desc: "Fill color as hex e.g. #FF5733"},
			paramSpec{Name: "cornerRadius", Kind: kindNumber, Desc: "Corner radius in pixels"},
			parentIDParam("Parent node ID in colon format."),
		),
	},
	{
		Name: "create_line",
		Desc: "Create a new line.",
		Params: append(positionParams(),
			paramSpec{Name: "length", Kind: kindNumber, Positive: true, Desc: "Length in pixels (default 100)"},
			paramSpec{Name: "rotation", Kind: kindNumber, Desc: "Rotation in degrees (default 0)"},
			paramSpec{Name: "strokeColor", Kind: kindString, IsHexColor: true, Desc: "Stroke color as hex e.g. #000000"},
			paramSpec{Name: "strokeWeight", Kind: kindNumber, Desc: "Stroke weight in pixels (default 1)"},
			parentIDParam("Parent node ID in colon format."),
		),
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
		Desc: "Import a base64-encoded image into Figma as a rectangle with an image fill. Use get_screenshot to capture images or provide your own base64 PNG/JPG.",
		Params: append([]paramSpec{
			{Name: "imageData", Kind: kindString, Required: true, Desc: "Base64-encoded image data (PNG or JPG)"},
		}, append(positionParams(),
			paramSpec{Name: "width", Kind: kindNumber, Positive: true, Desc: "Width in pixels (default 200)"},
			paramSpec{Name: "height", Kind: kindNumber, Positive: true, Desc: "Height in pixels (default 200)"},
			paramSpec{Name: "name", Kind: kindString, Desc: "Node name"},
			paramSpec{Name: "scaleMode", Kind: kindString, Enum: []string{"FILL", "FIT", "CROP", "TILE"},
				Desc: "Image scale mode: FILL (default), FIT, CROP, or TILE"},
			parentIDParam(defaultParentDesc),
		)...),
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
	{
		Name: "create_section",
		Desc: "Create a Figma Section node on the current page. Sections are the modern way to organize frames and groups on a page.",
		Params: append([]paramSpec{
			{Name: "name", Kind: kindString, Desc: "Section name (default 'Section')"},
		}, append(positionParams(),
			paramSpec{Name: "width", Kind: kindNumber, Positive: true, Desc: "Width in pixels"},
			paramSpec{Name: "height", Kind: kindNumber, Positive: true, Desc: "Height in pixels"},
		)...),
	},
}

func registerWriteCreateTools(s *server.MCPServer, node *Node) {
	registerSpecs(s, node, writeCreateSpecs)
}

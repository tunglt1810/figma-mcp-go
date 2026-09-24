package tools

// pageVariants say which arguments belong to which page action. Four page tools
// became one; without this the arguments of the other three would be accepted
// and silently dropped.
var pageVariants = map[string]variantSpec{
	"add":      {Allowed: []string{"name", "index"}},
	"delete":   {Allowed: []string{"pageId", "pageName"}},
	"rename":   {Allowed: []string{"pageId", "pageName", "newName"}, Required: []string{"newName"}},
	"navigate": {Allowed: []string{"pageId", "pageName"}},
}

// requirePageTarget accepts either a page id or an exact page name.
func requirePageTarget(_ []string, params map[string]any) string {
	pageID, _ := params["pageId"].(string)
	pageName, _ := params["pageName"].(string)
	if pageID == "" && pageName == "" {
		return "pageId or pageName is required"
	}
	return ""
}

var writePageSpecs = []toolSpec{
	{
		Name: "manage_page",
		Desc: "Add, delete, rename, or go to a page. Arguments per `action`: " +
			"add: name, index. " +
			"delete: pageId or pageName. " +
			"rename: pageId or pageName, plus newName. " +
			"navigate: pageId or pageName. " +
			"List pages with get_metadata.",
		Params: []paramSpec{
			{Name: "action", Kind: kindString, Required: true, Enum: variantKinds(pageVariants),
				Desc: "add, delete, rename, or navigate"},
			{Name: "pageId", Kind: kindString,
				Desc: "Page ID"},
			{Name: "pageName", Kind: kindString,
				Desc: "Exact page name, instead of pageId"},
			{Name: "name", Kind: kindString, Desc: "New page name (default 'Page')"},
			{Name: "index", Kind: kindNumber, Min: floatPtr(0),
				Desc: "Position, 0 = first (default last)"},
			{Name: "newName", Kind: kindString, Desc: "New name"},
		},
		Validate: func(nodeIDs []string, params map[string]any) string {
			if msg := requireVariant("action", pageVariants)(nodeIDs, params); msg != "" {
				return msg
			}
			// Everything but add works on a page that already exists.
			if action, _ := params["action"].(string); action != "add" {
				return requirePageTarget(nodeIDs, params)
			}
			return ""
		},
	},
}

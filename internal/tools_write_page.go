package internal

import "github.com/mark3labs/mcp-go/server"

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
		Desc: "Add, delete, rename, or navigate to a page. `action` selects which, and each takes its own arguments — " +
			"add: name, index. " +
			"delete: pageId or pageName. " +
			"rename: pageId or pageName, plus newName. " +
			"navigate: pageId or pageName. " +
			"An argument belonging to a different action is rejected rather than ignored. " +
			"Use get_pages to list page IDs and names.",
		Params: []paramSpec{
			{Name: "action", Kind: kindString, Required: true, Enum: variantKinds(pageVariants),
				Desc: "What to do: add, delete, rename, or navigate"},
			{Name: "pageId", Kind: kindString,
				Desc: "Page node ID in colon format e.g. '0:2' (delete, rename, navigate)"},
			{Name: "pageName", Kind: kindString,
				Desc: "Exact page name, an alternative to pageId (delete, rename, navigate)"},
			{Name: "name", Kind: kindString, Desc: "add: name for the new page (default 'Page')"},
			{Name: "index", Kind: kindNumber, Min: floatPtr(0),
				Desc: "add: position to insert at (0 = first). Defaults to last."},
			{Name: "newName", Kind: kindString, Desc: "rename: the page's new name"},
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

func registerWritePageTools(s *server.MCPServer, node *Node) {
	registerSpecs(s, node, writePageSpecs)
}

package internal

import "github.com/mark3labs/mcp-go/server"

// requirePageTarget accepts either a page id or an exact page name.
func requirePageTarget(_ []string, params map[string]interface{}) string {
	pageID, _ := params["pageId"].(string)
	pageName, _ := params["pageName"].(string)
	if pageID == "" && pageName == "" {
		return "pageId or pageName is required"
	}
	return ""
}

var writePageSpecs = []toolSpec{
	{
		Name: "add_page",
		Desc: "Add a new page to the Figma document.",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Desc: "Name for the new page (default 'Page')"},
			{Name: "index", Kind: kindNumber, Min: floatPtr(0),
				Desc: "Position index to insert the page (0 = first). Defaults to last position."},
		},
	},
	{
		Name: "delete_page",
		Desc: "Delete a page from the Figma document. Cannot delete the only remaining page.",
		Params: []paramSpec{
			{Name: "pageId", Kind: kindString, Desc: "Page node ID in colon format e.g. '0:2'"},
			{Name: "pageName", Kind: kindString, Desc: "Exact page name to delete (alternative to pageId)"},
		},
		Validate: requirePageTarget,
	},
	{
		Name: "rename_page",
		Desc: "Rename an existing page in the Figma document.",
		Params: []paramSpec{
			{Name: "pageId", Kind: kindString, Desc: "Page node ID in colon format e.g. '0:2'"},
			{Name: "pageName", Kind: kindString, Desc: "Current page name to find (alternative to pageId)"},
			{Name: "newName", Kind: kindString, Required: true, Desc: "New name for the page"},
		},
		Validate: requirePageTarget,
	},
}

func registerWritePageTools(s *server.MCPServer, node *Node) {
	registerSpecs(s, node, writePageSpecs)
}

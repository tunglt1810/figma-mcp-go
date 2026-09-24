package tools

// Note which tools carry their node id in the nodeIDs field and which carry it
// in params: the plugin handlers read one or the other, not both.
var readDocumentSpecs = []toolSpec{
	{
		Name: "get_document",
		Desc: "Get the node tree of the selection, the current page, or the whole file (`scope`). " +
			"Returns {fileName, scope, currentPage, nodes}. Full detail stops at 500 nodes by default. " +
			"If cut short, the result has `truncated`, and nodes with hidden children show `childCount`.",
		Params: []paramSpec{
			{Name: "scope", Kind: kindString, Enum: []string{"selection", "page", "document"},
				Desc: "page (default): current page. document: every page, one shared maxNodes limit. " +
					"selection: selected nodes, 2 levels deep by default; uses the page if nothing is selected. Best for exploring."},
			{Name: "depth", Kind: kindNumber, Min: floatPtr(0),
				Desc: "Levels below each root. 0 = root only. Default 2 for selection, no limit otherwise."},
			{Name: "maxNodes", Kind: kindNumber, Min: floatPtr(1),
				Desc: "Max nodes in full detail (default 500). Not used with `detail` or dedupe_components."},
			{Name: "detail", Kind: kindString, Enum: []string{"minimal", "compact", "full"},
				Desc: "minimal: id/name/type/bounds. compact: + fills/strokes/opacity. full (default): everything. Use lower levels for big files."},
			{Name: "dedupe_components", Wire: "dedupeComponents", Kind: kindBool,
				Desc: "Show instances in short form (mainComponentId, componentProperties, overrides) and list each component once in `componentDefs`. Saves tokens on screens with many instances."},
		},
	},
	{
		Name: "get_metadata",
		Desc: "Get file name, current page, and every page's ID and name. Cheap: loads no node trees.",
	},
	{
		Name: "get_selection",
		Desc: "Get the selected nodes, or the nodes pinned in the plugin panel. Empty array if none. For more detail use get_nodes_info.",
		Params: []paramSpec{
			{Name: "source", Kind: kindString, Enum: []string{"selection", "pinned"},
				Desc: "selection (default): what is selected now; changes when the user clicks. pinned: set saved in the panel; stays the same across calls."},
		},
	},
	{
		Name: "get_nodes_info",
		Desc: "Get full details of nodes by ID. Returns {nodes}, plus globalVars.styles when fills or strokes repeat. " +
			"Unknown IDs are listed in `missing`. Stops at 500 descendants by default; if cut short, has `truncated` and `childCount`.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Node IDs",
		Params: []paramSpec{
			{Name: "depth", Kind: kindNumber, Min: floatPtr(0),
				Desc: "Levels below each node. 0 = the nodes only. Default no limit."},
			{Name: "maxNodes", Kind: kindNumber, Min: floatPtr(1),
				Desc: "Max descendants across all nodes (default 500)."},
		},
	},
	{
		Name: "search_nodes",
		Desc: "Find nodes by name, type, or both. Searches the current page by default. " +
			"Returns up to `limit` (default 50) and sets `truncated` if there were more.",
		Params: []paramSpec{
			{Name: "query", Kind: kindString,
				Desc: "Text to find in node names (case-insensitive). Omit to match all nodes of `types`."},
			{Name: "nodeId", Kind: kindString, IsNodeID: true,
				Desc: "Search only inside this node. Overrides scope."},
			{Name: "scope", Kind: kindString, Enum: []string{"page", "document"},
				Desc: "page (default) or document (every page). A page search does not look at other pages."},
			{Name: "types", Kind: kindStringArray,
				Desc: "Node types e.g. ['TEXT', 'FRAME']"},
			{Name: "includeText", Kind: kindBool,
				Desc: "Add characters, fontSize and fontName to TEXT results (default false)."},
			{Name: "includeHidden", Kind: kindBool,
				Desc: "Search hidden nodes (default true). false skips hidden nodes and their children."},
			{Name: "limit", Kind: kindNumber, Min: floatPtr(1),
				Desc: "Max results (default 50)"},
		},
		Validate: requireAnyOf(
			"at least one of query or types is required — a search with neither would return every node on the page",
			"query", "types"),
	},
	{
		Name:       "get_reactions",
		Desc:       "Get a node's prototype reactions: each has a trigger (e.g. ON_CLICK) and actions. Change them with set_reactions.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID",
	},
	{
		Name: "get_viewport",
		Desc: "Get the viewport center, zoom, and visible bounds.",
	},
	{
		Name: "get_fonts",
		Desc: "List fonts used on the current page, most used first.",
	},
	{
		Name:       "get_instance_overrides",
		Desc:       "Get an instance's component properties (variant, boolean, text) with types and current values. Use before set_instance_overrides.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "INSTANCE node ID",
	},
}

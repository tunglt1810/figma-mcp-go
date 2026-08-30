package tools

// Note which tools carry their node id in the nodeIDs field and which carry it
// in params: the plugin handlers read one or the other, not both.
var readDocumentSpecs = []toolSpec{
	{
		Name: "get_document",
		Desc: "Get the node tree of the current page, or of every page with scope 'document'. Unbounded by default and can be very large; pass depth or maxNodes to cap it. A capped result sets `truncated`, and every node whose children were withheld reports `childCount` and `childrenOmitted`, so a short answer is never mistaken for a whole one. Prefer get_design_context for exploration or when token efficiency matters.",
		Params: []paramSpec{
			{Name: "scope", Kind: kindString, Enum: []string{"page", "document"},
				Desc: "'page' (default) walks the current page. 'document' walks every page in the file, sharing one depth/maxNodes budget across them and reporting each page as a child of the document."},
			{Name: "depth", Kind: kindNumber, Min: floatPtr(0),
				Desc: "How many levels below the page to walk. 0 returns the page alone. Omit for no limit."},
			{Name: "maxNodes", Kind: kindNumber, Min: floatPtr(1),
				Desc: "Stop after this many nodes, walking in tree order so the result is the same every time. Omit for no limit."},
		},
	},
	{
		Name: "get_metadata",
		Desc: "Get metadata about the current Figma document: file name, page count, which page is open, " +
			"and every page in the file with its ID and name. Loads no node trees, so it is the cheap way to find a page ID before working on it.",
	},
	{
		Name: "get_selection",
		Desc: "Get the nodes currently selected in Figma, or the set the user pinned in the plugin panel. Returns an empty array if nothing is selected or pinned. Use get_design_context or get_nodes_info to retrieve deeper detail about specific nodes by ID.",
		Params: []paramSpec{
			{Name: "source", Kind: kindString, Enum: []string{"selection", "pinned"},
				Desc: "'selection' (default) follows what is selected right now, which moves the moment the user clicks elsewhere. 'pinned' reads the set they pinned in the panel, which holds still across a conversation — prefer it when you need the same nodes over several calls."},
		},
	},
	{
		Name: "get_nodes_info",
		Desc: "Get full details for one or more nodes by ID in one round-trip. " +
			"Answers {nodes: [...]}, plus a globalVars.styles map when a fill or stroke was shared by more than one node and collapsed to a ref. " +
			"An ID that matches nothing is reported under `missing` rather than dropped, so a typo cannot read as a node with no content. " +
			"Node IDs must use colon format e.g. '4029:12345', never hyphens.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "List of node IDs in colon format e.g. ['4029:12345', '4029:67890']",
	},
	{
		Name: "get_design_context",
		Desc: "Get a depth-limited, token-efficient tree of the current selection or page. Use this instead of get_document when exploring large files. Supports detail levels (minimal/compact/full) and dedupe_components for pages heavy with repeated component instances.",
		Params: []paramSpec{
			{Name: "depth", Kind: kindNumber, Min: floatPtr(0),
				Desc: "How many levels deep to traverse (default 2)"},
			{Name: "detail", Kind: kindString, Enum: []string{"minimal", "compact", "full"},
				Desc: "Property verbosity: minimal (id/name/type/bounds only), compact (+fills/strokes/opacity), full (everything, default)"},
			{Name: "dedupe_components", Wire: "dedupeComponents", Kind: kindBool,
				Desc: "When true, INSTANCE nodes are serialized compactly (mainComponentId + componentProperties + overrides array of differing text/nested content) and unique component definitions are collected once in a top-level componentDefs map. Highly token-efficient for screens with many repeated component instances."},
		},
	},
	{
		Name: "search_nodes",
		Desc: "Find nodes by name substring, by type, or both — omit query to take every node of the given types, omit types to search by name alone. " +
			"Searches the current page by default; pass scope 'document' to search every page, or nodeId to search one subtree. " +
			"Pass includeText to read the content of the TEXT nodes it finds, and includeHidden false to skip hidden nodes. " +
			"The result reports `truncated` when the limit cut the answer short — raise `limit` when you mean to sweep a whole subtree, as it defaults to 50.",
		Params: []paramSpec{
			{Name: "query", Kind: kindString,
				Desc: "Name substring to match (case-insensitive). Omit to match every node, which is how you take all nodes of a type."},
			{Name: "nodeId", Kind: kindString, IsNodeID: true,
				Desc: "Scope search to this subtree, colon format e.g. '4029:12345'. Overrides scope."},
			{Name: "scope", Kind: kindString, Enum: []string{"page", "document"},
				Desc: "Where to search when no nodeId is given: 'page' for the current page (default), or 'document' for every page in the file. Use 'document' when a node may live on another page — a page search reports nothing rather than looking there."},
			{Name: "types", Kind: kindStringArray,
				Desc: "Filter by Figma node type e.g. ['TEXT', 'FRAME', 'COMPONENT']"},
			{Name: "includeText", Kind: kindBool,
				Desc: "Add characters, fontSize and fontName to every TEXT node in the answer (default false). Costs nothing on the other node types."},
			{Name: "includeHidden", Kind: kindBool,
				Desc: "Whether hidden nodes are searched (default true). Pass false to skip a hidden node and everything under it, which is what a designer usually means by 'what is on this screen'."},
			{Name: "limit", Kind: kindNumber, Min: floatPtr(1),
				Desc: "Maximum results to return (default: 50)"},
		},
		Validate: requireAnyOf(
			"at least one of query or types is required — a search with neither would return every node on the page",
			"query", "types"),
	},
	{
		Name:       "get_reactions",
		Desc:       "Get the prototype reactions defined on a node. Returns an array of reaction objects — each has a trigger (e.g. ON_CLICK, ON_HOVER, AFTER_TIMEOUT) and an actions array (navigate to node, open URL, go back, etc.). Use set_reactions to add or replace reactions, remove_reactions to delete them.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID in colon format e.g. '4029:12345'",
	},
	{
		Name: "get_viewport",
		Desc: "Get the current Figma viewport: scroll center, zoom level, and visible bounds.",
	},
	{
		Name: "get_fonts",
		Desc: "List all fonts used in the current page, sorted by usage frequency. Useful for understanding typography without scanning all text nodes.",
	},
	{
		Name:       "get_instance_overrides",
		Desc:       "Get the component properties (variants, booleans, text) of a component instance. Includes their current values and type information. Use this before set_instance_overrides to know what properties exist.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "INSTANCE node ID in colon format e.g. 4029:12345",
	},
}

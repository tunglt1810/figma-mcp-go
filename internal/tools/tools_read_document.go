package tools

// Note which tools carry their node id in the nodeIDs field and which carry it
// in params: the plugin handlers read one or the other, not both.
var readDocumentSpecs = []toolSpec{
	{
		Name: "get_document",
		Desc: "Get the node tree of the current page (not the whole file — only the active page). Unbounded by default and can be very large; pass depth or maxNodes to cap it. A capped result sets `truncated`, and every node whose children were withheld reports `childCount` and `childrenOmitted`, so a short answer is never mistaken for a whole one. Prefer get_design_context for exploration or when token efficiency matters.",
		Params: []paramSpec{
			{Name: "depth", Kind: kindNumber, Min: floatPtr(0),
				Desc: "How many levels below the page to walk. 0 returns the page alone. Omit for no limit."},
			{Name: "maxNodes", Kind: kindNumber, Min: floatPtr(1),
				Desc: "Stop after this many nodes, walking in tree order so the result is the same every time. Omit for no limit."},
		},
	},
	{
		Name: "get_pages",
		Desc: "List all pages in the document with their IDs and names. Lightweight alternative to get_document.",
	},
	{
		Name: "get_metadata",
		Desc: "Get metadata about the current Figma document: file name, pages, current page",
	},
	{
		Name: "get_selection",
		Desc: "Get the nodes currently selected in Figma. Returns an empty array if nothing is selected. Use get_design_context or get_node to retrieve deeper detail about a specific node by ID.",
	},
	{
		Name:       "get_node",
		Desc:       "Get a single node by ID with full detail. Use get_nodes_info to fetch multiple nodes in one round-trip instead of calling this repeatedly. Node ID must be colon format e.g. '4029:12345', never hyphens.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID in colon format e.g. '4029:12345'",
	},
	{
		Name:       "get_nodes_info",
		Desc:       "Get full details for multiple nodes by ID in one round-trip. Prefer this over calling get_node repeatedly when you need several nodes.",
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
		Desc: "Search for nodes by name substring and/or type. Searches the current page by default; pass scope 'document' to search every page, or nodeId to search one subtree. Use this when you know (part of) the node name. Use scan_nodes_by_types when you want all nodes of a type regardless of name. The result reports `truncated` when the limit cut the answer short.",
		Params: []paramSpec{
			{Name: "query", Kind: kindString, Required: true,
				Desc: "Name substring to match (case-insensitive)"},
			{Name: "nodeId", Kind: kindString, IsNodeID: true,
				Desc: "Scope search to this subtree, colon format e.g. '4029:12345'. Overrides scope."},
			{Name: "scope", Kind: kindString, Enum: []string{"page", "document"},
				Desc: "Where to search when no nodeId is given: 'page' for the current page (default), or 'document' for every page in the file. Use 'document' when a node may live on another page — a page search reports nothing rather than looking there."},
			{Name: "types", Kind: kindStringArray,
				Desc: "Filter by Figma node type e.g. ['TEXT', 'FRAME', 'COMPONENT']"},
			{Name: "limit", Kind: kindNumber, Min: floatPtr(1),
				Desc: "Maximum results to return (default: 50)"},
		},
	},
	{
		Name: "scan_text_nodes",
		Desc: "Scan all TEXT nodes in a subtree and return their content, font size, and font name. Includes hidden nodes. Note that scan_nodes_by_types(['TEXT']) is not a substitute: it returns no text content and skips hidden nodes.",
		Params: []paramSpec{
			{Name: "nodeId", Kind: kindString, Required: true, IsNodeID: true,
				Desc: "Root node ID to scan from, colon format e.g. '4029:12345'"},
		},
	},
	{
		Name: "scan_nodes_by_types",
		Desc: "Find all nodes of specific types in a subtree, regardless of name. Returns id, name, type, and bounds — not text content — and skips hidden nodes. Use search_nodes to filter by name, or scan_text_nodes to read text.",
		Params: []paramSpec{
			{Name: "nodeId", Kind: kindString, Required: true, IsNodeID: true,
				Desc: "Root node ID to scan from, colon format e.g. '4029:12345'"},
			{Name: "types", Kind: kindStringArray, Required: true,
				Desc: "Node types to find e.g. ['FRAME', 'COMPONENT', 'INSTANCE']"},
		},
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

package tools

// Selection and viewport. The one write tool here changes nothing in the
// document. It points the user at the nodes the model is talking about, which
// completes the "I built the card, take a look" loop.

var writeViewportSpecs = []toolSpec{
	{
		Name: "set_selection",
		Desc: "Select and zoom to nodes (switches page if needed) to show the user your work. No IDs clears the selection. Nodes must share a page.",

		NodeIDs:    nodeIDsMulti,
		NodeIDDesc: "Node IDs. An empty list clears the selection.",
		Params: []paramSpec{
			{Name: "select", Kind: kindBool,
				Desc: "Change selection (default true). false + zoom only moves the view."},
			{Name: "zoom", Kind: kindBool,
				Desc: "Zoom to fit (default true)"},
		},
		Validate: func(nodeIDs []string, params map[string]any) string {
			// Clearing the selection is the only call that takes no nodes, and it
			// makes no sense with select off: the call would have nothing left
			// to do.
			if len(nodeIDs) == 0 {
				if selecting, ok := params["select"].(bool); ok && !selecting {
					return "nodeIds is required when select is false"
				}
			}
			return ""
		},
	},
}

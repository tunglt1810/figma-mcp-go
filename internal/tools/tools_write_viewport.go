package tools

// Selection and viewport. The one write tool here changes nothing in the
// document — it points the user at nodes the model is talking about, which is
// what closes the loop on "I built the card, take a look".

var writeViewportSpecs = []toolSpec{
	{
		Name:       "set_selection",
		Desc:       "Select nodes and zoom to them, switching page if needed. Use it to show the user your work. No IDs clears the selection. Nodes must be on one page.",
		NodeIDs:    nodeIDsMulti,
		NodeIDDesc: "Node IDs; empty clears",
		Params: []paramSpec{
			{Name: "select", Kind: kindBool,
				Desc: "Change selection (default true). false + zoom only moves the view."},
			{Name: "zoom", Kind: kindBool,
				Desc: "Zoom to fit (default true)"},
		},
		Validate: func(nodeIDs []string, params map[string]any) string {
			// Clearing the selection is the only call that takes no nodes, and
			// it is meaningless with select off — there would be nothing left
			// for the call to do.
			if len(nodeIDs) == 0 {
				if selecting, ok := params["select"].(bool); ok && !selecting {
					return "nodeIds is required when select is false"
				}
			}
			return ""
		},
	},
}

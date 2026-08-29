package tools

// Selection and viewport. The one write tool here changes nothing in the
// document — it points the user at nodes the model is talking about, which is
// what closes the loop on "I built the card, take a look".

var writeViewportSpecs = []toolSpec{
	{
		Name:       "set_selection",
		Desc:       "Select nodes in Figma and scroll the viewport to them, switching pages if needed. Use this to show the user what you just created or changed, or to point at the nodes a question is about. Pass no node IDs to clear the selection. All nodes must be on the same page — a Figma selection cannot span pages.",
		NodeIDs:    nodeIDsMulti,
		NodeIDDesc: "Node IDs to select in colon format e.g. ['4029:12345']; omit or pass an empty list to clear the selection",
		Params: []paramSpec{
			{Name: "select", Kind: kindBool,
				Desc: "Change the selection (default true). Pass false with zoom to move the camera without disturbing what the user has selected."},
			{Name: "zoom", Kind: kindBool,
				Desc: "Scroll and zoom the viewport to fit the nodes (default true)"},
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

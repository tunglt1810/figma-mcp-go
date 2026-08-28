package tools

import "github.com/tunglt1810/figma-mcp-go/internal/figma"

const setReactionsDesc = `Set prototype reactions on a node. Use mode "replace" (default) to overwrite all reactions, or "append" to add to existing ones.

Supported triggers: ON_CLICK, ON_HOVER, ON_PRESS, ON_DRAG, AFTER_TIMEOUT, MOUSE_ENTER, MOUSE_LEAVE, MOUSE_UP, MOUSE_DOWN
Supported action types: NODE (navigation), BACK, CLOSE, URL
  NODE navigation values: NAVIGATE, OVERLAY, SCROLL_TO, SWAP, CHANGE_TO
Transition types: DISSOLVE, SMART_ANIMATE, MOVE_IN, MOVE_OUT, PUSH, SLIDE_IN, SLIDE_OUT
  DISSOLVE / SMART_ANIMATE: {"type":"DISSOLVE","duration":0.3,"easing":{"type":"EASE_OUT"}}
  Directional (PUSH, MOVE_IN, MOVE_OUT, SLIDE_IN, SLIDE_OUT): also require "direction" (LEFT|RIGHT|TOP|BOTTOM) and "matchLayers" (bool):
    {"type":"PUSH","direction":"LEFT","matchLayers":false,"duration":0.3,"easing":{"type":"EASE_OUT"}}

Each reaction has a "trigger" and an "actions" array (plural). Each action in the array is an Action object.

Example — on-click navigate with dissolve:
{"nodeId":"1:2","reactions":[{"trigger":{"type":"ON_CLICK"},"actions":[{"type":"NODE","destinationId":"1:3","navigation":"NAVIGATE","transition":{"type":"DISSOLVE","duration":0.3,"easing":{"type":"EASE_OUT"}},"preserveScrollPosition":false}]}]}

Example — on-click navigate with push (directional transition):
{"nodeId":"1:2","reactions":[{"trigger":{"type":"ON_CLICK"},"actions":[{"type":"NODE","destinationId":"1:3","navigation":"NAVIGATE","transition":{"type":"PUSH","direction":"LEFT","matchLayers":false,"duration":0.3,"easing":{"type":"EASE_OUT"}},"preserveScrollPosition":false}]}]}

Example — open URL on hover:
{"nodeId":"1:2","reactions":[{"trigger":{"type":"ON_HOVER"},"actions":[{"type":"URL","url":"https://example.com"}]}]}

Example — auto-advance after 3 seconds:
{"nodeId":"1:2","reactions":[{"trigger":{"type":"AFTER_TIMEOUT","timeout":3000},"actions":[{"type":"NODE","destinationId":"1:4","navigation":"NAVIGATE","transition":{"type":"DISSOLVE","duration":0.3,"easing":{"type":"EASE_OUT"}},"preserveScrollPosition":false}]}]}

Example — go back on click:
{"nodeId":"1:2","reactions":[{"trigger":{"type":"ON_CLICK"},"actions":[{"type":"BACK"}]}]}`

var writePrototypeSpecs = []toolSpec{
	{
		Name:       "set_reactions",
		Desc:       setReactionsDesc,
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID in colon format e.g. '4029:12345'",
		Params: []paramSpec{
			{Name: "reactions", Kind: kindObjectArray, Required: true,
				Desc: "Array of reaction objects. Each has a 'trigger' and an 'actions' array (plural) of Action objects."},
			{Name: "mode", Kind: kindString, Enum: []string{"replace", "append"},
				Desc: `"replace" (default) overwrites all existing reactions; "append" adds to them`},
		},
		Validate: func(_ []string, params map[string]any) string {
			reactions, _ := params["reactions"].([]any)
			for i, raw := range reactions {
				// The element type is already checked; only the contents remain.
				r, _ := raw.(map[string]any)
				if msg := figma.ValidateReaction(i, r); msg != "" {
					return msg
				}
			}
			return ""
		},
	},
	{
		Name:       "remove_reactions",
		Desc:       "Remove prototype reactions from a node. Omit indices to remove all reactions. Provide a zero-based indices array to remove specific reactions (use get_reactions first to see current indices).",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID in colon format e.g. '4029:12345'",
		Params: []paramSpec{
			{Name: "indices", Kind: kindNumberArray,
				Desc: "Zero-based indices of reactions to remove. Omit or pass [] to remove all."},
		},
	},
}

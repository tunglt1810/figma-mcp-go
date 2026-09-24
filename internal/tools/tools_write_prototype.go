package tools

import (
	"fmt"

	"github.com/tunglt1810/figma-mcp-go/internal/figma"
)

const setReactionsDesc = `Set or remove prototype reactions on a node. mode "replace" (default) overwrites all reactions, "append" adds to them. To remove, pass removeIndices instead of reactions: zero-based indices from get_reactions, or [] to remove all.

Each reaction is {"trigger":{...},"actions":[...]} ("actions" is plural).
Triggers: ON_CLICK, ON_HOVER, ON_PRESS, ON_DRAG, AFTER_TIMEOUT (+ "timeout" in ms), MOUSE_ENTER, MOUSE_LEAVE, MOUSE_UP, MOUSE_DOWN
Action types: NODE (+ destinationId, navigation: NAVIGATE|OVERLAY|SCROLL_TO|SWAP|CHANGE_TO, transition, preserveScrollPosition), BACK, CLOSE, URL (+ url)
Transitions: DISSOLVE, SMART_ANIMATE: {"type":"DISSOLVE","duration":0.3,"easing":{"type":"EASE_OUT"}}. PUSH, MOVE_IN, MOVE_OUT, SLIDE_IN, SLIDE_OUT also require "direction" (LEFT|RIGHT|TOP|BOTTOM) and "matchLayers" (bool).

Example — navigate on click:
{"nodeId":"1:2","reactions":[{"trigger":{"type":"ON_CLICK"},"actions":[{"type":"NODE","destinationId":"1:3","navigation":"NAVIGATE","transition":{"type":"PUSH","direction":"LEFT","matchLayers":false,"duration":0.3,"easing":{"type":"EASE_OUT"}},"preserveScrollPosition":false}]}]}
Example — back on click: {"nodeId":"1:2","reactions":[{"trigger":{"type":"ON_CLICK"},"actions":[{"type":"BACK"}]}]}`

var writePrototypeSpecs = []toolSpec{
	{
		Name:       "set_reactions",
		Desc:       setReactionsDesc,
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID",
		Params: []paramSpec{
			{Name: "reactions", Kind: kindObjectArray,
				Desc: "Array of reaction objects. Each has a 'trigger' and an 'actions' array (plural) of Action objects."},
			{Name: "mode", Kind: kindString, Enum: []string{"replace", "append"},
				Desc: `"replace" (default) overwrites all existing reactions; "append" adds to them`},
			{Name: "removeIndices", Kind: kindNumberArray,
				Desc: "Zero-based indices of the reactions to remove. An empty array removes all of them. Cannot be combined with reactions."},
		},
		Validate: func(_ []string, params map[string]any) string {
			_, hasReactions := params["reactions"]
			removeIndices, hasRemove := params["removeIndices"]
			if !hasReactions && !hasRemove {
				return "one of reactions or removeIndices is required"
			}
			// Absorbed remove_reactions. Setting and removing in one call has no
			// defined order, and an empty removeIndices means "remove them all" —
			// so a call carrying both would be ambiguous in the worst direction.
			if hasReactions && hasRemove {
				return "reactions and removeIndices cannot be combined — removing is its own call"
			}
			if hasRemove {
				for i, raw := range removeIndices.([]any) {
					if n, _ := raw.(float64); n < 0 {
						return fmt.Sprintf("removeIndices[%d] must not be negative, got: %g", i, n)
					}
				}
				return ""
			}
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
}

package tools

import (
	"fmt"

	"github.com/tunglt1810/figma-mcp-go/internal/figma"
)

const setReactionsDesc = `Set or remove prototype reactions on a node. Use mode "replace" (default) to overwrite all reactions, or "append" to add to existing ones.

To remove instead, pass removeIndices rather than reactions: a zero-based list of the reactions to drop (get_reactions first to see the indices), or an empty array to remove them all.

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

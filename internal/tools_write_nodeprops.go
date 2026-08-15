package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// nodePropertyKeys are the properties set_node_properties understands. They are
// all optional and independent; at least one must be supplied.
var nodePropertyKeys = []string{
	"visible", "locked", "opacity", "rotation", "blendMode", "constraints", "order",
}

func registerNodePropertyTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("set_node_properties",
		mcp.WithDescription("Set one or more display properties on nodes in a single call: visibility, lock state, opacity, rotation, blend mode, constraints, and z-order. "+
			"Every property is optional and independent — supply only the ones you want to change. "+
			"Each node reports which properties were applied; a property the node type does not support is reported against that property alone, leaving the others applied."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithBoolean("visible", mcp.Description("Show (true) or hide (false) the nodes")),
		mcp.WithBoolean("locked", mcp.Description("Lock (true) or unlock (false) the nodes against accidental edits")),
		mcp.WithNumber("opacity", mcp.Description("Opacity from 0 (transparent) to 1 (opaque)")),
		mcp.WithNumber("rotation", mcp.Description("Absolute rotation in degrees")),
		mcp.WithString("blendMode", mcp.Description("Blend mode e.g. NORMAL, MULTIPLY, SCREEN, OVERLAY, LUMINOSITY")),
		mcp.WithObject("constraints", mcp.Description("Responsive constraints {horizontal, vertical}, each MIN, MAX, CENTER, STRETCH, or SCALE. Axes you omit keep their current value.")),
		mcp.WithString("order", mcp.Description("Change z-order: bringToFront, sendToBack, bringForward, or sendBackward")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()

		raw, _ := args["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)

		params := map[string]interface{}{}
		for _, key := range nodePropertyKeys {
			if v, ok := args[key]; ok && v != nil {
				params[key] = v
			}
		}

		resp, err := sendWithFanout(ctx, node, "set_node_properties", nodeIDs, params,
			nodePropertiesFanout(params))
		return renderResponse(resp, err)
	})
}

// nodePropertiesFanout maps a merged request onto the single-purpose commands an
// older plugin understands. The order is fixed so the fallback behaves the same
// way every time.
func nodePropertiesFanout(params map[string]interface{}) []legacyCall {
	var calls []legacyCall

	if v, ok := params["visible"]; ok {
		calls = append(calls, legacyCall{"set_visible", "visible", map[string]interface{}{"visible": v}})
	}
	if v, ok := params["locked"].(bool); ok {
		tool := "unlock_nodes"
		if v {
			tool = "lock_nodes"
		}
		calls = append(calls, legacyCall{tool, "locked", nil})
	}
	if v, ok := params["opacity"]; ok {
		calls = append(calls, legacyCall{"set_opacity", "opacity", map[string]interface{}{"opacity": v}})
	}
	if v, ok := params["rotation"]; ok {
		calls = append(calls, legacyCall{"rotate_nodes", "rotation", map[string]interface{}{"rotation": v}})
	}
	if v, ok := params["blendMode"]; ok {
		calls = append(calls, legacyCall{"set_blend_mode", "blendMode", map[string]interface{}{"blendMode": v}})
	}
	if v, ok := params["constraints"].(map[string]interface{}); ok {
		calls = append(calls, legacyCall{"set_constraints", "constraints", v})
	}
	if v, ok := params["order"]; ok {
		// The legacy command reports the resulting z-index, which is what the
		// merged response records too.
		calls = append(calls, legacyCall{"reorder_nodes", "index", map[string]interface{}{"order": v}})
	}

	return calls
}

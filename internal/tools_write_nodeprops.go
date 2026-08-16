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

		resp, err := node.Send(ctx, "set_node_properties", nodeIDs, params)
		return renderResponse(resp, err)
	})
}

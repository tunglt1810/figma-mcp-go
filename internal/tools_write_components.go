package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerWriteComponentTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("navigate_to_page",
		mcp.WithDescription("Switch the active Figma page. Provide either pageId or pageName."),
		mcp.WithString("pageId", mcp.Description("Page node ID in colon format e.g. '0:1'")),
		mcp.WithString("pageName", mcp.Description("Exact page name to navigate to")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		if id, ok := req.GetArguments()["pageId"].(string); ok && id != "" {
			params["pageId"] = id
		}
		if name, ok := req.GetArguments()["pageName"].(string); ok && name != "" {
			params["pageName"] = name
		}
		resp, err := node.Send(ctx, "navigate_to_page", nil, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("group_nodes",
		mcp.WithDescription("Group two or more nodes into a GROUP. All nodes must share the same parent."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs to group (minimum 2), in colon format e.g. ['4029:12345', '4029:12346']"),
			mcp.WithStringItems(),
		),
		mcp.WithString("name", mcp.Description("Optional name for the new group")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		params := map[string]interface{}{}
		if name, ok := req.GetArguments()["name"].(string); ok && name != "" {
			params["name"] = name
		}
		resp, err := node.Send(ctx, "group_nodes", nodeIDs, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("ungroup_nodes",
		mcp.WithDescription("Ungroup one or more GROUP nodes, moving their children to the parent and removing the group."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("GROUP node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		resp, err := node.Send(ctx, "ungroup_nodes", nodeIDs, nil)
		return renderResponse(resp, err)
	})


	s.AddTool(mcp.NewTool("swap_component",
		mcp.WithDescription("Swap the main component of an existing INSTANCE node, replacing it with a different component while keeping position and size."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("INSTANCE node ID in colon format e.g. 4029:12345"),
		),
		mcp.WithString("componentId",
			mcp.Required(),
			mcp.Description("Target COMPONENT node ID in colon format (from get_local_components)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeID, _ := args["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		componentID, _ := args["componentId"].(string)
		componentID = NormalizeNodeID(componentID)
		params := map[string]interface{}{"componentId": componentID}
		resp, err := node.Send(ctx, "swap_component", []string{nodeID}, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("detach_instance",
		mcp.WithDescription("Detach one or more component instances, converting them to plain frames. The link to the main component is broken; all visual properties are preserved."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("INSTANCE node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		for i, id := range nodeIDs {
			nodeIDs[i] = NormalizeNodeID(id)
		}
		resp, err := node.Send(ctx, "detach_instance", nodeIDs, nil)
		return renderResponse(resp, err)
	})
	s.AddTool(mcp.NewTool("create_component_instance",
		mcp.WithDescription("Create an instance of a Component. If the target is a ComponentSet (Variant Set), it automatically instantiates the default variant. It can instantiate local components or library components (using componentKey)."),
		mcp.WithString("componentId",
			mcp.Description("ID of the local component or component set in colon format e.g. 4029:12345. Preferred over componentKey if available."),
		),
		mcp.WithString("componentKey",
			mcp.Description("Key of a component from a Team Library to import and instantiate."),
		),
		mcp.WithString("parentId",
			mcp.Description("Optional. Parent node ID to place the instance inside. If missing, places it on the current page."),
		),
		mcp.WithNumber("x", mcp.Description("Optional. X coordinate. If missing, it will be centered in the viewport.")),
		mcp.WithNumber("y", mcp.Description("Optional. Y coordinate.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		params := map[string]interface{}{}
		if id, ok := args["componentId"].(string); ok && id != "" {
			params["componentId"] = NormalizeNodeID(id)
		}
		if key, ok := args["componentKey"].(string); ok && key != "" {
			params["componentKey"] = key
		}
		if pid, ok := args["parentId"].(string); ok && pid != "" {
			params["parentId"] = NormalizeNodeID(pid)
		}
		if x, ok := args["x"].(float64); ok {
			params["x"] = x
		}
		if y, ok := args["y"].(float64); ok {
			params["y"] = y
		}
		resp, err := node.Send(ctx, "create_component_instance", nil, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_instance_overrides",
		mcp.WithDescription("Update Component Properties (variants, booleans, text) on a component instance. Will fail-fast if property name or type is invalid."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("INSTANCE node ID in colon format e.g. 4029:12345"),
		),
		mcp.WithObject("properties",
			mcp.Required(),
			mcp.Description("Map of property name to its new value. Example: {\"Size\": \"Large\", \"Show Icon\": true}"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeID, _ := args["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		params := map[string]interface{}{
			"properties": args["properties"],
		}
		resp, err := node.Send(ctx, "set_instance_overrides", []string{nodeID}, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_connector",
		mcp.WithDescription("Create a Connector line in FigJam. NOTE: Only works in FigJam files!"),
		mcp.WithString("startNodeId", mcp.Description("Optional. Start node ID in colon format e.g. 1:1")),
		mcp.WithString("endNodeId", mcp.Description("Optional. End node ID in colon format e.g. 2:2")),
		mcp.WithObject("startPosition", mcp.Description("Optional. Start coordinate {x, y}")),
		mcp.WithObject("endPosition", mcp.Description("Optional. End coordinate {x, y}")),
		mcp.WithString("lineType", mcp.Description("Optional. STRAIGHT or ELBOW")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		params := map[string]interface{}{}
		if id, ok := args["startNodeId"].(string); ok && id != "" {
			params["startNodeId"] = NormalizeNodeID(id)
		}
		if id, ok := args["endNodeId"].(string); ok && id != "" {
			params["endNodeId"] = NormalizeNodeID(id)
		}
		if pos, ok := args["startPosition"]; ok {
			params["startPosition"] = pos
		}
		if pos, ok := args["endPosition"]; ok {
			params["endPosition"] = pos
		}
		if lt, ok := args["lineType"].(string); ok && lt != "" {
			params["lineType"] = lt
		}
		resp, err := node.Send(ctx, "create_connector", nil, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_annotations",
		mcp.WithDescription("Set Dev Mode Annotations on a node. Note: Requires a paid Dev Mode seat."),
		mcp.WithString("nodeId", mcp.Required(), mcp.Description("Node ID in colon format")),
		mcp.WithArray("annotations", mcp.Required(), mcp.Description("Array of annotation objects. Example: [{\"label\": \"Main Button\"}]")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeID, _ := args["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		params := map[string]interface{}{
			"annotations": args["annotations"],
		}
		resp, err := node.Send(ctx, "set_annotations", []string{nodeID}, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("clear_annotations",
		mcp.WithDescription("Clear all Dev Mode Annotations from one or more nodes."),
		mcp.WithArray("nodeIds", mcp.Required(), mcp.Description("Array of node IDs in colon format"), mcp.WithStringItems()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		for i, id := range nodeIDs {
			nodeIDs[i] = NormalizeNodeID(id)
		}
		resp, err := node.Send(ctx, "clear_annotations", nodeIDs, nil)
		return renderResponse(resp, err)
	})
}

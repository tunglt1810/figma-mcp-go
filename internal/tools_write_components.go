package internal

import "github.com/mark3labs/mcp-go/server"

var writeComponentSpecs = []toolSpec{
	{
		Name:       "group_nodes",
		Desc:       "Group two or more nodes into a GROUP. All nodes must share the same parent.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		MinNodeIDs: 2,
		NodeIDDesc: "Node IDs to group (minimum 2), in colon format e.g. ['4029:12345', '4029:12346']",
		Params: []paramSpec{
			{Name: "name", Kind: kindString, Desc: "Optional name for the new group"},
		},
	},
	{
		Name:       "ungroup_nodes",
		Desc:       "Ungroup one or more GROUP nodes, moving their children to the parent and removing the group.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "GROUP node IDs in colon format e.g. ['4029:12345']",
	},
	{
		Name:       "swap_component",
		Desc:       "Swap the main component of an existing INSTANCE node, replacing it with a different component while keeping position and size.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "INSTANCE node ID in colon format e.g. 4029:12345",
		Params: []paramSpec{
			{Name: "componentId", Kind: kindString, Required: true, IsNodeID: true,
				Desc: "Target COMPONENT node ID in colon format (from get_local_components)"},
		},
	},
	{
		Name:       "detach_instance",
		Desc:       "Detach one or more component instances, converting them to plain frames. The link to the main component is broken; all visual properties are preserved.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "INSTANCE node IDs in colon format e.g. ['4029:12345']",
	},
	{
		Name: "create_component_instance",
		Desc: "Create an instance of a Component. If the target is a ComponentSet (Variant Set), it automatically instantiates the default variant. It can instantiate local components or library components (using componentKey).",
		Params: []paramSpec{
			{Name: "componentId", Kind: kindString, IsNodeID: true,
				Desc: "ID of the local component or component set in colon format e.g. 4029:12345. Preferred over componentKey if available."},
			{Name: "componentKey", Kind: kindString,
				Desc: "Key of a component from a Team Library to import and instantiate."},
			parentIDParam("Optional. Parent node ID to place the instance inside. If missing, places it on the current page."),
			{Name: "x", Kind: kindNumber, Desc: "Optional. X coordinate. If missing, it will be centered in the viewport."},
			{Name: "y", Kind: kindNumber, Desc: "Optional. Y coordinate."},
		},
		Validate: requireAnyOf("componentId or componentKey is required", "componentId", "componentKey"),
	},
	{
		Name:       "set_instance_overrides",
		Desc:       "Update Component Properties (variants, booleans, text) on a component instance. Will fail-fast if property name or type is invalid.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "INSTANCE node ID in colon format e.g. 4029:12345",
		Params: []paramSpec{
			{Name: "properties", Kind: kindObject, Required: true,
				Desc: "Map of property name to its new value. Example: {\"Size\": \"Large\", \"Show Icon\": true}"},
		},
	},
	{
		Name: "create_connector",
		Desc: "Create a Connector line in FigJam. NOTE: Only works in FigJam files!",
		Params: []paramSpec{
			{Name: "startNodeId", Kind: kindString, IsNodeID: true,
				Desc: "Optional. Start node ID in colon format e.g. 1:1"},
			{Name: "endNodeId", Kind: kindString, IsNodeID: true,
				Desc: "Optional. End node ID in colon format e.g. 2:2"},
			{Name: "startPosition", Kind: kindObject, Desc: "Optional. Start coordinate {x, y}"},
			{Name: "endPosition", Kind: kindObject, Desc: "Optional. End coordinate {x, y}"},
			{Name: "lineType", Kind: kindString, Enum: []string{"STRAIGHT", "ELBOW"},
				Desc: "Optional. STRAIGHT or ELBOW"},
		},
		Validate: requireAnyOf(
			"at least one of startNodeId, endNodeId, startPosition, or endPosition is required",
			"startNodeId", "endNodeId", "startPosition", "endPosition"),
	},
	{
		Name:       "set_annotations",
		Desc:       "Set Dev Mode Annotations on a node. Note: Requires a paid Dev Mode seat.",
		NodeIDs:    nodeIDsSingle,
		NodeIDsReq: true,
		NodeIDDesc: "Node ID in colon format",
		Params: []paramSpec{
			{Name: "annotations", Kind: kindArray, Required: true,
				Desc: "Array of annotation objects. Example: [{\"label\": \"Main Button\"}]"},
		},
	},
	{
		Name:       "clear_annotations",
		Desc:       "Clear all Dev Mode Annotations from one or more nodes.",
		NodeIDs:    nodeIDsMulti,
		NodeIDsReq: true,
		NodeIDDesc: "Array of node IDs in colon format",
	},
}

func registerWriteComponentTools(s *server.MCPServer, node *Node) {
	registerSpecs(s, node, writeComponentSpecs)
}

package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerWriteTools(s *server.MCPServer, node *Node) {
	registerWriteCreateTools(s, node)
	registerWriteModifyTools(s, node)
	registerNodePropertyTools(s, node)
	registerWriteStyleTools(s, node)
	registerWriteVariableTools(s, node)
	registerWriteComponentTools(s, node)
	registerWritePrototypeTools(s, node)
	registerWritePageTools(s, node)
	registerBatchPipelineTool(s, node)
}

func registerBatchPipelineTool(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("batch_execute_pipeline",
		mcp.WithDescription("Execute a batch pipeline of mutation steps in Figma, passing values between steps via $variables. "+
			"On failure with stop_on_error, rollback removes nodes the pipeline created and restores properties it changed on existing nodes "+
			"(position, size, fills, strokes, opacity, visibility, name, text, blend mode, constraints). "+
			"Rollback CANNOT undo deletions (delete_nodes, delete_page), structural changes (group/ungroup, detach_instance, reparent), "+
			"or steps that target a node by name instead of id."),
		mcp.WithBoolean("stop_on_error", mcp.Description("Whether to stop execution and rollback on error (default true)")),
		mcp.WithObject("steps", mcp.Description("Array of pipeline steps to execute in sequence")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "batch_execute_pipeline", nil, params)
		return renderResponse(resp, err)
	})
}

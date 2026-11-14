package prompts

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type prompt struct {
	name        string
	description string
	body        string
}

var all = []prompt{
	readDesignStrategy,
	designStrategy,
	textReplacementStrategy,
	annotationConversionStrategy,
	swapOverridesInstances,
	reactionToConnectorStrategy,
	styleAuditStrategy,
	bulkRenameStrategy,
	designTokenGenerationStrategy,
	generateColorPalette,
	generateTypeScale,
	generateComponentVariants,
}

// RegisterAll registers all MCP prompts on the server.
func RegisterAll(s *server.MCPServer) {
	for _, p := range all {
		s.AddPrompt(
			mcp.NewPrompt(p.name, mcp.WithPromptDescription(p.description)),
			func(context.Context, mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				return mcp.NewGetPromptResult(p.description, []mcp.PromptMessage{
					mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(p.body)),
				}), nil
			},
		)
	}
}

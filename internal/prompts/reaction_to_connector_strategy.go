package prompts

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func addReactionToConnectorStrategy(s *server.MCPServer) {
	s.AddPrompt(mcp.NewPrompt("reaction_to_connector_strategy",
		mcp.WithPromptDescription("Strategy for analyzing Figma prototype reactions and mapping interaction flows"),
	), func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return mcp.NewGetPromptResult(
			"Strategy for analyzing Figma prototype reactions and mapping interaction flows",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleUser,
					mcp.NewTextContent(`# Strategy: Analyze Figma Prototype Reactions and Map Interaction Flows

## Goal
Process the JSON output from the get_reactions tool to understand prototype flows
and produce a clear, structured map of interactions between screens/nodes.

## Input Data
You will receive JSON data from get_reactions. Each node may contain reactions like:
{
  "trigger": { "type": "ON_CLICK" },
  "action": {
    "type": "NAVIGATE",
    "destinationId": "destination-node-id"
  }
}

## Step-by-Step Process

### 1. Gather Context
- Call get_nodes_info(nodeIds: [...]) on all relevant nodes to get their names and types
- Call get_document(depth: 2, detail: "minimal") to understand the page structure

### 2. Filter and Transform Reactions
- Iterate through the get_reactions JSON output
- Keep only reactions where action type implies navigation:
  * NAVIGATE, OPEN_OVERLAY, SWAP_OVERLAY
  * Ignore: CHANGE_TO, CLOSE_OVERLAY, and others without a destinationId
- Extract per reaction:
  * sourceNodeId: the node the reaction belongs to
  * destinationId: action.destinationId
  * actionType: action.type
  * triggerType: trigger.type

### 3. Generate Flow Map
For each valid reaction, create a human-readable description:
- "On click → navigate to [Destination Name]"
- "On drag → open [Destination Name] overlay"
- "On hover → swap to [Destination Name]"

Combine these into a structured flow map grouped by source screen.

### 4. Output Format
Produce a summary like:

Flow Map:
- [Screen A] --ON_CLICK/NAVIGATE--> [Screen B]
- [Screen A] --ON_CLICK/OPEN_OVERLAY--> [Modal C]
- [Screen B] --ON_CLICK/NAVIGATE--> [Screen C]

### 5. Verification
- Use export_screenshots(items: [{nodeId: ...}]) on key screens to visually confirm the flow
- Cross-check node names from get_nodes_info with the flow map

## Notes
- Node IDs use colon format: 4029:12345 — never use hyphens
- Use get_reactions on a set of nodes that represent screens or interactive frames
- Focus on NAVIGATE actions for the primary user journey`),
				),
			},
		), nil
	})
}

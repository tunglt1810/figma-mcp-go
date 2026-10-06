# Specs: FigJam Connectors for the Figma MCP Plugin

## 1. Introduction

Connectors are a FigJam feature. They link points or nodes (such as sticky notes and shapes) to build diagrams, flowcharts, and mind maps. The MCP plugin provides the `create_connector` tool to create them.

## 2. Environment Constraints (FigJam Only)

The Figma Plugin API allows connectors only in a FigJam file (`figma.editorType === "figjam"`). The MCP plugin checks the editor type. If a client calls the tool from a regular Figma Design file, the MCP plugin returns this error:

> "The create_connector tool operates only in a FigJam file"

## 3. Create a Connector (`create_connector`)

### Input Payload

```json
{
  "startNodeId": "1:1",
  "endNodeId": "2:2",
  "startPosition": { "x": 100, "y": 200 },
  "endPosition": { "x": 500, "y": 200 },
  "lineType": "ELBOW"
}
```

*Note*: The payload must contain at least one of `startNodeId`, `endNodeId`, `startPosition`, and `endPosition`. A start point or an end point can be a node ID or coordinates.

### Initialization and Geometry Logic

1. **Initialize**: Call `const connector = figma.createConnector()`.
2. **Configure the start point (`connectorStart`)**:
   - If the payload contains `startNodeId`, set `endpointNodeId` and use `magnet = "AUTO"`. Figma then selects the best attachment point on the edge of the node:
     ```typescript
     connector.connectorStart = { endpointNodeId: startNode.id, magnet: "AUTO" };
     ```
   - If the payload contains `startPosition`, set the canvas coordinates directly:
     ```typescript
     connector.connectorStart = { position: p.startPosition };
     ```
3. **Configure the end point (`connectorEnd`)**:
   - Use the same procedure as for the start point. Accept `endNodeId` or `endPosition`.
4. **Set the line shape (`connectorLineType`)**:
   - A connector can be a straight line (`STRAIGHT`) or an elbow line (`ELBOW`). If the payload contains `lineType`, set `connector.connectorLineType = p.lineType`.


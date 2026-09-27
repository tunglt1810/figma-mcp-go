# Specs: FigJam Connectors for the Figma MCP Plugin

## 1. Introduction

Connectors are a FigJam feature. They link points or nodes (such as sticky notes and shapes) to build diagrams, flowcharts, and mind maps. The MCP plugin provides the `create_connector` tool to create them.

## 2. Environment Constraints (FigJam Only)

The Figma API only allows connectors in a FigJam file (`figma.editorType === "figjam"`). If the tool is called from a regular Figma Design file, the MCP plugin checks the editor type and returns this error:

> "create_connector is only supported in FigJam files"

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

*Note*: You must give a start point and an end point. Each one can be a node ID or coordinates.

### Initialization and Geometry Logic

1. **Initialize**: Call `const connector = figma.createConnector()`.
2. **Configure the start point (`connectorStart`)**:
   - If `startNodeId` is provided, set `endpointNodeId` and use `magnet = "AUTO"`, so Figma picks the best attachment point on the node's edge:
     ```typescript
     connector.connectorStart = { endpointNodeId: startNode.id, magnet: "AUTO" };
     ```
   - If `startPosition` is provided, set the canvas coordinates directly:
     ```typescript
     connector.connectorStart = { position: p.startPosition };
     ```
3. **Configure the end point (`connectorEnd`)**:
   - Same as the start point: accept either `endNodeId` or `endPosition`.
4. **Set the line shape (`connectorLineType`)**:
   - A connector can be a straight (`STRAIGHT`) or elbow (`ELBOW`) line. If `lineType` is provided, set `connector.connectorLineType = p.lineType`.


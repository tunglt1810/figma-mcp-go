# Specs: FigJam Connectors for the Figma MCP Plugin

## 1. Introduction

Connectors are a FigJam-specific feature that lets users connect points or Nodes (such as sticky notes and shapes) to create diagrams, flowcharts, and mind maps. The MCP Plugin provides the `create_connector` tool for manipulating these connectors directly.

## 2. Environment Constraints (FigJam Only)

The Figma API requires Connector operations to run only in a FigJam file (`figma.editorType === "figjam"`). If a user attempts to call the tool from a regular Figma Design file, the MCP Plugin checks the editor type and returns the error:

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

*Note*: At least one start point and one end point must be provided. Each can be specified either by Node ID or by coordinates.

### Initialization and Geometry Logic

1. **Initialize**: Call `const connector = figma.createConnector()`.
2. **Configure the start point (`connectorStart`)**:
   - If `startNodeId` is provided, set `endpointNodeId` and use the magnetic anchor `magnet = "AUTO"` so Figma automatically selects the best attachment point on the Node's edge:
     ```typescript
     connector.connectorStart = { endpointNodeId: startNode.id, magnet: "AUTO" };
     ```
   - If `startPosition` is provided, assign the coordinates directly on the canvas:
     ```typescript
     connector.connectorStart = { position: p.startPosition };
     ```
3. **Configure the end point (`connectorEnd`)**:
   - As with the start point, support either `endNodeId` or `endPosition`.
4. **Set the line shape (`connectorLineType`)**:
   - A Connector can use a straight (`STRAIGHT`) or elbow (`ELBOW`) line. If `lineType` is provided, assign `connector.connectorLineType = p.lineType`.

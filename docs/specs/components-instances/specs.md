# Specs: Components & Instances Management for the Figma MCP Plugin

## 1. Introduction

Figma uses components and instances to make UI reusable. A component can stand alone (`COMPONENT`) or be part of a set (`COMPONENT_SET`). The MCP plugin can create an instance from a component ID or component key, and can read and write override properties.

## 2. Create a Component Instance (`create_component_instance`)

### Input Payload

```json
{
  "componentId": "1:2",
  "componentKey": "abc123xyz...",
  "parentId": "3:4",
  "x": 100,
  "y": 200
}
```

### Initialization Logic

1. **Find the base component**:
   - If `componentId` is provided, use `figma.getNodeByIdAsync`.
   - If `componentKey` is provided, use `figma.importComponentByKeyAsync`.
2. **Handle a Component Set**:
   - If the node is a `COMPONENT_SET`, use its `defaultVariant` as the base component.
   - If there is no `defaultVariant`, use the first variant in the `children` array.
3. **Create the instance**: Call `baseComponent.createInstance()`.
4. **Position it and add it to the tree (parent)**:
   - If `parentId` is provided, add the instance to that parent.
   - Otherwise, add it to `figma.currentPage`.
   - Set `x` and `y` when provided. If they are not provided and the parent is a `PAGE`, center the instance in the viewport:
     ```typescript
     instance.x = figma.viewport.center.x - instance.width / 2;
     instance.y = figma.viewport.center.y - instance.height / 2;
     ```

## 3. Read Instance Overrides (`get_instance_overrides`)

### Purpose

Read the component properties currently set on an instance. These are the overrides shown in Figma's right-hand panel.

### Logic

- Find the node by `nodeId`.
- Check that its type is `INSTANCE`.
- Read `instance.componentProperties`. Return a map from each property name to an object with its `type` and `value`.

## 4. Write Instance Overrides (`set_instance_overrides`)

### Input Payload

```json
{
  "nodeId": "1:1",
  "properties": {
    "Size": "Large",
    "Show Icon": true
  }
}
```

### Logic

- Find the node by `nodeId` (it must be an `INSTANCE`).
- Call `instance.setProperties(properties)` with the full map `{ [propertyName: string]: value }`.
- **Fail fast**: if a property is invalid (wrong name or type), the Figma API throws an error. MCP catches it and returns it to the client.


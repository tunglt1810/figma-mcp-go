# Specs: Components & Instances Management for the Figma MCP Plugin

## 1. Introduction

Figma uses components and instances to make UI reusable. A component can be independent (`COMPONENT`) or be a part of a set (`COMPONENT_SET`). The MCP plugin can create an instance from a component ID or a component key. The MCP plugin can also read and write override properties.

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
   - If the payload contains `componentId`, call `figma.getNodeByIdAsync`.
   - If the payload contains `componentKey`, call `figma.importComponentByKeyAsync`. If that call fails, call `figma.importComponentSetByKeyAsync`.
2. **Handle a Component Set**:
   - If the node is a `COMPONENT_SET`, use its `defaultVariant` as the base component.
   - If there is no `defaultVariant`, use the first variant in the `children` array.
3. **Create the instance**: Call `baseComponent.createInstance()`.
4. **Add the instance to the tree (parent) and set the position**:
   - If the payload contains `parentId`, add the instance to that parent.
   - If the payload does not contain `parentId`, add the instance to `figma.currentPage`.
   - If the payload contains `x` and `y`, set the two values. The server rejects a payload that contains only one of the two.
   - If the payload contains neither `x` nor `y`, examine the parent type. If the parent is a `PAGE`, put the instance in the viewport center:
     ```typescript
     instance.x = figma.viewport.center.x - instance.width / 2;
     instance.y = figma.viewport.center.y - instance.height / 2;
     ```

## 3. Swap a Component (`swap_component`)

The tool changes the main component of an `INSTANCE` node. It finds the new component with the same logic as steps 1 and 2 of `create_component_instance`. Thus, the payload can contain `componentId` or `componentKey`, and a component set gives its default variant.

## 4. Read Instance Overrides (`get_instance_overrides`)

### Purpose

Read the component properties currently set on an instance. These properties are the overrides that Figma shows in its right-hand panel.

### Logic

- Find the node by `nodeId`.
- Check that its type is `INSTANCE`.
- Read `instance.componentProperties`. Return a map from each property name to an object with its `type` and `value`.

## 5. Write Instance Overrides (`set_instance_overrides`)

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

- Find the node by `nodeId`. The node must be an `INSTANCE`.
- Call `instance.setProperties(properties)` with the full map `{ [propertyName: string]: value }`.
- **Fail fast**: If a property is invalid (wrong name or wrong type), the Figma Plugin API throws an error. The MCP plugin catches the error and returns the error to the client.


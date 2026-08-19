# Specs: Components & Instances Management for the Figma MCP Plugin

## 1. Introduction

Figma uses Components and Instances to make UI reusable. A Component can be standalone (`COMPONENT`) or part of a set (`COMPONENT_SET`). The MCP Plugin supports creating a Component Instance from an ID or Component Key, as well as reading and writing override properties.

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
   - If the found node is a `COMPONENT_SET`, automatically select its `defaultVariant` as the base component.
   - If there is no `defaultVariant`, select the first variant in the `children` array.
3. **Create the instance**: Call `baseComponent.createInstance()`.
4. **Position and attach it to the tree (Parent)**:
   - If `parentId` is provided, attach the instance to that parent.
   - Otherwise, attach it to `figma.currentPage`.
   - Update `x` and `y` when provided. If they are not provided and the parent is a `PAGE`, automatically center the instance in the viewport:
     ```typescript
     instance.x = figma.viewport.center.x - instance.width / 2;
     instance.y = figma.viewport.center.y - instance.height / 2;
     ```

## 3. Read Instance Overrides (`get_instance_overrides`)

### Purpose

Read the list of Component Properties (equivalent to the overrides in Figma's right-hand panel) currently assigned to an instance.

### Logic

- Find the node by `nodeId`.
- Ensure that its type is `INSTANCE`.
- Read `instance.componentProperties`. Return a mapping from each property name to an object containing its `type` and `value`.

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
- Use `instance.setProperties(properties)` to pass the complete mapping object `{ [propertyName: string]: value }`.
- Use a **fail-fast** approach: if the properties are invalid (wrong name or type), the Figma API throws an error; MCP catches the error and returns it to the client.

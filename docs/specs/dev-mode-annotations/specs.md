# Specs: Dev Mode Annotations for the Figma MCP Plugin

## 1. Introduction

Dev Mode Annotations are a special Figma feature for handoff from design to development. Users with a Dev Mode seat can attach annotations to the design surface containing properties such as dimensions, colors, and border radius.

The MCP Plugin supports writing (creating or replacing) and deleting annotations.
*(The `get_annotations` read API has been implemented independently.)*

## 2. Environment Constraints (Paid Users / Dev Mode Seat)

- **UI visibility**: Only users with a paid license and Dev Mode access can see Annotations in the interface.
- **API layer (technical behavior)**: The Figma Plugin API (`node.annotations`) still allows reading and writing internal data on a Node even when the user does not have permission to display it. MCP uses this behavior so LLMs can prepare technical documentation on a design without encountering an error.

## 3. Write Annotations (`set_annotations`)

### Input Payload

```json
{
  "nodeId": "1:1",
  "annotations": [
    {
      "label": "Button Container",
      "properties": [
        { "type": "width" },
        { "type": "fills" },
        { "type": "cornerRadius" }
      ]
    }
  ]
}
```

### Replacement Logic

The Figma API defines a Node's `annotations` property as a `ReadonlyArray<Annotation>`. Adding annotations requires assigning the complete array rather than calling `.push()`:

```typescript
(node as any).annotations = p.annotations;
```

*Note:* This assignment replaces all existing Annotations on the Node.

## 4. Delete Annotations (`clear_annotations`)

### Purpose

Clear all existing technical annotations from one or more nodes at the same time.

### Input Payload

```json
{
  "nodeIds": ["1:1", "1:2"]
}
```

### Logic

Iterate over the list of IDs, check whether each node supports the `annotations` property (`"annotations" in node`), and then assign an empty array:

```typescript
(node as any).annotations = [];
```

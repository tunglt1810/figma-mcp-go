# Specs: Dev Mode Annotations for the Figma MCP Plugin

## 1. Introduction

Dev Mode annotations are a Figma feature for the handoff of designs to developers. Users with a Dev Mode seat can attach annotations to a design. The annotations show properties such as dimensions, colors, and border radius.

The MCP plugin can write (create or replace) and delete annotations.
*(A separate tool, `get_annotations`, reads annotations.)*

## 2. Environment Constraints (Paid Users / Dev Mode Seat)

- **UI visibility**: Only users with a paid license and Dev Mode access can see annotations in the interface.
- **API behavior**: The Figma Plugin API (`node.annotations`) can read and write annotations on a node, even when the user cannot see the annotations. The MCP plugin uses this behavior. Thus, LLMs can add technical notes to a design without an error.

## 3. Write Annotations (`set_annotations`)

### Input Payload

```json
{
  "nodeIds": ["1:1", "1:2"],
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

In the Figma Plugin API, the type of the `annotations` property of a node is `ReadonlyArray<Annotation>`. To add annotations, assign the full array. `.push()` does not work:

```typescript
(node as any).annotations = p.annotations;
```

*Note:* This assignment replaces all existing annotations on the node.

## 4. Delete Annotations

To delete annotations, call `set_annotations` with an empty array. There is no separate tool for deletion.

### Input Payload

```json
{
  "nodeIds": ["1:1", "1:2"],
  "annotations": []
}
```

### Logic

For each ID, do these steps:

1. Check that the node supports the `annotations` property (`"annotations" in node`).
2. Assign the array.

Some nodes do not support annotations. A seat without Dev Mode makes Figma throw an error. In each of these two cases, the MCP plugin records an error for that node in `results`. The full call does not fail.


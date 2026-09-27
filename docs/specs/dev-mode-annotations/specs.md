# Specs: Dev Mode Annotations for the Figma MCP Plugin

## 1. Introduction

Dev Mode annotations are a Figma feature for handing off designs to developers. Users with a Dev Mode seat can attach annotations to a design, showing properties such as dimensions, colors, and border radius.

The MCP plugin can write (create or replace) and delete annotations.
*(Reading them, through `get_annotations`, was built separately.)*

## 2. Environment Constraints (Paid Users / Dev Mode Seat)

- **UI visibility**: Only users with a paid license and Dev Mode access can see annotations in the interface.
- **API behavior**: The Figma Plugin API (`node.annotations`) can still read and write this data on a node, even when the user cannot see it. MCP relies on this, so LLMs can add technical notes to a design without an error.

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

The Figma API types a node's `annotations` property as `ReadonlyArray<Annotation>`. To add annotations, assign the whole array. `.push()` does not work:

```typescript
(node as any).annotations = p.annotations;
```

*Note:* This assignment replaces all existing annotations on the node.

## 4. Delete Annotations

To delete, make the same call with an empty array. There is no separate tool.

### Input Payload

```json
{
  "nodeIds": ["1:1", "1:2"],
  "annotations": []
}
```

### Logic

For each ID, check that the node supports the `annotations` property (`"annotations" in node`), then assign the array. Some nodes do not support annotations, and a seat without Dev Mode makes Figma throw. Either case is reported for that node in `results`, instead of failing the whole call.


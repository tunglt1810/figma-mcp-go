# Design: Search Node in the Plugin Panel

**Date:** 2026-11-27
**Size:** small change, plugin only.

## Intent

A person (not the AI agent) has a node ID from a document, code, a log, or a link. They want to jump to that node in the open Figma file to check the design. Today only the AI can do this, through the `set_selection` tool.

## Goals

- The panel has a text box and a **Search node** button. The Enter key does the same as the button.
- A click on a node item in the panel's selection list moves the view to that node.
- The search works when the MCP server is not connected. The panel talks to the plugin core directly.

## Non-goals

- Jump to a node in a different Figma file or tab.
- Bring the Figma window to the front.
- Change the behaviour of the `set_selection` MCP tool.

## Approaches

1. **Text box in the panel (picked).** Keeps the WebSocket connection to the AI. Accepts bare IDs.
2. Quick Actions command (`Cmd+/`). Rejected: Figma restarts the plugin for the new command, which closes the running session and its connection to the AI.
3. Paste a `?node-id=` link into Figma. No code, but the person must build the link from a bare ID by hand.

## Decisions

### Layout

- The search box is at the top of the panel, above the **File** row (mockup option A).
- A change of the Figma selection does not change the text in the search box.

### Input

The box accepts these forms. More than one ID can be given, separated by commas, spaces, or new lines:

- `1:2`
- `1-2` (the URL form). The panel changes `-` to `:`.
- A full Figma link. The panel takes the `node-id` query value and decodes it.

### Search node (button or Enter)

1. Switch to the page that holds the nodes, if it is not the current page.
2. Select the nodes.
3. Fit the nodes in the view, but do not zoom in past 100%.
4. Show a short grey line, for example "Went to 2 nodes · page Checkout".

Errors show as a red line under the box, and the box border turns red:

- A node ID is not in the file.
- The nodes are on more than one page.
- The ID is a page or the document.

### Click on a node item in the selection list

- The view moves to that node with the same 100% zoom cap.
- The selection does not change. A selection change would shrink the list to one node.

### Zoom cap

`scrollAndZoomIntoView` fits the node to the view. For a small node, such as a 16px icon, this zooms to thousands of percent and hides the design around it. After the fit, if the zoom is more than 100%, the panel sets it to 100%. The view stays centred on the node. A large frame still zooms out to fit.

## Left to the builder

- Reuse the lookup, page switch, and checks in `plugin/src/write-viewport.ts`. Do not copy them.
- The name of the new panel-to-core message.
- Put the ID parser in its own module, like `plugin/src/ui/activity.ts`, so tests can run without Svelte.
- What happens on an empty box (my call: nothing).
- Strings go in `plugin/src/ui/strings.ts`.

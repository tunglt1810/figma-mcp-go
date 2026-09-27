// Plugin core: entry point, UI bootstrap, and request dispatch.

import { readHandlers } from "./read-handlers";
import { handleWriteRequest } from "./write-handlers";
import { clearCancelled, markCancelled, throwIfCancelled } from "./cancellation";
import { enqueueWrite } from "./write-queue";
import { isMutating, PIPELINE_TOOL } from "./tool-classes";
import { registerCodegen } from "./codegen";
import { getPinned, setPinned } from "./pinned";
import { readHandlers as readHandlerMap } from "./read-handlers";
import { writeHandlers as writeHandlerMap } from "./write-handlers";

const sendStatus = () => {
  figma.ui.postMessage({
    type: "plugin-status",
    payload: {
      fileName: figma.root.name,
      pageName: figma.currentPage.name,
      selectionCount: figma.currentPage.selection.length,
      selectedNodes: figma.currentPage.selection.map(node => ({
        id: node.id,
        name: node.name,
      })),
    },
  });
};

// What this build can do. The UI passes it to the server on connect, so a tool
// the server has but this plugin lacks is reported as "update the plugin",
// not as "Unknown request type" at call time. The maps live here because
// importing them into the UI would pull all the write handlers into a bundle
// that only needs the names.
//
// Sent at startup and again on ui-ready, like sendStatus. The panel's listener
// is not installed yet when showUI returns. A capability list that arrives in
// that gap leaves the server thinking the plugin announced nothing, which it
// reads as "old plugin, allow everything".
const sendCapabilities = () => {
  figma.ui.postMessage({
    type: "plugin-capabilities",
    handlers: [
      ...Object.keys(readHandlerMap),
      ...Object.keys(writeHandlerMap),
      "batch_execute_pipeline",
    ],
  });
};

const runRequest = async (request: any) => {
  // Reads are answered from one merged map. Writes have their own entry
  // point, because the pipeline must intercept them before dispatch.
  const read = readHandlers[request.type];
  const result = read ? await read(request) : await handleWriteRequest(request);
  if (result === null) throw new Error(`Unknown request type: ${request.type}`);
  return result;
};

export const handleRequest = async (request: any) => {
  try {
    // Writes wait their turn; reads do not wait. Two interleaved writes would
    // put a plain write inside a pipeline's undo checkpoint (see write-queue).
    // A pipeline queues even when all its steps only read. Two pipelines
    // running at once share one undo checkpoint, so the user's Ctrl+Z would
    // undo a run they did not ask about.
    return isMutating(request.type, request.params) || request.type === PIPELINE_TOOL
      ? await enqueueWrite(async () => {
          // Time in the queue counts against the request's timeout, and a
          // queued write sends no progress to extend it. By the time it leaves
          // the queue, the server may have given up, told the caller it failed,
          // and sent a cancel. Changing the document now would apply an edit the
          // model was told did not happen, and has already retried.
          throwIfCancelled(request.requestId);
          return runRequest(request);
        })
      : await runRequest(request);
  } catch (error) {
    return {
      type: request.type,
      requestId: request.requestId,
      error: error instanceof Error ? error.message : String(error),
    };
  } finally {
    // Either way the request is over, so its cancellation flag is no
    // longer needed.
    clearCancelled(request.requestId);
  }
};

const startPanel = () => {
  figma.showUI(__html__, {
    width: 320,
    height: 230,
    title: `Figma MCP Go [v${__APP_VERSION__}]`,
    // The panel's dark palette depends on a `figma-dark` class, and Figma only
    // adds that class to the document when the plugin asks for it here. Without
    // this, the panel stayed light whatever the editor theme was.
    themeColors: true,
  });
  sendStatus();
  sendCapabilities();

  figma.on("selectionchange", () => {
    sendStatus();
  });

  figma.on("currentpagechange", () => {
    sendStatus();
  });

  figma.ui.onmessage = async (message) => {
    if (message.type === "ui-ready") {
      sendStatus();
      sendCapabilities();
      return;
    }
    if (message.type === "get_ws_config") {
      // Passed on as is. The UI owns the defaults (see ui/prefs.ts), so a
      // value this side does not know survives a round trip instead of being
      // replaced by a default here.
      const config = await figma.clientStorage.getAsync("ws_config");
      figma.ui.postMessage({ type: "ws_config", config: config ?? null });
      return;
    }
    if (message.type === "save_ws_config") {
      await figma.clientStorage.setAsync("ws_config", message.config);
      return;
    }
    if (message.type === "set_pinned_nodes") {
      const pinned = setPinned(message.nodeIds);
      // Sent back so the panel shows what the core really holds, not what it
      // hoped it sent. The core drops blanks and duplicates.
      figma.ui.postMessage({ type: "pinned_nodes", nodeIds: pinned });
      figma.notify(
        pinned.length > 0
          ? `Pinned ${pinned.length} node(s) for your AI tool`
          : "Pin cleared",
      );
      return;
    }
    if (message.type === "get_pinned_nodes") {
      figma.ui.postMessage({ type: "pinned_nodes", nodeIds: getPinned() });
      return;
    }
    if (message.type === "cancel-request") {
      markCancelled(message.requestId);
      return;
    }
    if (message.type === "resize_ui") {
      // Clamped, so a bad message cannot make the panel too small to use, or
      // larger than the smallest laptop screen this runs on.
      const width = Math.min(Math.max(Number(message.width) || 320, 240), 800);
      const height = Math.min(Math.max(Number(message.height) || 230, 160), 900);
      figma.ui.resize(width, height);
      return;
    }
    if (message.type === "trigger_undo") {
      // Undoes the last checkpoint. The batch pipeline commits once for the
      // whole run, so this undoes the model's whole last pipeline.
      if (typeof (figma as any).triggerUndo === "function") {
        (figma as any).triggerUndo();
      } else {
        figma.notify("Undo is not available in this Figma version", { error: true });
      }
      return;
    }
    if (message.type === "notify") {
      figma.notify(message.message, { error: true });
      return;
    }
    if (message.type === "server-request") {
      const response = await handleRequest(message.payload);
      try {
        figma.ui.postMessage(response);
      } catch (err) {
        figma.ui.postMessage({
          type: response.type,
          requestId: response.requestId,
          error: err instanceof Error ? err.message : String(err),
        });
      }
    }
  };
};

// Dev Mode's Code panel runs the plugin in codegen mode. There is no panel to
// show and no server to talk to, only "what code goes with this node". If this
// path waited on the WebSocket bootstrap, the Code panel would hang on a
// connection nobody made.
if ((figma as any).mode === "codegen") {
  registerCodegen();
} else {
  startPanel();
}

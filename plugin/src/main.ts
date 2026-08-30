// Plugin core — entry point, UI bootstrap, and request dispatch.

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

// What this build can actually do. The UI passes it to the server on connect,
// so a tool the server has and this plugin does not is reported as "update the
// plugin" rather than as "Unknown request type" at call time. The maps live here
// because importing them into the UI would pull the entire write surface into a
// bundle that only needs the names.
//
// Sent at startup and again on ui-ready, like sendStatus: the panel's listener
// is not installed when showUI returns, and a capability list that lands in that
// gap leaves the server believing the plugin announced nothing — which it reads
// as "old plugin, allow everything".
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
  // Reads answer from one merged map; writes go through their own entry point
  // because the pipeline has to intercept before dispatch.
  const read = readHandlers[request.type];
  const result = read ? await read(request) : await handleWriteRequest(request);
  if (result === null) throw new Error(`Unknown request type: ${request.type}`);
  return result;
};

export const handleRequest = async (request: any) => {
  try {
    // Writes take their turn; reads do not wait. Two writes interleaving would
    // put a plain write inside a pipeline's undo checkpoint — see write-queue.
    // A pipeline queues even when every step of it only reads: two pipelines in
    // flight at once share one undo checkpoint, so the user's Ctrl+Z would
    // reverse a run they did not ask about.
    return isMutating(request.type, request.params) || request.type === PIPELINE_TOOL
      ? await enqueueWrite(async () => {
          // Time in the queue counts against the request's timeout, and a queued
          // write emits no progress to extend it. By the time it is dequeued the
          // server may already have given up, told the caller it failed and sent
          // a cancel — mutating now would apply an edit the model has been told
          // did not happen, and has retried.
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
    // Either way the request is over, so its cancellation flag has nothing
    // left to answer.
    clearCancelled(request.requestId);
  }
};

const startPanel = () => {
  figma.showUI(__html__, {
    width: 320,
    height: 230,
    title: `Figma MCP Go [v${__APP_VERSION__}]`,
    // The panel's dark palette hangs off a `figma-dark` class, and Figma only
    // puts that class on the document when the plugin asks for it here. Without
    // this the panel stayed light however the editor was themed.
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
      // Handed over raw: the UI owns the defaults (see ui/prefs.ts), so a value
      // this side has never heard of survives a round trip instead of being
      // flattened into a default here.
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
      // Echoed back so the panel shows what the core actually holds rather than
      // what it hoped it sent — the core drops blanks and duplicates.
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
      // Clamped so a bad message cannot leave the panel unusably small or larger
      // than the smallest laptop screen this runs on.
      const width = Math.min(Math.max(Number(message.width) || 320, 240), 800);
      const height = Math.min(Math.max(Number(message.height) || 230, 160), 900);
      figma.ui.resize(width, height);
      return;
    }
    if (message.type === "trigger_undo") {
      // Reverses the last checkpoint — which, since the batch pipeline commits
      // once for the whole run, is the whole of the model's last pipeline.
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

// Dev Mode's Code panel runs the plugin in codegen mode: there is no panel to
// show and no server to talk to, only "what code goes with this node". Keeping
// that path clear of the WebSocket bootstrap is the difference between the Code
// panel working and it waiting on a connection nobody made.
if ((figma as any).mode === "codegen") {
  registerCodegen();
} else {
  startPanel();
}

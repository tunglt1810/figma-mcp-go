// Panel strings.
//
// The panel carried a Vietnamese translation for a while, and the result read
// half in one language and half in the other: tool names, the activity log, the
// refusal text the server receives and the version-mismatch banner are English
// by necessity — they are read by the MCP client, or by whoever a bug report
// goes to, not by the person holding the mouse. One language throughout beats a
// panel that switches mid-sentence, so the panel now speaks the same English as
// everything around it.
//
// Collecting the copy here rather than inline in the markup still earns its
// keep: it is the one place to read every string the panel can show.

export const t = {
  file: "File",
  page: "Page",
  selection: "Selection",
  nodeCount: (count) => `${count} node(s)`,
  autoCopyId: "Auto-copy ID",
  copy: "Copy",
  copyAll: "Copy All",
  copyAllTitle: "Copy all IDs",
  copyIdTitle: "Copy ID",
  pin: "Pin",
  pinned: "Pinned",
  pinTitle:
    "Hold this selection for your AI tool. It can then ask for get_selection(source: 'pinned') and get the same nodes however the selection moves.",
  pinnedCount: (count) => `📌 Pinned ${count}`,
  pinnedTitle: "Your AI tool reads this with get_selection(source: 'pinned')",
  clear: "Clear",
  clearPinTitle: "Clear the pin",
  autoCopyBroken:
    "⚠️ Auto-copy disabled (browser policy). Connect Go MCP Server for native auto-copy, or click to retry.",
  autoCopyBrokenTitle:
    "Your browser's clipboard security policy blocks automatic copies without user clicks; connect Go MCP Server for native auto-copy, or click to retry",
  copyFailed: "⚠️ Copy failed. Click here to retry.",
  copyFailedTitle:
    "Browsers require you to click here once after a reload to allow clipboard access",
  allow: "Allow",
  decline: "Decline",
  allowQuestion: "Allow",
  waiting: (count) => `+${count} waiting`,
  activity: "Activity",
  activityTitle: "Show the activity log",
  logEmpty: "Nothing yet.",
  copyLogTitle: "Copy the log for a bug report",
  undo: "Undo",
  undoTitle: "Undo the last change in Figma",
  cancel: "Cancel",
  cancelTitle: "Cancel",
  guardOff: "Guard: off",
  guardConfirm: "Guard: confirm",
  guardReadonly: "Guard: read-only",
  guardTitle:
    "Click to change: off runs everything, confirm asks before deletes and bulk rewrites, read-only blocks every change",
  connected: "Connected",
  connectedVersion: (version) => `Connected (v${version})`,
  disconnected: "Disconnected",
  exposed: "Server reachable from the network",
  exposedTitle:
    "The server is listening on an address other than 127.0.0.1, and the connection is not authenticated — anyone who can reach that port can read and edit this file. The confirm guard has been turned on.",
  serverAddressTitle: "Click to configure server address",
  apply: "Apply",
  dismiss: "Cancel",
  resizeTitle: "Drag to resize",
  reportBug: "Report a bug",
  suggestFeature: "Suggest a feature",
};

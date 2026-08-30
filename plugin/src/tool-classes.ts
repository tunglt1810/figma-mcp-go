// How the UI classifies an incoming request, without importing the handler
// modules. Those modules touch the `figma` global at call time and pull the
// whole write surface into the UI bundle, which is a separate build; a plain
// list keeps the panel small. tool-classes.test.ts pins the lists against the
// real handler maps, so a tool cannot be added without being classified.

/** Requests that only read. Everything else changes something. */
export const READ_TOOLS: ReadonlySet<string> = new Set([
  "export_frames_to_pdf", "export_tokens", "get_annotations", "get_design_context",
  "get_document", "get_fonts", "get_instance_overrides", "get_local_components",
  "get_image_bytes", "get_metadata", "get_nodes_info",
  "get_reactions",
  "get_screenshot", "get_selection", "get_styles", "get_variable_defs",
  "get_viewport", "search_nodes",
]);

/**
 * Write-side requests that leave the document exactly as they found it.
 *
 * They are dispatched as writes but read-only mode has no reason to block
 * them: pointing the user at a node changes nothing, and saving a named
 * version is the opposite of destructive.
 */
export const HARMLESS_WRITE_TOOLS: ReadonlySet<string> = new Set([
  "set_selection",
  "navigate_to_page",
  "save_version_checkpoint",
]);

/** Plugin-data actions that only read. */
const READ_ONLY_PLUGIN_DATA_ACTIONS = new Set(["get", "keys"]);

/**
 * Requests that destroy or rewrite existing work in one call.
 *
 * Deliberately tight. Anything a single Ctrl+Z puts back and that touches one
 * node the user just pointed at is not on this list — a prompt in front of
 * every edit is a prompt nobody reads.
 */
export const DESTRUCTIVE_TOOLS: ReadonlySet<string> = new Set([
  "delete_nodes",
  "delete_page",
  "delete_style",
  "delete_variable",
  "detach_instance",
  "find_replace_text",
  "batch_rename_nodes",
  // These consume the shapes they take and hand back one new node. Undo puts
  // them back, but the pipeline's rollback log cannot — it can remove a node it
  // created, not un-merge geometry Figma has already combined.
  "boolean_operation",
  "flatten_nodes",
]);

/** The name of the pipeline tool, whose steps are classified individually. */
export const PIPELINE_TOOL = "batch_execute_pipeline";

/** Whether a request changes the document at all. */
export function isMutating(type: string, params?: any): boolean {
  if (READ_TOOLS.has(type)) return false;
  if (HARMLESS_WRITE_TOOLS.has(type)) return false;
  // manage_page merged four page tools behind an action; only navigating is
  // harmless, and blocking it would stop the model moving around a file it is
  // allowed to look at.
  if (type === "manage_page") return params?.action !== "navigate";
  // Reading a node's stored metadata changes nothing.
  if (type === "manage_plugin_data") {
    return !READ_ONLY_PLUGIN_DATA_ACTIONS.has(params?.action);
  }
  if (type === PIPELINE_TOOL) {
    const steps = params?.steps;
    if (!Array.isArray(steps)) return true;
    return steps.some((step: any) => isMutating(step?.action, step?.params));
  }
  return true;
}

/** Whether a request needs the user to confirm before it runs. */
export function isDestructive(type: string, params?: any): boolean {
  if (DESTRUCTIVE_TOOLS.has(type)) return true;
  if (type === "manage_page") return params?.action === "delete";
  // Removing a component property changes every instance that used it.
  if (type === "manage_component_properties") return params?.action === "delete";
  // A pipeline is as destructive as its worst step. It runs as one unit inside
  // the plugin core, so this is the last point at which it can be stopped.
  if (type === PIPELINE_TOOL) {
    const steps = params?.steps;
    if (!Array.isArray(steps)) return false;
    return steps.some((step: any) => isDestructive(step?.action, step?.params));
  }
  return false;
}

/** Short human-readable reason a request is being held for confirmation. */
export function destructiveReason(type: string, params?: any): string {
  if (type === PIPELINE_TOOL) {
    const steps = Array.isArray(params?.steps) ? params.steps : [];
    const worst = steps.find((step: any) => isDestructive(step?.action, step?.params));
    return worst ? `pipeline containing ${worst.action}` : "pipeline";
  }
  if (type === "manage_page" && params?.action === "delete") return "delete_page";
  if (type === "manage_component_properties" && params?.action === "delete") {
    return `delete component property ${params?.property ?? ""}`.trim();
  }
  return type;
}

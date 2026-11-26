import { commitUndoCheckpoint } from "./undo-checkpoint";
import { HandlerMap, RawHandlerMap, withEnvelope } from "./dispatch";
// manage_page is one tool for four page actions. The four implementations
// below stay separate, and only manage_page can reach them.
const PAGE_ACTIONS: Record<string, string> = {
  add: "add_page",
  delete: "delete_page",
  rename: "rename_page",
  navigate: "navigate_to_page",
};

const raw: RawHandlerMap = {
  "manage_page": async (request) => {
  const { action, ...params } = request.params || {};
  const type = PAGE_ACTIONS[action];
  if (!type) {
    throw new Error(`action must be add, delete, rename, or navigate, got: ${action}`);
  }
  return raw[type]({ ...request, params });
  },

  "add_page": async (request) => {
    const p = request.params || {};
    const page = figma.createPage();
    if (p.name) page.name = p.name;
    if (p.index != null) {
      figma.root.insertChild(Number(p.index), page);
    }
    commitUndoCheckpoint();
    return {
      id: page.id,
      name: page.name,
      index: figma.root.children.indexOf(page),
    };
  },

  "delete_page": async (request) => {
    const p = request.params || {};
    let page: PageNode | undefined;
    if (p.pageId) {
      const found = await figma.getNodeByIdAsync(p.pageId);
      if (!found) throw new Error(`Page not found: ${p.pageId}`);
      if (found.type !== "PAGE") throw new Error(`Node ${p.pageId} is not a PAGE`);
      page = found as PageNode;
    } else if (p.pageName) {
      page = figma.root.children.find(pg => pg.name === p.pageName) as PageNode | undefined;
      if (!page) throw new Error(`Page not found with name: ${p.pageName}`);
    } else {
      throw new Error("pageId or pageName is required");
    }
    if (figma.root.children.length <= 1) {
      throw new Error("Cannot delete the only page in the document");
    }
    const deletedId = page.id;
    const deletedName = page.name;
    page.remove();
    commitUndoCheckpoint();
    return { id: deletedId, name: deletedName, deleted: true };
  },

  "navigate_to_page": async (request) => {
    const p = request.params || {};
    let page: PageNode | undefined;
    if (p.pageId) {
      const found = await figma.getNodeByIdAsync(p.pageId);
      if (!found) throw new Error(`Page not found: ${p.pageId}`);
      if (found.type !== "PAGE") throw new Error(`Node ${p.pageId} is not a PAGE`);
      page = found as PageNode;
    } else if (p.pageName) {
      page = figma.root.children.find(pg => pg.name === p.pageName) as PageNode | undefined;
      if (!page) throw new Error(`Page not found with name: ${p.pageName}`);
    } else {
      throw new Error("pageId or pageName is required");
    }
    await figma.setCurrentPageAsync(page);
    return { id: page.id, name: page.name };
  },

  "rename_page": async (request) => {
    const p = request.params || {};
    let page: PageNode | undefined;
    if (p.pageId) {
      const found = await figma.getNodeByIdAsync(p.pageId);
      if (!found) throw new Error(`Page not found: ${p.pageId}`);
      if (found.type !== "PAGE") throw new Error(`Node ${p.pageId} is not a PAGE`);
      page = found as PageNode;
    } else if (p.pageName) {
      page = figma.root.children.find(pg => pg.name === p.pageName) as PageNode | undefined;
      if (!page) throw new Error(`Page not found with name: ${p.pageName}`);
    } else {
      throw new Error("pageId or pageName is required");
    }
    if (!p.newName) throw new Error("newName is required");
    const oldName = page.name;
    page.name = p.newName;
    commitUndoCheckpoint();
    return { id: page.id, oldName, name: page.name };
  },
};

export const writePageHandlers: HandlerMap = withEnvelope(raw, Object.values(PAGE_ACTIONS));

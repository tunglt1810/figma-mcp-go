// manage_page replaced add/delete/rename/navigate_to_page on the MCP surface.
// The four implementations stay separate below; only the surface merged.
const PAGE_ACTIONS: Record<string, string> = {
  add: "add_page",
  delete: "delete_page",
  rename: "rename_page",
  navigate: "navigate_to_page",
};

export const handleWritePageRequest = async (request: any): Promise<any> => {
  if (request.type === "manage_page") {
    const { action, ...params } = request.params || {};
    const type = PAGE_ACTIONS[action];
    if (!type) {
      throw new Error(`action must be add, delete, rename, or navigate, got: ${action}`);
    }
    const result = await handleWritePageRequest({ ...request, type, params });
    // Answer under the name the caller used, not the one we delegated to.
    return { ...result, type: request.type };
  }

  switch (request.type) {
    case "add_page": {
      const p = request.params || {};
      const page = figma.createPage();
      if (p.name) page.name = p.name;
      if (p.index != null) {
        figma.root.insertChild(Number(p.index), page);
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          id: page.id,
          name: page.name,
          index: figma.root.children.indexOf(page),
        },
      };
    }

    case "delete_page": {
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
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: deletedId, name: deletedName, deleted: true },
      };
    }

    case "navigate_to_page": {
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
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: page.id, name: page.name },
      };
    }

    case "rename_page": {
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
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: page.id, oldName, name: page.name },
      };
    }

    default:
      return null;
  }
};

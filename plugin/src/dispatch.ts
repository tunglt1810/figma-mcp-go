// One request name, one handler. A chain such as
// `handleRead(request) ?? handleWrite(request)` lets the first module that
// claims a name silently win. A map answers in one lookup, and merging the
// modules' maps turns a duplicate name into an error thrown at module load,
// which means at the first test that imports it.

export type PluginHandler = (request: any) => Promise<any>;

export type HandlerMap = Record<string, PluginHandler>;

/** A handler as a module writes it: it returns only the response data. */
export type RawHandlerMap = Record<string, (request: any) => Promise<any>>;

/** Wrap each handler's data in the response frame the server matches on. */
export function withEnvelope(raw: RawHandlerMap): HandlerMap {
  const wrapped: HandlerMap = {};
  for (const name of Object.keys(raw)) {
    wrapped[name] = async (request) => ({
      type: request.type,
      requestId: request.requestId,
      data: await raw[name](request),
    });
  }
  return wrapped;
}

export function mergeHandlers(...maps: HandlerMap[]): HandlerMap {
  const merged: HandlerMap = {};
  for (const map of maps) {
    for (const name of Object.keys(map)) {
      if (name in merged) {
        throw new Error(`Duplicate plugin handler for request type: ${name}`);
      }
      merged[name] = map[name];
    }
  }
  return merged;
}

// One request name, one handler. A chain such as
// `handleRead(request) ?? handleWrite(request)` lets the first module that
// claims a name silently win. A map answers in one lookup, and merging the
// modules' maps turns a duplicate name into an error thrown at module load,
// which means at the first test that imports it.

export type PluginHandler = (request: any) => Promise<any>;

export type HandlerMap = Record<string, PluginHandler>;

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

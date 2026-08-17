// One request name, one handler. The modules used to be chained —
// `handleRead(request) ?? handleWrite(request)` — so every write request walked
// three read switches first, and two modules claiming the same name meant
// whichever came first in the chain silently won. A map answers in one lookup,
// and merging the modules' maps turns a duplicate name into a thrown error at
// module load, which is to say at the first test that imports it.

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

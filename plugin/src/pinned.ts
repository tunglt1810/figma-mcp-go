// The pinned context set.
//
// get_selection follows whatever is selected right now, which makes it useless
// the moment the designer clicks somewhere else — so the working answer has been
// to copy node ids by hand and paste them into the conversation. Pinning holds a
// set still: the designer picks the nodes once, and every later call asks for
// the pin instead of the selection.
//
// It lives in the plugin core's memory rather than in plugin data. It is a
// working set for one sitting, not a property of the document, and writing it
// into the file would sync one person's scratch selection to the whole team.

let pinned: string[] = [];

/** Replace the pinned set. Duplicates and blanks are dropped. */
export function setPinned(nodeIds: unknown): string[] {
  const list = Array.isArray(nodeIds) ? nodeIds : [];
  const seen = new Set<string>();
  pinned = [];
  for (const raw of list) {
    const id = typeof raw === "string" ? raw.trim() : "";
    if (!id || seen.has(id)) continue;
    seen.add(id);
    pinned.push(id);
  }
  return pinned;
}

export function getPinned(): string[] {
  return pinned.slice();
}

export function clearPinned(): void {
  pinned = [];
}

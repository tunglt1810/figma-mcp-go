// Turns what a person pastes into the panel's search box into node IDs.

const NODE_ID = /^I?\d+:\d+(;\d+:\d+)*$/;

export function parseNodeIds(input: string): { ids: string[]; invalid: string[] } {
  const ids: string[] = [];
  const invalid: string[] = [];
  for (const token of input.split(/[\s,]+/)) {
    if (!token) continue;
    let raw: string | null = token;
    if (/^https?:\/\//i.test(token)) {
      try {
        raw = new URL(token).searchParams.get("node-id");
      } catch {
        raw = null;
      }
    }
    const id = raw?.replace(/-/g, ":");
    if (!id || !NODE_ID.test(id)) {
      invalid.push(token);
    } else if (!ids.includes(id)) {
      ids.push(id);
    }
  }
  return { ids, invalid };
}

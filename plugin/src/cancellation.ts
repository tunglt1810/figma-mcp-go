// Cancellation.
//
// The server sends a cancel frame when a request's caller has walked away or
// its budget ran out. Without it the plugin runs a long scan to completion for
// an answer nobody will read, holding the single WebSocket against the next
// request.
//
// It is advisory by design: a handler that never checks simply finishes, and
// its response is dropped on the server as "a request that is already gone".
// So a new long loop that forgets to check is slow, never broken.

const cancelled = new Set<string>();

// Ids are remembered until the request they belong to finishes, but a cancel
// can arrive for a request that already finished, and that id would then be
// remembered forever. The set is bounded and evicts oldest-first; a cancel is
// only ever consulted within the life of one request, so an evicted id cannot
// be one that still matters.
const MAX_TRACKED = 256;

export function markCancelled(requestId: string): void {
  if (!requestId) return;
  cancelled.add(requestId);
  while (cancelled.size > MAX_TRACKED) {
    const oldest = cancelled.values().next().value;
    if (oldest === undefined) break;
    cancelled.delete(oldest);
  }
}

export function isCancelled(requestId: string | undefined): boolean {
  return !!requestId && cancelled.has(requestId);
}

/** Stop remembering a request, once it is over either way. */
export function clearCancelled(requestId: string | undefined): void {
  if (requestId) cancelled.delete(requestId);
}

/**
 * Abort the current handler if its request was cancelled.
 *
 * The error travels back as an ordinary failure response, which the server
 * discards along with the rest of the request — the caller already has the
 * cancellation error it was waiting for.
 */
export function throwIfCancelled(requestId: string | undefined): void {
  if (isCancelled(requestId)) {
    throw new Error("Request cancelled");
  }
}

/** Test seam: forget everything. */
export function resetCancellations(): void {
  cancelled.clear();
}

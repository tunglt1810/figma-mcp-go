// Cancellation.
//
// The server sends a cancel frame when a request's caller has left or its
// time ran out. Without it, the plugin runs a long scan to the end for an
// answer nobody will read, and holds the single WebSocket while the next
// request waits.
//
// It is only a hint, by design. A handler that never checks just finishes,
// and the server drops its response as "a request that is already gone".
// So a new long loop that forgets to check is slow, but never broken.

const cancelled = new Set<string>();

// Ids are kept until their request finishes. But a cancel can arrive for a
// request that already finished, and that id would then be kept forever. So
// the set has a size limit and drops the oldest first. A cancel only matters
// during the life of one request, so a dropped id can never be one that
// still matters.
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

/** Stop tracking a request once it is over, either way. */
export function clearCancelled(requestId: string | undefined): void {
  if (requestId) cancelled.delete(requestId);
}

/**
 * Abort the current handler if its request was cancelled.
 *
 * The error goes back as a normal failure response, and the server discards
 * it with the rest of the request. The caller already has the cancellation
 * error it was waiting for.
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

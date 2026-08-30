// Progress reporting from the plugin core to the panel, and from there to the
// server, which extends a request's timeout each time one arrives.
//
// Two things every caller used to repeat and one of them would eventually get
// wrong: the message shape, and the `await` that lets Figma paint. A handler
// that walks thousands of nodes holds the only thread the plugin has, so a
// progress message posted without yielding is queued behind the very work it is
// reporting on and arrives all at once at the end.

/** Percentages are 1–99: 0 reads as "not started" and 100 as "done". */
export const clampProgress = (progress: number): number =>
  Math.max(1, Math.min(99, Math.round(progress)));

/**
 * Turn "step 3 of 8" into a percentage, without ever reporting 100 for the last
 * one — the response itself is what says the work finished.
 */
export const stepProgress = (done: number, total: number): number =>
  total <= 0 ? 1 : clampProgress((done / total) * 99);

export const reportProgress = async (
  requestId: string,
  progress: number,
  message: string,
): Promise<void> => {
  figma.ui.postMessage({
    type: "progress_update",
    requestId,
    progress: clampProgress(progress),
    message,
  });
  // Yields to Figma's event loop so the message actually leaves before the next
  // chunk of work starts.
  await new Promise((r) => setTimeout(r, 0));
};

// Progress reporting from the plugin core to the panel, and from there to the
// server, which extends a request's timeout each time one arrives.
//
// Every caller used to repeat two things, and sooner or later one would get
// them wrong: the message shape, and the `await` that lets Figma paint. A
// handler that walks thousands of nodes holds the plugin's only thread. So a
// progress message posted without yielding waits behind the work it reports
// on, and they all arrive at once at the end.

/** Percentages are 1–99: 0 means "not started" and 100 means "done". */
export const clampProgress = (progress: number): number =>
  Math.max(1, Math.min(99, Math.round(progress)));

/**
 * Turn "step 3 of 8" into a percentage, but never report 100 for the last
 * step. The response itself is what says the work is finished.
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
  // Yield to Figma's event loop, so the message is really sent before the
  // next piece of work starts.
  await new Promise((r) => setTimeout(r, 0));
};

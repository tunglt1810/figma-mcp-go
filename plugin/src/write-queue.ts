// One write at a time.
//
// figma.ui.onmessage is async and the server can have several requests in
// flight, so two writes could interleave. That was untidy before and is now a
// correctness problem: withSingleUndoCheckpoint swallows figma.commitUndo for
// the length of a pipeline, and a plain write landing in that window would have
// its own checkpoint swallowed too — its change would join the pipeline's undo
// step, or be rolled back with it.
//
// Reads are not queued. They change nothing, and putting a long get_document
// ahead of every write would make the queue the slowest thing in the plugin.

type Work<T> = () => Promise<T>;

let tail: Promise<unknown> = Promise.resolve();

/** Run `work` after everything already queued, and return what it returns. */
export function enqueueWrite<T>(work: Work<T>): Promise<T> {
  // Both arms run `work`: a predecessor that rejected has already reported its
  // own failure, and must not stop the next request from being served.
  const result = tail.then(work, work);
  // The queue tracks completion, not success, so one failure cannot poison it.
  tail = result.then(
    () => undefined,
    () => undefined,
  );
  return result;
}

/** Test seam: drop anything still queued. */
export function resetWriteQueue(): void {
  tail = Promise.resolve();
}

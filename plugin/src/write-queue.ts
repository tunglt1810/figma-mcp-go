// One write at a time.
//
// figma.ui.onmessage is async, and the server can have several requests in
// flight, so two writes could interleave. That used to be only untidy. Now it
// is a correctness problem. withSingleUndoCheckpoint ignores figma.commitUndo
// while a pipeline runs, and a plain write that arrives during that time
// would lose its own checkpoint too. Its change would join the pipeline's
// undo step, or be rolled back with it.
//
// Reads are not queued. They change nothing, and putting a long get_document
// ahead of every write would make the queue the slowest part of the plugin.

type Work<T> = () => Promise<T>;

let tail: Promise<unknown> = Promise.resolve();

/** Run `work` after everything already queued, and return what it returns. */
export function enqueueWrite<T>(work: Work<T>): Promise<T> {
  // Both branches run `work`. A previous request that rejected has already
  // reported its own failure, and must not block the next request.
  const result = tail.then(work, work);
  // The queue tracks completion, not success, so one failure cannot break it.
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

// All plugin-owned write handlers request checkpoints here rather than
// replacing figma.commitUndo, which is read-only in the Figma sandbox.
let checkpointDepth = 0;
let anyStepCommitted = false;

export function commitUndoCheckpoint(): void {
  if (checkpointDepth > 0) {
    anyStepCommitted = true;
  } else {
    figma.commitUndo();
  }
}

/** Group handler checkpoints, including rollback, into one undo step.
 *
 * The counter handles nested and overlapping scopes. Production request
 * dispatch serializes pipelines and standalone writes via enqueueWrite;
 * otherwise unrelated concurrent writes would share this undo step.
 */
export async function withSingleUndoCheckpoint<T>(work: () => Promise<T>): Promise<T> {
  if (typeof figma === 'undefined' || typeof figma.commitUndo !== 'function') return work();
  if (checkpointDepth === 0) anyStepCommitted = false;
  checkpointDepth++;
  try {
    return await work();
  } finally {
    checkpointDepth--;
    if (checkpointDepth === 0) {
      const shouldCommit = anyStepCommitted;
      anyStepCommitted = false;
      if (shouldCommit) figma.commitUndo();
    }
  }
}

/** Test seam: reset only when no work is running. */
export function resetUndoCheckpointState(): void {
  checkpointDepth = 0;
  anyStepCommitted = false;
}

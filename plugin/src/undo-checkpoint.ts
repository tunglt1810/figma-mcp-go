// All plugin-owned write handlers request checkpoints here rather than
// replacing figma.commitUndo, which is read-only in the Figma sandbox.
let checkpointDepth = 0;
let anyStepCommitted = false;

export function commitUndoCheckpoint(): void {
  if (checkpointDepth > 0) {
    anyStepCommitted = true;
  } else if (typeof figma.commitUndo === 'function') {
    figma.commitUndo();
  }
}

// Callers must serialize writes (enqueueWrite). Otherwise a concurrent write
// shares this undo step.
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
      // A throw here would replace the result of work(), so the caller would
      // lose failed_step and rollback_executed.
      if (shouldCommit) {
        try {
          figma.commitUndo();
        } catch (e) {
          console.error('commitUndo failed', e);
        }
      }
    }
  }
}

/** Test seam: reset only when no work is running. */
export function resetUndoCheckpointState(): void {
  checkpointDepth = 0;
  anyStepCommitted = false;
}

# Codebase cleanup plan

**Date:** 2026-11-14
**Status:** pull request #10 merged everything except Phase 10. The decisions are in `docs/specs/2026-11-15-codebase-cleanup-decisions.md`.

## Phase 10 — Pipeline rollback through Figma undo (needs an experiment first)

1. Experiment in Figma with a development build of the plugin: commit, run a pipeline that deletes, groups and reparents, call `figma.triggerUndo()`. Record whether the document returns to the committed state, and whether it does so in Dev Mode and FigJam.
2. If yes: replace the write-ahead log in `plugin/src/batch-pipeline.ts` (`SNAPSHOT_PROPS`, `snapshotNode`, `restoreNodeProperties`, `executeRollback`, `isCreateStep`, `CREATE_ACTIONS`) with commit-before and undo-on-failure. Update the tool description in `internal/tools/tools_write.go` and regenerate the golden schema.
3. If no: stop this phase.

Check: `make test`, then the experiment of step 1 as a manual regression.

## Last step

Write the rollback method in section 3 of `docs/specs/2026-11-15-codebase-cleanup-decisions.md`. Then delete this plan.

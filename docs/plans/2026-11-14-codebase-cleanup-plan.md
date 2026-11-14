# Codebase cleanup plan

**Date:** 2026-11-14
**Source:** codebase audit of 2026-11-14.
**Status:** phases 1 to 9 and 12 are done in pull request #10. Git log holds what they changed. This file lists only the work that remains.

## Dropped after a closer look

- **Move `internal/figma` into `internal/tools`.** `docs/walkthroughs/2026-08-28-go-backend-architecture.md` records the split as a decision: the rules hold with or without a tool table. The merge would only remove five `deps-check` edges.
- **Version skew in one place.** The panel must compare versions itself. A server older than the change cannot send the result, and "the server is old" is the case the banner is for.
- **Generate the README tool table.** No test reads the table, and a generator is more code than the table.
- **Dispatcher calls `figma.commitUndo()`.** Handlers that do not commit (selection, navigation) do so on purpose, and the pipeline counts the calls.

## Phase 10 — Pipeline rollback through Figma undo (needs an experiment first)

1. Experiment in Figma with a development build of the plugin: commit, run a pipeline that deletes, groups and reparents, call `figma.triggerUndo()`. Record whether the document returns to the committed state, and whether it does so in Dev Mode and FigJam.
2. If yes: replace the write-ahead log in `plugin/src/batch-pipeline.ts` (`SNAPSHOT_PROPS`, `snapshotNode`, `restoreNodeProperties`, `executeRollback`, `isCreateStep`, `CREATE_ACTIONS`) with commit-before and undo-on-failure. Update the tool description in `internal/tools/tools_write.go` and regenerate the golden schema.
3. If no: stop this phase and write the result in the spec.

Check: `make test`, then the experiment of step 1 as a manual regression.

## Phase 11 — Product decisions (need the owner's answer before work)

1. One read/write classification. Decide whether `set_selection`, page navigation and `save_version_checkpoint` are allowed in read-only mode. Then make the server flag and the panel guard use the same list, generated from the Go spec table.
2. Auto-copy. Decide between: default off, or delete it (`plugin/src/ui/copy-helper.ts`, `internal/bridge/clipboard.go`, the `copy_to_clipboard` frame, the `autoCopyBroken` state) and keep pin plus manual copy.

Check: `make test`; manual check of the panel in Figma.

## Last step

Move each lasting decision to `docs/specs` (the `/rpc` rule, the pipeline contract, the `get_document` node limit, the rollback method, the read-only list, the auto-copy decision). Then delete this plan.

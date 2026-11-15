# Specs: Lasting Decisions from the Codebase Cleanup

**Date:** 2026-11-15
**Source:** pull request #10 (the 2026-11-14 codebase audit and cleanup).

This document keeps the rules that the cleanup set. The git history keeps the changes themselves.

## 1. Leader `/rpc` endpoint

- A follower is a Go process. It never sends an `Origin` header.
- The leader refuses a request that has an `Origin` header, with 403. A browser adds this header to every cross-site POST, and a script cannot remove it.
- The leader refuses a `Content-Type` other than `application/json`, with 415. A page cannot post `application/json` without a preflight, and the server answers no preflight.
- The leader applies the same guard as a local call: `tools.Check`, or `tools.CheckReadOnly` with `--read-only`.

## 2. Tool calls

- The server refuses a tool name that it does not know, on every entry point: the MCP handler and `/rpc`.
- `internalSpecs` in `internal/tools/toolspec.go` declares the plugin requests that a Go handler sends itself (`get_screenshot`, and the plugin side of `export_frames_to_pdf`). An internal spec takes the place of a public spec with the same name in the registry. The public tool's handler checks against its own spec.

## 3. `batch_execute_pipeline` steps

- A step must name a write tool from the tool list. The server refuses read tools, Go-side tools, the pipeline itself and the export tools in `readOnlyModeExports`. The plugin refuses the same names.
- The server checks each step's arguments with the rules of a direct call to that tool, before the pipeline starts.
- An argument that holds a `$variable` reference is not checked. `$$name` is a literal string, not a reference. `TestPipelineVariable_MatchesThePlugin` keeps the Go pattern equal to the plugin's.
- The step that reaches the plugin carries the same arguments as the direct call: the server removes null, empty and undeclared arguments.
- Rollback uses the plugin's write-ahead log in `plugin/src/batch-pipeline.ts`. A move to `figma.triggerUndo()` needs an experiment in Figma first. See `docs/plans/2026-11-14-codebase-cleanup-plan.md`.

## 4. Node limit in `get_document` and `get_nodes_info`

- `maxNodes` (default 500) counts every node of the result: the pages, the selected nodes, the requested nodes, each `componentDefs` entry and each instance override.
- One budget is shared across the whole call. A tree walk spends it in tree order, so the same call gives the same answer.
- The limit applies at every `detail` level and with `dedupe_components`.
- A component definition spends the budget but ignores the depth limit, because a definition cut at that depth cannot rebuild its instances.
- A result that stops early has `truncated: true`. A node with children left out has `childCount` and `childrenOmitted`.

## 5. Read-only mode

- The Go spec table is the one definition of what read-only mode runs:
  - every spec marked `ReadOnly`, and the export tools in `readOnlyModeExports`;
  - every spec marked `Harmless`: `set_selection`, `save_version_checkpoint`;
  - the `HarmlessActions` of a spec: `manage_page` with `navigate`, `manage_plugin_data` with `get` and `keys`.
- `--read-only` refuses the other actions of a partly harmless tool, in the MCP handler and in the `/rpc` guard.
- `internal/tools/testdata/read_only_calls.json` is a snapshot of this list. `plugin/src/tool-contract.test.ts` checks the panel's `read-only` guard (`plugin/src/tool-classes.ts`) against it, for each tool and each action.

## 6. Auto-copy

- The panel copies the selected node ids when the selection changes. It is on by default.
- The owner uses it all the time. Keep it.

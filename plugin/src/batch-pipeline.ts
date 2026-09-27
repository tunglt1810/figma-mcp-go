import { isCancelled } from './cancellation';
import { reportProgress, stepProgress } from './progress';

export type SymbolTable = Map<string, any>;

export type LogEntry =
  | { type: 'CREATE'; nodeId: string }
  | { type: 'MODIFY'; nodeId: string; previousState: Record<string, any> };

export type WALStack = LogEntry[];

// Actions that add a NEW node to the document. Only these may be rolled back
// by removing the node. Every other action returns the id of a node the user
// already had, and removing it would destroy their work.
//
// Keep this in sync when adding a create-style handler. `rename_page` is the
// warning example: it returns the id of an existing PAGE.
export const CREATE_ACTIONS = new Set([
  'create_node',
  'create_frame',
  'create_rectangle',
  'create_ellipse',
  'create_star',
  'create_polygon',
  'create_line',
  'create_text',
  'create_section',
  'create_component',
  'create_component_instance',
  'create_connector',
  'import_image',
  'clone_node',
  'add_page',
]);

/**
 * Whether a step added a new node to the document, so it may be rolled back
 * by removing the node. manage_page merged four page tools behind an `action`
 * argument, so the step's name alone no longer tells us. Only `add` creates a
 * page. Treating the others as creates would remove a page the user had.
 */
export function isCreateStep(action: string, params: any): boolean {
  if (action === 'manage_page') {
    return params?.action === 'add';
  }
  return CREATE_ACTIONS.has(action);
}

// Properties saved before a mutating step, so rollback can put them back.
// On purpose, only plain node properties that can be assigned directly.
const SNAPSHOT_PROPS = [
  'x', 'y', 'width', 'height', 'rotation', 'opacity', 'visible', 'locked',
  'name', 'characters', 'fills', 'strokes', 'strokeWeight', 'blendMode',
  'constraints', 'cornerRadius',
];

// Params that name an EXISTING node the step is about to change. `parentId` is
// left out on purpose: create steps do not change the parent's own properties.
// Targets given by name (e.g. rename_page's `pageName`) cannot be turned into
// an id here, so those steps get no undo record.
const TARGET_ID_PARAMS = ['nodeId', 'pageId'];

/** Pull the target node ids out of a step's resolved params. */
export function extractNodeIds(params: any): string[] {
  if (!params) return [];
  const ids: string[] = [];
  if (Array.isArray(params.nodeIds)) {
    ids.push(...params.nodeIds.filter((v: any) => typeof v === 'string'));
  }
  for (const key of TARGET_ID_PARAMS) {
    if (typeof params[key] === 'string') ids.push(params[key]);
  }
  return [...new Set(ids)];
}

/** Capture the restorable properties a node currently has. */
export function snapshotNode(node: any): Record<string, any> {
  const state: Record<string, any> = {};
  for (const prop of SNAPSHOT_PROPS) {
    if (!(prop in node)) continue;
    const value = node[prop];
    // figma.mixed is a symbol and cannot be assigned back. Skip it instead
    // of storing something that throws on restore.
    if (typeof value === 'symbol') continue;
    state[prop] = Array.isArray(value) ? [...value] : value;
  }
  return state;
}

/** Put a snapshot back onto a node, property by property, as far as possible. */
export async function restoreNodeProperties(
  node: any,
  previousState: Record<string, any>
): Promise<void> {
  if (previousState.characters !== undefined && typeof node.fontName !== 'symbol') {
    try {
      if (typeof figma !== 'undefined' && typeof figma.loadFontAsync === 'function') {
        await figma.loadFontAsync(node.fontName);
      }
    } catch {
      // Font unavailable. The characters assignment below will just fail.
    }
  }

  for (const [key, value] of Object.entries(previousState)) {
    if (key === 'width' || key === 'height') continue; // restored together via resize()
    try {
      node[key] = value;
    } catch {
      // Read-only on this node type. Nothing better to do during rollback.
    }
  }

  const wantsResize = previousState.width !== undefined || previousState.height !== undefined;
  if (wantsResize && typeof node.resize === 'function') {
    try {
      node.resize(previousState.width ?? node.width, previousState.height ?? node.height);
    } catch {
      // The node refused the resize. Leave it as it is.
    }
  }
}

// A variable reference is the whole string and looks like an identifier.
// Treating every string that starts with $ as a reference made "$100" stop the
// pipeline with "Undefined pipeline variable: $100".
const VARIABLE_REFERENCE = /^\$[A-Za-z_][A-Za-z0-9_]*$/;

export function resolveParams(params: any, symbolTable: SymbolTable): any {
  if (typeof params === 'string') {
    // $$ escapes a literal $, for the rare string that really starts with
    // one and would otherwise look like a reference.
    if (params.startsWith('$$')) {
      return params.slice(1);
    }
    if (VARIABLE_REFERENCE.test(params)) {
      if (!symbolTable.has(params)) {
        throw new Error(`Undefined pipeline variable: ${params}`);
      }
      return symbolTable.get(params);
    }
    return params;
  }
  if (Array.isArray(params)) {
    return params.map(item => resolveParams(item, symbolTable));
  }
  if (params !== null && typeof params === 'object') {
    const resolved: Record<string, any> = {};
    for (const key of Object.keys(params)) {
      resolved[key] = resolveParams(params[key], symbolTable);
    }
    return resolved;
  }
  return params;
}

export async function executeRollback(
  stack: WALStack,
  getNodeById: (id: string) => Promise<any>
): Promise<number> {
  let count = 0;
  while (stack.length > 0) {
    const entry = stack.pop()!;
    try {
      const node = await getNodeById(entry.nodeId);
      if (!node) continue;
      if (entry.type === 'CREATE') {
        if (typeof node.remove === 'function') {
          node.remove();
          count++;
        }
      } else {
        await restoreNodeProperties(node, entry.previousState);
        count++;
      }
    } catch (err) {
      console.error('Rollback entry error:', err);
    }
  }
  return count;
}

export interface PipelineStep {
  id: string;
  action: string;
  params: Record<string, any>;
  export_vars?: Record<string, string>;
}

export interface BatchPipelineRequest {
  stop_on_error?: boolean;
  steps: PipelineStep[];
}

export interface BatchPipelineResponse {
  success: boolean;
  completed_steps: number;
  exports?: Record<string, any>;
  results?: Array<Record<string, any>>;
  failed_step?: {
    index: number;
    step_id: string;
    action: string;
    error: string;
  };
  rollback_executed?: boolean;
  rolled_back_steps?: number;
}

export async function executeBatchPipeline(
  req: BatchPipelineRequest,
  handlerDispatcher: (action: string, params: any) => Promise<any>,
  getNodeById: (id: string) => Promise<any> = async (id) =>
    typeof figma !== 'undefined' ? (figma as any).getNodeByIdAsync(id) : null,
  // Checked between steps. A pipeline is the longest thing the plugin runs.
  // A step boundary is the only place it can stop and still leave the
  // document in a state the rollback log describes.
  isCancelled: () => boolean = () => false,
  // Called before each step. It is passed in instead of posting to figma.ui
  // directly, for the same reason as getNodeById: this function is tested
  // without a Figma global.
  onProgress: (done: number, total: number, action: string) => Promise<void> =
    async () => {},
): Promise<BatchPipelineResponse> {
  const symbolTable: SymbolTable = new Map();
  const walStack: WALStack = [];
  const results: Array<Record<string, any>> = [];
  const exports: Record<string, any> = {};

  const stopOnError = req.stop_on_error !== false;

  for (let i = 0; i < req.steps.length; i++) {
    const step = req.steps[i];
    if (isCancelled()) {
      // Roll back whatever stop_on_error says. That flag is about accepting a
      // step that failed by itself. A cancelled run is different, and a
      // half-built pipeline left in place is worse than none.
      const rolledBackCount = await executeRollback(walStack, getNodeById);
      return {
        success: false,
        completed_steps: i,
        results,
        failed_step: {
          index: i,
          step_id: step.id,
          action: step.action,
          error: 'Request cancelled',
        },
        rollback_executed: true,
        rolled_back_steps: rolledBackCount,
      };
    }
    // Before the step, not after. A pipeline's last step is often its slowest,
    // and a caller wants to know what is running, not what has finished.
    await onProgress(i, req.steps.length, step.action);
    try {
      const resolvedParams = resolveParams(step.params || {}, symbolTable);
      const isCreate = isCreateStep(step.action, resolvedParams);

      // Save a snapshot before changing anything, so rollback can restore it.
      // A failed snapshot must not stop the step. It only means this node has no undo record.
      if (!isCreate) {
        for (const nodeId of extractNodeIds(resolvedParams)) {
          try {
            const node = await getNodeById(nodeId);
            if (node) {
              walStack.push({ type: 'MODIFY', nodeId, previousState: snapshotNode(node) });
            }
          } catch {
            // The node cannot be found. The handler will report the real error.
          }
        }
      }

      const res = await handlerDispatcher(step.action, resolvedParams);

      // Only real creates can be removed. Modify handlers return the id of a
      // node the user already had. Treating that as a create made rollback
      // delete their work.
      if (isCreate && res && res.id) {
        walStack.push({ type: 'CREATE', nodeId: res.id });
      }

      if (step.export_vars && res) {
        for (const [resKey, varName] of Object.entries(step.export_vars)) {
          if (resKey === 'id' && res.id) {
            symbolTable.set(varName, res.id);
            exports[varName] = res.id;
          } else if (res[resKey] !== undefined) {
            symbolTable.set(varName, res[resKey]);
            exports[varName] = res[resKey];
          }
        }
      }

      results.push({ step_id: step.id, status: 'ok', node_id: res?.id || null });
    } catch (err: any) {
      const errorMsg = err?.message || String(err);
      if (stopOnError) {
        const rolledBackCount = await executeRollback(walStack, getNodeById);
        return {
          success: false,
          completed_steps: i,
          results,
          failed_step: {
            index: i,
            step_id: step.id,
            action: step.action,
            error: errorMsg,
          },
          rollback_executed: true,
          rolled_back_steps: rolledBackCount,
        };
      } else {
        results.push({ step_id: step.id, status: 'error', error: errorMsg });
      }
    }
  }

  return {
    success: true,
    completed_steps: req.steps.length,
    exports,
    results,
  };
}

/**
 * Run `work` so all of it lands on the undo stack as one step.
 *
 * Every write handler commits its own undo checkpoint. That is right when the
 * handler is all the user asked for, but not inside a pipeline: a twenty-step
 * build left twenty checkpoints. Undoing it took twenty Ctrl+Z presses, and
 * each one left the design in a state nobody asked for.
 *
 * Figma has no way to pause commitUndo, so the handlers' calls are ignored
 * and one call is made at the end.
 *
 * The swap is counted, not saved per call, because scopes do not always nest.
 * A pipeline where every step only reads does not count as mutating, so it
 * skips the write queue and can overlap another pipeline. With a per-call
 * save and restore, the first scope to finish puts the real function back
 * while the second is still running. Then the second puts the first's stub
 * back for good, and from then on every write in the session silently loses
 * its checkpoint. A counter cannot do that: the real function goes back
 * exactly once, when the last scope leaves, whatever order they started in.
 */
let checkpointDepth = 0;
let suspendedCommitUndo: (() => void) | null = null;
let anyStepCommitted = false;

export async function withSingleUndoCheckpoint<T>(work: () => Promise<T>): Promise<T> {
  const api: any = typeof figma !== 'undefined' ? figma : null;
  if (!api || typeof api.commitUndo !== 'function') return work();

  if (checkpointDepth === 0) {
    // Kept unbound and called with the receiver below, so the exact function
    // that was there goes back. Restoring a bound copy would work, but each
    // pipeline would wrap the previous wrapper, and nothing could then check
    // that the swap was really undone.
    suspendedCommitUndo = api.commitUndo;
    anyStepCommitted = false;
    api.commitUndo = () => {
      anyStepCommitted = true;
    };
  }
  checkpointDepth++;
  try {
    return await work();
  } finally {
    checkpointDepth--;
    if (checkpointDepth === 0) {
      const realCommitUndo = suspendedCommitUndo;
      suspendedCommitUndo = null;
      api.commitUndo = realCommitUndo;
      // Nothing changed the document. A checkpoint here would be an empty
      // undo step the user has to press through.
      if (anyStepCommitted && realCommitUndo) realCommitUndo.call(api);
    }
  }
}

/** Test seam: forget any suspended swap. */
export function resetUndoCheckpointState(): void {
  checkpointDepth = 0;
  suspendedCommitUndo = null;
  anyStepCommitted = false;
}

export async function handleBatchPipelineRequest(
  request: any,
  writeDispatcher: (subReq: any) => Promise<any>
) {
  if (request.type !== 'batch_execute_pipeline') {
    return null;
  }

  const dispatcher = async (action: string, params: any) => {
    // Write handlers read `request.nodeIds`, not `params.nodeId`. Take the ids
    // from the step params so tools that use nodeIds work inside a pipeline.
    const { nodeId, nodeIds, ...rest } = params ?? {};
    const ids = Array.isArray(nodeIds) ? nodeIds : typeof nodeId === 'string' ? [nodeId] : undefined;
    const subReq = {
      type: action,
      requestId: `${request.requestId}_${action}`,
      nodeIds: ids,
      params: rest,
    };
    const res = await writeDispatcher(subReq);
    if (!res) {
      throw new Error(`Unknown pipeline action: ${action}`);
    }
    if (res.error) {
      throw new Error(res.error);
    }
    return res.data;
  };

  const pipelineParams = request.params || request;
  // Rollback runs inside executeBatchPipeline, so it is inside the checkpoint
  // too. A pipeline that fails and reverses itself leaves the undo stack as it
  // was, instead of adding steps that cancel each other out.
  const res = await withSingleUndoCheckpoint(() =>
    executeBatchPipeline(
      pipelineParams,
      dispatcher,
      undefined,
      () => isCancelled(request.requestId),
      async (done, total, action) => {
        // A one-step pipeline does not need a progress message. The response
        // arrives at about the same moment.
        if (total < 2) return;
        await reportProgress(
          request.requestId,
          stepProgress(done, total),
          `Step ${done + 1}/${total}: ${action}`,
        );
      },
    ),
  );
  return {
    type: request.type,
    requestId: request.requestId,
    data: res,
  };
}



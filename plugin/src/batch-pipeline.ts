export type SymbolTable = Map<string, any>;

export type LogEntry =
  | { type: 'CREATE'; nodeId: string }
  | { type: 'MODIFY'; nodeId: string; previousState: Record<string, any> };

export type WALStack = LogEntry[];

// Actions that bring a NEW node into the document. Only these may be rolled
// back by removal — every other action returns the id of a node the user
// already had, and removing it would destroy their work.
//
// Keep in sync when adding a create-style handler; `rename_page` is the
// cautionary example: it returns an existing PAGE id.
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
 * Whether a step brought a new node into the document, and may therefore be
 * rolled back by removal. manage_page merged four page tools behind an `action`
 * argument, so the step's name alone no longer answers this: only `add` creates
 * a page, and treating the others as creates would remove a page the user had.
 */
export function isCreateStep(action: string, params: any): boolean {
  if (action === 'manage_page') {
    return params?.action === 'add';
  }
  return CREATE_ACTIONS.has(action);
}

// Properties captured before a mutating step so rollback can put them back.
// Deliberately limited to plain, directly assignable node properties.
const SNAPSHOT_PROPS = [
  'x', 'y', 'width', 'height', 'rotation', 'opacity', 'visible', 'locked',
  'name', 'characters', 'fills', 'strokes', 'strokeWeight', 'blendMode',
  'constraints', 'cornerRadius',
];

// Params that name an EXISTING node the step is about to mutate. `parentId` is
// excluded on purpose: create steps do not change the parent's own properties.
// Name-based targeting (e.g. rename_page's `pageName`) cannot be resolved to an
// id here, so those steps simply get no undo record.
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
    // figma.mixed is a symbol and cannot be assigned back — skip it rather
    // than storing something that throws on restore.
    if (typeof value === 'symbol') continue;
    state[prop] = Array.isArray(value) ? [...value] : value;
  }
  return state;
}

/** Put a snapshot back onto a node, best-effort, property by property. */
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
      // Font unavailable — the characters assignment below will simply fail.
    }
  }

  for (const [key, value] of Object.entries(previousState)) {
    if (key === 'width' || key === 'height') continue; // restored together via resize()
    try {
      node[key] = value;
    } catch {
      // Read-only on this node type — nothing better to do during rollback.
    }
  }

  const wantsResize = previousState.width !== undefined || previousState.height !== undefined;
  if (wantsResize && typeof node.resize === 'function') {
    try {
      node.resize(previousState.width ?? node.width, previousState.height ?? node.height);
    } catch {
      // Node refused the resize — leave it as-is.
    }
  }
}

// A variable reference is the whole string and looks like an identifier.
// Treating every $-prefixed string as one meant "$100" aborted the pipeline
// with "Undefined pipeline variable: $100".
const VARIABLE_REFERENCE = /^\$[A-Za-z_][A-Za-z0-9_]*$/;

export function resolveParams(params: any, symbolTable: SymbolTable): any {
  if (typeof params === 'string') {
    // $$ escapes a literal $, for the rare string that really does start with
    // one and would otherwise read as a reference.
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
    typeof figma !== 'undefined' ? (figma as any).getNodeByIdAsync(id) : null
): Promise<BatchPipelineResponse> {
  const symbolTable: SymbolTable = new Map();
  const walStack: WALStack = [];
  const results: Array<Record<string, any>> = [];
  const exports: Record<string, any> = {};

  const stopOnError = req.stop_on_error !== false;

  for (let i = 0; i < req.steps.length; i++) {
    const step = req.steps[i];
    try {
      const resolvedParams = resolveParams(step.params || {}, symbolTable);
      const isCreate = isCreateStep(step.action, resolvedParams);

      // Snapshot before mutating so rollback can restore. Failing to snapshot
      // must not abort the step — it only means this node has no undo record.
      if (!isCreate) {
        for (const nodeId of extractNodeIds(resolvedParams)) {
          try {
            const node = await getNodeById(nodeId);
            if (node) {
              walStack.push({ type: 'MODIFY', nodeId, previousState: snapshotNode(node) });
            }
          } catch {
            // Unresolvable node — the handler will report the real error.
          }
        }
      }

      const res = await handlerDispatcher(step.action, resolvedParams);

      // Only genuine creates are removable. Modify handlers return the id of a
      // node the user already had; treating that as a create made rollback
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

export async function handleBatchPipelineRequest(
  request: any,
  writeDispatcher: (subReq: any) => Promise<any>
) {
  if (request.type !== 'batch_execute_pipeline') {
    return null;
  }

  const dispatcher = async (action: string, params: any) => {
    // Write handlers read `request.nodeIds`, not `params.nodeId`. Lift the ids
    // out of the step params so nodeIds-based tools work inside a pipeline.
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
  const res = await executeBatchPipeline(pipelineParams, dispatcher);
  return {
    type: request.type,
    requestId: request.requestId,
    data: res,
  };
}



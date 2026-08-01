export type SymbolTable = Map<string, any>;

export type LogEntry =
  | { type: 'CREATE'; nodeId: string }
  | { type: 'MODIFY'; nodeId: string; previousState: Record<string, any> };

export type WALStack = LogEntry[];

export function resolveParams(params: any, symbolTable: SymbolTable): any {
  if (typeof params === 'string') {
    if (params.startsWith('$')) {
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
      if (entry.type === 'CREATE') {
        const node = await getNodeById(entry.nodeId);
        if (node && typeof node.remove === 'function') {
          node.remove();
          count++;
        }
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
  getNodeById: (id: string) => Promise<any> = async (id) => (figma as any).getNodeByIdAsync(id)
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
      const res = await handlerDispatcher(step.action, resolvedParams);

      if (res && res.id) {
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
    const subReq = { type: action, requestId: `${request.requestId}_${action}`, params };
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



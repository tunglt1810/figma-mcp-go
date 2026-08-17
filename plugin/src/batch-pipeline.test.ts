import { beforeEach, describe, expect, it } from 'vitest';
import { executeRollback, resolveParams, SymbolTable, WALStack } from './batch-pipeline';
import { handleWriteRequest } from './write-handlers';

describe('SymbolTable & resolveParams', () => {
  it('replaces variable references correctly', () => {
    const table: SymbolTable = new Map();
    table.set('$header_id', '123:456');

    const params = {
      parent_id: '$header_id',
      title: 'Hello',
      size: 100
    };

    const resolved = resolveParams(params, table);
    expect(resolved.parent_id).toBe('123:456');
    expect(resolved.title).toBe('Hello');
  });

  it('throws error on missing variable', () => {
    const table: SymbolTable = new Map();
    const params = { parent_id: '$missing_var' };
    expect(() => resolveParams(params, table)).toThrow('Undefined pipeline variable: $missing_var');
  });

  // Any string starting with $ used to be treated as a variable reference, so
  // a price, a CSS variable or a shell-looking string aborted the pipeline.
  it.each(['$100', '$1,299.00', '$', '$ 50', '$--brand-color', '$3.50/mo'])(
    'leaves %o alone — it is not a variable name',
    (text) => {
      const table: SymbolTable = new Map();
      expect(resolveParams({ text }, table).text).toBe(text);
    },
  );

  it('still resolves real variable names', () => {
    const table: SymbolTable = new Map();
    table.set('$header_id', '1:2');
    expect(resolveParams({ id: '$header_id' }, table).id).toBe('1:2');
  });

  it('unescapes $$ to a literal $', () => {
    const table: SymbolTable = new Map();
    expect(resolveParams({ text: '$$header_id' }, table).text).toBe('$header_id');
  });
});

describe('WAL Rollback', () => {
  it('calls remove for created nodes in LIFO order', async () => {
    const removed: string[] = [];
    const fakeNodes: Record<string, any> = {
      'node_1': { remove: () => removed.push('node_1') },
      'node_2': { remove: () => removed.push('node_2') },
    };

    const stack: WALStack = [
      { type: 'CREATE', nodeId: 'node_1' },
      { type: 'CREATE', nodeId: 'node_2' }
    ];

    const count = await executeRollback(stack, async (id) => fakeNodes[id]);
    expect(removed).toEqual(['node_2', 'node_1']);
    expect(count).toBe(2);
  });
});

describe('executeBatchPipeline', () => {
  it('executes steps, resolves variables, exports vars, and handles error rollback', async () => {
    const removed: string[] = [];
    const fakeNodes: Record<string, any> = {
      'frame_1': { remove: () => removed.push('frame_1') },
    };

    const mockDispatcher = async (action: string, params: any) => {
      if (action === 'create_frame') {
        return { id: 'frame_1', name: params.name };
      }
      if (action === 'create_text') {
        if (params.parent_id !== 'frame_1') {
          throw new Error('Parent frame missing');
        }
        throw new Error('Font missing');
      }
      return {};
    };

    const req = {
      steps: [
        {
          id: 'step_1',
          action: 'create_frame',
          params: { name: 'Header' },
          export_vars: { id: '$header_id' }
        },
        {
          id: 'step_2',
          action: 'create_text',
          params: { parent_id: '$header_id', text: 'Hello' }
        }
      ]
    };

    const { executeBatchPipeline } = await import('./batch-pipeline');
    const res = await executeBatchPipeline(req, mockDispatcher, async (id) => fakeNodes[id]);

    expect(res.success).toBe(false);
    expect(res.completed_steps).toBe(1);
    expect(res.rollback_executed).toBe(true);
    expect(res.rolled_back_steps).toBe(1);
    expect(removed).toEqual(['frame_1']);
  });
});

// ── P0-2: the dispatcher must forward nodeIds to write handlers ───────────────
//
// Write handlers read `request.nodeIds[0]`, not `params.nodeId`. The pipeline
// dispatcher used to build `{ type, requestId, params }` with no nodeIds field,
// so every nodeIds-based tool failed with "nodeId is required".

describe('handleBatchPipelineRequest — nodeIds wiring', () => {
  const runWithSpy = async (params: any) => {
    const seen: any[] = [];
    const writeDispatcher = async (subReq: any) => {
      seen.push(subReq);
      return { data: { id: subReq.nodeIds?.[0] ?? 'new_1' } };
    };
    const { handleBatchPipelineRequest } = await import('./batch-pipeline');
    await handleBatchPipelineRequest(
      {
        type: 'batch_execute_pipeline',
        requestId: 'req-1',
        params: { steps: [{ id: 's1', action: 'set_fills', params }] },
      },
      writeDispatcher,
    );
    return seen[0];
  };

  it('forwards a nodeIds array to the write handler', async () => {
    const sub = await runWithSpy({ nodeIds: ['1:1'], color: '#ff0000' });
    expect(sub.nodeIds).toEqual(['1:1']);
  });

  it('accepts singular nodeId and normalizes it to a nodeIds array', async () => {
    const sub = await runWithSpy({ nodeId: '1:1', color: '#ff0000' });
    expect(sub.nodeIds).toEqual(['1:1']);
  });

  it('does not leak nodeId/nodeIds into params', async () => {
    const sub = await runWithSpy({ nodeId: '1:1', color: '#ff0000' });
    expect(sub.params).toEqual({ color: '#ff0000' });
    expect(sub.params.nodeId).toBeUndefined();
    expect(sub.params.nodeIds).toBeUndefined();
  });
});

// ── P0-1: rollback must never delete a node the pipeline did not create ───────
//
// The WAL used to record `{ type: 'CREATE' }` for any result carrying an `id`.
// Modify handlers return the id of an EXISTING node, so a later failure had the
// rollback delete the user's own nodes. `rename_page` is reachable today.

describe('WAL — only true creates are rolled back', () => {
  const pipelineThatFailsAfter = async (firstStep: any, fakeNodes: Record<string, any>) => {
    const mockDispatcher = async (action: string) => {
      if (action === 'boom') throw new Error('deliberate failure');
      if (action === 'rename_page') return { id: 'page_1', name: 'Landing' };
      if (action === 'create_frame') return { id: 'frame_1', name: 'Header' };
      return {};
    };
    const { executeBatchPipeline } = await import('./batch-pipeline');
    return executeBatchPipeline(
      { steps: [firstStep, { id: 's2', action: 'boom', params: {} }] },
      mockDispatcher,
      async (id) => fakeNodes[id],
    );
  };

  it('does not remove a pre-existing page returned by rename_page', async () => {
    const removed: string[] = [];
    const fakeNodes = { page_1: { id: 'page_1', remove: () => removed.push('page_1') } };

    const res = await pipelineThatFailsAfter(
      { id: 's1', action: 'rename_page', params: { pageName: 'Home', newName: 'Landing' } },
      fakeNodes,
    );

    expect(res.success).toBe(false);
    expect(removed).toEqual([]);
  });

  it('still removes nodes created by create_* actions', async () => {
    const removed: string[] = [];
    const fakeNodes = { frame_1: { id: 'frame_1', remove: () => removed.push('frame_1') } };

    const res = await pipelineThatFailsAfter(
      { id: 's1', action: 'create_frame', params: { name: 'Header' } },
      fakeNodes,
    );

    expect(res.success).toBe(false);
    expect(removed).toEqual(['frame_1']);
  });
});

// ── P0-3: rollback must restore properties changed on existing nodes ──────────
//
// The design doc specified MODIFY snapshot/restore; only CREATE was implemented,
// so "transactional" mutations on existing nodes were silently irreversible.

describe('WAL — MODIFY snapshot and restore', () => {
  it('restores the previous properties of a modified node on rollback', async () => {
    const node: any = {
      id: '1:1',
      name: 'Button',
      x: 10,
      y: 20,
      opacity: 1,
      fills: [{ type: 'SOLID', color: { r: 0, g: 0, b: 0 } }],
    };
    const fakeNodes: Record<string, any> = { '1:1': node };

    const mockDispatcher = async (action: string, params: any) => {
      if (action === 'set_fills') {
        node.fills = [{ type: 'SOLID', color: { r: 1, g: 0, b: 0 } }];
        return { id: node.id };
      }
      if (action === 'move_nodes') {
        node.x = params.x;
        return { id: node.id };
      }
      throw new Error('deliberate failure');
    };

    const { executeBatchPipeline } = await import('./batch-pipeline');
    const res = await executeBatchPipeline(
      {
        steps: [
          { id: 's1', action: 'set_fills', params: { nodeIds: ['1:1'], color: '#ff0000' } },
          { id: 's2', action: 'move_nodes', params: { nodeIds: ['1:1'], x: 999 } },
          { id: 's3', action: 'boom', params: { nodeIds: ['1:1'] } },
        ],
      },
      mockDispatcher,
      async (id) => fakeNodes[id],
    );

    expect(res.success).toBe(false);
    expect(node.x).toBe(10);
    expect(node.fills).toEqual([{ type: 'SOLID', color: { r: 0, g: 0, b: 0 } }]);
  });

  it('only snapshots properties the node actually has', async () => {
    const node: any = { id: '1:1', opacity: 1 };
    const fakeNodes: Record<string, any> = { '1:1': node };

    const mockDispatcher = async (action: string) => {
      if (action === 'set_opacity') { node.opacity = 0.2; return { id: node.id }; }
      throw new Error('deliberate failure');
    };

    const { executeBatchPipeline } = await import('./batch-pipeline');
    await executeBatchPipeline(
      {
        steps: [
          { id: 's1', action: 'set_opacity', params: { nodeIds: ['1:1'], opacity: 0.2 } },
          { id: 's2', action: 'boom', params: {} },
        ],
      },
      mockDispatcher,
      async (id) => fakeNodes[id],
    );

    expect(node.opacity).toBe(1);
    expect('x' in node).toBe(false);
    expect('fills' in node).toBe(false);
  });

  it('reports rolled_back_steps covering both creates and restores', async () => {
    const removed: string[] = [];
    const existing: any = { id: '1:1', opacity: 1 };
    const fakeNodes: Record<string, any> = {
      '1:1': existing,
      frame_1: { id: 'frame_1', remove: () => removed.push('frame_1') },
    };

    const mockDispatcher = async (action: string) => {
      if (action === 'create_frame') return { id: 'frame_1' };
      if (action === 'set_opacity') { existing.opacity = 0.5; return { id: '1:1' }; }
      throw new Error('deliberate failure');
    };

    const { executeBatchPipeline } = await import('./batch-pipeline');
    const res = await executeBatchPipeline(
      {
        steps: [
          { id: 's1', action: 'create_frame', params: {} },
          { id: 's2', action: 'set_opacity', params: { nodeIds: ['1:1'], opacity: 0.5 } },
          { id: 's3', action: 'boom', params: {} },
        ],
      },
      mockDispatcher,
      async (id) => fakeNodes[id],
    );

    expect(res.rolled_back_steps).toBe(2);
    expect(removed).toEqual(['frame_1']);
    expect(existing.opacity).toBe(1);
  });
});

// ── P2-11: a failed pipeline must still report what ran ──────────────────────

describe('executeBatchPipeline — results on failure', () => {
  it('returns per-step results even when the pipeline fails', async () => {
    const mockDispatcher = async (action: string) => {
      if (action === 'create_frame') return { id: 'frame_1' };
      throw new Error('deliberate failure');
    };
    const { executeBatchPipeline } = await import('./batch-pipeline');
    const res = await executeBatchPipeline(
      {
        steps: [
          { id: 's1', action: 'create_frame', params: {} },
          { id: 's2', action: 'boom', params: {} },
        ],
      },
      mockDispatcher,
      async () => null,
    );

    expect(res.results).toHaveLength(1);
    expect(res.results?.[0]).toMatchObject({ step_id: 's1', status: 'ok' });
  });
});



// ── Integration: pipeline through the REAL write dispatcher ──────────────────
//
// The unit tests above drive executeBatchPipeline with a mock dispatcher, which
// is exactly why P0-2 went unnoticed: the nodeIds were dropped in
// handleBatchPipelineRequest, a layer the mocks never exercised. These tests go
// through handleWriteRequest so the whole chain is covered.

describe('batch pipeline → real write handlers', () => {
  let mockNodes: Record<string, any>;

  beforeEach(() => {
    mockNodes = {};
    (globalThis as any).figma = {
      getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
      commitUndo: () => {},
    };
  });

  const runPipeline = (steps: any[]) =>
    handleWriteRequest({
      type: 'batch_execute_pipeline',
      requestId: 'req-int-1',
      params: { steps },
    });

  it('runs a nodeIds-based modify tool inside a pipeline', async () => {
    mockNodes['1:1'] = { id: '1:1', name: 'Button', fills: [] };

    const res = await runPipeline([
      { id: 's1', action: 'set_fills', params: { nodeId: '1:1', color: '#FF0000' } },
    ]);

    expect(res.data.success).toBe(true);
    expect(res.data.completed_steps).toBe(1);
    expect(mockNodes['1:1'].fills).toEqual([
      { type: 'SOLID', color: { r: 1, g: 0, b: 0 } },
    ]);
  });

  it('restores an existing node when a later step fails', async () => {
    mockNodes['1:1'] = { id: '1:1', name: 'Button', opacity: 1 };

    const res = await runPipeline([
      { id: 's1', action: 'set_opacity', params: { nodeIds: ['1:1'], opacity: 0.25 } },
      { id: 's2', action: 'set_text', params: { nodeId: '9:9', text: 'nope' } },
    ]);

    expect(res.data.success).toBe(false);
    expect(res.data.rollback_executed).toBe(true);
    expect(mockNodes['1:1'].opacity).toBe(1);
  });

  it('does not delete a renamed page when a later step fails', async () => {
    let removed = false;
    const page = {
      id: 'page_1',
      type: 'PAGE',
      name: 'Home',
      remove: () => { removed = true; },
    };
    mockNodes['page_1'] = page;

    const res = await runPipeline([
      { id: 's1', action: 'rename_page', params: { pageId: 'page_1', newName: 'Landing' } },
      { id: 's2', action: 'set_text', params: { nodeId: '9:9', text: 'nope' } },
    ]);

    expect(res.data.success).toBe(false);
    expect(removed).toBe(false);
    expect(page.name).toBe('Home'); // rolled back to the original name
  });
});

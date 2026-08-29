import { beforeEach, describe, expect, it } from 'bun:test';
import { CREATE_ACTIONS, executeBatchPipeline, executeRollback, isCreateStep, resetUndoCheckpointState, resolveParams, SymbolTable, WALStack, withSingleUndoCheckpoint } from './batch-pipeline';
import { writeHandlers } from './write-handlers';
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

let progressMessages: any[] = [];

describe('batch pipeline → real write handlers', () => {
  let mockNodes: Record<string, any>;

  beforeEach(() => {
    mockNodes = {};
    progressMessages = [];
    (globalThis as any).figma = {
      getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
      commitUndo: () => {},
      ui: { postMessage: (msg: any) => progressMessages.push(msg) },
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

// manage_page merged four page tools, so whether a step created something is no
// longer decided by the action name alone. Getting this wrong either leaks a
// page the pipeline created or, far worse, removes one the user already had.
describe('isCreateStep', () => {
  it('treats manage_page add as a create', () => {
    expect(isCreateStep('manage_page', { action: 'add', name: 'Specs' })).toBe(true);
  });

  it.each(['delete', 'rename', 'navigate'])('does not treat manage_page %s as a create', (action) => {
    expect(isCreateStep('manage_page', { action, pageId: '0:2' })).toBe(false);
  });

  it('still recognises the plain create actions', () => {
    expect(isCreateStep('create_frame', {})).toBe(true);
    expect(isCreateStep('rename_node', { nodeId: '1:1' })).toBe(false);
  });
});

// ── withSingleUndoCheckpoint ──────────────────────────────────────────────────

describe('withSingleUndoCheckpoint', () => {
  const withFigma = (commitUndo: any) => {
    const previous = (globalThis as any).figma;
    (globalThis as any).figma = { ...(previous ?? {}), commitUndo };
    return () => {
      (globalThis as any).figma = previous;
    };
  };

  it('collapses many handler checkpoints into one', async () => {
    let commits = 0;
    const restore = withFigma(() => {
      commits++;
    });
    try {
      await withSingleUndoCheckpoint(async () => {
        (globalThis as any).figma.commitUndo();
        (globalThis as any).figma.commitUndo();
        (globalThis as any).figma.commitUndo();
      });
      expect(commits).toBe(1);
    } finally {
      restore();
    }
  });

  it('commits nothing when no step touched the document', async () => {
    let commits = 0;
    const restore = withFigma(() => {
      commits++;
    });
    try {
      await withSingleUndoCheckpoint(async () => {});
      expect(commits).toBe(0);
    } finally {
      restore();
    }
  });

  it('restores commitUndo and still checkpoints when the work throws', async () => {
    let commits = 0;
    const real = () => {
      commits++;
    };
    const restore = withFigma(real);
    try {
      const boom = withSingleUndoCheckpoint(async () => {
        (globalThis as any).figma.commitUndo();
        throw new Error('step failed');
      });
      expect(boom).rejects.toThrow('step failed');
      await boom.catch(() => {});
      expect(commits).toBe(1);
      // A swallowing stub left behind would silently disable undo for the
      // rest of the session.
      expect((globalThis as any).figma.commitUndo).toBe(real);
    } finally {
      restore();
    }
  });

  it('returns the work’s value', async () => {
    const restore = withFigma(() => {});
    try {
      expect(await withSingleUndoCheckpoint(async () => 'done')).toBe('done');
    } finally {
      restore();
    }
  });

  it('runs the work unchanged when there is no commitUndo to swap', async () => {
    const previous = (globalThis as any).figma;
    (globalThis as any).figma = {};
    try {
      expect(await withSingleUndoCheckpoint(async () => 'ok')).toBe('ok');
    } finally {
      (globalThis as any).figma = previous;
    }
  });

  // Two pipelines can be in flight at once: a pipeline whose every step reads is
  // not classified as mutating, so it skips the write queue. Their scopes then
  // interleave rather than nest, and a save/restore that assumes LIFO has the
  // first to finish put the real function back while the second still runs, and
  // the second put the first's stub back for good — after which every write in
  // the session loses its checkpoint, silently.
  it('restores the real commitUndo after interleaved scopes', async () => {
    const deferred = () => {
      let resolve!: () => void;
      const promise = new Promise<void>((res) => {
        resolve = res;
      });
      return { promise, resolve };
    };

    let commits = 0;
    const real = () => {
      commits++;
    };
    const restore = withFigma(real);
    try {
      resetUndoCheckpointState();
      const first = deferred();
      const second = deferred();
      const outer = withSingleUndoCheckpoint(async () => {
        (globalThis as any).figma.commitUndo();
        await first.promise;
      });
      const inner = withSingleUndoCheckpoint(async () => {
        (globalThis as any).figma.commitUndo();
        await second.promise;
      });

      first.resolve();
      await outer;
      second.resolve();
      await inner;

      expect((globalThis as any).figma.commitUndo).toBe(real);
      expect(commits).toBe(1);
    } finally {
      restore();
      resetUndoCheckpointState();
    }
  });
});

// ── cancellation between steps ───────────────────────────────────────────────

describe('executeBatchPipeline cancellation', () => {
  const twoSteps = {
    steps: [
      { id: 's1', action: 'create_frame', params: {} },
      { id: 's2', action: 'create_frame', params: {} },
    ],
  };

  it('runs every step when nothing is cancelled', async () => {
    const ran: string[] = [];
    const res = await executeBatchPipeline(
      twoSteps as any,
      async (action) => {
        ran.push(action);
        return { id: `n${ran.length}` };
      },
      async () => null,
      () => false,
    );
    expect(res.success).toBe(true);
    expect(ran.length).toBe(2);
  });

  it('stops at the next step boundary and rolls back what ran', async () => {
    const removed: string[] = [];
    let cancelled = false;
    const res = await executeBatchPipeline(
      twoSteps as any,
      async () => {
        // Cancelled while the first step is in flight; it completes, and the
        // second never starts.
        cancelled = true;
        return { id: 'n1' };
      },
      async (id) => ({ id, remove: () => removed.push(id) }),
      () => cancelled,
    );
    expect(res.success).toBe(false);
    expect(res.completed_steps).toBe(1);
    expect(res.failed_step?.error).toBe('Request cancelled');
    expect(res.rollback_executed).toBe(true);
    expect(removed).toEqual(['n1']);
  });

  it('rolls back even when stop_on_error is off', async () => {
    // stop_on_error tolerates a step that failed on its own terms; a cancelled
    // run has no terms left to tolerate.
    const res = await executeBatchPipeline(
      { ...twoSteps, stop_on_error: false } as any,
      async () => ({ id: 'n1' }),
      async () => null,
      () => true,
    );
    expect(res.success).toBe(false);
    expect(res.completed_steps).toBe(0);
    expect(res.rollback_executed).toBe(true);
  });
});

describe('batch pipeline progress', () => {
  const noopDispatch = async () => ({ id: '1:1' });

  it('reports the step about to run, not the one just finished', async () => {
    const seen: Array<[number, number, string]> = [];
    await executeBatchPipeline(
      { steps: [{ action: 'rename_node' }, { action: 'move_nodes' }, { action: 'set_paint' }] } as any,
      noopDispatch,
      async () => null,
      () => false,
      async (done, total, action) => { seen.push([done, total, action]); },
    );
    expect(seen).toEqual([
      [0, 3, 'rename_node'],
      [1, 3, 'move_nodes'],
      [2, 3, 'set_paint'],
    ]);
  });

  it('stops reporting once a step has failed the run', async () => {
    const seen: string[] = [];
    await executeBatchPipeline(
      { steps: [{ action: 'a' }, { action: 'b' }] } as any,
      async () => { throw new Error('nope'); },
      async () => null,
      () => false,
      async (_done, _total, action) => { seen.push(action); },
    );
    expect(seen).toEqual(['a']);
  });
});

describe('batch pipeline progress through handleWriteRequest', () => {
  beforeEach(() => {
    progressMessages = [];
    (globalThis as any).figma = {
      getNodeByIdAsync: async () => ({ id: '1:1', name: 'n' }),
      commitUndo: () => {},
      ui: { postMessage: (msg: any) => progressMessages.push(msg) },
    };
  });

  it('posts one progress message per step of a multi-step run', async () => {
    await handleWriteRequest({
      type: 'batch_execute_pipeline',
      requestId: 'req-progress',
      params: {
        steps: [
          { action: 'rename_node', params: { nodeId: '1:1', name: 'a' } },
          { action: 'rename_node', params: { nodeId: '1:1', name: 'b' } },
        ],
      },
    });
    const updates = progressMessages.filter((m) => m.type === 'progress_update');
    expect(updates.length).toBe(2);
    expect(updates[0].message).toBe('Step 1/2: rename_node');
    expect(updates[0].requestId).toBe('req-progress');
  });

  // The response lands at about the same moment, so a message would only add noise.
  it('stays quiet for a one-step pipeline', async () => {
    await handleWriteRequest({
      type: 'batch_execute_pipeline',
      requestId: 'req-single',
      params: { steps: [{ action: 'rename_node', params: { nodeId: '1:1', name: 'a' } }] },
    });
    expect(progressMessages.filter((m) => m.type === 'progress_update')).toEqual([]);
  });
});

// ── CREATE_ACTIONS must keep up with the write handlers ──────────────────────
//
// The comment above CREATE_ACTIONS warns that a create-style handler added
// without a matching entry rolls back wrongly — but nothing enforced it, and
// `rename_page` is the cautionary example in the other direction: it returns an
// id the user already had, so removing it destroys their page.
//
// So every write handler is listed here as CREATE or KEEPS. Adding a handler
// fails this test until it is classified, which is the point: the decision is
// cheap to make now and expensive to discover after a rollback removed
// somebody's work.

const KEEPS_EXISTING_NODES = [
  'batch_rename_nodes',
  'bind_variable_to_node',
  'boolean_operation',
  'clear_annotations',
  'combine_as_variants',
  'create_style',
  'create_variable',
  'create_variable_collection',
  'add_variable_mode',
  'apply_style_to_node',
  'delete_nodes',
  'delete_style',
  'delete_variable',
  'detach_instance',
  'find_replace_text',
  'flatten_nodes',
  'group_nodes',
  'manage_component_properties',
  'manage_page',
  'manage_plugin_data',
  'move_nodes',
  'outline_stroke',
  'remove_reactions',
  'rename_node',
  'reparent_nodes',
  'resize_nodes',
  'save_version_checkpoint',
  'set_annotations',
  'set_auto_layout',
  'set_codegen_result',
  'set_corner_radius',
  'set_effects',
  'set_export_settings',
  'set_instance_overrides',
  'set_layout_grids',
  'set_layout_sizing',
  'set_node_properties',
  'set_paint',
  'set_reactions',
  'set_selection',
  'set_text',
  'set_text_ranges',
  'set_variable_value',
  'swap_component',
  'ungroup_nodes',
  'update_paint_style',
  'create_vector',
  // Internal delegation targets — the names the merged tools dispatch to. They
  // are real entries in the write handler map, so they need classifying too.
  'set_fills',
  'set_gradient_fills',
  'set_strokes',
  'delete_page',
  'navigate_to_page',
  'rename_page',
  // These create a style, not a node. Rollback removes by node id, so a style
  // is not something it can take back — which is why they are not creates here.
  'create_paint_style',
  'create_text_style',
  'create_effect_style',
  'create_grid_style',
];

describe('CREATE_ACTIONS', () => {
  it('classifies every write handler as creating or not', () => {
    const classified = new Set([...CREATE_ACTIONS, ...KEEPS_EXISTING_NODES]);
    const unclassified = Object.keys(writeHandlers).filter((name) => !classified.has(name));
    expect(unclassified).toEqual([]);
  });

  it('lists nothing that is not a write handler', () => {
    const handlers = new Set(Object.keys(writeHandlers));
    // The internal names the merged tools delegate to are not in the map, and
    // are the ones CREATE_ACTIONS legitimately names (create_frame and friends
    // behind create_node), so only the KEEPS side is checked here.
    for (const name of KEEPS_EXISTING_NODES) {
      expect(handlers.has(name), `${name} is not a write handler`).toBe(true);
    }
  });

  // These two are why the list exists at all.
  it('does not treat a merged action as a create by its name alone', () => {
    expect(isCreateStep('manage_page', { action: 'add' })).toBe(true);
    expect(isCreateStep('manage_page', { action: 'rename' })).toBe(false);
    expect(isCreateStep('manage_page', { action: 'delete' })).toBe(false);
  });

  it('treats a node-returning modify as something to keep', () => {
    for (const name of KEEPS_EXISTING_NODES) {
      expect(isCreateStep(name, {}), `${name} would be removed on rollback`).toBe(false);
    }
  });
});

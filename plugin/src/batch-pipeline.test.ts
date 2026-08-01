import { describe, expect, it } from 'vitest';
import { executeRollback, resolveParams, SymbolTable, WALStack } from './batch-pipeline';

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



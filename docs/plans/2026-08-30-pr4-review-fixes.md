# PR #4 Review Fixes — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sửa 7 lỗi được xác nhận trong code review PR #4 (`claude/plugin-upgrade-features-avneku`), từ lỗi mất undo checkpoint vĩnh viễn tới các lỗi im lặng nhỏ hơn, mà không đụng tới version numbering.

**Architecture:** Toàn bộ thay đổi nằm trong `plugin/src/` (TypeScript/Svelte) cộng một thay đổi `.gitignore`. Mỗi task sửa một lỗi độc lập; task nào kiểm chứng được bằng unit test thì viết test đỏ trước rồi mới sửa. Task 2 dựng scaffold `main.test.ts` (mock `figma` global + import động) mà Task 3 dùng lại.

**Tech Stack:** Bun 1.4 test runner, Vite, Svelte 5, Go 1.27 (chỉ chạy `go test` để xác nhận không hồi quy).

**Spec:** Không có spec riêng — nguồn yêu cầu là kết quả `/code-review` trên PR #4, chép nguyên vào phần "Findings" dưới đây.

## Global Constraints

- Làm việc trên branch `claude/plugin-upgrade-features-avneku`, **không** trên `main`.
- **KHÔNG** đổi version (`npm/package.json`, `plugin/manifest.json`, `plugin/package.json`, `__APP_VERSION__`). Anh guộc đã quyết bỏ qua finding #4 (nâng 0.4.0) — đừng sửa `internal/bridge/version_skew.go` hay bất kỳ chuỗi version nào.
- **KHÔNG** đụng finding #9 (`Bridge.PluginVersion()` chỉ dùng trong test): nó là accessor public hợp lệ của package `bridge` và `version_skew_test.go` dùng nó — xóa sẽ phải sửa test mà không thu được gì.
- Comment trong code viết bằng **tiếng Anh**, khớp phong cách hiện có của repo. Chỉ tài liệu (`docs/`) mới dùng tiếng Việt.
- Không refactor thứ nằm ngoài phạm vi từng task. Không đổi format, không đổi tên biến hàng xóm.
- Lệnh test: `cd plugin && bun test` (680 test đang xanh trước khi bắt đầu) và `go test ./...` (7/7 xanh) từ gốc repo.

## Findings được sửa trong plan này

| # | Mức | File | Vấn đề | Task |
|---|-----|------|--------|------|
| 1 | CRITICAL | `plugin/src/batch-pipeline.ts:342` | Hai `withSingleUndoCheckpoint` chồng chéo (không lồng nhau) giết `figma.commitUndo` vĩnh viễn | 1 |
| 2 | HIGH | `plugin/src/main.ts:41` | Write đã queue vẫn chạy sau khi server timeout và báo caller là fail | 2 |
| 5 | MEDIUM | `plugin/src/main.ts:70` | `plugin-capabilities` chỉ post một lần, có thể mất trước khi iframe nghe | 3 |
| 3 | MEDIUM | `plugin/src/write-text.ts:79` | `set_text_ranges` load font này nhưng apply font khác | 4 |
| 6 | LOW/MED | `plugin/src/ui/App.svelte:137` | `pendingApprovals` sống sót qua socket close | 5 |
| 7 | LOW | `internal/tools/testdata/tools_schema.json.actual` | Artifact debug bị commit, chưa gitignore | 6 |
| 8 | LOW | `plugin/src/write-create.ts:27,258` | `import_image` nuốt `width`/`height` đơn lẻ; `append` vỡ với fills mixed | 7 |
| — | note | `plugin/src/read-document.ts` | 4 chỗ post progress thủ công, bypass `clampProgress` | 8 |

---

## Chuẩn bị

- [ ] **Step 0: Checkout branch PR**

```bash
git fetch origin claude/plugin-upgrade-features-avneku
git checkout -B claude/plugin-upgrade-features-avneku origin/claude/plugin-upgrade-features-avneku
cd plugin && bun install
```

- [ ] **Step 0b: Xác nhận baseline xanh**

Run: `cd plugin && bun test` → Expected: 680 pass, 0 fail
Run: `go test ./...` (từ gốc repo) → Expected: tất cả `ok`

---

## Task 1: `withSingleUndoCheckpoint` an toàn khi hai pipeline chồng chéo

**Files:**
- Modify: `plugin/src/batch-pipeline.ts:328-363` (hàm `withSingleUndoCheckpoint`)
- Modify: `plugin/src/main.ts:37-43` (nhánh dispatch trong `handleRequest`)
- Test: `plugin/src/batch-pipeline.test.ts` (thêm describe block mới ở cuối)

**Interfaces:**
- Consumes: `PIPELINE_TOOL` từ `./tool-classes` (đã export sẵn), `enqueueWrite` từ `./write-queue`.
- Produces: `resetUndoCheckpointState()` export mới từ `batch-pipeline.ts` — test seam, chỉ test dùng.

**Bối cảnh (đọc trước khi sửa):** Bản hiện tại lưu `figma.commitUndo` vào biến cục bộ rồi khôi phục trong `finally`. Điều đó chỉ đúng khi các scope lồng nhau LIFO. Nhưng `main.ts` chỉ đẩy request **mutating** vào queue, mà `isMutating("batch_execute_pipeline", {steps: [...toàn read...]})` trả `false` → một pipeline read-only chạy ngoài queue và chồng lên pipeline khác. Khi đó P1 khôi phục hàm gốc trong khi P2 vẫn đang chạy, rồi P2 khôi phục stub của P1 → `figma.commitUndo` là no-op mãi mãi, mọi write sau đó mất undo checkpoint.

Sửa hai chỗ, mỗi chỗ trị một hệ quả khác nhau:
1. `withSingleUndoCheckpoint` chuyển sang depth counter ở module scope → hàm tự an toàn bất kể ai gọi, `commitUndo` không bao giờ bị bỏ lại là stub.
2. `main.ts` đẩy **mọi** `batch_execute_pipeline` vào write queue → hai pipeline không còn chạy đồng thời, nên undo step của chúng không bị gộp làm một. Giá phải trả: một pipeline toàn read phải xếp hàng sau các write đang chạy; pipeline vốn nặng và hiếm nên chi phí này không đáng kể.

- [ ] **Step 1: Viết test đỏ cho hai pipeline chồng chéo**

Thêm vào cuối `plugin/src/batch-pipeline.test.ts`:

```ts
describe('withSingleUndoCheckpoint under overlap', () => {
  const deferred = () => {
    let resolve!: () => void;
    const promise = new Promise<void>((res) => { resolve = res; });
    return { promise, resolve };
  };

  beforeEach(() => resetUndoCheckpointState());

  // Two pipelines can be in flight at once: a read-only pipeline is not
  // classified as mutating, so it does not go through the write queue. Their
  // scopes then interleave rather than nest, and a save/restore that assumes
  // LIFO puts a dead stub back as figma.commitUndo — for good.
  it('restores the real commitUndo after interleaved scopes', async () => {
    const commits: string[] = [];
    const real = () => { commits.push('commit'); };
    (globalThis as any).figma = { commitUndo: real };

    const first = deferred();
    const second = deferred();
    const p1 = withSingleUndoCheckpoint(async () => {
      (globalThis as any).figma.commitUndo();
      await first.promise;
    });
    const p2 = withSingleUndoCheckpoint(async () => {
      (globalThis as any).figma.commitUndo();
      await second.promise;
    });

    first.resolve();
    await p1;
    second.resolve();
    await p2;

    expect((globalThis as any).figma.commitUndo).toBe(real);
    expect(commits).toEqual(['commit']);
  });

  it('still restores after a throw', async () => {
    const real = () => {};
    (globalThis as any).figma = { commitUndo: real };
    await expect(
      withSingleUndoCheckpoint(async () => { throw new Error('boom'); }),
    ).rejects.toThrow('boom');
    expect((globalThis as any).figma.commitUndo).toBe(real);
  });

  it('commits nothing when no step asked for a checkpoint', async () => {
    const commits: string[] = [];
    (globalThis as any).figma = { commitUndo: () => commits.push('commit') };
    await withSingleUndoCheckpoint(async () => {});
    expect(commits).toEqual([]);
  });
});
```

Cập nhật dòng import đầu file để thêm `resetUndoCheckpointState` (giữ nguyên thứ tự alphabet của các tên đang có):

```ts
import { CREATE_ACTIONS, executeBatchPipeline, executeRollback, isCreateStep, resetUndoCheckpointState, resolveParams, SymbolTable, WALStack, withSingleUndoCheckpoint } from './batch-pipeline';
```

- [ ] **Step 2: Chạy test để thấy nó đỏ**

Run: `cd plugin && bun test batch-pipeline`
Expected: FAIL — `resetUndoCheckpointState is not a function` ở lần chạy đầu; sau khi thêm hàm rỗng thì `expect(figma.commitUndo).toBe(real)` fail vì đang là stub của P1.

- [ ] **Step 3: Chuyển `withSingleUndoCheckpoint` sang depth counter**

Thay toàn bộ hàm ở `plugin/src/batch-pipeline.ts:342-363` và phần docstring ngay trên nó:

```ts
let checkpointDepth = 0;
let suspendedCommitUndo: (() => void) | null = null;
let anyStepCommitted = false;

/**
 * Run `work` so the whole of it lands on the undo stack as one step.
 *
 * Every write handler commits its own undo checkpoint, which is right when it
 * is the whole of what the user asked for. Inside a pipeline it is not: a
 * twenty-step build left twenty checkpoints, so undoing it meant twenty
 * Ctrl+Z, each one leaving the design in a state no one asked for.
 *
 * Figma offers no way to suspend commitUndo, so the handlers' calls are
 * swallowed and one is made at the end.
 *
 * The swap is counted, not saved per call. Scopes do not always nest: a
 * read-only pipeline is not classified as mutating, so it skips the write
 * queue and can overlap another pipeline. A per-call save/restore then has the
 * first scope to finish put the real function back while the second is still
 * running, and the second put the first's stub back for good — after which
 * every write in the session loses its checkpoint silently. A counter cannot
 * do that: the real function goes back exactly once, when the last scope
 * leaves, whatever order they started in.
 */
export async function withSingleUndoCheckpoint<T>(work: () => Promise<T>): Promise<T> {
  const api: any = typeof figma !== 'undefined' ? figma : null;
  if (!api || typeof api.commitUndo !== 'function') return work();

  if (checkpointDepth === 0) {
    // Held unbound and called with the receiver below, so what goes back is the
    // exact function that was there.
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
      const real = suspendedCommitUndo;
      suspendedCommitUndo = null;
      api.commitUndo = real;
      // Nothing mutated the document — a checkpoint here would be an empty undo
      // step the user has to press through.
      if (anyStepCommitted && real) real.call(api);
    }
  }
}

/** Test seam: forget any suspended swap. */
export function resetUndoCheckpointState(): void {
  checkpointDepth = 0;
  suspendedCommitUndo = null;
  anyStepCommitted = false;
}
```

- [ ] **Step 4: Chạy test để thấy nó xanh**

Run: `cd plugin && bun test batch-pipeline`
Expected: PASS, toàn bộ file

- [ ] **Step 5: Đẩy mọi pipeline vào write queue**

Ở `plugin/src/main.ts`, sửa import dòng 7:

```ts
import { isMutating, PIPELINE_TOOL } from "./tool-classes";
```

và sửa nhánh dispatch (dòng 39-43):

```ts
    // Writes take their turn; reads do not wait. Two writes interleaving would
    // put a plain write inside a pipeline's undo checkpoint — see write-queue.
    // A pipeline queues even when every step of it only reads: two pipelines in
    // flight at once share one undo checkpoint, so the user's Ctrl+Z would
    // reverse a run they did not ask about.
    return isMutating(request.type, request.params) || request.type === PIPELINE_TOOL
      ? await enqueueWrite(() => runRequest(request))
      : await runRequest(request);
```

- [ ] **Step 6: Chạy toàn bộ test**

Run: `cd plugin && bun test`
Expected: PASS, số test tăng đúng 3 so với baseline

- [ ] **Step 7: Commit**

```bash
git add plugin/src/batch-pipeline.ts plugin/src/batch-pipeline.test.ts plugin/src/main.ts
git commit -m "fix(plugin): keep commitUndo alive when pipeline scopes overlap"
```

---

## Task 2: Không chạy write đã bị server hủy

**Files:**
- Modify: `plugin/src/main.ts:28-55` (export `handleRequest`, thêm `throwIfCancelled` trong closure)
- Create: `plugin/src/main.test.ts`

**Interfaces:**
- Consumes: `throwIfCancelled`, `markCancelled`, `resetCancellations` từ `./cancellation`; `resetWriteQueue` từ `./write-queue`.
- Produces: `handleRequest` được export từ `plugin/src/main.ts` — Task 3 dùng lại scaffold mock trong `main.test.ts`.

**Bối cảnh:** `enqueueWrite(() => runRequest(request))` không hề kiểm tra cờ hủy trước khi chạy. Timeout mặc định của tool là 30s (`internal/bridge/timeout.go`), riêng `batch_execute_pipeline` là 120s. Một `set_text` đến giữa pipeline dài nằm im trong queue, không phát progress, nên hết 30s server trả "request timed out", gửi `cancel_request`, `markCancelled` ghi nhận — nhưng closure khi được dequeue vẫn mutate document. Model thấy fail, retry, edit áp dụng hai lần.

- [ ] **Step 1: Tạo scaffold test với mock `figma` global**

`main.ts` chạy `startPanel()` ngay khi import, nên global phải có trước. Tạo `plugin/src/main.test.ts`:

```ts
import { describe, expect, it, beforeEach } from "bun:test";
import { markCancelled, resetCancellations } from "./cancellation";
import { resetWriteQueue } from "./write-queue";

// main.ts runs startPanel() at import time, so the globals it touches have to
// exist before the import. Vite defines __html__ and __APP_VERSION__ at build
// time; here they are plain globals.
const posted: any[] = [];
let uiHandler: ((message: any) => any) | null = null;

(globalThis as any).__html__ = "<html></html>";
(globalThis as any).__APP_VERSION__ = "0.0.0-test";
(globalThis as any).figma = {
  root: { name: "File", children: [] },
  currentPage: { name: "Page 1", selection: [] },
  ui: {
    postMessage: (message: any) => posted.push(message),
    get onmessage() { return uiHandler; },
    set onmessage(fn: any) { uiHandler = fn; },
    resize: () => {},
  },
  showUI: () => {},
  on: () => {},
  notify: () => {},
  getNodeByIdAsync: async () => null,
  clientStorage: { getAsync: async () => null, setAsync: async () => {} },
};

const { handleRequest } = await import("./main");

beforeEach(() => {
  resetWriteQueue();
  resetCancellations();
});

describe("handleRequest", () => {
  // The plugin queue is invisible to the server's clock: a write waiting behind
  // a long pipeline emits no progress, so its 30s timer runs out, the caller is
  // told it failed, and a cancel arrives. Running it anyway lands the edit for
  // a caller that has already retried.
  it("does not run a queued write the server has cancelled", async () => {
    markCancelled("r1");
    const response = await handleRequest({
      type: "set_text",
      requestId: "r1",
      nodeIds: ["1:1"],
      params: { text: "hello" },
    });
    expect(response.error).toBe("Request cancelled");
  });

  it("runs a queued write that was not cancelled", async () => {
    const response = await handleRequest({
      type: "set_text",
      requestId: "r2",
      nodeIds: ["1:1"],
      params: { text: "hello" },
    });
    // The mock has no such node, so it fails on the node lookup — which is
    // proof the handler was reached rather than skipped.
    expect(response.error).not.toBe("Request cancelled");
  });
});
```

- [ ] **Step 2: Chạy test để thấy nó đỏ**

Run: `cd plugin && bun test main`
Expected: FAIL — `handleRequest is not a function` (chưa export), sau khi export thì test đầu fail với error là "Node not found: 1:1" thay vì "Request cancelled".

- [ ] **Step 3: Export `handleRequest` và chặn request đã hủy**

Ở `plugin/src/main.ts`, sửa import dòng 5 và hàm `handleRequest`:

```ts
import { clearCancelled, markCancelled, throwIfCancelled } from "./cancellation";
```

```ts
export const handleRequest = async (request: any) => {
  try {
    // Writes take their turn; reads do not wait. Two writes interleaving would
    // put a plain write inside a pipeline's undo checkpoint — see write-queue.
    // A pipeline queues even when every step of it only reads: two pipelines in
    // flight at once share one undo checkpoint, so the user's Ctrl+Z would
    // reverse a run they did not ask about.
    return isMutating(request.type, request.params) || request.type === PIPELINE_TOOL
      ? await enqueueWrite(async () => {
          // Time in the queue counts against the request's timeout, and a
          // queued write emits no progress to extend it. By the time it is
          // dequeued the server may already have given up, told the caller it
          // failed and sent a cancel — mutating now would apply an edit the
          // model has been told did not happen, and will retry.
          throwIfCancelled(request.requestId);
          return runRequest(request);
        })
      : await runRequest(request);
  } catch (error) {
```

Phần `catch`/`finally` giữ nguyên.

- [ ] **Step 4: Chạy test để thấy nó xanh**

Run: `cd plugin && bun test main`
Expected: PASS (2 test)

- [ ] **Step 5: Commit**

```bash
git add plugin/src/main.ts plugin/src/main.test.ts
git commit -m "fix(plugin): drop a queued write the server already cancelled"
```

---

## Task 3: Gửi lại `plugin-capabilities` khi panel báo sẵn sàng

**Files:**
- Modify: `plugin/src/main.ts:57-91` (tách `sendCapabilities`, gọi thêm ở nhánh `ui-ready`)
- Modify: `plugin/src/main.test.ts` (thêm describe block)

**Interfaces:**
- Consumes: scaffold `posted` và `uiHandler` từ Task 2.
- Produces: không có export mới.

**Bối cảnh:** `plugin-capabilities` được post ngay sau `figma.showUI`, tức trước khi iframe kịp cài listener `message`. Chính file này gửi `sendStatus()` hai lần — lúc startup và lại khi `ui-ready` — đúng vì lý do đó. Message mới không có lần thứ hai. Nếu nó rơi, `pluginHandlers` mãi là `[]`, server ghi "announced nothing", và `checkPluginSupports` fail-open thành allow-everything mà không có triệu chứng nào.

- [ ] **Step 1: Viết test đỏ**

Thêm vào cuối `plugin/src/main.test.ts`:

```ts
describe("plugin-capabilities", () => {
  const capabilityMessages = () => posted.filter(m => m.type === "plugin-capabilities");

  // The panel's message listener is not installed yet when showUI returns, so
  // the first post can land in the gap. sendStatus is already sent twice for
  // this reason; the capability list needs the same second chance, or the
  // server is left thinking the plugin announced nothing and stops checking.
  it("is re-sent when the panel says it is ready", async () => {
    const before = capabilityMessages().length;
    expect(before).toBe(1);
    await uiHandler!({ type: "ui-ready" });
    expect(capabilityMessages().length).toBe(before + 1);
  });

  it("lists the batch pipeline alongside the handlers", () => {
    expect(capabilityMessages()[0].handlers).toContain("batch_execute_pipeline");
    expect(capabilityMessages()[0].handlers).toContain("get_document");
  });
});
```

- [ ] **Step 2: Chạy test để thấy nó đỏ**

Run: `cd plugin && bun test main`
Expected: FAIL — `expected 2, got 1` ở test đầu tiên

- [ ] **Step 3: Tách `sendCapabilities` và gọi lại ở `ui-ready`**

Ở `plugin/src/main.ts`, chuyển khối `figma.ui.postMessage({ type: "plugin-capabilities", ... })` (dòng 70-77) thành một hàm cạnh `sendStatus`, ngay sau nó:

```ts
// What this build can actually do. The UI passes it to the server on connect,
// so a tool the server has and this plugin does not is reported as "update
// the plugin" rather than as "Unknown request type" at call time. The maps
// live here because importing them into the UI would pull the entire write
// surface into a bundle that only needs the names.
//
// Sent at startup and again on ui-ready, like sendStatus: the panel's listener
// is not installed when showUI returns, and a capability list that lands in
// that gap leaves the server believing the plugin announced nothing — which it
// reads as "old plugin, allow everything".
const sendCapabilities = () => {
  figma.ui.postMessage({
    type: "plugin-capabilities",
    handlers: [
      ...Object.keys(readHandlerMap),
      ...Object.keys(writeHandlerMap),
      "batch_execute_pipeline",
    ],
  });
};
```

Trong `startPanel`, thay khối postMessage cũ bằng lời gọi:

```ts
  sendStatus();
  sendCapabilities();
```

Và trong nhánh `ui-ready`:

```ts
    if (message.type === "ui-ready") {
      sendStatus();
      sendCapabilities();
      return;
    }
```

- [ ] **Step 4: Chạy test để thấy nó xanh**

Run: `cd plugin && bun test main`
Expected: PASS (4 test)

- [ ] **Step 5: Commit**

```bash
git add plugin/src/main.ts plugin/src/main.test.ts
git commit -m "fix(plugin): re-send plugin-capabilities on ui-ready"
```

---

## Task 4: `set_text_ranges` apply đúng font đã load

**Files:**
- Modify: `plugin/src/write-text.ts:73-113` (`applyRange` nhận font), `:125-142` (`rangeFonts`), `:152-172` (handler)
- Test: `plugin/src/write-text.test.ts` (thêm describe block)

**Interfaces:**
- Consumes: `FontName`, `loadFonts` từ `./fonts` (đã import sẵn).
- Produces: `rangeFonts(node, ranges)` đổi kiểu trả về từ `FontName[]` thành `(FontName | null)[]` **song song theo chỉ số** với `ranges`.

**Bối cảnh:** `rangeFonts` resolve phần font kế thừa qua `getRangeFontName` **trước** khi ghi range nào; `applyRange` resolve lại **sau** khi các range trước đã đổi text. Với `ranges: [{0,10,fontFamily:"Roboto"}, {4,8,fontStyle:"Bold"}]`, "Inter Bold" được preload nhưng "Roboto Bold" được apply → Figma ném "font not loaded" ở range thứ hai, sau khi range đầu đã ghi, để node styled dở dang.

**Quyết định ngữ nghĩa:** mỗi range kế thừa từ trạng thái node **lúc gọi**, không phải từ kết quả của range trước. Nó deterministic, không phụ thuộc thứ tự, và khớp với thiết kế "load hết trước khi ghi" đã có. Font được apply chính là font đã được load.

- [ ] **Step 1: Viết test đỏ**

Thêm vào cuối `plugin/src/write-text.test.ts`:

```ts
describe("set_text_ranges when an earlier range changes the font", () => {
  // A mock whose getRangeFontName reflects what has already been written — the
  // way the real node does. A mock that always answers the same thing cannot
  // catch a font resolved twice.
  let fontAt: any[];

  beforeEach(() => {
    fontAt = new Array(11).fill({ family: "Inter", style: "Regular" });
    node.getRangeFontName = (start: number, end: number) => {
      const first = fontAt[start];
      const uniform = fontAt
        .slice(start, end)
        .every(f => f.family === first.family && f.style === first.style);
      return uniform ? first : Symbol("mixed");
    };
    node.setRangeFontName = (start: number, end: number, font: any) => {
      calls.push({ name: "setRangeFontName", args: [start, end, font] });
      for (let i = start; i < end; i++) fontAt[i] = font;
    };
  });

  it("applies exactly the fonts it loaded", async () => {
    await call({
      ranges: [
        { start: 0, end: 10, fontFamily: "Roboto" },
        { start: 4, end: 8, fontStyle: "Bold" },
      ],
    });
    const applied = callsNamed("setRangeFontName").map(c => c.args[2]);
    for (const font of applied) {
      expect(loadedFonts).toContainEqual(font);
    }
  });

  // Every range reads the node as the caller saw it, so the answer does not
  // depend on which range happens to be written first.
  it("inherits from the text as it was when the call started", async () => {
    await call({
      ranges: [
        { start: 0, end: 10, fontFamily: "Roboto" },
        { start: 4, end: 8, fontStyle: "Bold" },
      ],
    });
    expect(callsNamed("setRangeFontName").map(c => c.args[2])).toEqual([
      { family: "Roboto", style: "Regular" },
      { family: "Inter", style: "Bold" },
    ]);
  });
});
```

- [ ] **Step 2: Chạy test để thấy nó đỏ**

Run: `cd plugin && bun test write-text`
Expected: FAIL — test đầu: font `{family:"Roboto", style:"Bold"}` được apply nhưng không có trong `loadedFonts`; test hai: nhận `Roboto Bold` thay vì `Inter Bold`.

- [ ] **Step 3: Truyền font đã resolve vào `applyRange`**

Ở `plugin/src/write-text.ts`, sửa chữ ký và hai dòng đầu của `applyRange` (dòng 73-80):

```ts
const applyRange = async (node: any, range: TextRange, font: FontName | null) => {
  const [start, end] = resolveRange(range, node.characters.length);

  // The font was resolved and loaded before the first range was written, and
  // is applied exactly as resolved. Resolving it again here would read a node
  // an earlier range has already changed, so the font written would be one
  // that was never loaded — and Figma refuses it, mid-way through the edit.
  if (font) node.setRangeFontName(start, end, font);
```

Phần còn lại của `applyRange` giữ nguyên.

- [ ] **Step 4: Đổi `rangeFonts` sang mảng song song**

Thay docstring và thân `rangeFonts` (dòng 125-142):

```ts
/**
 * The font each range asks for, resolved against the node as the caller found
 * it — one entry per range, in the same order, null where the range asks for
 * no font change.
 *
 * Resolved before anything is written, for two reasons. A range asking for a
 * font the file does not have used to fail on that range, after the ranges
 * before it had already been applied. And a range that inherits half its font
 * from the text would otherwise resolve differently at load time and at write
 * time, once an earlier range has changed that text — so the font written was
 * one that was never loaded.
 */
export const rangeFonts = (node: any, ranges: TextRange[]): (FontName | null)[] =>
  ranges.map(range => {
    const [start, end] = resolveRange(range, node.characters.length);
    return resolveRangeFont(range, node.getRangeFontName(start, end));
  });
```

- [ ] **Step 5: Dùng mảng đó trong handler**

Ở `plugin/src/write-text.ts:162-171`, thay:

```ts
    const sorted = sortRanges(ranges);
    // One load for the node's existing fonts and every font the ranges ask for.
    // Nothing is written until they are all in.
    const fonts = rangeFonts(node, sorted);
    await loadFonts([
      ...node.getRangeAllFontNames(0, node.characters.length),
      ...fonts.filter((font): font is FontName => font !== null),
    ]);
    for (let i = 0; i < sorted.length; i++) {
      await applyRange(node, sorted[i], fonts[i]);
    }
```

- [ ] **Step 6: Chạy test để thấy nó xanh**

Run: `cd plugin && bun test write-text`
Expected: PASS, cả các test cũ (`applies no range at all when a later one asks for a missing font`, `asks Figma for each font once`) vẫn xanh

- [ ] **Step 7: Commit**

```bash
git add plugin/src/write-text.ts plugin/src/write-text.test.ts
git commit -m "fix(plugin): apply the range font that was actually loaded"
```

---

## Task 5: Bỏ các approval đang treo khi socket đóng

**Files:**
- Modify: `plugin/src/ui/App.svelte:137-155` (`ws.onclose`)

**Interfaces:**
- Consumes: `startEntry`, `finishEntry` từ `./activity` (đã import ở dòng 23).
- Produces: không có export mới.

**Bối cảnh:** Guard mode `confirm` giữ request destructive trong `pendingApprovals`. Timeout phía server gửi `cancel_request` và `cancelRequest()` xóa nó — nhưng socket drop thường (server restart, máy ngủ, mất mạng) thì không; `ws.onclose` chỉ viết lại `activityLog`. User quay lại bấm **Allow**, `delete_nodes` chạy cho một request không còn ai đợi, và response đi vào socket mới, bị server log là "response for a request that is already gone".

**Ghi chú:** `App.svelte` không có unit test (logic testable đã được tách sang `ui/activity.ts`, `ui/prefs.ts`). Task này verify bằng type-check + build, không bằng test — đừng dựng scaffold test cho Svelte component chỉ vì một thay đổi 6 dòng.

- [ ] **Step 1: Xóa approval treo trong `onclose`**

Ở `plugin/src/ui/App.svelte`, trong `ws.onclose`, thêm ngay sau khối `activityLog = activityLog.map(...)` và trước khối `if (reconnectTimer === null)`:

```ts
      // A request held for approval was waiting on a caller that is now gone.
      // The server's own cancel frame drops one of these, but a socket that
      // simply dropped sends nothing — and the dialog would outlive the request,
      // so Allow would run a destructive edit for nobody and answer into a
      // socket the server no longer associates with it.
      for (const held of pendingApprovals) {
        activityLog = startEntry(activityLog, held.payload.requestId, held.payload.type, Date.now());
        activityLog = finishEntry(activityLog, held.payload.requestId, "connection lost", Date.now());
      }
      pendingApprovals = [];
```

- [ ] **Step 2: Verify build và type-check**

Run: `cd plugin && bun run build`
Expected: build xong, không lỗi Svelte/TS

Run: `cd plugin && bun test`
Expected: PASS, không hồi quy

- [ ] **Step 3: Commit**

```bash
git add plugin/src/ui/App.svelte
git commit -m "fix(plugin): drop held approvals when the socket closes"
```

---

## Task 6: Xóa artifact debug của golden test

**Files:**
- Delete: `internal/tools/testdata/tools_schema.json.actual`
- Modify: `.gitignore`

**Interfaces:** không có.

**Bối cảnh:** File này chỉ được ghi bởi `internal/tools/tools_golden_test.go:89` **khi golden test fail**. Nó dài 2975 dòng, đã stale (thiếu `set_export_settings`), và chưa được gitignore nên sẽ bẩn lại working tree mỗi lần golden mismatch.

- [ ] **Step 1: Xác nhận đây đúng là artifact và không ai đọc nó**

Run: `git grep -n "\.actual" -- '*.go'`
Expected: chỉ khớp trong `internal/tools/tools_golden_test.go` ở chỗ **ghi** file khi test fail — không có chỗ nào đọc

- [ ] **Step 2: Xóa file và ignore nó**

```bash
git rm internal/tools/testdata/tools_schema.json.actual
```

Thêm vào `.gitignore`, ngay dưới khối `# Go build artifacts`:

```gitignore
# Written by the schema golden test when it fails, to diff against the golden.
*.actual
```

- [ ] **Step 3: Verify golden test vẫn xanh và không sinh lại file**

Run: `go test ./internal/tools/...`
Expected: `ok` — và `git status --short` sạch (nếu golden test fail, file sẽ được sinh lại nhưng bị ignore; test phải xanh nên không sinh)

- [ ] **Step 4: Commit**

```bash
git add .gitignore internal/tools/testdata/tools_schema.json.actual
git commit -m "chore: drop the golden test's failure artifact and ignore it"
```

---

## Task 7: `import_image` tôn trọng width/height đơn lẻ và fills mixed

**Files:**
- Modify: `plugin/src/write-create.ts:19-39` (`imageSize`), `:254-265` (nhánh `nodeId`)
- Test: `plugin/src/write-create.test.ts` (thêm describe block)

**Interfaces:**
- Consumes: `imageSize(image, p)` — đã export sẵn, chữ ký không đổi.
- Produces: không có export mới.

**Bối cảnh:** `imageSize` chỉ dùng dimension khi có **cả hai**; schema Go (`internal/tools/tools_write_create.go:195`) nhận `width` và `height` như hai param optional độc lập, không ràng buộc cặp (khác `create_vector` vốn enforce "width and height must be given together"). `import_image({imageUrl, width: 300})` đặt ảnh ở kích thước tự nhiên và `width` của caller biến mất, không lỗi. Và ở dòng 258, `mode: "append"` làm `[...target.fills, fill]` → ném "is not iterable" khó hiểu khi fills của target là `figma.mixed`.

**Quyết định:** một chiều được cho → giữ tỉ lệ ảnh, tính chiều còn lại. Hữu ích hơn là ném lỗi, và không bao giờ im lặng bỏ qua con số caller đưa.

- [ ] **Step 1: Viết test đỏ**

Thêm vào cuối `plugin/src/write-create.test.ts`:

```ts
describe("imageSize", () => {
  const image = { getSizeAsync: async () => ({ width: 800, height: 400 }) };

  it("takes both dimensions when both are given", async () => {
    expect(await imageSize(image, { width: 300, height: 100 })).toEqual({ width: 300, height: 100 });
  });

  // The schema takes width and height independently, so a caller giving one is
  // asking for something — dropping it left the image at a size nobody chose.
  it("keeps the aspect ratio when only a width is given", async () => {
    expect(await imageSize(image, { width: 300 })).toEqual({ width: 300, height: 150 });
  });

  it("keeps the aspect ratio when only a height is given", async () => {
    expect(await imageSize(image, { height: 100 })).toEqual({ width: 200, height: 100 });
  });

  it("scales a large image down to fit when neither is given", async () => {
    const big = { getSizeAsync: async () => ({ width: 4000, height: 2000 }) };
    expect(await imageSize(big, {})).toEqual({ width: 1000, height: 500 });
  });

  it("falls back to the caller's numbers when the image cannot be measured", async () => {
    const broken = { getSizeAsync: async () => { throw new Error("malformed"); } };
    expect(await imageSize(broken, { width: 300 })).toEqual({ width: 300, height: 200 });
  });
});
```

Cập nhật import ở đầu `write-create.test.ts` để thêm `imageSize` vào danh sách các tên đang lấy từ `./write-create`.

- [ ] **Step 2: Chạy test để thấy nó đỏ**

Run: `cd plugin && bun test write-create`
Expected: FAIL — "keeps the aspect ratio when only a width is given" nhận `{width: 800, height: 400}`

- [ ] **Step 3: Sửa `imageSize`**

Thay hàm ở `plugin/src/write-create.ts:19-39`:

```ts
/**
 * The size to give a newly placed image.
 *
 * An explicit width and height win. One of the two is still an instruction —
 * the schema takes them independently — so the other is derived from the
 * image's aspect ratio rather than dropped. With neither, the image's own
 * dimensions are used, scaled down to fit a sensible box so a 4000px photo does
 * not land as a 4000px rectangle. getSizeAsync can fail on a malformed image,
 * and a placeholder is a better outcome there than a failed import.
 */
export const imageSize = async (image: any, p: any) => {
  const width = p.width != null ? Number(p.width) : null;
  const height = p.height != null ? Number(p.height) : null;
  if (width != null && height != null) return { width, height };
  const MAX = 1000;
  try {
    const size = await image.getSizeAsync();
    if (width != null) return { width, height: Math.round(width * (size.height / size.width)) };
    if (height != null) return { width: Math.round(height * (size.width / size.height)), height };
    const scale = Math.min(1, MAX / Math.max(size.width, size.height));
    return { width: Math.round(size.width * scale), height: Math.round(size.height * scale) };
  } catch {
    return { width: width ?? 200, height: height ?? 200 };
  }
};
```

- [ ] **Step 4: Chạy test để thấy nó xanh**

Run: `cd plugin && bun test write-create`
Expected: PASS

- [ ] **Step 5: Viết test đỏ cho `append` trên fills mixed**

Thêm vào `plugin/src/write-create.test.ts`, trong describe của `import_image` nếu đã có, hoặc thành describe riêng:

```ts
describe("import_image onto an existing node", () => {
  // A node whose children carry different fills reports figma.mixed, which is a
  // symbol. Spreading it threw "is not iterable" — an error that names neither
  // the node nor what to do about it.
  it("names the problem when appending to mixed fills", async () => {
    const target = { id: "1:1", name: "Card", type: "FRAME", fills: Symbol("mixed") };
    (globalThis as any).figma.getNodeByIdAsync = async () => target;
    await expect(
      writeCreateHandlers["import_image"]({
        type: "import_image",
        requestId: "r1",
        params: { imageData: "abc", nodeId: "1:1", mode: "append" },
      }),
    ).rejects.toThrow(/mixed fills/);
  });
});
```

Test này cần mock `figma.createImage` trả `{ hash: "h" }` — khớp với cách các test `import_image` sẵn có trong file dựng mock; tái dùng đúng mock đó thay vì dựng cái mới.

- [ ] **Step 6: Chạy test để thấy nó đỏ**

Run: `cd plugin && bun test write-create`
Expected: FAIL — ném "target.fills is not iterable" thay vì thông báo có nghĩa

- [ ] **Step 7: Kiểm tra fills trước khi append**

Ở `plugin/src/write-create.ts`, thay dòng 258:

```ts
      if (p.mode === "append") {
        // figma.mixed is a symbol, and spreading it throws "is not iterable" —
        // an error that names neither the node nor the fix.
        if (!Array.isArray(target.fills)) {
          throw new Error(
            `Node ${p.nodeId} has mixed fills — mode "append" needs a node whose fills are all the same`,
          );
        }
        target.fills = [...target.fills, fill];
      } else {
        target.fills = [fill];
      }
```

- [ ] **Step 8: Chạy test để thấy nó xanh**

Run: `cd plugin && bun test`
Expected: PASS toàn bộ

- [ ] **Step 9: Commit**

```bash
git add plugin/src/write-create.ts plugin/src/write-create.test.ts
git commit -m "fix(plugin): honour a lone width or height, and name mixed fills"
```

---

## Task 8: `read-document` dùng `reportProgress` như mọi chỗ khác

**Files:**
- Modify: `plugin/src/read-document.ts:1-4` (import), `:26-34`, `:467-475`, `:524-532`, `:569-577`

**Interfaces:**
- Consumes: `reportProgress`, `stepProgress` từ `./progress`.
- Produces: không có export mới.

**Bối cảnh:** Bốn chỗ trong file này post `progress_update` thủ công và bypass `clampProgress`, nên `Math.round(...) + 1` có thể ra `100` ở request cuối — trong khi `progress.ts` được thêm trong chính PR này tồn tại đúng để không chỗ nào phải lặp lại cặp "post message + await yield". Hôm nay vô hại (bridge chỉ dùng nó để gia hạn timer), nhưng là bất nhất ngay trong cùng một PR.

- [ ] **Step 1: Thêm import**

Ở `plugin/src/read-document.ts` dòng 1-4, thêm:

```ts
import { reportProgress, stepProgress } from "./progress";
```

- [ ] **Step 2: Thay chỗ thứ nhất (`get_document`, scope document)**

Thay khối ở dòng 26-34:

```ts
        if (figma.root.children.length > 1) {
          await reportProgress(
            request.requestId,
            stepProgress(pages.length, figma.root.children.length),
            `Serialized ${page.name} (${pages.length}/${figma.root.children.length})`,
          );
        }
```

- [ ] **Step 3: Thay chỗ thứ hai (`search_nodes`)**

Thay khối ở dòng 467-475:

```ts
      if (searchingPages && roots.length > 1) {
        await reportProgress(
          request.requestId,
          stepProgress(i + 1, roots.length),
          `Searched ${root.name}: ${results.length} match(es) so far`,
        );
      }
```

- [ ] **Step 4: Thay chỗ thứ ba (`scan_text_nodes`)**

Thay khối ở dòng 524-532:

```ts
    await reportProgress(request.requestId, 10, "Scanning text nodes...");
```

- [ ] **Step 5: Thay chỗ thứ tư (`scan_nodes_by_types`)**

Thay khối ở dòng 569-577:

```ts
    await reportProgress(request.requestId, 10, `Scanning for types: ${types.join(", ")}...`);
```

- [ ] **Step 6: Chạy test**

Run: `cd plugin && bun test`
Expected: PASS — `dynamic-page.test.ts` và `read-search.test.ts` chạm vào các đường này qua `posted`; nếu một assert bám vào giá trị `progress` cũ thì sửa assert theo `stepProgress`, đừng đổi lại code.

- [ ] **Step 7: Commit**

```bash
git add plugin/src/read-document.ts
git commit -m "refactor(plugin): route read-document progress through reportProgress"
```

---

## Kiểm chứng cuối

- [ ] **Step 1: Toàn bộ test**

Run: `cd plugin && bun test` → Expected: PASS, ≥ 690 test (680 baseline + các test mới)
Run: `go test ./...` → Expected: tất cả `ok`

- [ ] **Step 2: Build plugin**

Run: `cd plugin && bun run build` → Expected: cả hai bundle build xong, không lỗi

- [ ] **Step 3: Xác nhận không đụng version**

Run: `git diff origin/claude/plugin-upgrade-features-avneku --stat -- npm/package.json plugin/package.json plugin/manifest.json internal/bridge/version_skew.go`
Expected: không có output — không file version nào bị đổi

- [ ] **Step 4: Đẩy lên PR**

```bash
git push origin claude/plugin-upgrade-features-avneku
```

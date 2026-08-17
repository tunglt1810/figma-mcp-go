# Architecture & Bug Review — figma-mcp-go

**Ngày:** 2026-08-15
**Phạm vi:** toàn bộ Go server (`internal/`, `cmd/`) + Figma plugin (`plugin/src/`)
**Trạng thái:** research only — tài liệu này để chọn hạng mục triển khai. Phần thân giữ nguyên nội dung ngày khảo sát; xem bảng dưới để biết mục nào đã làm.

### Đã triển khai từ báo cáo này

| Hạng mục | Kết quả |
|---|---|
| **G1** — P0-1, P0-2, P0-3 | Rollback pipeline snapshot đúng node đích, khôi phục thuộc tính, trả `results` khi lỗi (P2-11 hết theo) |
| **G2** — P1-5, P1-6, P1-7 | Validation vào `Node.Send`; một bảng timeout cho bridge/follower/progress; validate hex color ở cả Go và plugin |
| **G3** — P2-8 → P2-16 | `gofmt`/`go vet` vào CI; mixed fills/strokes; `$variable` chỉ khớp identifier; normalize node ID trong cả cây params; ghi đè được file export; check Origin của WebSocket; xoá `BatchPipeline*` dead code |
| **G4** | Thu gọn tool Tier 1: 8 tool node-property gộp thành `set_node_properties` (breaking, cần cài lại plugin) |
| **G5** | Thu gọn tool Tier 2: shape 7→1, paint 3→1, style 4→1, page 4→1. 77 → 63 tool |
| **G6** | Bảng tool declarative — mọi tool khai báo trong `toolSpec`; `ValidateRPC` chỉ còn tra bảng. P1-4 (`steps` sai kiểu) hết theo |
| **G7** — B3 | **Không làm** — giữ leader/follower; xem phần dưới |
| **G8** — B4 + B5 | Bridge ping mỗi 20s, drop connection không pong; dispatch plugin thành map, trùng tên tool là lỗi lúc load |

Không còn bug nào trong danh sách này để mở.

**B5 đã làm, nhưng lý do trong báo cáo không đúng.** Hiệu năng không phải vấn đề:
10 lần `switch` trên string là nano giây, round-trip là mili giây. Cái map thật sự
mua được là (a) trùng tên tool giữa hai module thành lỗi ném lúc load thay vì
"module nào đứng trước thì thắng", (b) một chỗ duy nhất để tra tên. Kèm theo vẫn
giữ test so bảng tool Go với handler plugin (`tools_plugin_test.go`).

**Về cảnh báo trong mục Tier 2** ("gộp quá tay thì LLM chọn sai param nhiều hơn
chọn sai tool"): cảnh báo đúng, nên mỗi tool gộp có `requireVariant` — param
thuộc variant khác bị **báo lỗi kèm tên param và tên variant**, không bị bỏ qua
âm thầm. Đổi một lỗi khó thấy lấy một lỗi nói thẳng.

**G7 — quyết định: không làm, giữ leader/follower.** Lý do trong B3 đã yếu đi
nhiều: hai nguồn bug được nêu ở đó (P1-5 validation lệch, P1-6 timeout lệch) đều
đã sửa, nên phần còn lại chỉ là ~500 dòng đang chạy tốt.

Hai topology tồn tại song song và làm hai việc khác nhau:

- **Nhiều client → một file Figma**, không cần cấu hình: process đầu tiên chiếm
  1994 giữ WebSocket, các process sau proxy qua `/rpc`. Đây là leader/follower.
- **Nhiều client → nhiều file Figma**: mỗi client một `--port`, mỗi plugin
  instance trỏ vào port riêng. Không có follower nào chạy.

Cách hai vốn đã làm được nhưng chưa từng được ghi ở đâu; nay có trong README.
Xoá leader/follower sẽ lấy mất cách một, tức trường hợp zero-config hai
terminal cùng tác động lên một file — trong khi package đã publish npm công
khai. Giữ.

---

## 0. Số liệu nền

| Chỉ số | Giá trị | Đo bằng |
|---|---|---|
| Go LOC (không tính test) | ~4.900 | `wc -l` |
| Plugin TS LOC (không tính test) | ~2.400 | `wc -l` |
| Số MCP tool | 84 | `tools_schema_test.go` |
| Payload `tools/list` | **58.931 bytes ≈ 14.7k token** | `HandleMessage` + `json.Marshal` |
| Số dòng validation (`schema.go`) | 940 | chỉ chạy trên 1 trong 2 code path (xem P1-5) |
| `go test ./...` | PASS | — |
| `gofmt -l .` | **5 file lỗi format** | election.go, schema.go, schema_test.go, tools_write.go, tools_write_components.go |

14.7k token schema = ~7% cửa sổ context 200k, trả phí ở **mọi** session, trước khi làm bất cứ việc gì.

---

## PHẦN A — BUG

### 🔴 P0-1 — `batch_execute_pipeline` rollback xoá node có sẵn của user (mất dữ liệu)

**Vị trí:** `plugin/src/batch-pipeline.ts:99-101`

```ts
if (res && res.id) {
  walStack.push({ type: 'CREATE', nodeId: res.id });
}
```

WAL coi **mọi** kết quả có `.id` là node vừa được tạo. Nhưng rất nhiều handler trả về `id` của node **đã tồn tại**.

**Repro reachable ngay hôm nay:**

```json
{"steps":[
  {"id":"s1","action":"rename_page","params":{"pageName":"Home","newName":"Landing"}},
  {"id":"s2","action":"create_frame","params":{"parentId":"$khong_ton_tai"}}
]}
```

- `rename_page` trả `data: { id: page.id, ... }` (`plugin/src/write-page.ts:71`) — id của **page thật của user**
- s2 fail → `executeRollback` gọi `page.remove()`
- → **Toàn bộ page và mọi thứ trên đó bị xoá vĩnh viễn.**

Sau khi fix P0-2 (nối `nodeIds`), lỗ hổng này mở rộng ra ~20 handler nữa: `set_fills`, `set_text`, `set_strokes`, `rename_node`, `set_auto_layout`… tất cả đều trả `id` của node có sẵn.

**Fix:** chỉ push `CREATE` khi action thuộc allow-list `create_*` / `clone_node` / `add_page` / `import_image`, hoặc để handler tự khai báo `data.__created = true`. Cách sau an toàn hơn vì không phụ thuộc quy ước đặt tên.

---

### 🔴 P0-2 — Pipeline không truyền `nodeIds` → ~34/57 write tool không dùng được

**Vị trí:** `plugin/src/batch-pipeline.ts:155`

```ts
const subReq = { type: action, requestId: `${request.requestId}_${action}`, params };
```

Không có field `nodeIds`. Nhưng handler đọc như thế này (`plugin/src/write-modify.ts:8`):

```ts
const nodeId = request.nodeIds && request.nodeIds[0];
if (!nodeId) throw new Error("nodeId is required");
```

**Thống kê `request.nodeIds` trong plugin write handlers:**

| File | Số lần dùng | Trạng thái trong pipeline |
|---|---|---|
| `write-modify.ts` | 20 | ❌ hỏng toàn bộ |
| `write-components.ts` | 8 | ❌ hỏng phần lớn |
| `write-styles.ts` | 3 | ❌ `apply_style_to_node`, `set_effects`, `bind_variable_to_node` |
| `write-prototype.ts` | 2 | ❌ hỏng |
| `write-create.ts` | 1 | ❌ `create_component` |
| `write-page.ts`, `write-variables.ts` | 0 | ✅ chạy được |

Nghĩa là pipeline **chỉ tạo node được, không sửa được node nào**. Tính năng flagship "transactional mutation" thực tế chỉ hoạt động một nửa.

**Vì sao test không bắt được:** `batch-pipeline.test.ts` chỉ test `executeBatchPipeline` với `mockDispatcher` tự viết — không bao giờ chạy qua `handleBatchPipelineRequest` → `dispatchSingle`, chính là chỗ mất `nodeIds`.

**Fix:**
```ts
const dispatcher = async (action: string, params: any) => {
  const { nodeId, nodeIds, ...rest } = params ?? {};
  const ids = nodeIds ?? (nodeId ? [nodeId] : undefined);
  const subReq = { type: action, requestId: `${request.requestId}_${action}`, nodeIds: ids, params: rest };
  ...
};
```
+ thêm integration test đi qua `handleWriteRequest` thật.

---

### 🔴 P0-3 — WAL `MODIFY` chưa implement, trái với chính design doc của nó

**Vị trí:** `plugin/src/batch-pipeline.ts:32-52`

`LogEntry` khai báo `MODIFY` với `previousState`, nhưng:
- không chỗ nào push entry `MODIFY`
- `executeRollback` chỉ có nhánh `if (entry.type === 'CREATE')`

Design doc `docs/specs/2026-08-01-batch-execute-pipeline-design.md:142,148` đã spec rõ: snapshot `fills/strokes/x/y/width/height/characters` trước MODIFY, rollback bằng `restoreNodeProperties()`. Phần này chưa được viết.

**Hệ quả:** tool description nói *"transactional … with rollback support"* nhưng thực tế mọi thay đổi lên node có sẵn là **không thể hoàn tác**. LLM tin vào description → dùng pipeline cho các thao tác nguy hiểm.

**Fix:** hoặc implement MODIFY snapshot đúng spec, hoặc sửa description thành *"rollback chỉ xoá node mới tạo; thay đổi lên node có sẵn không được hoàn tác"*. Cái sau rẻ và trung thực, nên làm ngay kèm P0-1.

---

### 🟠 P1-4 — Schema `steps` khai sai kiểu, lại không `required`

**Vị trí:** `internal/tools_write.go:25`

Schema thực tế server phát ra:
```json
"steps": { "type": "object", "properties": {}, "description": "Array of pipeline steps to execute in sequence" }
```

Ba vấn đề cùng lúc:
1. `type: "object"` nhưng plugin lặp `req.steps[i]` → cần **array**. MCP client nào validate strict sẽ reject.
2. `properties: {}` — LLM không có tí thông tin nào về shape của một step (`id`/`action`/`params`/`export_vars`).
3. `required: []` — `steps` không bắt buộc.

**Fix:** `mcp.WithArray("steps", mcp.Required(), mcp.WithObjectItems(...))` mô tả đầy đủ shape step. Thêm case `batch_execute_pipeline` vào `ValidateRPC` (hiện không có).

---

### 🟠 P1-5 — 940 dòng validation chỉ chạy trên code path phụ

**Vị trí:** `internal/leader.go:127` — **call site duy nhất** của `ValidateRPC`.

Luồng thực tế (`internal/node.go:75-78`):

```
MCP client → tool handler → node.Send()
                              ├── role == LEADER   → bridge.Send()        ← KHÔNG validate
                              └── role == FOLLOWER → follower.Send()
                                                      → leader /rpc → ValidateRPC ← có validate
```

Process đầu tiên khởi động luôn là **leader**. Nghĩa là với đại đa số user (1 MCP client), **toàn bộ `schema.go` là dead code**.

**Hệ quả cụ thể:** cùng một lệnh `set_opacity(opacity=5)`
- leader → gửi thẳng xuống Figma → lỗi khó hiểu từ plugin API, hoặc âm thầm sai
- follower → `"opacity must be between 0 and 1"`

Hành vi khác nhau tuỳ process nào start trước — cực khó debug.

**Fix:** đưa `ValidateRPC` vào đầu `Node.Send()`. Leader `/rpc` giữ nguyên (defense in depth cho input từ mạng). Một dòng code, thu hồi 940 dòng logic đang bỏ không.

---

### 🟠 P1-6 — Thang timeout không nhất quán → `get_document` / pipeline luôn fail ở follower

| Nơi | Timeout |
|---|---|
| `bridge.go:193` mặc định | 30s |
| `bridge.go:194` `get_document` | 60s |
| `bridge.go:196` `batch_execute_pipeline` | 120s |
| `bridge.go:110` reset khi có progress | **60s cứng** |
| `follower.go:29` HTTP client | **35s cứng** |

Hai lỗi:

1. **Follower không bao giờ chờ nổi tool dài.** File lớn cần 45s → leader vẫn đang chờ, follower đã đứt ở 35s. `batch_execute_pipeline` (120s) qua follower thực tế trần là 35s.
2. **Progress update rút ngắn timeout của pipeline.** `entry.timer.Reset(60 * time.Second)` — một progress update ở giây thứ 10 của pipeline 120s hạ trần xuống 70s. Cơ chế "kéo dài timeout" lại làm ngược.

Comment ở `follower.go:28` (`35s > 30s bridge timeout`) đúng ở thời điểm viết, nhưng đã lạc hậu sau khi thêm 2 timeout đặc biệt.

**Fix:** một bảng timeout dùng chung, `follower.client.Timeout` derive từ đó (`timeoutFor(tool) + 5s`), progress reset dùng `timeoutFor(tool)` chứ không hardcode 60s. Thêm trần tổng để plugin gửi progress vô hạn không giữ request sống mãi.

---

### 🟠 P1-7 — Không có tầng nào validate màu hex

**Vị trí:** `plugin/src/write-helpers.ts:5-13`

```ts
export const hexToRgb = (hex: string) => {
  const clean = hex.replace("#", "");
  return { r: parseInt(clean.slice(0,2),16)/255, ... };
};
```

| Input | Kết quả |
|---|---|
| `#f00` (shorthand 3 ký tự) | `b = parseInt("", 16)/255 = NaN` → fill hỏng, **không báo lỗi** |
| `red` | `{r: NaN, g: NaN, b: NaN}` |
| `rgb(255,0,0)` | NaN |

`schema.go:363` chỉ kiểm `color != ""`. LLM rất hay sinh `#f00` hoặc tên màu.

**Fix:** validate + expand shorthand ở Go (`^#?([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`), trả lỗi rõ ràng thay vì NaN âm thầm. Áp cho `set_fills`, `set_strokes`, `create_paint_style`, `set_gradient_fills.stops[].color`.

---

### 🟡 P2-8 — `set_fills` mode=`append` vỡ khi `fills` là `figma.mixed`

`plugin/src/write-modify.ts:56-58` spread thẳng `(node as any).fills`. Nếu node có fills hỗn hợp, `fills` là `symbol` → `TypeError: fills is not iterable`.

`set_gradient_fills:34` đã có guard `Array.isArray(...)`. Bất nhất giữa hai handler cùng loại.

---

### 🟡 P2-9 — `resolveParams` nuốt mọi string bắt đầu bằng `$`

`plugin/src/batch-pipeline.ts:11-16`: `if (params.startsWith('$'))` → throw nếu không có trong symbol table.

`create_text(text: "$100")` trong pipeline → `Error: Undefined pipeline variable: $100`. Giá tiền, biến CSS, template string đều dính.

**Fix:** chỉ nhận pattern `^\$[A-Za-z_][A-Za-z0-9_]*$`, và hỗ trợ escape `$$` → `$`.

---

### 🟡 P2-10 — WebSocket tắt hoàn toàn Origin check

`internal/bridge.go:52`: `InsecureSkipVerify: true`.

Kết hợp với `bridge.go:64-70` (connection mới **thay thế** connection cũ), một trang web bất kỳ user đang mở có thể:
1. `new WebSocket("ws://127.0.0.1:1994/ws")` — không bị CORS chặn với WS
2. Đá văng plugin thật
3. Nhận toàn bộ tool request tiếp theo, trả dữ liệu giả

Severity thấp (cần user chạy server + mở trang độc hại), nhưng chi phí fix gần bằng 0.

**Fix:** `OriginPatterns` allow-list thay vì skip; Figma plugin iframe gửi `Origin: null` hoặc `https://www.figma.com`.

---

### 🟡 P2-11..16 — Nhóm nhỏ

| # | Vấn đề | Vị trí |
|---|---|---|
| 11 | Pipeline lỗi → mất `results`, caller không biết step nào đã chạy | `batch-pipeline.ts:120-131` |
| 12 | `Node.Send` mutate slice/map của caller (side effect) | `node.go:63-71` |
| 13 | `NormalizeNodeID` chỉ áp `nodeId`/`parentId` top-level — bỏ sót `componentId`, `startNodeId`, `endNodeId`, và **toàn bộ params lồng trong pipeline step** | `node.go:66-71` |
| 14 | 5 file fail `gofmt`; CI chỉ `test` + `build`, không có `gofmt -l` / `go vet` | `.github/workflows/ci.yml` |
| 15 | `save_screenshots` dùng `O_EXCL` → không bao giờ ghi đè được, user phải xoá file thủ công mỗi lần chụp lại | `tools.go:223` |
| 16 | Dead code: `BatchPipelineRequest/Step/Response` khai trong Go nhưng không ai dùng (plugin có bản TS riêng) | `schema.go:1091-1110` |

---

## PHẦN B — THU GỌN KIẾN TRÚC

### B1. 84 handler boilerplate → 1 bảng declarative

Mọi tool hiện viết tay cùng một pattern ~25-40 dòng:

```go
s.AddTool(mcp.NewTool("set_strokes",
    mcp.WithDescription(...), mcp.WithString("nodeId", ...), mcp.WithString("color", ...), ...
), func(ctx, req) (*mcp.CallToolResult, error) {
    nodeID, _ := req.GetArguments()["nodeId"].(string)
    params := map[string]interface{}{"color": req.GetArguments()["color"]}
    if sw, ok := req.GetArguments()["strokeWeight"].(float64); ok { params["strokeWeight"] = sw }
    ...
    resp, err := node.Send(ctx, "set_strokes", []string{nodeID}, params)
    return renderResponse(resp, err)
})
```

Tổng ~1.400 dòng trên 8 file `tools_*.go` mà **không có một dòng logic riêng nào** — chỉ là copy arg vào map.

**Đề xuất:**
```go
type toolSpec struct {
    Name, Desc  string
    NodeIDsFrom string          // "nodeId" | "nodeIds" | ""
    Params      []paramSpec     // name, type, required, desc, enum
}
```
Một generic handler đọc `Params`, một loop đăng ký. Ước tính giảm **~70% code Go**, và quan trọng hơn: mở đường cho B2.

### B2. Một nguồn sự thật cho tool contract

Hiện một tool được mô tả ở **4 nơi độc lập**:

| Nơi | File |
|---|---|
| MCP JSON Schema | `internal/tools_*.go` |
| Validation runtime | `internal/schema.go` |
| Implementation | `plugin/src/write-*.ts` (switch-case) |
| Tài liệu | `README.md`, `docs/specs/` |

**Bug P1-4, P1-5, P1-7 đều là hệ quả trực tiếp của việc 4 nguồn này lệch nhau.** Không có cơ chế nào phát hiện lệch — `tools_schema_test.go` chỉ đếm số tool và check `items.type`.

**Đề xuất:** một file `tools.yaml` sinh ra (a) Go registration, (b) Go validator, (c) TS dispatch type. Lệch schema thành lỗi compile thay vì lỗi runtime ở nhà user.

### B3. Leader/Follower — cân nhắc bỏ hoặc đảo ngược

Chi phí hiện tại: `election.go` + `leader.go` + `follower.go` + `node.go` + RPC types ≈ **500 dòng**, và quan trọng hơn là **2 code path song song** — chính là nguồn của P1-5 (validation lệch) và P1-6 (timeout lệch).

Câu hỏi cần trả lời trước khi động vào: **thực tế có bao nhiêu user chạy nhiều MCP client cùng lúc?** Nếu ít:

- **Phương án A (bỏ):** một process, một bridge, `EADDRINUSE` → báo lỗi rõ ràng "đã có instance đang chạy". Xoá ~500 dòng + 1 code path.
- **Phương án B (đảo ngược, khuyến nghị nếu cần multi-client):** tách daemon `figma-mcp-go serve` giữ bridge; mọi process stdio đều là follower thuần. **Chỉ còn một code path** cho tool call → P1-5 và P1-6 biến mất theo cấu trúc, không cần fix thủ công.

### B4. Bridge cần keepalive + trần request

Không có ping/pong → connection chết âm thầm, chỉ phát hiện khi request đầu tiên timeout (30s). Thêm `conn.Ping()` mỗi 20s. Và đặt trần tổng thời gian sống cho request để progress update không giữ nó vô hạn (xem P1-6).

### B5. Plugin: dispatch chain 7 tầng → map

`plugin/src/main.ts:24`:
```ts
(await handleReadRequest(request)) ?? (await handleWriteRequest(request))
```
Mọi request **write** đều phải đi qua 3 read handler trước. Rồi `write-handlers.ts:12-18` xâu chuỗi tiếp 7 handler nữa, mỗi handler là một `switch` lớn.

Thay bằng `Record<string, Handler>` — O(1), và trùng tên tool trở thành lỗi build thay vì "handler nào đứng trước thì thắng".

---

## PHẦN C — THU GỌN TOOL SURFACE

**Chi phí hiện tại: 58.931 bytes ≈ 14.7k token mỗi session.**

Ngoài chi phí context, số tool lớn còn làm giảm độ chính xác chọn tool của model — 84 lựa chọn với nhiều cặp gần trùng nghĩa.

### Tier 1 — Gộp an toàn (cùng shape, rủi ro thấp): **84 → 72**

| Gộp | Tool hiện tại | Thành |
|---|---|---|
| Thuộc tính node (8→1) | `set_visible`, `lock_nodes`, `unlock_nodes`, `rotate_nodes`, `reorder_nodes`, `set_blend_mode`, `set_constraints`, `set_opacity` | `set_node_properties(nodeIds, {visible?, locked?, rotation?, order?, blendMode?, constraints?, opacity?})` |
| Hình học (2→1) | `move_nodes`, `resize_nodes` | `transform_nodes(nodeIds, {x?, y?, width?, height?})` |
| Scan (2→1) | `scan_text_nodes` | bỏ — đúng bằng `scan_nodes_by_types(['TEXT'])`, **description hiện tại đã tự thừa nhận là "shorthand"** |
| Đọc node (2→1) | `get_node` | bỏ — đúng bằng `get_nodes_info([id])`, description đã khuyên dùng cái kia |
| Reactions (2→1) | `remove_reactions` | bỏ — đúng bằng `set_reactions(mode='replace', reactions=[])` |
| Annotations (2→1) | `clear_annotations` | bỏ — đúng bằng `set_annotations([])` |

Cả 8 tool nhóm 1 đều dùng chung shape `nodeIds[] + 1 thuộc tính` → gộp không làm mờ ngữ nghĩa. 4 nhóm còn lại là **xoá tool thừa hoàn toàn**, không mất năng lực nào.

Ước tính: **-12 tool, tiết kiệm ~3.5k token.**

### Tier 2 — Gộp mạnh tay (cần cân nhắc): **72 → 58**

| Gộp | Tool | Thành | Rủi ro |
|---|---|---|---|
| Shape (7→1) | `create_rectangle/ellipse/star/polygon/line/frame/section` | `create_node(type, ...)` | Trung bình — params khác nhau (`radius` vs `width/height` vs `pointCount`), schema thành union lỏng lẻo |
| Paint (3→1) | `set_fills`, `set_gradient_fills`, `set_strokes` | `set_paint(nodeId, target, paint)` | Trung bình |
| Style (4→1) | `create_paint/text/effect/grid_style` | `create_style(type, ...)` | Thấp — đã cùng pattern |
| Page (4→1) | `add/delete/rename/navigate_to_page` | `manage_page(action, ...)` | Thấp |

Ước tính thêm: **-14 tool, tiết kiệm ~4k token.** Tổng còn ~7k token schema (giảm 52%).

**Đánh đổi cần biết:** gộp quá tay thì LLM chọn sai param nhiều hơn chọn sai tool. Nhóm shape (7→1) là rủi ro nhất vì params thực sự khác nhau. Khuyến nghị làm Tier 1 trước, đo lại chất lượng, rồi mới quyết Tier 2.

**Breaking change:** cả 2 tier đều phá prompt/workflow của user hiện tại. Nên gói vào một major version, giữ alias deprecated 1-2 release.

---

## PHẦN D — MENU GÓI VIỆC

| Gói | Nội dung | Ước lượng | Breaking | Ưu tiên |
|---|---|---|---|---|
| **G1** | P0-1, P0-2, P0-3 — sửa data-loss + nối `nodeIds` + integration test thật cho pipeline | ~1 ngày | Không | 🔴 Nên làm ngay |
| **G2** | P1-4, P1-5, P1-6, P1-7 — schema `steps`, đưa validation vào `Node.Send`, hợp nhất timeout, validate hex | ~1 ngày | Không | 🟠 Cao |
| **G3** | P2-8 → P2-16 — dọn bug nhỏ + thêm `gofmt`/`go vet` vào CI + xoá dead code | ~0.5 ngày | Không | 🟡 Trung bình |
| **G4** | Thu gọn tool Tier 1 (84→72) | ~1-2 ngày | **Có** | 🟡 Trung bình |
| **G5** | Thu gọn tool Tier 2 (72→58) | ~2-3 ngày | **Có** | ⚪ Thấp |
| **G6** | B1 + B2 — bảng tool declarative + single source of truth | ~3-4 ngày | Không (nội bộ) | 🟠 Cao (ngăn bug tái phát) |
| **G7** | B3 — đơn giản hoá leader/follower | ~2 ngày | Có thể (CLI flag) | 🟡 Trung bình |
| **G8** | B4 + B5 — keepalive bridge + dispatch map plugin | ~0.5 ngày | Không | 🟡 Trung bình |

### Gợi ý thứ tự

**Nếu muốn giá trị/công sức cao nhất:** G1 → G2 → G3 (2.5 ngày, không breaking, xử lý hết mọi lỗi đã xác minh).

**Nếu muốn giải quyết gốc rễ:** G1 → G6 → G4. G6 làm trước G4 khiến việc gộp tool chỉ là sửa một file config thay vì viết lại 8 file Go — và ngăn lớp bug "4 nguồn lệch nhau" tái diễn.

**Nếu ưu tiên chi phí context:** G4 trước (thấy ngay -3.5k token/session), nhưng sẽ phải làm lại phần lớn khi G6 vào sau.

# G4 — Tool Consolidation Tier 1 (TDD Plan)

**Ngày:** 2026-08-15
**Quyết định đã chốt:** handler plugin mới + fallback fanout ở Go; xoá hẳn tên tool cũ khỏi MCP schema (giữ wire handler ở plugin).
**Tiền đề:** [Architecture & Bug Review](./2026-08-15-architecture-and-bug-review.md)

---

## 0. ĐÍNH CHÍNH report trước

Khi verify code để lên plan, **4 tool tao xếp vào nhóm "xoá thừa hoàn toàn, không mất năng lực" đều sai.** Chi tiết:

| Tool | Report cũ nói | Sự thật (đã verify) |
|---|---|---|
| `scan_text_nodes` | = `scan_nodes_by_types(['TEXT'])` | ❌ **Sai hoàn toàn.** `scan_nodes_by_types` trả `{id,name,type,bbox}` — **không có `characters`**. Nó còn `if (!n.visible) return` bỏ qua node ẩn, `scan_text_nodes` thì không. Hai tool trả dữ liệu khác hẳn nhau. |
| `get_node` | = `get_nodes_info([id])` | ⚠️ Cùng `serializeNode`, nhưng `get_node` **throw** khi không thấy node, `get_nodes_info` **âm thầm filter mất** (`read-document.ts:76`). Xoá đi là mất khả năng chẩn đoán. |
| `remove_reactions` | = `set_reactions(replace, [])` | ⚠️ Chỉ đúng khi xoá tất cả. Với `indices: [1,3]` (xoá reaction cụ thể) thì **không có cách thay thế** ngoài get→filter→set 2 vòng. |
| `clear_annotations` | = `set_annotations([])` | ⚠️ `clear_annotations` nhận **`nodeIds[]` nhiều node**, `set_annotations` chỉ nhận **1 `nodeId`**. Xoá 10 node thành 10 call. |

Nguồn gốc sai: tao tin vào description của chính tool (`scan_text_nodes` tự mô tả là *"Shorthand for scan_nodes_by_types with ['TEXT']"*) thay vì đọc implementation. Description đó **sai** và cần sửa luôn trong plan này.

### Con số token cũng sai

Đo chính xác từng tool (`tools/list` = 58.802 bytes):

| Nhóm | Bytes hiện tại | Sau khi gộp (ước tính) | Tiết kiệm |
|---|---|---|---|
| 8 tool node-property | 4.070 | ~1.100 (`set_node_properties`) | **2.970** |
| `move_nodes` + `resize_nodes` | 1.176 | ~700 (`transform_nodes`) | **476** |
| `remove_reactions` | 636 | 0 (+150 vào `set_reactions`) | **486** |
| `clear_annotations` | 376 | 0 (+120 vào `set_annotations`) | **256** |
| `get_node` | 495 | 0 (+80 vào `get_nodes_info`) | **415** |
| `scan_text_nodes` | 487 | **giữ nguyên** | 0 |
| | | **Tổng** | **~4.600 bytes ≈ 1.150 token** |

**Report cũ tao ghi "-12 tool, ~3.5k token" — thực tế là -11 tool, ~1.15k token** (7,8% schema, ~0,6% cửa sổ 200k). Lý do: nhóm tool này nhỏ hơn trung bình (~500 vs 700 bytes/tool), và 3 tool sống sót phải phình ra để giữ năng lực.

### Nên cân nhắc lại trước khi làm

Nếu động lực chính của mày là **cắt token**, G4 không đáng 2-3 ngày cho 1,15k token. Phần lớn tiết kiệm nằm ở đúng **một** hạng mục: gộp 8 tool node-property (2.970 bytes = 65% tổng lợi ích, và là phần sạch nhất về kỹ thuật).

Ba đường đi:

- **G4-mini** — chỉ làm Phase 1+2 (node properties + transform). -8 tool, ~3.4k bytes, ~1 ngày, không có regression nào. **Tỷ lệ lợi ích/công sức tốt nhất.**
- **G4 đầy đủ** — như plan dưới đây. -11 tool, ~4.6k bytes, 2-3 ngày.
- **Đổi sang Tier 2** — các tool to hơn nhiều (`create_*` 7 tool, `create_*_style` 4 tool). Tiết kiệm lớn hơn hẳn nhưng rủi ro LLM chọn sai param cao hơn.

Plan dưới viết cho **G4 đầy đủ**; Phase 1-2 tách rời được nên cắt xuống G4-mini chỉ là dừng sau Phase 2.

---

## 1. Phạm vi chốt lại

| Phase | Thay đổi | Δ tool |
|---|---|---|
| 1 | `set_visible`, `lock_nodes`, `unlock_nodes`, `rotate_nodes`, `reorder_nodes`, `set_blend_mode`, `set_constraints`, `set_opacity` → **`set_node_properties`** | −7 |
| 2 | `move_nodes`, `resize_nodes` → **`transform_nodes`** | −1 |
| 3 | `remove_reactions` → gộp vào `set_reactions` qua `removeIndices` | −1 |
| 4 | `clear_annotations` → `set_annotations` nhận `nodeIds[]` | −1 |
| 5 | `get_node` → `get_nodes_info` báo id thiếu tường minh | −1 |
| 6 | `scan_text_nodes` **giữ nguyên**, chỉ sửa description đang nói sai | 0 |
| | **84 → 73** | **−11** |

**Nguyên tắc xuyên suốt:** không phase nào được làm mất năng lực. Mọi thao tác làm được trước đây phải làm được sau đây, bằng đúng **một** tool call.

---

## 2. Tiền đề kỹ thuật — test seam cho fanout

Fallback fanout không test được nếu không tách được lớp gửi. Hiện `Node.Send` gọi thẳng `*Bridge` / `*Follower` (`node.go:75-78`).

**Phase 0 phải làm trước tiên:**

```go
type sender interface {
    Send(ctx context.Context, tool string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error)
}
```

`Node` giữ `sender`, test inject fake. `*Bridge` và `*Follower` đã có đúng signature này → không cần sửa gì ở hai bên.

Bonus: seam này cũng là thứ cần cho **P1-5** (đưa `ValidateRPC` vào `Node.Send`) ở gói G2 sau này.

---

## 3. Cơ chế fallback

Plugin cũ không biết `set_node_properties` → `main.ts:26` throw `Unknown request type: set_node_properties` → về Go thành `resp.Error`.

```go
const unknownTypePrefix = "Unknown request type"

func sendWithFanout(ctx, s sender, modern string, nodeIDs []string,
                    params map[string]any, legacy []legacyCall) (BridgeResponse, error) {
    resp, err := s.Send(ctx, modern, nodeIDs, params)
    if err != nil || !strings.HasPrefix(resp.Error, unknownTypePrefix) {
        return resp, err          // plugin mới, hoặc lỗi thật → trả thẳng
    }
    return fanout(ctx, s, nodeIDs, legacy)  // plugin cũ
}
```

**Đánh đổi phải ghi vào doc cho user biết:** đường fanout gọi N lệnh riêng → **N undo entry** (Ctrl+Z phải bấm N lần) và không atomic (lệnh thứ 2 lỗi thì lệnh thứ 1 đã áp dụng rồi). Plugin mới không bị. → thêm một dòng vào README khuyên update plugin.

Match theo prefix string là mong manh. Chấp nhận được vì chuỗi đó do **chính repo này** sinh ra (`main.ts:26`), không phải từ Figma API. Thêm test khoá chuỗi đó ở cả hai phía để không ai đổi lệch.

---

## 4. TDD — Phase 0: golden tool set

Thay `TestToolSchemas_AllToolsRegistered` (đang assert `const want = 84`) bằng assertion theo **danh sách tên**, để mỗi phase có tín hiệu RED→GREEN chính xác thay vì chỉ đếm số.

**RED** — viết test với 84 tên hiện tại → phải GREEN ngay (đây là baseline, chưa đổi gì):

```go
// internal/tools_schema_test.go
var expectedTools = []string{ /* 84 tên, sort sẵn */ }

func TestToolSchemas_ExpectedToolSet(t *testing.T) {
    got := toolNames(listTools(t))   // sorted
    if diff := cmp.Diff(expectedTools, got); diff != "" {
        t.Errorf("tool set thay đổi ngoài dự kiến (-want +got):\n%s", diff)
    }
}
```

Mỗi phase sau: sửa `expectedTools` **trước** → RED → implement → GREEN.

Giữ luôn `TestToolSchemas_ArrayItemsHaveType` (đang bảo vệ khỏi lỗi Copilot validation) — tool mới có `nodeIds` array nên test này phải phủ được.

---

## 5. Phase 1 — `set_node_properties` (8 → 1)

Đây là phase to nhất và là 65% lợi ích. Tám handler plugin hiện có **skeleton giống hệt nhau** (đã verify `write-modify.ts:157-320`):

```
for nid of nodeIds:
  n = await getNodeByIdAsync(nid)
  if !n              → results.push({nodeId, error: "Node not found"});     continue
  if !(prop in n)    → results.push({nodeId, error: "does not support X"}); continue
  apply prop
  results.push({nodeId, <prop>: value})
commitUndo()
```

→ gộp thành một vòng lặp áp nhiều thuộc tính là biến đổi thuần tuý, không có logic nào bị mất.

### Shape chốt

```jsonc
// request
{ "nodeIds": ["1:1","2:2"],
  "visible": true, "locked": false, "opacity": 0.5, "rotation": 45,
  "order": "bringToFront", "blendMode": "MULTIPLY",
  "constraints": { "horizontal": "STRETCH", "vertical": "MIN" } }

// response — lỗi ở mức từng thuộc tính, không phải cả node
{ "results": [
    { "nodeId": "1:1", "applied": { "opacity": 0.5, "visible": true } },
    { "nodeId": "2:2", "applied": { "opacity": 0.5 },
      "errors": { "rotation": "Node does not support rotation" } },
    { "nodeId": "9:9", "error": "Node not found" }
] }
```

Lý do lỗi ở mức thuộc tính: một node có thể hỗ trợ `opacity` nhưng không hỗ trợ `rotation`. Gộp mà báo lỗi cả node sẽ **mất thông tin** so với 8 tool cũ.

### RED — Go

```
internal/tools_schema_test.go
  ✎ expectedTools: bỏ 8 tên, thêm "set_node_properties"

internal/tools_handler_test.go
  + TestSetNodeProperties_Schema
      - nodeIds required, type array, items.type == "string"
      - có đủ 7 optional: visible/locked/opacity/rotation/order/blendMode/constraints
      - constraints là object có horizontal + vertical

internal/schema_test.go
  + TestValidateRPC_SetNodeProperties  (table-driven)
      - nodeIds rỗng                          → "nodeIds is required"
      - không truyền thuộc tính nào           → "at least one property is required"
      - opacity = 5                           → "opacity must be between 0 and 1"
      - opacity = 0 và 1                      → hợp lệ (biên)
      - blendMode = "NEON"                    → invalid
      - order = "bringToMiddle"               → invalid
      - constraints.horizontal = "MIDDLE"     → invalid
      - nodeId sai format                     → invalid
      - hợp lệ đầy đủ 7 thuộc tính            → ""

internal/fanout_test.go            (mới — cần seam Phase 0)
  + TestFanout_ModernPluginSingleCall
      fake sender trả OK → đúng 1 call, tool == "set_node_properties"
  + TestFanout_LegacyPluginFansOut
      fake trả Error "Unknown request type: set_node_properties"
      → 3 call kế tiếp: set_visible / set_opacity / set_blend_mode
      → results gộp lại giống shape đường modern
  + TestFanout_RealErrorNotRetried
      fake trả Error "Node not found" → KHÔNG fanout, trả thẳng (chống retry bão)
  + TestFanout_PreservesNodeIDs
```

### RED — Plugin

```
plugin/src/write-modify.test.ts
  + describe("set_node_properties")
      - áp nhiều thuộc tính trong 1 call, đúng 1 lần commitUndo
      - node không hỗ trợ rotation → errors.rotation, opacity vẫn áp
      - node không tồn tại        → results[].error == "Node not found"
      - nodeIds rỗng              → throw "nodeIds is required"
      - order = "bringToFront" đổi đúng index (mock parent.children)
      - constraints merge với giá trị cũ, không ghi đè trục không truyền
      - không truyền thuộc tính nào → throw
```

Test `constraints` merge quan trọng: handler cũ (`write-modify.ts:310-313`) làm `{...n.constraints}` rồi chỉ ghi đè trục được truyền. Phải giữ đúng hành vi đó.

### GREEN
- `plugin/src/write-modify.ts`: thêm `case "set_node_properties"`, **giữ nguyên 8 case cũ** (fanout cần).
- `internal/tools_write_nodeprops.go` (mới): đăng ký tool + bảng `legacyCall`.
- `internal/fanout.go` (mới): `sendWithFanout` + `fanout`.
- `internal/schema.go`: thêm case `set_node_properties`.

### REFACTOR
- Xoá 8 đăng ký ở `tools_write_modify.go` + 8 case ở `schema.go`.
- **Không** xoá 8 case ở plugin.
- Chạy `gofmt -w` (5 file đang lỗi format sẵn — xem P2-14).

---

## 6. Phase 2 — `transform_nodes` (2 → 1)

Cùng khuôn Phase 1, nhỏ hơn. Giữ **nguyên semantic tuyệt đối** của `move_nodes` (description hiện tại nói rõ *"not a relative offset"*) — không thêm chế độ relative, đó là scope khác.

```jsonc
{ "nodeIds": ["1:1"], "x": 100, "y": 200, "width": 300, "height": 400 }
```

### RED
```
Go   ✎ expectedTools: −move_nodes −resize_nodes +transform_nodes
Go   + TestValidateRPC_TransformNodes
        - không có x/y/width/height nào  → "at least one of x, y, width, or height is required"
        - width <= 0                     → invalid
        - height <= 0                    → invalid
        - chỉ x                          → hợp lệ
Go   + TestFanout_TransformNodes → legacy fanout ra move_nodes + resize_nodes
TS   + describe("transform_nodes")
        - chỉ x/y  → không gọi resize()
        - chỉ width → resize(w, n.height) giữ nguyên chiều cao
        - node không có "resize" → errors.resize, x/y vẫn áp
        - đúng 1 commitUndo cho cả move + resize   ← đường cũ tốn 2
```

Test cuối là lợi ích thật nhìn thấy được: đặt lại vị trí + kích thước giờ là **một** undo entry thay vì hai.

---

## 7. Phase 3 — `set_reactions` hấp thụ `remove_reactions`

Thêm `removeIndices` để giữ năng lực xoá theo index (thứ mà `set_reactions(replace, [])` **không** làm được).

```jsonc
{ "nodeId": "1:1", "removeIndices": [1, 3] }   // xoá reaction #1 và #3
{ "nodeId": "1:1", "reactions": [...], "mode": "append" }
```

### RED
```
Go  ✎ expectedTools: −remove_reactions
Go  + TestValidateRPC_SetReactions_RemoveIndices
       - có cả reactions lẫn removeIndices     → "reactions and removeIndices are mutually exclusive"
       - không có cả hai                        → "reactions or removeIndices is required"
       - removeIndices chứa phần tử không số   → invalid
       - removeIndices số âm                    → invalid   (ràng buộc mới, cũ không check)
       - removeIndices: []                      → hợp lệ, nghĩa là xoá tất cả  ← giữ đúng hành vi cũ
TS  + set_reactions với removeIndices [1,3] → còn lại index 0,2
    + removeIndices: [] → xoá sạch          (khớp write-prototype.ts:69-73)
    + removeIndices ngoài range → bỏ qua, không throw
    + reactions vẫn hoạt động như cũ (regression)
```

Chú ý giữ đúng quirk cũ: `indices` **rỗng** nghĩa là *xoá tất cả*, không phải *không xoá gì* (`write-prototype.ts:70-73`). Dễ làm hỏng nếu viết lại từ đầu.

---

## 8. Phase 4 — `set_annotations` nhận nhiều node

`nodeId` (1 node) → `nodeIds[]` (nhiều node). `annotations: []` thay cho `clear_annotations`.

**Breaking response shape:** `set_annotations` đang trả `{id, success}`, sau đổi thành `{results:[...]}` giống mọi tool multi-node khác. Phải ghi vào migration note.

### RED
```
Go  ✎ expectedTools: −clear_annotations
Go  + TestValidateRPC_SetAnnotations
       - nodeIds rỗng          → invalid
       - annotations thiếu     → "annotations array is required"
       - annotations: []       → hợp lệ (đường clear)
       - 1 trong 3 id sai format → invalid, báo đúng id nào
TS  + set_annotations nhiều node → results[] mỗi node
    + annotations: [] xoá sạch trên nhiều node
    + node không hỗ trợ annotations → results[].error, node khác vẫn chạy
```

Lưu ý: `tools_schema_test.go:67` đang **exclude** `set_annotations` khỏi check `items.type`. Sửa schema xong thì bỏ exclusion đó — coi như dọn luôn một khoản nợ.

---

## 9. Phase 5 — `get_nodes_info` báo id thiếu

Xoá `get_node`, đồng thời **sửa bug im lặng**: `read-document.ts:76` filter mất node không tồn tại mà không nói gì.

```jsonc
// trước: [ {...}, {...} ]          ← id sai biến mất không dấu vết
// sau:   { "nodes": [...], "missing": ["9:9"] }
```

**Breaking response shape** — đây là đổi lớn nhất của G4 với user hiện tại. Ghi rõ vào migration note.

### RED
```
Go  ✎ expectedTools: −get_node
Go  + TestValidateRPC_GetNodesInfo: 1 id hợp lệ vẫn pass (thay get_node)
TS  + get_nodes_info trả { nodes, missing }
    + id không tồn tại → vào missing, KHÔNG biến mất
    + node DOCUMENT    → lọc ra (giữ hành vi cũ)
    + 1 id             → nodes có đúng 1 phần tử
    + tất cả id sai    → { nodes: [], missing: [tất cả] }
```

---

## 10. Phase 6 — Docs & release

1. `README.md` — bảng tool (14 dòng ở L124-212), thêm mục **Migration** ánh xạ tên cũ → mới.
2. `README.md` — thêm cảnh báo: plugin cũ vẫn chạy qua fanout nhưng tốn nhiều undo entry, nên tải lại zip.
3. `glama.json` — 85 entry, phải khớp schema thật.
4. **Sửa description sai của `scan_text_nodes`** — bỏ chữ *"Shorthand for scan_nodes_by_types with ['TEXT']"*, thay bằng nội dung đúng: nó trả `characters`/`fontSize`/`fontName` và **quét cả node ẩn**, còn `scan_nodes_by_types` thì không trả text và bỏ qua node ẩn. Đây chính là câu description đã lừa tao ở report trước.
5. Bump **major** version (`version.go`, `server.json`, `plugin/package.json`).
6. CI: thêm `gofmt -l` + `go vet` (P2-14) để phase này không đẻ thêm file lệch format.

---

## 11. Thứ tự & ước lượng

| Phase | Nội dung | Est. | Cắt được? |
|---|---|---|---|
| 0 | Golden tool set + `sender` seam | 0.5d | Không (nền) |
| 1 | `set_node_properties` + fanout | 1.0d | Không (65% lợi ích) |
| 2 | `transform_nodes` | 0.5d | Không |
| 3 | `set_reactions` + `removeIndices` | 0.25d | ✓ |
| 4 | `set_annotations` đa node | 0.25d | ✓ |
| 5 | `get_nodes_info` + missing | 0.25d | ✓ |
| 6 | Docs, glama, version, CI | 0.5d | Không |
| | **Tổng** | **~3.25d** | **G4-mini = Phase 0+1+2+6 ≈ 2.5d** |

Nhích lên so với ước lượng 1-2 ngày ở report trước, vì (a) 3 tool sống sót phải sửa để giữ năng lực, (b) fallback fanout cần test seam.

---

## 12. Điều kiện hoàn thành

- [ ] `go test ./...` xanh; `make test-ts` xanh
- [ ] `gofmt -l .` không ra file nào; `go vet ./...` sạch
- [ ] `TestToolSchemas_ExpectedToolSet` khớp đúng 73 tên
- [ ] `TestToolSchemas_ArrayItemsHaveType` xanh **không còn exclusion** cho `set_annotations`
- [ ] Test fanout phủ cả 3 nhánh: plugin mới / plugin cũ / lỗi thật
- [ ] Đo lại `tools/list`, xác nhận ~54.200 bytes (từ 58.802)
- [ ] Smoke tay trên Figma thật: plugin mới 1 undo entry; plugin cũ (zip release trước) vẫn chạy qua fanout
- [ ] Không tool nào mất năng lực — đối chiếu từng dòng bảng ở §1

## 13. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Match `"Unknown request type"` theo prefix bị lệch khi ai đó sửa `main.ts` | Test khoá chuỗi ở cả hai phía; đặt hằng số kèm comment trỏ chéo |
| Fanout đẻ N undo entry gây khó chịu | Ghi rõ trong README + nhắc update plugin; đường modern không bị |
| Đổi shape `get_nodes_info` phá workflow user | Bump major + migration note; đây là phase cắt được nếu muốn hoãn |
| LLM chọn sai param trong tool gộp | Description ghi rõ từng thuộc tính là optional và độc lập; giữ enum trong schema để client tự validate |
| Gộp làm mất lỗi từng thuộc tính | Shape `applied` + `errors` ở §5 giữ nguyên độ chi tiết của 8 tool cũ |

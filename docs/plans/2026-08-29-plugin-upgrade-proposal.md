# Đề xuất nâng cấp & tính năng mới cho Figma plugin

Ngày: 2026-08-29 · Phạm vi: `plugin/` (và phần giao thức chạm tới `internal/bridge`)

Tài liệu này là **đề xuất**. Mỗi mục ghi rõ vấn đề hiện tại, đề xuất, file liên
quan và ước lượng công sức (S/M/L).

> **Trạng thái 2026-08-29** — cả bốn đợt đã làm xong, đánh dấu ✅ ở bảng dưới và
> ở từng mục. Hai quyết định lệch khỏi đề xuất ban đầu, ghi rõ ở 3.2 và 2:
> pairing token bị loại vì phá trải nghiệm kết nối liền mạch, và Dev Mode
> codegen được làm theo hướng không cần MCP sampling.
>
> Phần còn lại — những mục nhỏ chưa làm — gom ở mục 7 cuối tài liệu.

---

## 0. Tóm tắt ưu tiên

| # | Hạng mục | Vì sao | Công sức | Trạng thái |
|---|----------|--------|----------|------------|
| 1 | Auto-layout sizing (`HUG`/`FILL`), absolute positioning, min/max | Chặn mọi layout responsive — đau nhất hiện nay | S | ✅ |
| 2 | Cảnh báo lệch version plugin ↔ server | Plugin cũ + server mới = "Unknown request type" khó hiểu | S | ✅ |
| 3 | `set_selection` | Human-in-the-loop: "cho tôi xem cái vừa tạo" | S | ✅ |
| 4 | Activity log + chế độ duyệt (approve) trong UI | Quan sát được và an toàn khi AI ghi | M | ✅ |
| 5 | Rich text theo range | Không tạo được text nhiều style / link | M | ✅ |
| 6 | Vector & boolean ops | Không dựng được icon | M | ✅ |
| 7 | Component set / variants / component properties | Không dựng được design system hoàn chỉnh | M | ✅ |
| 8 | Tìm kiếm toàn document | Sai âm thầm với `documentAccess: dynamic-page` | S | ✅ |
| 9 | Dev Mode codegen provider | Khác biệt lớn so với các Figma MCP khác | L | ✅ (xem 2) |
| 10 | Huỷ request + phân trang phản hồi lớn | Ổn định trên file lớn | M | ✅ |
| 11 | Gộp undo cho pipeline + checkpoint version history | Hoàn tác một pipeline không còn là 20 lần Ctrl+Z | S | ✅ |
| 12 | Auto-copy node id bật sẵn | Việc mọi phiên đều bắt đầu bằng | S | ✅ |

---

## 1. Bổ sung độ phủ Figma Plugin API (tool mới)

### 1.1 Auto-layout còn thiếu phần quan trọng nhất — ✅ **đã làm**
`applyAutoLayout` (`plugin/src/write-helpers.ts`) chỉ set `layoutMode`, padding,
`itemSpacing`, align, sizing mode, wrap. Thiếu:

- `layoutSizingHorizontal` / `layoutSizingVertical` (`FIXED` | `HUG` | `FILL`) —
  đây mới là API mà designer thực sự dùng; `primaryAxisSizingMode` là API cũ và
  không diễn tả được "fill container" của child.
- `layoutPositioning: "ABSOLUTE"` + `constraints` cho phần tử nổi trong auto layout.
- `minWidth` / `maxWidth` / `minHeight` / `maxHeight` (responsive).
- `layoutGrow`, `layoutAlign` cho từng child.
- `itemReverseZIndex`, `strokesIncludedInLayout`, `clipsContent`.

Ngoài ra `set_auto_layout` (`plugin/src/write-modify.ts`) chặn cứng
`node.type !== "FRAME"`, trong khi `COMPONENT`, `COMPONENT_SET` và `INSTANCE`
đều hỗ trợ auto layout → nên nới thành kiểm tra `"layoutMode" in node`.

**Đã làm**: mở rộng `applyAutoLayout` với `layoutSizingHorizontal/Vertical`,
`minWidth/maxWidth/minHeight/maxHeight` (null để xoá), `layoutPositioning`,
`layoutAlign`, `layoutGrow`, `itemReverseZIndex`, `strokesIncludedInLayout`,
`clipsContent`; bỏ ràng buộc `type !== FRAME`. Thêm cờ `Nullable` ở `paramSpec`
phía Go để null đi được tới plugin thay vì bị loại như tham số vắng mặt.
Chưa làm: tool `set_layout_sizing` áp cho nhiều node cùng lúc.

### 1.2 Rich text theo range
`set_text` chỉ ghi toàn bộ `characters` với một font/màu. Thiếu:

- Đọc: `getStyledTextSegments([...])` → serializer hiện trả text như một khối phẳng,
  nên "design → code" mất bold/link/màu inline.
- Ghi: `setRangeFontName`, `setRangeFills`, `setRangeFontSize`,
  `setRangeTextDecoration`, `setRangeHyperlink`, `setRangeListOptions`,
  `setRangeTextStyleId`.
- Thuộc tính đoạn: `paragraphSpacing`, `paragraphIndent`, `textAutoResize`,
  `textTruncation`, `maxLines`, `leadingTrim`.

**Đề xuất**: tool `set_text_ranges` nhận mảng `{start, end, style}` và mở rộng
`serializeNode` trả `styledSegments`. Công sức: M.

### 1.3 Vector & boolean operations — hiện **hoàn toàn chưa có**
Không có `booleanOperation`, `flatten`, `outlineStroke`, `vectorPaths`,
`setVectorNetworkAsync`. Hệ quả: AI không dựng được icon, không đơn giản hoá
hình, không import SVG path.

**Đề xuất**: `boolean_operation` (union/subtract/intersect/exclude),
`flatten_nodes`, `outline_stroke`, `create_vector` (nhận SVG path data —
`figma.createNodeFromSvg` là đường ngắn nhất). Công sức: M.

### 1.4 Component set, variants, component properties
Hiện có `create_component`, `swap_component`, `detach_instance`,
`set_instance_overrides`. Thiếu:

- `figma.combineAsVariants` → tạo `COMPONENT_SET`.
- `componentPropertyDefinitions`: add / edit / delete property
  (`BOOLEAN`, `TEXT`, `INSTANCE_SWAP`, `VARIANT`).
- Gắn property vào node (`componentPropertyReferences`).
- Đổi variant của instance theo `{Size: "Large", State: "Hover"}`
  thay vì phải biết id của component con.

**Đề xuất**: nhóm tool `manage_component_properties` + `set_variant`. Đây là mảnh
còn thiếu để nói "full design-system automation". Công sức: M.

### 1.5 Selection & viewport (ghi) — ✅ **đã làm**
Plugin chỉ **đọc** selection. Không có cách nào để AI nói "đây, nhìn cái này".

**Đã làm**: một tool `set_selection` với hai cờ `select`/`zoom` thay vì hai tool —
`select:false, zoom:true` là "focus mà không đụng selection của người dùng".
Tự `setCurrentPageAsync` khi node ở page khác; từ chối danh sách trải nhiều page
vì selection của Figma thuộc về đúng một page.

### 1.6 Duyệt toàn document với `dynamic-page` — ✅ **đã làm**
`manifest.json` khai `documentAccess: "dynamic-page"`, nhưng không nơi nào gọi
`figma.loadAllPagesAsync()`. `search_nodes` chỉ đi từ `figma.currentPage`, còn
`get_document` cũng chỉ serialize page hiện tại. Với file nhiều page, kết quả
"không tìm thấy" là **sai âm thầm**, không phải lỗi.

**Đã làm**: `search_nodes` nhận `scope: "page" | "document"`. Dùng
`page.loadAsync()` từng page thay vì `loadAllPagesAsync()` để file lớn trả tiền
theo từng page, phát `progress_update` mỗi page nên không cần timeout riêng, và
trả thêm `truncated` để phân biệt câu trả lời đầy đủ với câu trả lời bị cắt.
Còn lại: `get_document` vẫn chỉ serialize page hiện tại.

### 1.7 Mask, layout grid, và các thuộc tính node còn thiếu
- `isMask` / `maskType` — không tạo được mask.
- `layoutGrids` trên frame (đang chỉ có *grid style*, không có grid trực tiếp).
- `effects` trên từng node so với effect style — cần kiểm tra lại độ phủ.
- `strokeCap`, `strokeJoin`, `dashPattern`, `strokeMiterLimit` (serializer có đọc
  `dashPattern` nhưng phía ghi thì chưa).
- `exportSettings` trên node (đặt sẵn cấu hình export cho designer).

### 1.8 Hình ảnh
- `figma.createImageAsync(url)` — import ảnh theo URL, khỏi phải nhồi base64 qua
  WebSocket (hiện `import_image` bắt buộc `imageData` base64, rất tốn băng thông).
- Đọc ảnh ngược lại: `getImageByHash(hash).getBytesAsync()` — cần cho luồng
  "design → code" khi phải xuất asset.
- `imageTransform` (crop), `filters` (exposure/contrast/saturation).

### 1.9 Metadata gắn vào file
- `setPluginData` / `setSharedPluginData`: lưu liên kết component ↔ file code,
  đánh dấu node do AI sinh, lưu design-token binding. Sau này chạy lại có ngữ cảnh
  bền vững thay vì hỏi lại từ đầu.
- `setRelaunchData`: hiện nút "Sửa bằng AI" ngay trên node trong Figma.
- ✅ `figma.saveVersionHistoryAsync(title)`: **đã làm** — tool
  `save_version_checkpoint`. Rollback trong WAL của `batch-pipeline.ts` chết cùng
  plugin, nên một version đặt tên là đường lui duy nhất sống lâu hơn phiên.

### 1.10 Mở rộng editor type
`manifest.json` khai `["figma", "dev"]`. Có thể thêm `"figjam"` và `"slides"` —
đã có `create_connector` (FigJam) rồi mà editor lại không bật FigJam.

---

## 2. Dev Mode codegen provider — ✅ **đã làm, theo hướng khác**

Manifest đã có `capabilities: ["inspect"]` nhưng plugin **không** đăng ký
`figma.codegen.on("generate", ...)`. Nếu đăng ký, panel Code của Dev Mode sẽ hiển
thị code do chính MCP server sinh ra cho node đang chọn — designer chọn node,
thấy ngay React/SwiftUI/Compose theo codebase thật của họ.

**Luồng ban đầu đề xuất** — `codegen.on("generate")` → WebSocket → server Go →
hỏi AI client → trả code về — đòi hai thứ: một chiều request mới
(plugin khởi xướng) qua bridge, và **MCP sampling** để server hỏi ngược client
một completion. Sampling là tuỳ chọn trong giao thức MCP và không có ở các
client mà server này nhắm tới, nên xây cả một chiều giao thức cho một tính năng
không client nào chạy được là không đáng.

**Đã làm — lật ngược luồng.** Client sinh code với cả repository trước mặt rồi
gắn lên node bằng `set_codegen_result`; codegen provider phục vụ lại thứ đã
gắn. Code viết vào **shared plugin data** nên nó đi theo file: Dev Mode của cả
nhóm thấy, không chỉ máy đã sinh ra nó. Tra ngược từ node đang chọn → component
mà instance sinh ra → các tổ tiên, nên gắn code lên một COMPONENT là phủ mọi
instance của nó.

Đổi lại, code không tự cập nhật khi design đổi — phải gọi lại tool. Chấp nhận
được, và bù lại code sinh ra tốt hơn vì client nhìn thấy codebase thật.

**Còn lại**: `figma.codegen.on("preferenceschange")` cho chọn ngôn ngữ/framework.

---

## 3. UI plugin (`plugin/src/ui/App.svelte`)

Hiện UI 320×230 chỉ hiện: file/page/selection, danh sách node + copy id, banner
"AI is working…", địa chỉ server, badge kết nối.

### 3.1 Activity log — ✅ **đã làm**
"AI is working…" không cho biết AI đang làm gì. Payload gửi qua UI **đã có**
`payload.type` và `requestId` — chỉ cần hiển thị.

**Đã làm**: 20 request gần nhất với tên tool, thời lượng, ✓/✗, lỗi, và nút copy
để dán vào bug report. Trạng thái "đang chạy" đọc từ chính log thay vì một Set
riêng, nên banner và log không thể mâu thuẫn. Panel tự cao lên khi mở log.

### 3.2 Chế độ an toàn / duyệt thao tác — pairing đã loại
`networkAccess.allowedDomains: ["*"]` và không có xác thực: **bất kỳ tiến trình
nào trên máy mở được WebSocket đều điều khiển được file Figma đang mở**.

**Đã làm** (1) và (2), gộp thành một nút Guard ba trạng thái off / confirm /
read-only, mặc định off để giữ nguyên hành vi mọi người dùng hiện có:
1. **read-only** — chặn mọi tool ghi ở phía UI.
2. **confirm** — hộp xác nhận cho các tool phá huỷ (`delete_nodes`,
   `delete_page`, `delete_style`, `delete_variable`, `detach_instance`,
   `find_replace_text`, `batch_rename_nodes`, `boolean_operation`,
   `flatten_nodes`, và pipeline chứa bất kỳ bước nào trong số đó).
3. ~~**Pairing token**~~ — **đã loại khỏi lộ trình**: nó đánh đổi bằng trải
   nghiệm kết nối liền mạch hiện tại, mà đó lại là điểm mạnh của plugin so với
   các Figma MCP dùng REST API. Rủi ro vẫn còn và được ghi lại ở đây; nếu quay
   lại chủ đề này thì hướng ít phá UX nhất là chỉ yêu cầu ghép mã cho các tool
   phá huỷ, không phải cho toàn bộ kết nối.

Công sức: S cho (1), M cho (2).

### 3.3 Hoàn tác từ UI — ✅ **đã làm**
**Đã làm**: nút Undo dùng `figma.triggerUndo()`. Kết hợp với việc pipeline gộp
thành một mốc undo, một lần bấm hoàn tác trọn một pipeline.

### 3.4 Những cải thiện UI nhỏ nhưng đáng
- `figma.ui.resize` + nhớ kích thước (hiện cố định 320×230, activity log sẽ chật).
- Theo theme sáng/tối của Figma thay vì hard-code nền `#1e1e1e`.
- Nút "Gửi selection cho AI" — pin một tập context ổn định thay vì copy id thủ công.
- `figma.notify` khi có lỗi ghi, kèm nút nhảy tới node lỗi.
- i18n VI/EN.
- Hiện tiến độ % khi có `progress_update` (bridge đã hỗ trợ, UI thì bỏ qua).

---

## 4. Giao thức & hiệu năng

### 4.1 Handshake plugin → server — ✅ **phần version đã làm**
Server gửi `get_server_info` → plugin trả version. Chiều ngược lại thì không:
server không biết plugin có handler nào. Plugin cũ gặp tool mới sẽ báo
"Unknown request type: X" — người dùng không hiểu là do plugin cũ.

**Đề xuất**: plugin trả `{pluginVersion, protocolVersion, handlers: [...]}` ngay
khi kết nối. Server có `Object.keys(readHandlers)` + `writeHandlers` sẵn, gần như
không tốn gì. Từ đó:
- Báo lỗi rõ: "tool cần plugin ≥ v1.4, bạn đang chạy v1.2".
- Ẩn tool client không dùng được (`tools/list_changed` của MCP).
- Test contract: mọi tool trong `internal/tools/toolspec.go` phải có handler tương
  ứng trong plugin. `toolspec_wire_test.go` đã pin wire shape; còn thiếu đúng
  phần đối chiếu tên handler.

**Đã làm**: plugin gửi frame `plugin-info` kèm version ngay khi kết nối; bridge
ghi nhận và log cảnh báo, UI hiện banner chỉ đúng bên đang cũ kèm cách khắc
phục. So sánh theo `major.minor` — plugin cài tay từ release zip còn server tự
cập nhật qua `npx @latest`, nên lệch patch là chuyện thường; version không đọc
được (bản dev) thì im lặng.

**Còn lại**: plugin chưa gửi danh sách handler, nên server vẫn chưa ẩn được tool
mà plugin không có, và chưa có test đối chiếu tên handler với `toolspec.go`.

### 4.2 Huỷ request — ✅ **đã làm**
Không có cách huỷ. Một `get_document` trên file lớn sẽ chạy tới khi hết timeout,
UI đứng "AI is working…". **Đã làm**: bridge gửi `cancel_request` khi caller huỷ context hoặc request hết
ngân sách. Cancel là advisory — handler không kiểm tra thì cứ chạy hết và phản
hồi bị bỏ, nên một vòng lặp mới quên kiểm tra chỉ chậm chứ không hỏng. Đã cắm
kiểm tra ở `search_nodes`, `get_local_components`, `scan_text_nodes`,
`scan_nodes_by_types`, và giữa các bước pipeline.

### 4.3 Payload lớn — ✅ **phần get_document đã làm**
- `get_document` serialize toàn bộ page trong một lần → file lớn dễ vỡ ở
  `postMessage` giữa UI và core, hoặc làm nghẽn context của AI client.
- **Đã làm**: `get_document` nhận `depth` và `maxNodes`. Ngân sách dùng chung
  cho cả lượt duyệt, tiêu theo thứ tự cây nên kết quả cắt ngắn là tái lập được;
  node bị cắt con báo `childCount`/`childrenOmitted` và cả cây có cờ `truncated`.
- **Còn lại**: `deduplicateStyles`/`globalVars` mới chỉ áp cho `get_document`;
  nên áp cả cho `get_nodes_info` và `get_design_context`.

### 4.4 Hàng đợi ghi & gộp undo — ✅ **phần undo đã làm**
`figma.ui.onmessage` xử lý async, nhiều request ghi có thể xen kẽ. Mỗi handler ghi
tự gọi `figma.commitUndo()` → một pipeline 20 bước tạo 20 mốc undo.

**Đã làm**: `withSingleUndoCheckpoint` nuốt `commitUndo` của từng handler trong
pipeline rồi gọi đúng một lần ở cuối. Rollback nằm trong phạm vi checkpoint nên
pipeline lỗi và tự đảo ngược trả undo stack về nguyên trạng.

**Cũng đã làm**: hàng đợi tuần tự cho request ghi — và nó vá một lỗi mà chính
việc gộp undo tạo ra: một lệnh ghi thường rơi vào lúc pipeline đang chạy sẽ bị
nuốt mốc undo cùng, rồi dính vào bước undo của pipeline. Đọc không xếp hàng.

### 4.5 Progress phủ rộng hơn
Chỉ 3 handler phát `progress_update` (`read-document.ts` ×2, `read-styles.ts` ×1).
Nên phủ: từng bước batch pipeline, export nhiều node, `find_replace_text`,
`scan_text_nodes`.

---

## 5. Chất lượng & kiểm thử

- ✅ Test contract tên handler ↔ toolspec Go: chạy phía plugin, đọc chính golden
  schema của server. Mọi tool server chào phải có handler, và mọi handler phải
  tới được — hoặc là một tool, hoặc nằm trong danh sách đích uỷ nhiệm có ghi chú.
- `mergeHandlers` đã bắt trùng tên lúc load — tốt. Nên thêm test khẳng định mọi
  handler ghi mới đều được cân nhắc cho `CREATE_ACTIONS` trong `batch-pipeline.ts`
  (comment trong file đã cảnh báo đúng rủi ro này, nhưng chưa có test canh).
- Test cho luồng `dynamic-page`: giả lập file nhiều page để bắt lỗi thiếu
  `loadAllPagesAsync`.
- Kiểm tra font: báo cáo font thiếu thay vì để `loadFontAsync` ném lỗi giữa chừng.

---

## 6. Đề xuất lộ trình

**Đợt 1 — rẻ, tác động lớn (S) — ✅ xong**
Auto-layout sizing · cảnh báo lệch version · `set_selection` · tìm kiếm toàn
document · gộp undo cho pipeline · checkpoint version history · auto-copy bật sẵn.

**Đợt 2 — trải nghiệm & an toàn (M) — ✅ xong**
Activity log · read-only + confirm phá huỷ · huỷ request · nút hoàn tác trên UI.
(Pairing token đã loại — xem 3.2.)

**Đợt 3 — độ phủ API (M) — ✅ xong**
Rich text theo range · vector/boolean · component set & properties · mask &
layout grid · ảnh theo URL · đọc lại bytes ảnh gốc.

**Đợt 4 — khác biệt hoá (L) — ✅ xong**
Dev Mode codegen provider (xem 2) · giới hạn kích thước `get_document` ·
FigJam/Slides · handshake danh sách handler · hàng đợi ghi.

---

## 7. Còn lại

Không mục nào trong số này chặn việc gì; gom lại đây để lần sau khỏi phải đọc
lại cả tài liệu.

**Độ phủ API**
- `set_layout_sizing` áp cho nhiều node cùng lúc (1.1).
- `get_document` vẫn chỉ serialize page hiện tại — chưa có `scope: "document"`
  như `search_nodes` (1.6).
- `strokeCap`, `strokeJoin`, `dashPattern`, `strokeMiterLimit` phía ghi (1.7).
- `exportSettings` đặt sẵn trên node (1.7).
- `imageTransform` (crop) và `filters` cho ảnh (1.8).
- `figma.codegen.on("preferenceschange")` cho chọn ngôn ngữ/framework (2).

**Hiệu năng**
- `deduplicateStyles` cho `get_nodes_info` và `get_design_context` (4.3).
- Phủ `progress_update` rộng hơn: từng bước pipeline, export nhiều node,
  `find_replace_text` (4.5).

**UI**
- Nhớ kích thước panel người dùng tự kéo (3.4).
- Theo theme sáng/tối của Figma thay vì hard-code nền tối (3.4).
- Nút "Gửi selection cho AI" — pin một tập context ổn định (3.4).
- i18n VI/EN (3.4).

**Kiểm thử**
- Test cho luồng `dynamic-page` với file nhiều page (5).
- Test canh việc thêm handler ghi mới phải cân nhắc `CREATE_ACTIONS` (5).
- Báo cáo font thiếu thay vì để `loadFontAsync` ném giữa chừng (5).

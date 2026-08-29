// Panel strings, in English and Vietnamese.
//
// Only the panel is translated. Everything that leaves the panel — the refusal
// text the server receives, the activity log a user pastes into a bug report,
// tool names — stays in English: those are read by the MCP client and by
// whoever the bug report goes to, not by the person holding the mouse.

export type Locale = "en" | "vi";

export const LOCALES: readonly Locale[] = ["en", "vi"];

/** The strings the panel shows. Keys are English so a missing one reads sensibly. */
export interface Strings {
  file: string;
  page: string;
  selection: string;
  nodeCount: (count: number) => string;
  autoCopyId: string;
  copy: string;
  copyAll: string;
  copyAllTitle: string;
  copyIdTitle: string;
  pin: string;
  pinned: string;
  pinTitle: string;
  pinnedCount: (count: number) => string;
  pinnedTitle: string;
  clear: string;
  clearPinTitle: string;
  autoCopyBroken: string;
  autoCopyBrokenTitle: string;
  copyFailed: string;
  copyFailedTitle: string;
  allow: string;
  decline: string;
  allowQuestion: string;
  waiting: (count: number) => string;
  activity: string;
  activityTitle: string;
  logEmpty: string;
  copyLogTitle: string;
  undo: string;
  undoTitle: string;
  cancel: string;
  cancelTitle: string;
  guardOff: string;
  guardConfirm: string;
  guardReadonly: string;
  guardTitle: string;
  connected: string;
  connectedVersion: (version: string) => string;
  disconnected: string;
  exposed: string;
  exposedTitle: string;
  serverAddressTitle: string;
  apply: string;
  dismiss: string;
  resizeTitle: string;
  reportBug: string;
  suggestFeature: string;
}

const en: Strings = {
  file: "File",
  page: "Page",
  selection: "Selection",
  nodeCount: (count) => `${count} node(s)`,
  autoCopyId: "Auto-copy ID",
  copy: "Copy",
  copyAll: "Copy All",
  copyAllTitle: "Copy all IDs",
  copyIdTitle: "Copy ID",
  pin: "Pin",
  pinned: "Pinned",
  pinTitle:
    "Hold this selection for your AI tool. It can then ask for get_selection(source: 'pinned') and get the same nodes however the selection moves.",
  pinnedCount: (count) => `📌 Pinned ${count}`,
  pinnedTitle: "Your AI tool reads this with get_selection(source: 'pinned')",
  clear: "Clear",
  clearPinTitle: "Clear the pin",
  autoCopyBroken:
    "⚠️ Auto-copy disabled (browser policy). Connect Go MCP Server for native auto-copy, or click to retry.",
  autoCopyBrokenTitle:
    "Your browser's clipboard security policy blocks automatic copies without user clicks; connect Go MCP Server for native auto-copy, or click to retry",
  copyFailed: "⚠️ Copy failed. Click here to retry.",
  copyFailedTitle:
    "Browsers require you to click here once after a reload to allow clipboard access",
  allow: "Allow",
  decline: "Decline",
  allowQuestion: "Allow",
  waiting: (count) => `+${count} waiting`,
  activity: "Activity",
  activityTitle: "Show the activity log",
  logEmpty: "Nothing yet.",
  copyLogTitle: "Copy the log for a bug report",
  undo: "Undo",
  undoTitle: "Undo the last change in Figma",
  cancel: "Cancel",
  cancelTitle: "Cancel",
  guardOff: "Guard: off",
  guardConfirm: "Guard: confirm",
  guardReadonly: "Guard: read-only",
  guardTitle:
    "Click to change: off runs everything, confirm asks before deletes and bulk rewrites, read-only blocks every change",
  connected: "Connected",
  connectedVersion: (version) => `Connected (v${version})`,
  disconnected: "Disconnected",
  exposed: "Server reachable from the network",
  exposedTitle:
    "The server is listening on an address other than 127.0.0.1, and the connection is not authenticated — anyone who can reach that port can read and edit this file. The confirm guard has been turned on.",
  serverAddressTitle: "Click to configure server address",
  apply: "Apply",
  dismiss: "Cancel",
  resizeTitle: "Drag to resize",
  reportBug: "Report a bug",
  suggestFeature: "Suggest a feature",
};

const vi: Strings = {
  file: "Tệp",
  page: "Trang",
  selection: "Đang chọn",
  nodeCount: (count) => `${count} node`,
  autoCopyId: "Tự chép ID",
  copy: "Chép",
  copyAll: "Chép hết",
  copyAllTitle: "Chép tất cả ID",
  copyIdTitle: "Chép ID",
  pin: "Ghim",
  pinned: "Đã ghim",
  pinTitle:
    "Giữ lại lựa chọn này cho công cụ AI. Nó gọi get_selection(source: 'pinned') là lấy đúng các node này, dù bạn có chọn sang chỗ khác.",
  pinnedCount: (count) => `📌 Đã ghim ${count}`,
  pinnedTitle: "Công cụ AI đọc phần này bằng get_selection(source: 'pinned')",
  clear: "Bỏ",
  clearPinTitle: "Bỏ ghim",
  autoCopyBroken:
    "⚠️ Tự chép bị tắt (chính sách trình duyệt). Kết nối Go MCP Server để chép tự động, hoặc bấm để thử lại.",
  autoCopyBrokenTitle:
    "Chính sách clipboard của trình duyệt chặn việc chép tự động khi không có cú bấm chuột; kết nối Go MCP Server để chép tự động, hoặc bấm để thử lại",
  copyFailed: "⚠️ Chép thất bại. Bấm vào đây để thử lại.",
  copyFailedTitle:
    "Sau khi tải lại, trình duyệt yêu cầu bạn bấm một lần vào đây mới cho phép truy cập clipboard",
  allow: "Cho phép",
  decline: "Từ chối",
  allowQuestion: "Cho phép",
  waiting: (count) => `+${count} đang chờ`,
  activity: "Nhật ký",
  activityTitle: "Hiện nhật ký hoạt động",
  logEmpty: "Chưa có gì.",
  copyLogTitle: "Chép nhật ký để báo lỗi",
  undo: "Hoàn tác",
  undoTitle: "Hoàn tác thay đổi cuối trong Figma",
  cancel: "Huỷ",
  cancelTitle: "Huỷ",
  guardOff: "Chốt: tắt",
  guardConfirm: "Chốt: hỏi",
  guardReadonly: "Chốt: chỉ đọc",
  guardTitle:
    "Bấm để đổi: tắt là chạy hết, hỏi là xác nhận trước khi xoá hoặc sửa hàng loạt, chỉ đọc là chặn mọi thay đổi",
  connected: "Đã kết nối",
  connectedVersion: (version) => `Đã kết nối (v${version})`,
  disconnected: "Chưa kết nối",
  exposed: "Máy chủ mở ra mạng ngoài",
  exposedTitle:
    "Máy chủ đang lắng nghe ở địa chỉ khác 127.0.0.1, và kết nối không có xác thực — ai vào được cổng đó cũng đọc và sửa được tệp này. Chốt \"hỏi\" đã được bật.",
  serverAddressTitle: "Bấm để đổi địa chỉ máy chủ",
  apply: "Áp dụng",
  dismiss: "Huỷ",
  resizeTitle: "Kéo để đổi kích thước",
  reportBug: "Báo lỗi",
  suggestFeature: "Đề xuất tính năng",
};

const TABLES: Record<Locale, Strings> = { en, vi };

/**
 * Pick a locale from the browser's language tags.
 *
 * The panel runs in an iframe Figma owns, and Figma exposes no locale of its
 * own, so the browser's list is the only signal there is. Anything that is not
 * Vietnamese falls back to English rather than to a half-translated panel.
 */
export function pickLocale(languages: readonly string[] | undefined): Locale {
  for (const tag of languages ?? []) {
    if (typeof tag === "string" && tag.toLowerCase().startsWith("vi")) return "vi";
  }
  return "en";
}

export function strings(locale: Locale): Strings {
  return TABLES[locale] ?? en;
}

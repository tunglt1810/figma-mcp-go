// UI preferences persisted through the plugin core (figma.clientStorage), since
// localStorage is unavailable inside Figma's data: URL sandbox.
//
// They ride in the same stored object as the server address rather than a
// second key: the UI blocks its first connect on that read, and a second round
// trip would either delay the connect or let the checkbox flip under the user
// a moment after the panel appears.

export const DEFAULT_HOST = "127.0.0.1";
export const DEFAULT_PORT = "1994";

/**
 * Panel size limits, matching the clamp the plugin core applies to a resize
 * message. Figma windows are not resizable by themselves — the panel draws its
 * own grip — so these bounds are the only thing between a slip of the mouse and
 * a window too small to find again.
 */
export const MIN_PANEL_WIDTH = 240;
export const MAX_PANEL_WIDTH = 800;
export const MIN_PANEL_HEIGHT = 160;
export const MAX_PANEL_HEIGHT = 900;

/** The panel's size with the activity log closed, before the user drags it. */
export const DEFAULT_PANEL_WIDTH = 320;
export const DEFAULT_PANEL_HEIGHT = 230;

/** How much taller the panel gets when the activity log is open. */
export const LOG_EXTRA_HEIGHT = 230;

/**
 * How much the panel gets in the way of a write.
 *
 * "off" is the default and preserves the behaviour every existing user has.
 * Turning either guard on is an opt-in: a prompt in front of work the user
 * asked for is only welcome when they asked for the prompt too.
 */
export type GuardMode = "off" | "confirm" | "readonly";

export const GUARD_MODES: readonly GuardMode[] = ["off", "confirm", "readonly"];

export interface Prefs {
  host: string;
  port: string;
  autoCopy: boolean;
  guardMode: GuardMode;
  showLog: boolean;
  /** The panel's size with the log closed; the log's extra height is added on top. */
  panelWidth: number;
  panelHeight: number;
}

/** Clamp a host string to something connectable, falling back to the default. */
export function sanitizeHost(host: unknown): string {
  const trimmed = typeof host === "string" ? host.trim() : "";
  return trimmed || DEFAULT_HOST;
}

/** Clamp a port to a valid TCP port, falling back to the default. */
export function sanitizePort(port: unknown): string {
  const parsed = parseInt(String(port ?? ""), 10);
  return parsed > 0 && parsed <= 65535 ? String(parsed) : DEFAULT_PORT;
}

/**
 * Read stored preferences, filling in defaults for anything missing.
 *
 * `autoCopy` defaults to ON. Copying a node id is the one thing every session
 * starts with, and with the server connected the copy goes through the native
 * OS clipboard, so it costs the user nothing. Absent means "never chose", which
 * is what upgrading users look like — they get the new default; anyone who
 * turns it off has `false` stored and keeps it off.
 */
export function normalizeStoredPrefs(stored: unknown): Prefs {
  const config = (stored ?? {}) as Record<string, unknown>;
  return {
    host: sanitizeHost(config.host),
    port: sanitizePort(config.port),
    autoCopy: config.autoCopy === undefined ? true : config.autoCopy !== false,
    guardMode: sanitizeGuardMode(config.guardMode),
    showLog: config.showLog === true,
    panelWidth: sanitizePanelWidth(config.panelWidth),
    panelHeight: sanitizePanelHeight(config.panelHeight),
  };
}

/** Clamp a stored size, falling back to the default rather than to a window the
 * user cannot see or cannot fit on screen. */
const sanitizeSize = (value: unknown, min: number, max: number, fallback: number): number => {
  const parsed = Math.round(Number(value));
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(Math.max(parsed, min), max);
};

export const sanitizePanelWidth = (value: unknown): number =>
  sanitizeSize(value, MIN_PANEL_WIDTH, MAX_PANEL_WIDTH, DEFAULT_PANEL_WIDTH);

export const sanitizePanelHeight = (value: unknown): number =>
  sanitizeSize(value, MIN_PANEL_HEIGHT, MAX_PANEL_HEIGHT, DEFAULT_PANEL_HEIGHT);

/** An unrecognised stored mode falls back to off rather than to a guard the
 * user never chose and cannot see the reason for. */
export function sanitizeGuardMode(mode: unknown): GuardMode {
  return GUARD_MODES.includes(mode as GuardMode) ? (mode as GuardMode) : "off";
}

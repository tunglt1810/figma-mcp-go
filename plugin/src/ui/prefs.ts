// UI preferences saved through the plugin core (figma.clientStorage), because
// localStorage is not available inside Figma's data: URL sandbox.
//
// They are stored in the same object as the server address, not under a
// second key. The UI waits for that read before its first connect. A second
// round trip would either delay the connect or make the checkbox flip a
// moment after the panel appears.

export const DEFAULT_HOST = "127.0.0.1";
export const DEFAULT_PORT = "1994";

/**
 * Panel size limits, matching the clamp the plugin core applies to a resize
 * message. Figma windows cannot be resized by themselves, so the panel draws
 * its own grip. These limits are all that stops a slip of the mouse from
 * leaving a window too small to find again.
 */
export const MIN_PANEL_WIDTH = 240;
export const MAX_PANEL_WIDTH = 800;
export const MIN_PANEL_HEIGHT = 160;
export const MAX_PANEL_HEIGHT = 900;

/** The panel's size with the activity log closed, before the user resizes it. */
export const DEFAULT_PANEL_WIDTH = 320;
export const DEFAULT_PANEL_HEIGHT = 230;

/** How much taller the panel gets when the activity log is open. */
export const LOG_EXTRA_HEIGHT = 230;

/**
 * How much the panel interrupts a write.
 *
 * "off" is the default and keeps the behaviour every existing user has.
 * Turning either guard on is opt-in: a prompt before work the user asked for
 * is only welcome when they also asked for the prompt.
 */
export type GuardMode = "off" | "confirm" | "readonly";

export const GUARD_MODES: readonly GuardMode[] = ["off", "confirm", "readonly"];

export interface Prefs {
  host: string;
  port: string;
  autoCopy: boolean;
  guardMode: GuardMode;
  showLog: boolean;
  /** The panel's size with the log closed. The log's extra height is added on top. */
  panelWidth: number;
  panelHeight: number;
}

/** Clean up a host string so it can be connected to, or fall back to the default. */
export function sanitizeHost(host: unknown): string {
  const trimmed = typeof host === "string" ? host.trim() : "";
  return trimmed || DEFAULT_HOST;
}

/** Clamp a port to a valid TCP port, or fall back to the default. */
export function sanitizePort(port: unknown): string {
  const parsed = parseInt(String(port ?? ""), 10);
  return parsed > 0 && parsed <= 65535 ? String(parsed) : DEFAULT_PORT;
}

/**
 * Read stored preferences, and fill in defaults for anything missing.
 *
 * `autoCopy` defaults to ON. Copying a node id is how every session starts,
 * and with the server connected the copy goes through the native OS
 * clipboard, so it costs the user nothing. Absent means "never chose", which
 * is the case for upgrading users, so they get the new default. Anyone who
 * turns it off has `false` stored and it stays off.
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

/** Clamp a stored size. Fall back to the default, not to a window the user
 * cannot see or that does not fit on screen. */
const sanitizeSize = (value: unknown, min: number, max: number, fallback: number): number => {
  const parsed = Math.round(Number(value));
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(Math.max(parsed, min), max);
};

export const sanitizePanelWidth = (value: unknown): number =>
  sanitizeSize(value, MIN_PANEL_WIDTH, MAX_PANEL_WIDTH, DEFAULT_PANEL_WIDTH);

export const sanitizePanelHeight = (value: unknown): number =>
  sanitizeSize(value, MIN_PANEL_HEIGHT, MAX_PANEL_HEIGHT, DEFAULT_PANEL_HEIGHT);

/** An unknown stored mode falls back to off, not to a guard the user never
 * chose and would not know the reason for. */
export function sanitizeGuardMode(mode: unknown): GuardMode {
  return GUARD_MODES.includes(mode as GuardMode) ? (mode as GuardMode) : "off";
}

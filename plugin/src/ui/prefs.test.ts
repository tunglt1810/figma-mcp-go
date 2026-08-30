import { describe, expect, test } from "bun:test";
import {
  DEFAULT_PANEL_HEIGHT,
  DEFAULT_PANEL_WIDTH,
  MAX_PANEL_HEIGHT,
  MAX_PANEL_WIDTH,
  MIN_PANEL_HEIGHT,
  MIN_PANEL_WIDTH,
  normalizeStoredPrefs,
  sanitizeGuardMode,
  sanitizeHost,
  sanitizePanelHeight,
  sanitizePanelWidth,
  sanitizePort,
} from "./prefs";

describe("sanitizeHost", () => {
  test("keeps a real host and trims it", () => {
    expect(sanitizeHost("  192.168.1.5 ")).toBe("192.168.1.5");
  });

  test("falls back for empty, blank, and non-string input", () => {
    expect(sanitizeHost("")).toBe("127.0.0.1");
    expect(sanitizeHost("   ")).toBe("127.0.0.1");
    expect(sanitizeHost(undefined)).toBe("127.0.0.1");
    expect(sanitizeHost(42)).toBe("127.0.0.1");
  });
});

describe("sanitizePort", () => {
  test("keeps a valid port, as a string", () => {
    expect(sanitizePort("8080")).toBe("8080");
    expect(sanitizePort(8080)).toBe("8080");
  });

  test("falls back for out-of-range and unparseable ports", () => {
    expect(sanitizePort("0")).toBe("1994");
    expect(sanitizePort("70000")).toBe("1994");
    expect(sanitizePort("-1")).toBe("1994");
    expect(sanitizePort("abc")).toBe("1994");
    expect(sanitizePort(undefined)).toBe("1994");
  });
});

describe("normalizeStoredPrefs", () => {
  test("auto-copy is on for a user who has never stored preferences", () => {
    expect(normalizeStoredPrefs(undefined).autoCopy).toBe(true);
    expect(normalizeStoredPrefs(null).autoCopy).toBe(true);
    expect(normalizeStoredPrefs({}).autoCopy).toBe(true);
  });

  test("an upgrading user with only an address stored gets the new defaults", () => {
    expect(normalizeStoredPrefs({ host: "10.0.0.2", port: "2000" })).toEqual({
      host: "10.0.0.2",
      port: "2000",
      autoCopy: true,
      guardMode: "off",
      showLog: false,
      panelWidth: 320,
      panelHeight: 230,
    });
  });

  test("an explicit opt-out is honoured", () => {
    expect(normalizeStoredPrefs({ autoCopy: false }).autoCopy).toBe(false);
  });

  test("an explicit opt-in is honoured", () => {
    expect(normalizeStoredPrefs({ autoCopy: true }).autoCopy).toBe(true);
  });

  test("a stored address is still sanitized", () => {
    expect(normalizeStoredPrefs({ host: "  ", port: "99999" })).toEqual({
      host: "127.0.0.1",
      port: "1994",
      autoCopy: true,
      guardMode: "off",
      showLog: false,
      panelWidth: 320,
      panelHeight: 230,
    });
  });

  test("guards stay off unless the user turned one on", () => {
    expect(normalizeStoredPrefs({}).guardMode).toBe("off");
    expect(normalizeStoredPrefs({ guardMode: "confirm" }).guardMode).toBe("confirm");
    expect(normalizeStoredPrefs({ guardMode: "readonly" }).guardMode).toBe("readonly");
  });

  test("the log panel stays closed unless the user opened it", () => {
    expect(normalizeStoredPrefs({}).showLog).toBe(false);
    expect(normalizeStoredPrefs({ showLog: true }).showLog).toBe(true);
  });
});

describe("sanitizeGuardMode", () => {
  test("keeps a mode the UI knows", () => {
    expect(sanitizeGuardMode("readonly")).toBe("readonly");
  });

  // Falling back to a guard the user never chose would block writes with no
  // visible reason.
  test("falls back to off for anything else", () => {
    expect(sanitizeGuardMode("paranoid")).toBe("off");
    expect(sanitizeGuardMode(undefined)).toBe("off");
    expect(sanitizeGuardMode(7)).toBe("off");
  });
});

describe("panel size", () => {
  test("falls back to the default when nothing is stored", () => {
    expect(normalizeStoredPrefs({}).panelWidth).toBe(DEFAULT_PANEL_WIDTH);
    expect(normalizeStoredPrefs({}).panelHeight).toBe(DEFAULT_PANEL_HEIGHT);
  });

  test("remembers a size the user dragged", () => {
    const prefs = normalizeStoredPrefs({ panelWidth: 500, panelHeight: 400 });
    expect(prefs.panelWidth).toBe(500);
    expect(prefs.panelHeight).toBe(400);
  });

  // A slip of the mouse must not leave a window too small to find again, or one
  // larger than the screen it is on.
  test("clamps a stored size to something usable", () => {
    expect(sanitizePanelWidth(10)).toBe(MIN_PANEL_WIDTH);
    expect(sanitizePanelWidth(5000)).toBe(MAX_PANEL_WIDTH);
    expect(sanitizePanelHeight(0)).toBe(MIN_PANEL_HEIGHT);
    expect(sanitizePanelHeight(5000)).toBe(MAX_PANEL_HEIGHT);
  });

  test("falls back rather than storing NaN", () => {
    expect(sanitizePanelWidth("wide")).toBe(DEFAULT_PANEL_WIDTH);
    expect(sanitizePanelHeight(undefined)).toBe(DEFAULT_PANEL_HEIGHT);
  });
});

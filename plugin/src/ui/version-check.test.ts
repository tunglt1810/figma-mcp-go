import { describe, expect, test } from "bun:test";
import { compareVersions, parseVersion, versionWarning, versionWarningSummary } from "./version-check";

describe("parseVersion", () => {
  test("reads major and minor", () => {
    expect(parseVersion("0.3.0")).toEqual({ major: 0, minor: 3 });
  });

  test("tolerates a v prefix, surrounding space, and a prerelease suffix", () => {
    expect(parseVersion(" v1.4.2-beta.1 ")).toEqual({ major: 1, minor: 4 });
  });

  test("returns null for anything without a major.minor", () => {
    expect(parseVersion("dev")).toBeNull();
    expect(parseVersion("1")).toBeNull();
    expect(parseVersion("")).toBeNull();
    expect(parseVersion(null)).toBeNull();
    expect(parseVersion(undefined)).toBeNull();
  });
});

describe("compareVersions", () => {
  test("equal major.minor is ok", () => {
    expect(compareVersions("0.3.0", "0.3.0")).toBe("ok");
  });

  test("patch drift is expected, not a mismatch", () => {
    expect(compareVersions("0.3.0", "0.3.9")).toBe("ok");
    expect(compareVersions("0.3.9", "0.3.0")).toBe("ok");
  });

  test("names which side is behind on a minor gap", () => {
    expect(compareVersions("0.3.0", "0.4.0")).toBe("plugin-old");
    expect(compareVersions("0.4.0", "0.3.0")).toBe("server-old");
  });

  test("names which side is behind on a major gap", () => {
    expect(compareVersions("0.9.0", "1.0.0")).toBe("plugin-old");
    expect(compareVersions("2.0.0", "1.9.0")).toBe("server-old");
  });

  test("a major gap outranks the minor numbers", () => {
    // 1.9 vs 2.0: the plugin's higher minor must not read as "server-old".
    expect(compareVersions("1.9.0", "2.0.0")).toBe("plugin-old");
  });

  test("an unreadable version on either side stays silent", () => {
    expect(compareVersions("dev", "0.3.0")).toBe("unknown");
    expect(compareVersions("0.3.0", "")).toBe("unknown");
    expect(compareVersions("0.3.0", undefined)).toBe("unknown");
  });
});

describe("versionWarning", () => {
  test("says nothing when the versions line up or cannot be read", () => {
    expect(versionWarning("0.3.0", "0.3.4")).toBeNull();
    expect(versionWarning("dev", "0.3.0")).toBeNull();
  });

  test("tells the user to re-import the plugin when it is behind", () => {
    const warning = versionWarning("0.3.0", "0.4.0");
    expect(warning).toContain("Plugin v0.3.0");
    expect(warning).toContain("Re-import");
  });

  test("tells the user to update the server when it is behind", () => {
    const warning = versionWarning("0.4.0", "0.3.0");
    expect(warning).toContain("Server v0.3.0");
    expect(warning).toContain("Update the MCP server");
  });
});

describe("versionWarningSummary", () => {
  test("says nothing when the versions line up or cannot be read", () => {
    expect(versionWarningSummary("0.3.0", "0.3.4")).toBeNull();
    expect(versionWarningSummary("dev", "0.3.0")).toBeNull();
  });

  test("fits both versions and the remedy on one short line", () => {
    const summary = versionWarningSummary("0.3.0", "0.4.0")!;
    expect(summary).toContain("v0.3.0");
    expect(summary).toContain("v0.4.0");
    expect(summary).toContain("re-import");
    // The panel is 320px wide; a headline much longer than this wraps past the
    // two lines the banner has room for.
    expect(summary.length).toBeLessThanOrEqual(70);
  });

  test("points at the server when it is the one behind", () => {
    const summary = versionWarningSummary("0.4.0", "0.3.0")!;
    expect(summary).toContain("update the server");
    expect(summary.length).toBeLessThanOrEqual(70);
  });
});

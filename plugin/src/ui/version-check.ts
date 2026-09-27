// The plugin and server share one version string (npm/package.json feeds both
// the plugin build and server.json), so the two can be compared directly.
// A patch difference is expected and harmless. The plugin is installed by hand
// from a release zip, while the server updates itself through `npx @latest`,
// so almost every user runs a patch behind at some point. Only a major or
// minor gap means the set of tools changed, and that is worth a warning.

export type VersionStatus = "ok" | "unknown" | "plugin-old" | "server-old";

interface Parsed {
  major: number;
  minor: number;
}

/** Parse the leading `major.minor` of a semver string. Returns null if it cannot. */
export function parseVersion(version: string | null | undefined): Parsed | null {
  if (!version) return null;
  const match = /^v?(\d+)\.(\d+)/.exec(version.trim());
  if (!match) return null;
  return { major: Number(match[1]), minor: Number(match[2]) };
}

/**
 * Compare the running plugin with the connected server.
 *
 * "unknown" means a version we cannot read at all: a dev build, or a server
 * too old to send one. Guessing here would show a warning to every
 * contributor who runs from source, so it stays silent.
 */
export function compareVersions(
  pluginVersion: string | null | undefined,
  serverVersion: string | null | undefined,
): VersionStatus {
  const plugin = parseVersion(pluginVersion);
  const server = parseVersion(serverVersion);
  if (!plugin || !server) return "unknown";
  if (plugin.major !== server.major) {
    return plugin.major < server.major ? "plugin-old" : "server-old";
  }
  if (plugin.minor !== server.minor) {
    return plugin.minor < server.minor ? "plugin-old" : "server-old";
  }
  return "ok";
}

/**
 * One short line for the panel, or null when there is nothing to say.
 *
 * The panel is 320px wide and 230px tall with no spare room. So the banner
 * shows the headline, and `versionWarning` shows the fix on hover.
 */
export function versionWarningSummary(
  pluginVersion: string | null | undefined,
  serverVersion: string | null | undefined,
): string | null {
  switch (compareVersions(pluginVersion, serverVersion)) {
    case "plugin-old":
      return `Plugin v${pluginVersion} is behind server v${serverVersion} — re-import the plugin`;
    case "server-old":
      return `Server v${serverVersion} is behind plugin v${pluginVersion} — update the server`;
    default:
      return null;
  }
}

/** The full explanation, for the banner's tooltip. Null when nothing is wrong. */
export function versionWarning(
  pluginVersion: string | null | undefined,
  serverVersion: string | null | undefined,
): string | null {
  switch (compareVersions(pluginVersion, serverVersion)) {
    case "plugin-old":
      return `Plugin v${pluginVersion} is older than server v${serverVersion}. Re-import the plugin from the latest release — newer tools will fail with "Unknown request type".`;
    case "server-old":
      return `Server v${serverVersion} is older than plugin v${pluginVersion}. Update the MCP server (npx @tunglt1810/figma-mcp-go@latest) — it will not expose the plugin's newer tools.`;
    default:
      return null;
  }
}

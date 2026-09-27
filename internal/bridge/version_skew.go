package bridge

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The plugin and the server share one version string, but they update in
// different ways. The server updates itself on every `npx @latest`. The plugin
// is imported by hand from a release zip. So they often differ by a patch, and
// that means nothing. A major or minor gap is different. That is where the set
// of tools changed, and where an older plugin answers a new tool with "Unknown
// request type".

var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)`)

// VersionSkew names which side is behind, if either.
type VersionSkew int

const (
	// SkewUnknown means at least one version could not be read: a dev build,
	// or a plugin too old to announce itself. Guessing here would warn every
	// contributor who runs from source, so it stays silent.
	SkewUnknown VersionSkew = iota
	SkewNone
	SkewPluginOld
	SkewServerOld
)

// parseMajorMinor reads the leading major.minor of a semver string.
func parseMajorMinor(version string) (major, minor int, ok bool) {
	match := versionPattern.FindStringSubmatch(version)
	if match == nil {
		return 0, 0, false
	}
	// Both groups matched digits, so neither Atoi can fail.
	major, _ = strconv.Atoi(match[1])
	minor, _ = strconv.Atoi(match[2])
	return major, minor, true
}

// CompareVersions reports how the plugin's version relates to the server's,
// ignoring the patch component.
func CompareVersions(pluginVersion, serverVersion string) VersionSkew {
	pluginMajor, pluginMinor, pluginOK := parseMajorMinor(pluginVersion)
	serverMajor, serverMinor, serverOK := parseMajorMinor(serverVersion)
	if !pluginOK || !serverOK {
		return SkewUnknown
	}
	if pluginMajor != serverMajor {
		if pluginMajor < serverMajor {
			return SkewPluginOld
		}
		return SkewServerOld
	}
	if pluginMinor != serverMinor {
		if pluginMinor < serverMinor {
			return SkewPluginOld
		}
		return SkewServerOld
	}
	return SkewNone
}

// VersionSkewMessage explains a mismatch, or returns "" when there is nothing
// worth telling the user.
func VersionSkewMessage(pluginVersion, serverVersion string) string {
	switch CompareVersions(pluginVersion, serverVersion) {
	case SkewPluginOld:
		return fmt.Sprintf(
			"the Figma plugin (v%s) is older than this server (v%s) — re-import the plugin from the latest release, or newer tools will fail with \"Unknown request type\"",
			pluginVersion, serverVersion,
		)
	case SkewServerOld:
		return fmt.Sprintf(
			"this server (v%s) is older than the Figma plugin (v%s) — update it with `npx -y @tunglt1810/figma-mcp-go@latest`, or the plugin's newer tools stay hidden",
			serverVersion, pluginVersion,
		)
	default:
		return ""
	}
}

// setPluginInfo records what the connected plugin announced about itself.
func (b *Bridge) setPluginInfo(version string, handlers []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pluginVersion = version
	if len(handlers) == 0 {
		b.pluginHandlers = nil
		return
	}
	set := make(map[string]bool, len(handlers))
	for _, name := range handlers {
		set[name] = true
	}
	b.pluginHandlers = set
}

// unsupportedTool builds the message for a tool the plugin lacks. The same
// message is used wherever that is found: before the call, from the announced
// handler list, or after it, from the plugin's own reply.
func unsupportedTool(version, tool string) string {
	where := "the Figma plugin"
	if version != "" {
		where = fmt.Sprintf("the Figma plugin (v%s)", version)
	}
	return fmt.Sprintf(
		"%s does not support %s — re-import the plugin from the latest release to use it",
		where, tool,
	)
}

// checkPluginSupports reports why a tool cannot run, or "" when it can.
//
// A plugin that announced nothing is trusted. It is older than the
// announcement, and refusing all its calls would break a working setup. A
// plugin that did announce is taken at its word. A tool it lacks fails here
// with a fix, instead of reaching the plugin and coming back as "Unknown
// request type".
func (b *Bridge) checkPluginSupports(tool string) string {
	b.mu.RLock()
	handlers := b.pluginHandlers
	version := b.pluginVersion
	b.mu.RUnlock()

	if len(handlers) == 0 || handlers[tool] {
		return ""
	}
	return unsupportedTool(version, tool)
}

// explainUnknownRequest adds a fix to the plugin's bare "Unknown request type".
//
// This is the cost of trusting old plugins in checkPluginSupports. A plugin
// too old to announce its handlers gets every call, so a call for a tool it
// does not know comes back with no explanation. The caller is usually a model.
// It reads that as a temporary failure and retries a tool this plugin will never
// have. Only that one error is rewritten. A handler's own error is already the
// useful answer.
func (b *Bridge) explainUnknownRequest(tool, pluginErr string) string {
	if !strings.HasPrefix(pluginErr, "Unknown request type") {
		return pluginErr
	}
	return unsupportedTool(b.PluginVersion(), tool)
}

// PluginVersion returns the version the connected plugin announced. It returns
// "" when no plugin has connected, or when the plugin is too old to announce.

func (b *Bridge) PluginVersion() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.pluginVersion
}

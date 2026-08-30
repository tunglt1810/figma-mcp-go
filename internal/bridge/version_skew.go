package bridge

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The plugin and the server ship from one version string, but they update
// through different channels: the server refreshes itself on every `npx @latest`
// while the plugin is imported by hand from a release zip. So the two drift by a
// patch routinely and that means nothing. A major or minor gap is different —
// that is where the tool surface moved, and where the user gets "Unknown request
// type" from a plugin that predates the tool the server just called.

var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)`)

// VersionSkew names which side is behind, if either.
type VersionSkew int

const (
	// SkewUnknown means at least one version could not be read — a dev build,
	// or a plugin old enough not to announce itself. Guessing a direction there
	// would warn every contributor running from source, so it stays silent.
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

// unsupportedTool words the one answer a tool the plugin lacks should get,
// wherever that is discovered — before the call from the announced handler
// list, or after it from the plugin's own reply.
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
// A plugin that announced nothing gets the benefit of the doubt: it predates
// the announcement, and refusing its every call would break a setup that works.
// One that did announce is taken at its word, so a tool it lacks fails here
// with a remedy instead of reaching it and coming back "Unknown request type".
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

// explainUnknownRequest gives the plugin's bare "Unknown request type" the
// remedy it lacks.
//
// This is what the fail-open path in checkPluginSupports costs: a plugin too
// old to announce its handlers is not second-guessed, so a call for a tool it
// has never heard of reaches it and comes back named but unexplained. The
// caller is usually a model, which reads that as a transient failure and
// retries a tool this plugin will never have. Only that one error is rewritten
// — a handler's own failure is the useful answer.
func (b *Bridge) explainUnknownRequest(tool, pluginErr string) string {
	if !strings.HasPrefix(pluginErr, "Unknown request type") {
		return pluginErr
	}
	return unsupportedTool(b.PluginVersion(), tool)
}

// PluginVersion returns the version the connected plugin announced, or "" when
// none has connected or it is too old to announce one.
func (b *Bridge) PluginVersion() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.pluginVersion
}

package tools

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The Go table and the plugin's switch statements are two halves of one
// contract, and nothing linked them. A tool declared here with no handler
// there compiled, shipped, and failed on the user's machine with "Unknown
// request type". This test checks that the halves match.

// pluginOnlyInGo are the tools the plugin does not handle, on purpose.
var pluginOnlyInGo = map[string]string{
	"export_screenshots": "never reaches the plugin — the Go handler calls get_screenshot once per item and writes the files",
}

// A handler claims a tool by being the map entry under its name. The pipeline
// is the exception: it takes the whole request before dispatch, so it is
// compared against request.type instead.
func pluginClaims(sources, tool string) bool {
	return strings.Contains(sources, `"`+tool+`": async (request)`) ||
		strings.Contains(sources, `request.type !== '`+tool+`'`)
}

func TestEveryToolHasAPluginHandler(t *testing.T) {
	sources := readPluginSources(t)

	for name := range specRegistry {
		if reason, expected := pluginOnlyInGo[name]; expected {
			if pluginClaims(sources, name) {
				t.Errorf("%s has a plugin handler after all — drop it from pluginOnlyInGo (%s)", name, reason)
			}
			continue
		}
		if !pluginClaims(sources, name) {
			t.Errorf("tool %q is declared in the table but no plugin handler claims it", name)
		}
	}
}

func readPluginSources(t *testing.T) string {
	t.Helper()

	dir := filepath.Join("..", "..", "plugin", "src")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("plugin sources not available: %v", err)
	}

	var b strings.Builder
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".test.ts") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		b.Write(content)
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		t.Fatal("no plugin sources found")
	}
	return b.String()
}

// The plugin decides at run time which step arguments are $variables, and the
// server skips those arguments when it checks a step. If the two rules differ,
// the server checks a value the plugin replaces, or skips a value the plugin
// passes on as written.
func TestPipelineVariable_MatchesThePlugin(t *testing.T) {
	sources := readPluginSources(t)

	m := regexp.MustCompile(`const VARIABLE_REFERENCE = /(.+)/;`).FindStringSubmatch(sources)
	if m == nil {
		t.Fatal("VARIABLE_REFERENCE not found in the plugin sources")
	}
	if m[1] != pipelineVariable.String() {
		t.Errorf("plugin pattern %s, server pattern %s", m[1], pipelineVariable)
	}

	// The plugin reads a string that starts with $$ as an escaped literal,
	// before it looks for a reference. The server must not see a reference there.
	if !strings.Contains(sources, "startsWith('$$')") {
		t.Error("the plugin no longer escapes with $$. Update hasPipelineVariable to match")
	}
	if hasPipelineVariable("$$name") {
		t.Error("the server reads $$name as a reference. The plugin reads it as the literal $name")
	}
}

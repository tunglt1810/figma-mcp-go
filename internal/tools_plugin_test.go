package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Go table and the plugin's switch statements are two halves of one
// contract, and nothing connected them: a tool declared here with no handler
// there compiled, shipped, and failed at the user's machine with "Unknown
// request type". This checks the halves line up.

// pluginOnlyInGo are the tools the plugin deliberately does not handle.
var pluginOnlyInGo = map[string]string{
	"save_screenshots": "never reaches the plugin — the Go handler calls get_screenshot once per item and writes the files",
}

// A handler claims a tool either with a switch case or, for the tools that are
// routed before the switch, with a comparison against request.type.
func pluginClaims(sources, tool string) bool {
	return strings.Contains(sources, `case "`+tool+`"`) ||
		strings.Contains(sources, `request.type === "`+tool+`"`) ||
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

	dir := filepath.Join("..", "plugin", "src")
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

package prompts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The prompts tell the model which tools to call. A prompt naming a tool that
// no longer exists is worse than a stale comment: it walks the model straight
// into "Unknown request type". Renaming a tool has to rename it here too.
var promptToolCall = regexp.MustCompile(`\b([a-z][a-z0-9_]{3,})\s*\(`)

func TestPromptsOnlyNameRealTools(t *testing.T) {
	dir := "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read prompts: %v", err)
	}

	// Words that look like a call but are not tool names.
	ignore := map[string]bool{
		"func": true, "string": true, "sprintf": true, "printf": true,
		"append": true, "return": true, "make": true, "range": true,
		"prompt": true, "result": true, "text": true, "content": true,
	}

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, m := range promptToolCall.FindAllStringSubmatch(string(body), -1) {
			word := m[1]
			if ignore[word] || !strings.Contains(word, "_") {
				continue
			}
			// Only flag words that look like tool names: a retired tool is one
			// that used to be in the registry, so require the shape.
			if ok := retiredTools[word]; ok {
				t.Errorf("%s: prompt names %q, which is no longer a tool", e.Name(), word)
			}
		}
	}
}

// retiredTools are names that were tools once. Keeping the list means a prompt
// still using one is caught by name rather than by a user hitting it.
var retiredTools = map[string]bool{
	"set_visible": true, "lock_nodes": true, "unlock_nodes": true,
	"set_opacity": true, "rotate_nodes": true, "set_blend_mode": true,
	"set_constraints": true, "reorder_nodes": true,
	"create_paint_style": true, "create_text_style": true,
	"create_effect_style": true, "create_grid_style": true,
	"add_page": true, "delete_page": true, "rename_page": true, "navigate_to_page": true,
	"set_fills": true, "set_gradient_fills": true, "set_strokes": true,
}

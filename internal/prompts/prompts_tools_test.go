package prompts

import (
	"encoding/json/v2"
	"os"
	"regexp"
	"testing"
)

// The prompts tell the model which tools to call. A prompt that names a tool
// the server does not have leads the model straight into an error.
var promptToolCall = regexp.MustCompile(`\b([a-z][a-z0-9]*(?:_[a-z0-9]+)+)\s*\(`)

func TestPromptsOnlyNameRealTools(t *testing.T) {
	raw, err := os.ReadFile("../tools/testdata/tools_schema.json")
	if err != nil {
		t.Fatalf("read the tool schema: %v", err)
	}
	var tools map[string]any
	if err := json.Unmarshal(raw, &tools); err != nil {
		t.Fatalf("parse the tool schema: %v", err)
	}

	calls := 0
	for _, p := range all {
		for _, m := range promptToolCall.FindAllStringSubmatch(p.body, -1) {
			calls++
			if _, ok := tools[m[1]]; !ok {
				t.Errorf("%s: prompt calls %q, which is not a tool", p.name, m[1])
			}
		}
	}
	if calls == 0 {
		t.Fatal("found no tool call in any prompt: the pattern no longer matches")
	}
}

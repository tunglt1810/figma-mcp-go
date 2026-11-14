package tools

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// updateGolden rewrites the snapshot instead of comparing against it:
//
//	go test ./internal/tools/ -run TestToolSchemas_Golden -update
var updateGolden = flag.Bool("update", false, "rewrite the tools/list golden snapshot")

const goldenPath = "testdata/tools_schema.json"

// toolsListJSON returns the tools/list result as indented JSON. Deterministic
// sorts map keys, so the output is the same on every run.
func toolsListJSON(t *testing.T) []byte {
	t.Helper()
	s, _ := newTestServer(t)
	raw := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	if raw == nil {
		t.Fatal("HandleMessage returned nil for tools/list")
	}

	var envelope struct {
		Result struct {
			Tools []jsontext.Value `json:"tools"`
		} `json:"result"`
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal tools/list: %v", err)
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}

	// Key by name, so reordering the registration calls does not cause a diff.
	byName := map[string]jsontext.Value{}
	for _, tool := range envelope.Result.Tools {
		var nm struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(tool, &nm); err != nil {
			t.Fatalf("unmarshal tool: %v", err)
		}
		byName[nm.Name] = tool
	}

	out, err := json.Marshal(byName, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		t.Fatalf("marshal golden: %v", err)
	}
	return append(out, '\n')
}

// TestToolSchemas_Golden pins the exact JSON schema advertised for every tool.
//
// This is the safety net for changing how tools are declared. The schema is
// the contract every MCP client sees, so a change that alters it silently is a
// breaking change. Any intended change shows up as a diff to review.
func TestToolSchemas_Golden(t *testing.T) {
	got := toolsListJSON(t)

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("wrote %s (%d bytes)", goldenPath, len(got))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create it): %v", err)
	}

	if string(got) != string(want) {
		t.Errorf("tool schemas changed. If this is intended, review the diff and re-run with -update.\n"+
			"golden = %d bytes, current = %d bytes", len(want), len(got))
		if err := os.WriteFile(goldenPath+".actual", got, 0o644); err == nil {
			t.Logf("current output written to %s.actual for diffing", goldenPath)
		}
	}
}

const readOnlyCallsPath = "testdata/read_only_calls.json"

// TestReadOnlyCalls_Golden pins which calls read-only mode runs: true for a
// whole tool, and for a tool that is only partly harmless, each of its actions
// with true or false.
// The plugin panel has its own read-only guard, and its tests read this file,
// so the two modes cannot drift apart.
func TestReadOnlyCalls_Golden(t *testing.T) {
	calls := map[string]any{}
	for _, spec := range readOnlyModeSpecs() {
		if len(spec.HarmlessActions) > 0 {
			actions := map[string]bool{}
			for _, p := range spec.Params {
				if p.Name != "action" {
					continue
				}
				for _, action := range p.Enum {
					actions[action] = slices.Contains(spec.HarmlessActions, action)
				}
			}
			calls[spec.Name] = actions
		} else {
			calls[spec.Name] = true
		}
	}
	got, err := json.Marshal(calls, jsontext.WithIndent("  "), json.Deterministic(true))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = append(got, '\n')

	if *updateGolden {
		if err := os.WriteFile(readOnlyCallsPath, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(readOnlyCallsPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create it): %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("read-only calls changed. If this is intended, re-run with -update.\ngot:\n%s", got)
	}
}

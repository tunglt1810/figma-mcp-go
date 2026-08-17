package internal

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// updateGolden rewrites the snapshot instead of comparing against it:
//
//	go test ./internal/ -run TestToolSchemas_Golden -update
var updateGolden = flag.Bool("update", false, "rewrite the tools/list golden snapshot")

const goldenPath = "testdata/tools_schema.json"

// toolsListJSON returns the tools/list result as indented JSON. Map keys are
// sorted by encoding/json, so the output is stable across runs.
func toolsListJSON(t *testing.T) []byte {
	t.Helper()
	s, _ := newTestServer(t)
	raw := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	if raw == nil {
		t.Fatal("HandleMessage returned nil for tools/list")
	}

	var envelope struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal tools/list: %v", err)
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}

	// Key by name so a reordering of registration calls is not a diff.
	byName := map[string]json.RawMessage{}
	for _, tool := range envelope.Result.Tools {
		var nm struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(tool, &nm); err != nil {
			t.Fatalf("unmarshal tool: %v", err)
		}
		byName[nm.Name] = tool
	}

	out, err := json.MarshalIndent(byName, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden: %v", err)
	}
	return append(out, '\n')
}

// TestToolSchemas_Golden pins the exact JSON schema advertised for every tool.
//
// This is the safety net for refactoring how tools are declared: the schema is
// the contract every MCP client sees, so a refactor that changes it silently is
// a breaking change. Any intended change shows up as a reviewable diff.
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

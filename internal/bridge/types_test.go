package bridge

import (
	"encoding/json/v2"
	"strings"
	"testing"
)

func TestRequestJSONRoundTrip(t *testing.T) {
	req := Request{
		Type:      "get_nodes_info",
		RequestID: "req-120000-1",
		NodeIDs:   []string{"1:1", "2:2"},
		Params:    map[string]any{"depth": float64(2)},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Request
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Type != req.Type || got.RequestID != req.RequestID {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	if len(got.NodeIDs) != 2 || got.NodeIDs[0] != "1:1" {
		t.Errorf("NodeIDs mismatch: got %v", got.NodeIDs)
	}
}

func TestResponseOmitsEmptyFields(t *testing.T) {
	resp := Response{Type: "ping", RequestID: "r1"}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// omitempty fields must not appear when zero
	if strings.Contains(s, `"data"`) {
		t.Errorf("expected 'data' to be omitted, got: %s", s)
	}
	if strings.Contains(s, `"error"`) {
		t.Errorf("expected 'error' to be omitted, got: %s", s)
	}
	if strings.Contains(s, `"progress"`) {
		t.Errorf("expected 'progress' to be omitted, got: %s", s)
	}
}

func TestResponseWithError(t *testing.T) {
	resp := Response{RequestID: "r1", Error: "node not found"}
	b, _ := json.Marshal(resp)
	var got Response
	json.Unmarshal(b, &got)
	if got.Error != "node not found" {
		t.Errorf("error field mismatch: %q", got.Error)
	}
}

func TestResponseProgress(t *testing.T) {
	resp := Response{RequestID: "r1", Progress: 50, Message: "halfway"}
	b, _ := json.Marshal(resp)
	var got Response
	json.Unmarshal(b, &got)
	if got.Progress != 50 || got.Message != "halfway" {
		t.Errorf("progress mismatch: %+v", got)
	}
}

package internal

import (
	"encoding/json/v2"
	"strings"
	"testing"
)

func TestBridgeRequestJSONRoundTrip(t *testing.T) {
	req := BridgeRequest{
		Type:      "get_node",
		RequestID: "req-120000-1",
		NodeIDs:   []string{"1:1", "2:2"},
		Params:    map[string]any{"depth": float64(2)},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got BridgeRequest
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

func TestBridgeResponseOmitsEmptyFields(t *testing.T) {
	resp := BridgeResponse{Type: "ping", RequestID: "r1"}
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

func TestBridgeResponseWithError(t *testing.T) {
	resp := BridgeResponse{RequestID: "r1", Error: "node not found"}
	b, _ := json.Marshal(resp)
	var got BridgeResponse
	json.Unmarshal(b, &got)
	if got.Error != "node not found" {
		t.Errorf("error field mismatch: %q", got.Error)
	}
}

func TestBridgeResponseProgress(t *testing.T) {
	resp := BridgeResponse{RequestID: "r1", Progress: 50, Message: "halfway"}
	b, _ := json.Marshal(resp)
	var got BridgeResponse
	json.Unmarshal(b, &got)
	if got.Progress != 50 || got.Message != "halfway" {
		t.Errorf("progress mismatch: %+v", got)
	}
}

func TestRPCRequestJSONRoundTrip(t *testing.T) {
	req := RPCRequest{
		Tool:    "move_nodes",
		NodeIDs: []string{"1:1"},
		Params:  map[string]any{"x": float64(10)},
	}
	b, _ := json.Marshal(req)
	var got RPCRequest
	json.Unmarshal(b, &got)
	if got.Tool != req.Tool || len(got.NodeIDs) != 1 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestRPCResponseJSONRoundTrip(t *testing.T) {
	resp := RPCResponse{Data: map[string]any{"id": "1:1"}, Error: ""}
	b, _ := json.Marshal(resp)
	var got RPCResponse
	json.Unmarshal(b, &got)
	if got.Error != "" {
		t.Errorf("expected empty error, got %q", got.Error)
	}
}

func TestRoleConstants(t *testing.T) {
	if RoleUnknown == RoleLeader {
		t.Error("RoleUnknown must differ from RoleLeader")
	}
	if RoleLeader == RoleFollower {
		t.Error("RoleLeader must differ from RoleFollower")
	}
	if RoleUnknown == RoleFollower {
		t.Error("RoleUnknown must differ from RoleFollower")
	}
}

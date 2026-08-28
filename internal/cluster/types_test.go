package cluster

import (
	"encoding/json/v2"
	"testing"
)

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

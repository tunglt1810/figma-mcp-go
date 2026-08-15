package internal

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// The Figma plugin is installed by hand from a release zip while the server
// auto-updates through npx, so a new server routinely meets an older plugin.
// These tests pin the three outcomes: a plugin that knows the merged command, a
// plugin that does not, and a genuine error that must not trigger a retry.

// scriptedSender replays a canned response per tool and records every call.
type scriptedSender struct {
	responses map[string]BridgeResponse
	err       error
	calls     []string
}

func (s *scriptedSender) Send(_ context.Context, tool string, _ []string, _ map[string]interface{}) (BridgeResponse, error) {
	s.calls = append(s.calls, tool)
	if s.err != nil {
		return BridgeResponse{}, s.err
	}
	if resp, ok := s.responses[tool]; ok {
		return resp, nil
	}
	return BridgeResponse{}, nil
}

// legacyResultsFor builds the {results:[{nodeId, <key>: value}]} shape the
// single-purpose plugin handlers return.
func legacyResultsFor(nodeID, key string, value interface{}) BridgeResponse {
	return BridgeResponse{Data: map[string]interface{}{
		"results": []interface{}{
			map[string]interface{}{"nodeId": nodeID, key: value},
		},
	}}
}

func TestSendWithFanout_ModernPluginIssuesOneCall(t *testing.T) {
	s := &scriptedSender{responses: map[string]BridgeResponse{
		"set_node_properties": {Data: map[string]interface{}{"results": []interface{}{}}},
	}}

	params := map[string]interface{}{"opacity": 0.5, "visible": false}
	if _, err := sendWithFanout(context.Background(), s, "set_node_properties",
		[]string{"1:1"}, params, nodePropertiesFanout(params)); err != nil {
		t.Fatalf("sendWithFanout: %v", err)
	}

	if len(s.calls) != 1 || s.calls[0] != "set_node_properties" {
		t.Errorf("expected a single set_node_properties call, got %v", s.calls)
	}
}

func TestSendWithFanout_LegacyPluginFansOut(t *testing.T) {
	s := &scriptedSender{responses: map[string]BridgeResponse{
		"set_node_properties": {Error: "Unknown request type: set_node_properties"},
		"set_visible":         legacyResultsFor("1:1", "visible", false),
		"set_opacity":         legacyResultsFor("1:1", "opacity", 0.5),
		"set_blend_mode":      legacyResultsFor("1:1", "blendMode", "MULTIPLY"),
	}}

	params := map[string]interface{}{"visible": false, "opacity": 0.5, "blendMode": "MULTIPLY"}
	resp, err := sendWithFanout(context.Background(), s, "set_node_properties",
		[]string{"1:1"}, params, nodePropertiesFanout(params))
	if err != nil {
		t.Fatalf("sendWithFanout: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("fanout returned an error: %s", resp.Error)
	}

	want := []string{"set_node_properties", "set_visible", "set_opacity", "set_blend_mode"}
	if len(s.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", s.calls, want)
	}
	for i, c := range want {
		if s.calls[i] != c {
			t.Errorf("call %d = %q, want %q", i, s.calls[i], c)
		}
	}

	// The merged result must look the same as the modern path's.
	applied := firstApplied(t, resp)
	for key, expected := range map[string]interface{}{
		"visible": false, "opacity": 0.5, "blendMode": "MULTIPLY",
	} {
		if applied[key] != expected {
			t.Errorf("applied[%q] = %v, want %v", key, applied[key], expected)
		}
	}
}

func TestSendWithFanout_LockedMapsToLockOrUnlock(t *testing.T) {
	for _, c := range []struct {
		locked   bool
		wantTool string
	}{{true, "lock_nodes"}, {false, "unlock_nodes"}} {
		s := &scriptedSender{responses: map[string]BridgeResponse{
			"set_node_properties": {Error: "Unknown request type: set_node_properties"},
			c.wantTool:            legacyResultsFor("1:1", "locked", c.locked),
		}}

		params := map[string]interface{}{"locked": c.locked}
		if _, err := sendWithFanout(context.Background(), s, "set_node_properties",
			[]string{"1:1"}, params, nodePropertiesFanout(params)); err != nil {
			t.Fatalf("sendWithFanout: %v", err)
		}
		if len(s.calls) != 2 || s.calls[1] != c.wantTool {
			t.Errorf("locked=%v produced calls %v, want second call %q", c.locked, s.calls, c.wantTool)
		}
	}
}

// A real plugin error must be returned as-is. Retrying it as a fanout would
// re-apply work and bury the actual message.
func TestSendWithFanout_RealErrorIsNotRetried(t *testing.T) {
	s := &scriptedSender{responses: map[string]BridgeResponse{
		"set_node_properties": {Error: "Node not found: 9:9"},
	}}

	params := map[string]interface{}{"opacity": 0.5}
	resp, err := sendWithFanout(context.Background(), s, "set_node_properties",
		[]string{"9:9"}, params, nodePropertiesFanout(params))
	if err != nil {
		t.Fatalf("sendWithFanout: %v", err)
	}
	if resp.Error != "Node not found: 9:9" {
		t.Errorf("error = %q, want it passed through unchanged", resp.Error)
	}
	if len(s.calls) != 1 {
		t.Errorf("expected no fanout, got calls %v", s.calls)
	}
}

func TestSendWithFanout_TransportErrorIsNotRetried(t *testing.T) {
	s := &scriptedSender{err: errors.New("plugin not connected")}

	params := map[string]interface{}{"opacity": 0.5}
	if _, err := sendWithFanout(context.Background(), s, "set_node_properties",
		[]string{"1:1"}, params, nodePropertiesFanout(params)); err == nil {
		t.Fatal("expected the transport error to be returned")
	}
	if len(s.calls) != 1 {
		t.Errorf("expected no fanout, got calls %v", s.calls)
	}
}

// A node missing during fanout must surface as one node-level error, matching
// the modern handler, rather than one entry per property.
func TestFanout_MissingNodeReportedOnce(t *testing.T) {
	missing := BridgeResponse{Data: map[string]interface{}{
		"results": []interface{}{
			map[string]interface{}{"nodeId": "9:9", "error": "Node not found"},
		},
	}}
	s := &scriptedSender{responses: map[string]BridgeResponse{
		"set_node_properties": {Error: "Unknown request type: set_node_properties"},
		"set_visible":         missing,
		"set_opacity":         missing,
	}}

	params := map[string]interface{}{"visible": false, "opacity": 0.5}
	resp, err := sendWithFanout(context.Background(), s, "set_node_properties",
		[]string{"9:9"}, params, nodePropertiesFanout(params))
	if err != nil {
		t.Fatalf("sendWithFanout: %v", err)
	}

	entry := firstResult(t, resp)
	if entry["error"] != "Node not found" {
		t.Errorf("entry = %+v, want a node-level error", entry)
	}
	if _, ok := entry["errors"]; ok {
		t.Errorf("missing node should not produce per-property errors: %+v", entry)
	}
}

// nodePropertiesFanout must emit calls in a fixed order so behaviour is
// reproducible and testable.
func TestNodePropertiesFanout_StableOrder(t *testing.T) {
	params := map[string]interface{}{
		"order": "bringToFront", "blendMode": "SCREEN", "constraints": map[string]interface{}{"horizontal": "MIN"},
		"rotation": 45.0, "opacity": 0.5, "locked": true, "visible": true,
	}
	want := []string{"set_visible", "lock_nodes", "set_opacity", "rotate_nodes",
		"set_blend_mode", "set_constraints", "reorder_nodes"}

	got := nodePropertiesFanout(params)
	if len(got) != len(want) {
		t.Fatalf("got %d calls, want %d", len(got), len(want))
	}
	for i, tool := range want {
		if got[i].tool != tool {
			t.Errorf("call %d = %q, want %q", i, got[i].tool, tool)
		}
	}
}

func TestNodePropertiesFanout_OnlyRequestedProperties(t *testing.T) {
	got := nodePropertiesFanout(map[string]interface{}{"opacity": 0.5})
	if len(got) != 1 || got[0].tool != "set_opacity" {
		t.Errorf("got %+v, want only set_opacity", got)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func firstResult(t *testing.T, resp BridgeResponse) map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var wrapper struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(wrapper.Results) == 0 {
		t.Fatalf("no results in response: %s", b)
	}
	return wrapper.Results[0]
}

func firstApplied(t *testing.T, resp BridgeResponse) map[string]interface{} {
	t.Helper()
	entry := firstResult(t, resp)
	applied, ok := entry["applied"].(map[string]interface{})
	if !ok {
		t.Fatalf("result has no applied map: %+v", entry)
	}
	return applied
}

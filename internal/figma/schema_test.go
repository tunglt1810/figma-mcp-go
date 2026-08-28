package figma

import (
	"testing"
)

// ── ValidNodeID ──────────────────────────────────────────────────────────────

func TestValidNodeID(t *testing.T) {
	valid := []string{
		"4029:12345",
		"0:1",
		"1:1",
		"I44:9;44:3",
		"I2167:9091;186:1579;186:1745",
	}
	for _, id := range valid {
		if !ValidNodeID(id) {
			t.Errorf("expected %q to be valid", id)
		}
	}

	invalid := []string{
		"",
		"4029-12345",
		"4029:12345:6789",
		"abc:def",
		"4029:",
		":12345",
		"4029",
	}
	for _, id := range invalid {
		if ValidNodeID(id) {
			t.Errorf("expected %q to be invalid", id)
		}
	}
}

// ── NormalizeNodeID ───────────────────────────────────────────────────────────

func TestNormalizeNodeID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"4029-12345", "4029:12345"},
		{"4029:12345", "4029:12345"},       // already valid, no-op
		{"not-a-node-id", "not-a-node-id"}, // hyphen but not a node ID
		{"", ""},
	}
	for _, c := range cases {
		got := NormalizeNodeID(c.input)
		if got != c.want {
			t.Errorf("NormalizeNodeID(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ── ValidateRPC ───────────────────────────────────────────────────────────────

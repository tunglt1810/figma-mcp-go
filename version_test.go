package figmamcpgo

import (
	"testing"
)

func TestGetVersion(t *testing.T) {
	v := GetVersion()
	if v == "" || v == "dev" {
		t.Errorf("expected valid version from server.json, got %q", v)
	}
	if v != "0.1.0" {
		t.Errorf("expected 0.1.0, got %q", v)
	}
}

package figmamcpgo

import (
	"encoding/json/v2"
	"os"
	"testing"
)

func TestGetVersion(t *testing.T) {
	v := GetVersion()
	if v == "" || v == "dev" {
		t.Errorf("expected valid version from server.json, got %q", v)
	}

	data, err := os.ReadFile("server.json")
	if err != nil {
		t.Fatalf("failed to read server.json: %v", err)
	}

	var cfg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to unmarshal server.json: %v", err)
	}

	if v != cfg.Version {
		t.Errorf("expected %q from server.json, got %q", cfg.Version, v)
	}
}

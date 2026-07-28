package figmamcpgo

import (
	_ "embed"
	"encoding/json"
)

//go:embed server.json
var serverJSON []byte

type serverConfig struct {
	Version string `json:"version"`
}

// GetVersion returns the version string embedded from server.json.
// Falls back to "dev" if missing or unparseable.
func GetVersion() string {
	var cfg serverConfig
	if err := json.Unmarshal(serverJSON, &cfg); err == nil && cfg.Version != "" {
		return cfg.Version
	}
	return "dev"
}

package figmamcpgo

import (
	_ "embed"
	"encoding/json/v2"
)

//go:embed server.json
var serverJSON []byte

type serverConfig struct {
	Version string `json:"version"`
}

// GetVersion returns the version string embedded from server.json.
// It falls back to "dev" if the file is missing or cannot be parsed.
func GetVersion() string {
	var cfg serverConfig
	if err := json.Unmarshal(serverJSON, &cfg); err == nil && cfg.Version != "" {
		return cfg.Version
	}
	return "dev"
}

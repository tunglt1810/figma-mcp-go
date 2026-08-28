package cluster

import (
	"net"
	"testing"
)

// freePort finds an available TCP port on 127.0.0.1.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// fakeCall is one recorded call.
type fakeCall struct {
	tool    string
	nodeIDs []string
	params  map[string]any
}

// passthroughGuard accepts everything. Tests that care about checking live in
// the tools package, where the real rules are; these care about routing.
func passthroughGuard(_ string, nodeIDs []string, params map[string]any) ([]string, map[string]any, error) {
	return nodeIDs, params, nil
}
